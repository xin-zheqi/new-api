package openai

import (
	"fmt"
	"io"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

func OaiResponsesHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)

	// read response body
	var responsesResponse dto.OpenAIResponsesResponse
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	err = common.Unmarshal(responseBody, &responsesResponse)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	info.ObserveUpstreamResponseModel(responsesResponse.Model)
	if oaiError := responsesResponse.GetOpenAIError(); oaiError != nil && oaiError.Type != "" {
		return nil, types.WithOpenAIError(*oaiError, resp.StatusCode)
	}

	// Keep model aliases client-visible. The upstream model is still retained
	// in RelayInfo for routing, billing and audit data.
	if patched, patchErr := relaycommon.RewriteResponseModel(info, responseBody); patchErr == nil {
		responseBody = patched
	} else {
		logger.LogWarn(c, "failed to normalize response model: "+patchErr.Error())
	}

	// 写入新的 response body
	service.IOCopyBytesGracefully(c, resp, responseBody)

	// compute usage
	usage := &dto.Usage{}
	service.ApplyResponsesUsage(usage, responsesResponse.Usage)
	// Count actual tool invocations from Output (not tool declarations).
	for _, output := range responsesResponse.Output {
		switch output.Type {
		case dto.BuildInCallWebSearchCall:
			info.CountBillableToolCall(dto.BuildInCallWebSearchCall, "")
		case dto.BuildInCallFileSearchCall:
			info.CountBillableToolCall(dto.BuildInCallFileSearchCall, "")
		case dto.BuildInCallFunctionCall:
			info.CountBillableToolCall(dto.BuildInCallFunctionCall, output.Name)
		}
	}
	info.ApplyVendorToolUsage(responseBody)

	imageCounter := &relaycommon.ImageGenerationCallCounter{}
	for i := range responsesResponse.Output {
		imageCounter.Observe(&responsesResponse.Output[i], &i)
	}
	imageCounter.Commit(info)

	return usage, nil
}

func OaiResponsesStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		logger.LogError(c, "invalid response or response body")
		return nil, types.NewError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse)
	}

	defer service.CloseResponseBodyGracefully(resp)

	accumulator := service.NewResponsesUsageAccumulator(info)
	reasoningRepair := relaycommon.NewResponsesReasoningRepair(info.ReasoningContentRepairActive)

	scanErr := helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {

		// 检查当前数据是否包含 completed 状态和 usage 信息
		var streamResponse dto.ResponsesStreamResponse
		if err := common.UnmarshalJsonStr(data, &streamResponse); err != nil {
			logger.LogError(c, "failed to unmarshal stream response: "+err.Error())
			sr.Error(err)
			return
		}
		accumulator.Observe(&streamResponse, common.StringToByteSlice(data))
		// Account from the original upstream event. Repair only the plaintext
		// history returned to the client; usage and failure handling stay intact.
		if repaired, err := reasoningRepair.Repair(data); err == nil {
			data = repaired
		} else {
			logger.LogWarn(c, "reasoning content repair skipped: "+err.Error())
		}
		if patched, err := relaycommon.RewriteResponseModel(info, common.StringToByteSlice(data)); err == nil {
			data = string(patched)
		} else {
			logger.LogWarn(c, "failed to normalize response model: "+err.Error())
		}
		sendResponsesStreamData(c, streamResponse, data)
	})
	if scanErr != nil {
		return nil, scanErr
	}

	return accumulator.Finish(), nil
}
