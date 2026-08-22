package claude

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/pkg/tlsfingerprint"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service/relayconvert"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type Adaptor struct {
}

func (a *Adaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ClaudeRequest) (any, error) {
	if info != nil && info.ChannelOtherSettings.ClaudeCodeMimic {
		applyClaudeCodeMimicBody(request)
	}
	claudeSettings := model_setting.GetClaudeSettings()
	if info != nil && claudeSettings.ShouldApplyThinkingSignatureCompatibility(info.ChannelId, info.ChannelType, info.OriginModelName) {
		request.RemoveThinkingBlocksFromMessages()
	}
	return request, nil
}

// applyClaudeCodeMimicBody adds the stable, non-secret request traits used by
// Claude Code. Existing downstream metadata/system content is preserved.
func applyClaudeCodeMimicBody(request *dto.ClaudeRequest) {
	if request == nil {
		return
	}
	if len(request.Metadata) == 0 {
		metadata, _ := common.Marshal(map[string]string{"user_id": claudeCodeMimicUserID(request)})
		request.Metadata = metadata
	} else {
		var metadata map[string]any
		if common.Unmarshal(request.Metadata, &metadata) == nil {
			if metadata == nil {
				metadata = map[string]any{}
			}
			if value, ok := metadata["user_id"].(string); !ok || strings.TrimSpace(value) == "" {
				metadata["user_id"] = claudeCodeMimicUserID(request)
			}
			if encoded, err := common.Marshal(metadata); err == nil {
				request.Metadata = encoded
			}
		}
	}

	identity := "You are Claude Code, Anthropic's official CLI for Claude."
	billing := claudeCodeBillingHeader(request)
	switch system := request.System.(type) {
	case nil:
		request.System = []dto.ClaudeMediaMessage{{Type: "text", Text: &identity}, {Type: "text", Text: &billing}}
	case string:
		if !strings.Contains(system, "Claude Code") {
			request.System = []dto.ClaudeMediaMessage{{Type: "text", Text: &identity}, {Type: "text", Text: &billing}, {Type: "text", Text: &system}}
		}
	case []dto.ClaudeMediaMessage:
		if !claudeCodeSystemPresent(system) {
			request.System = append([]dto.ClaudeMediaMessage{{Type: "text", Text: &identity}, {Type: "text", Text: &billing}}, system...)
		}
	default:
		// Normalize JSON-decoded array/map representations while preserving all
		// existing blocks. This is the common path for requests parsed from JSON.
		entries := request.ParseSystem()
		if len(entries) > 0 && !claudeCodeSystemPresent(entries) {
			request.System = append([]dto.ClaudeMediaMessage{{Type: "text", Text: &identity}, {Type: "text", Text: &billing}}, entries...)
		}
	}
	ensureClaudeCodeBillingBlock(request, billing)
}

func claudeCodeSystemPresent(entries []dto.ClaudeMediaMessage) bool {
	for _, entry := range entries {
		if strings.Contains(entry.GetText(), "Claude Code") {
			return true
		}
	}
	return false
}

func ensureClaudeCodeBillingBlock(request *dto.ClaudeRequest, billing string) {
	if request == nil || billing == "" {
		return
	}
	entries := request.ParseSystem()
	for _, entry := range entries {
		if entry.GetText() == billing {
			return
		}
	}
	if len(entries) == 0 {
		if system, ok := request.System.(string); ok && system != "" {
			request.System = []dto.ClaudeMediaMessage{{Type: "text", Text: &system}}
			entries = request.ParseSystem()
		}
	}
	if len(entries) > 0 {
		request.System = append([]dto.ClaudeMediaMessage{{Type: "text", Text: &billing}}, entries...)
	}
}

func claudeCodeMimicUserID(request *dto.ClaudeRequest) string {
	seed := "new-api-claude-code-mimic"
	if request != nil {
		for _, message := range request.Messages {
			if message.Role == "user" {
				seed += "::" + message.GetStringContent()
				break
			}
		}
	}
	sum := sha256.Sum256([]byte(seed))
	return `{"device_id":"` + hex.EncodeToString(sum[:]) + `","account_uuid":"","session_id":"` + uuid.NewSHA1(uuid.NameSpaceOID, []byte(seed)).String() + `"}`
}

