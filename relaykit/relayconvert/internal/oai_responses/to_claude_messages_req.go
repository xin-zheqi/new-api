package oairesponses

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/convmeta"
	relaymedia "github.com/QuantumNous/new-api/relaykit/relayconvert/internal/media"
	sharedclaude "github.com/QuantumNous/new-api/relaykit/relayconvert/internal/shared/claude"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/internal/toolconv"
	kitutil "github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/reasoning"
	"github.com/QuantumNous/new-api/relaykit/types"
)

func convertOpenAIResponsesRequestToClaudeMessages(c context.Context, info convmeta.Meta, request any) (any, error) {
	responsesRequest, err := OpenAIResponsesRequestFromAny(request)
	if err != nil {
		return nil, err
	}
	return OpenAIResponsesRequestToClaudeMessages(c, info, responsesRequest)
}

func OpenAIResponsesRequestToClaudeMessages(c context.Context, info convmeta.Meta, req *dto.OpenAIResponsesRequest) (*dto.ClaudeRequest, error) {
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}
	if req.Model == "" {
		return nil, fmt.Errorf("model is required")
	}
	if err := ValidateRequestChatUnsupportedFields(req); err != nil {
		return nil, err
	}

	claudeRequest := &dto.ClaudeRequest{
		Model:       req.Model,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		Stream:      req.Stream,
	}
	if req.MaxOutputTokens != nil && *req.MaxOutputTokens > 0 {
		claudeRequest.MaxTokens = kitutil.GetPointer(*req.MaxOutputTokens)
	}
	// Use the shared tool conversion pipeline for definitions and choices. The
	// earlier Responses converter only copied function tools, which silently
	// dropped computer_use, shell, apply_patch, web_search_preview and other
	// native tools. Hosted history is handled below together with ordinary input
	// so message ordering remains deterministic across multiple turns.
	_, toolSet, err := toolconv.ExtractRequest(types.RelayFormatOpenAIResponses, req)
	if err != nil {
		return nil, err
	}
	toolSet.History = nil
	attached, _, err := toolconv.AttachRequest(types.RelayFormatClaude, claudeRequest, toolSet, convmeta.OptionsOf(info))
	if err != nil {
		return nil, err
	}
	var attachedOK bool
	claudeRequest, attachedOK = attached.(*dto.ClaudeRequest)
	if !attachedOK {
		return nil, fmt.Errorf("expected Anthropic Messages request, got %T", attached)
	}
	sourceReasoning, err := reasoning.FromOpenAIResponses(req)
	if err != nil {
		return nil, reasoning.AsClientError(err)
	}
	if err := sharedclaude.ApplyReasoning(claudeRequest, info, sourceReasoning); err != nil {
		return nil, reasoning.AsClientError(err)
	}
	if claudeRequest.MaxTokens == nil {
		if defaultMaxTokens, configured := convmeta.OptionsOf(info).Claude.DefaultMaxTokensFor(claudeRequest.Model); configured {
			value := uint(defaultMaxTokens)
			claudeRequest.MaxTokens = &value
		}
	}

	systemMessages := make([]dto.ClaudeMediaMessage, 0)
	if RawJSONPresent(req.Instructions) {
		instructions, err := JSONString(req.Instructions)
		if err != nil {
			return nil, fmt.Errorf("invalid instructions: %w", err)
		}
		if strings.TrimSpace(instructions) != "" {
			systemMessages = append(systemMessages, dto.ClaudeMediaMessage{
				Type: "text",
				Text: kitutil.GetPointer(instructions),
			})
		}
	}
	// A literal prompt can be represented as additional system guidance. Prompt
	// template/resource objects still have no local equivalent and are left out
	// without failing the request.
	if kitutil.GetJsonType(req.Prompt) == "string" {
		prompt, err := JSONString(req.Prompt)
		if err != nil {
			return nil, fmt.Errorf("invalid prompt: %w", err)
		}
		if strings.TrimSpace(prompt) != "" {
			systemMessages = append(systemMessages, dto.ClaudeMediaMessage{
				Type: "text",
				Text: kitutil.GetPointer(prompt),
			})
		}
	}

	inputItems, err := InputItems(req.Input)
	if err != nil {
		return nil, err
	}
	for _, item := range inputItems {
		itemType := strings.TrimSpace(kitutil.Interface2String(item["type"]))
		switch itemType {
		case ResponsesInputTypeFunctionCall:
			claudeRequest.Messages = appendClaudeToolUse(claudeRequest.Messages, responsesFunctionCallItemToClaudeToolUse(item, "arguments"))
		case ResponsesInputTypeCustomToolCall:
			claudeRequest.Messages = appendClaudeToolUse(claudeRequest.Messages, responsesFunctionCallItemToClaudeToolUse(item, "input"))
		case ResponsesInputTypeFunctionCallOutput, ResponsesInputTypeCustomToolOutput:
			claudeRequest.Messages = appendClaudeToolResult(claudeRequest.Messages, responsesFunctionOutputItemToClaudeToolResult(item))
		default:
			if nativeBlock, ok := responsesNativeToolItemToClaude(item); ok {
				if nativeBlock.Type == "tool_result" || nativeBlock.Type == "mcp_tool_result" || nativeBlock.Type == "web_search_tool_result" {
					claudeRequest.Messages = appendClaudeToolResult(claudeRequest.Messages, nativeBlock)
				} else {
					claudeRequest.Messages = appendClaudeToolUse(claudeRequest.Messages, nativeBlock)
				}
				continue
			}
			sourceRole := strings.TrimSpace(kitutil.Interface2String(item["role"]))
			role := responsesClaudeRole(sourceRole)
			parts, err := responsesInputContentToClaudeMediaMessages(c, item["content"])
			if err != nil {
				return nil, err
			}
			if sourceRole == "" && len(parts) == 0 {
				continue
			}
			if role == "system" {
				for _, part := range parts {
					if part.Type == "text" {
						systemMessages = append(systemMessages, part)
					}
				}
				continue
			}
			if len(parts) == 0 {
				parts = []dto.ClaudeMediaMessage{
					{
						Type: "text",
						Text: kitutil.GetPointer("..."),
					},
				}
			}
			claudeRequest.Messages = append(claudeRequest.Messages, dto.ClaudeMessage{
				Role:    role,
				Content: parts,
			})
		}
	}
	claudeRequest.Messages = omitTrailingAssistantText(c, claudeRequest.Messages,
		func(message dto.ClaudeMessage) bool { return message.Role == "user" },
		func(message dto.ClaudeMessage) bool {
			return message.Role == "assistant" && !slices.ContainsFunc(claudeMessageContentParts(message.Content), func(part dto.ClaudeMediaMessage) bool { return part.Type == "tool_use" })
		})

	if len(systemMessages) > 0 {
		claudeRequest.System = systemMessages
	}
	if len(claudeRequest.Messages) > 0 || len(systemMessages) > 0 {
		claudeRequest.Messages = ensureClaudeMessagesStartWithUser(claudeRequest.Messages)
	}
	if len(claudeRequest.Messages) == 0 {
		// A state-only Responses request has no local history to replay. Claude
		// still requires at least one message, so keep the request valid with a
		// neutral placeholder rather than failing conversion.
		claudeRequest.Messages = ensureClaudeMessagesStartWithUser(nil)
	}
	// Checked last so every injection path has had its chance to satisfy the
	// required field.
	if claudeRequest.MaxTokens == nil {
		return nil, sharedclaude.ErrMissingMaxTokens
	}
	return claudeRequest, nil
}

func responsesFunctionDeclarationsToClaudeTools(functions []dto.FunctionRequest) []any {
	tools := make([]any, 0, len(functions))
	for _, function := range functions {
		tools = append(tools, &dto.Tool{
			Name:        function.Name,
			Description: function.Description,
			InputSchema: sharedclaude.FunctionParametersToInputSchema(function.Parameters),
		})
	}
	return tools
}

// responsesNativeToolItemToClaude maps Responses client and hosted tool
// continuation items into Claude content blocks. Messages has no dedicated
// field for these items, but its tool-use/result blocks preserve the call id
// and action payload so a later response conversion can reconstruct the
// Responses conversation without rejecting the request.
func responsesNativeToolItemToClaude(item map[string]any) (dto.ClaudeMediaMessage, bool) {
	itemType := strings.TrimSpace(kitutil.Interface2String(item["type"]))
	callID := CallID(item)
	if callID == "" {
		return dto.ClaudeMediaMessage{}, false
	}

	if strings.HasSuffix(itemType, "_output") {
		resultType := "tool_result"
		if itemType == "mcp_call_output" {
			resultType = "mcp_tool_result"
		} else if itemType == "web_search_call_output" {
			resultType = "web_search_tool_result"
		}
		failed := strings.EqualFold(strings.TrimSpace(kitutil.Interface2String(item["status"])), "failed")
		return dto.ClaudeMediaMessage{
			Type:      resultType,
			ToolUseId: callID,
			Content:   responsesToolOutputValue(item["output"]),
			IsError:   kitutil.GetPointer(failed),
		}, true
	}

	name := strings.TrimSpace(kitutil.Interface2String(item["name"]))
	blockType := "tool_use"
	inputKey := "action"
	switch itemType {
	case "computer_call":
		name = "computer"
	case "local_shell_call":
		name = "bash"
	case "apply_patch_call":
		name = "str_replace_editor"
	case "web_search_call":
		name = "web_search"
		blockType = "server_tool_use"
	case "mcp_call":
		blockType = "mcp_tool_use"
		inputKey = "arguments"
	case "file_search_call":
		name = "file_search"
	case "code_interpreter_call":
		name = "code_execution"
	case "image_generation_call":
		name = "image_generation"
	default:
		return dto.ClaudeMediaMessage{}, false
	}
	if name == "" {
		name = itemType
	}
	return dto.ClaudeMediaMessage{
		Type:       blockType,
		Id:         callID,
		Name:       name,
		Input:      ObjectValue(item[inputKey], inputKey),
		ServerName: strings.TrimSpace(kitutil.Interface2String(item["server_label"])),
		Caller:     rawJSONValue(item["caller"]),
	}, true
}