func claudeCodeBillingHeader(request *dto.ClaudeRequest) string {
	text := ""
	if request != nil {
		for _, message := range request.Messages {
			if message.Role == "user" {
				text = message.GetStringContent()
				break
			}
		}
	}
	chars := []byte{'0', '0', '0'}
	for outputIndex, sourceIndex := range []int{4, 7, 20} {
		if sourceIndex < len(text) {
			chars[outputIndex] = text[sourceIndex]
		}
	}
	sum := sha256.Sum256([]byte("59cf53e54c78" + string(chars) + "2.1.220"))
	return "x-anthropic-billing-header: cc_version=2.1.220." + hex.EncodeToString(sum[:])[:3] + "; cc_entrypoint=cli;"
}

func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	requestURL := fmt.Sprintf("%s/v1/messages", info.ChannelBaseUrl)
	if !shouldAppendClaudeBetaQuery(info) {
		return requestURL, nil
	}

	parsedURL, err := url.Parse(requestURL)
	if err != nil {
		return "", err
	}
	query := parsedURL.Query()
	query.Set("beta", "true")
	parsedURL.RawQuery = query.Encode()
	return parsedURL.String(), nil
}

func shouldAppendClaudeBetaQuery(info *relaycommon.RelayInfo) bool {
	if info == nil {
		return false
	}
	if info.IsClaudeBetaQuery {
		return true
	}
	if info.ChannelOtherSettings.ClaudeBetaQuery {
		return true
	}
	return false
}

func CommonClaudeHeadersOperation(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) {
	// common headers operation
	anthropicBeta := c.Request.Header.Get("anthropic-beta")
	if anthropicBeta != "" {
		req.Set("anthropic-beta", anthropicBeta)
	}
	model_setting.GetClaudeSettings().WriteHeaders(info.OriginModelName, req)
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	req.Set("x-api-key", info.ApiKey)
	anthropicVersion := c.Request.Header.Get("anthropic-version")
	if anthropicVersion == "" {
		anthropicVersion = "2023-06-01"
	}
	req.Set("anthropic-version", anthropicVersion)
	CommonClaudeHeadersOperation(c, req, info)
	if info != nil && info.ChannelOtherSettings.ClaudeCodeMimic {
		applyClaudeCodeMimicHeaders(req)
	}
	return nil
}

func applyClaudeCodeMimicHeaders(req *http.Header) {
	for key, value := range map[string]string{
		"User-Agent":                                "claude-cli/2.1.220 (external, cli)",
		"X-Stainless-Lang":                          "js",
		"X-Stainless-Package-Version":               "0.94.0",
		"X-Stainless-OS":                            "Linux",
		"X-Stainless-Arch":                          "arm64",
		"X-Stainless-Runtime":                       "node",
		"X-Stainless-Runtime-Version":               "v24.3.0",
		"X-Stainless-Retry-Count":                   "0",
		"X-Stainless-Timeout":                       "600",
		"X-App":                                     "cli",
		"Anthropic-Dangerous-Direct-Browser-Access": "true",
	} {
		req.Set(key, value)
	}
	req.Set("anthropic-beta", "claude-code-20250219,oauth-2025-04-20,interleaved-thinking-2025-05-14,prompt-caching-scope-2026-01-05,effort-2025-11-24,context-management-2025-06-27,extended-cache-ttl-2025-04-11")
}

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	result, err := relayconvert.ConvertRequest(c, info, types.RelayFormatClaude, request)
	if err != nil {
		return nil, err
	}
	if info != nil && info.ChannelOtherSettings.ClaudeCodeMimic {
		if claudeRequest, ok := result.Value.(*dto.ClaudeRequest); ok {
			applyClaudeCodeMimicBody(claudeRequest)
		}
	}
	return result.Value, nil
}

func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, nil
}

func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	// TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	if info != nil && info.ChannelOtherSettings.ClaudeCodeMimic && info.UpstreamHTTPClient == nil {
		client, err := tlsfingerprint.NewHTTPClient(info.ChannelSetting.Proxy, 0)
		if err != nil {
			return nil, fmt.Errorf("create Claude Code TLS client: %w", err)
		}
		info.UpstreamHTTPClient = client
	}
	return channel.DoApiRequest(a, c, info, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	info.FinalRequestRelayFormat = types.RelayFormatClaude
	if info.IsStream {
		return ClaudeStreamHandler(c, resp, info)
	} else {
		return ClaudeHandler(c, resp, info)
	}
}

func (a *Adaptor) GetModelList() []string {
	return ModelList
}

func (a *Adaptor) GetChannelName() string {
	return ChannelName
}