func rawJSONValue(value any) []byte {
	if value == nil {
		return nil
	}
	encoded, err := kitutil.Marshal(value)
	if err != nil {
		return nil
	}
	return encoded
}

func responsesInputContentToClaudeMediaMessages(c context.Context, content any) ([]dto.ClaudeMediaMessage, error) {
	contentParts, err := ContentParts(content)
	if err != nil {
		return nil, err
	}

	parts := make([]dto.ClaudeMediaMessage, 0, len(contentParts))
	for _, contentPart := range contentParts {
		partType := strings.TrimSpace(kitutil.Interface2String(contentPart["type"]))
		switch partType {
		case "input_text", "output_text", "text":
			text := kitutil.Interface2String(contentPart["text"])
			if text != "" {
				parts = append(parts, dto.ClaudeMediaMessage{
					Type: "text",
					Text: kitutil.GetPointer(text),
				})
			}
		case "input_image", "input_file", "input_audio", "input_video":
			source := ContentPartToFileSource(contentPart)
			if source == nil {
				continue
			}
			base64Data, mimeType, err := relaymedia.ResolveBase64Data(c, source, "formatting Responses input for Claude")
			if err != nil {
				return nil, fmt.Errorf("get file data failed: %s", err.Error())
			}
			claudePart := dto.ClaudeMediaMessage{
				Source: &dto.ClaudeMessageSource{
					Type:      "base64",
					MediaType: mimeType,
					Data:      base64Data,
				},
			}
			if strings.HasPrefix(mimeType, "application/pdf") {
				claudePart.Type = "document"
			} else {
				claudePart.Type = "image"
			}
			parts = append(parts, claudePart)
		}
	}
	return parts, nil
}

func responsesFunctionCallItemToClaudeToolUse(item map[string]any, inputKey string) dto.ClaudeMediaMessage {
	return dto.ClaudeMediaMessage{
		Type:  "tool_use",
		Id:    CallID(item),
		Name:  strings.TrimSpace(kitutil.Interface2String(item["name"])),
		Input: ObjectValue(item[inputKey], inputKey),
	}
}

func responsesFunctionOutputItemToClaudeToolResult(item map[string]any) dto.ClaudeMediaMessage {
	return dto.ClaudeMediaMessage{
		Type:      "tool_result",
		ToolUseId: CallID(item),
		Content:   responsesToolOutputValue(item["output"]),
	}
}

func responsesToolOutputValue(value any) any {
	if value == nil {
		return ""
	}
	return value
}

func appendClaudeToolUse(messages []dto.ClaudeMessage, toolUse dto.ClaudeMediaMessage) []dto.ClaudeMessage {
	if len(messages) > 0 && messages[len(messages)-1].Role == "assistant" {
		last := messages[len(messages)-1]
		parts := claudeMessageContentParts(last.Content)
		parts = append(parts, toolUse)
		last.Content = parts
		messages[len(messages)-1] = last
		return messages
	}
	return append(messages, dto.ClaudeMessage{
		Role:    "assistant",
		Content: []dto.ClaudeMediaMessage{toolUse},
	})
}

func appendClaudeToolResult(messages []dto.ClaudeMessage, toolResult dto.ClaudeMediaMessage) []dto.ClaudeMessage {
	if len(messages) > 0 && messages[len(messages)-1].Role == "user" {
		last := messages[len(messages)-1]
		parts := claudeMessageContentParts(last.Content)
		parts = append(parts, toolResult)
		last.Content = parts
		messages[len(messages)-1] = last
		return messages
	}
	return append(messages, dto.ClaudeMessage{
		Role:    "user",
		Content: []dto.ClaudeMediaMessage{toolResult},
	})
}

func claudeMessageContentParts(content any) []dto.ClaudeMediaMessage {
	switch typed := content.(type) {
	case []dto.ClaudeMediaMessage:
		return typed
	case string:
		if typed == "" {
			return nil
		}
		return []dto.ClaudeMediaMessage{
			{
				Type: "text",
				Text: kitutil.GetPointer(typed),
			},
		}
	default:
		parts, _ := kitutil.Any2Type[[]dto.ClaudeMediaMessage](content)
		return parts
	}
}

func responsesClaudeRole(role string) string {
	switch role {
	case "assistant":
		return "assistant"
	case "system", "developer":
		return "system"
	default:
		return "user"
	}
}

func ensureClaudeMessagesStartWithUser(messages []dto.ClaudeMessage) []dto.ClaudeMessage {
	if len(messages) > 0 && messages[0].Role == "user" {
		return messages
	}
	return append([]dto.ClaudeMessage{
		{
			Role: "user",
			Content: []dto.ClaudeMediaMessage{
				{
					Type: "text",
					Text: kitutil.GetPointer("..."),
				},
			},
		},
	}, messages...)
}
