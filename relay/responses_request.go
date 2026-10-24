package relay

import (
	"bytes"
	"fmt"
	"io"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	relaychannel "github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/gin-gonic/gin"
)

// PrepareResponsesRequest applies the same model, conversion and channel rules
// for HTTP and WebSocket requests. The caller closes closer after the attempt;
// passthrough bodies remain owned by the incoming request's BodyStorage.
// The returned adaptor retains route/conversion state for DoRequest/DoResponse.
func PrepareResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, req *dto.OpenAIResponsesRequest) (relaychannel.Adaptor, common.ReplayableBody, io.Closer, *types.NewAPIError) {
	info.InitChannelMeta(c)
	if info.RelayMode == relayconstant.RelayModeResponsesCompact &&
		!common.SupportsResponsesCompact(info.ChannelType, info.ApiType) {
		return nil, nil, nil, types.NewErrorWithStatusCode(
			fmt.Errorf("unsupported endpoint %q for api type %d", "/v1/responses/compact", info.ApiType),
			types.ErrorCodeInvalidRequest,
			http.StatusBadRequest,
			types.ErrOptionWithSkipRetry(),
		)
	}

	request, err := common.DeepCopy(req)
	if err != nil {
		return nil, nil, nil, types.NewError(fmt.Errorf("failed to copy responses request: %w", err), types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}
	if err := helper.ModelMappedHelper(c, info, request); err != nil {
		return nil, nil, nil, types.NewError(err, types.ErrorCodeChannelModelMappedError, types.ErrOptionWithSkipRetry())
	}
	if err := helper.ApplyReasoningModelSuffix(info, request); err != nil {
		return nil, nil, nil, newConvertRequestFailedError(c, info, err)
	}

	adaptor := GetAdaptor(info.ApiType)
	if adaptor == nil {
		return nil, nil, nil, types.NewError(fmt.Errorf("invalid api type: %d", info.ApiType), types.ErrorCodeInvalidApiType, types.ErrOptionWithSkipRetry())
	}
	adaptor.Init(info)
	if model_setting.GetGlobalSettings().PassThroughRequestEnabled || info.ChannelSetting.PassThroughBodyEnabled {
		storage, err := common.GetBodyStorage(c)
		if err != nil {
			return nil, nil, nil, types.NewError(err, types.ErrorCodeReadRequestBodyFailed, types.ErrOptionWithSkipRetry())
		}
		body := common.NewReplayableBodyReader(storage)
		return adaptor, body, io.NopCloser(body), nil
	}

	convertedRequest, err := adaptor.ConvertOpenAIResponsesRequest(c, info, *request)
	if err != nil {
		return nil, nil, nil, newConvertRequestFailedError(c, info, err)
	}
	relaycommon.AppendRequestConversionFromRequest(info, convertedRequest)
	jsonData, err := common.Marshal(convertedRequest)
	if err != nil {
		return nil, nil, nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}
	jsonData, err = relaycommon.RemoveDisabledFields(jsonData, info.ChannelOtherSettings, info.ChannelSetting.PassThroughBodyEnabled)
	if err != nil {
		return nil, nil, nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}
	if len(info.ParamOverride) > 0 {
		jsonData, err = relaycommon.ApplyParamOverrideWithRelayInfo(jsonData, info)
		if err != nil {
			return nil, nil, nil, newAPIErrorFromParamOverride(err)
		}
	}

	logger.LogDebug(c, "requestBody: %s", jsonData)
	body, closer, err := relaycommon.NewChannelJSONBody(jsonData, info)
	if err != nil {
		return nil, nil, nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}
	return adaptor, body, closer, nil
}

// doResponsesRequest performs one targeted retry for a provider validation
// error that names a missing Responses tool output. The normal request path
// remains one network call; this branch only runs when the provider explicitly
// confirms the missing output and the channel opted into tool repair.
func doResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, adaptor relaychannel.Adaptor, requestBody io.Reader) (any, error) {
	response, err := adaptor.DoRequest(c, info, requestBody)
	if err != nil {
		return nil, err
	}
	httpResponse, ok := response.(*http.Response)
	if !ok || httpResponse == nil || httpResponse.StatusCode < http.StatusBadRequest || httpResponse.StatusCode >= http.StatusInternalServerError {
		return response, nil
	}
	if info == nil || info.ChannelMeta == nil || !info.ChannelSetting.ToolOutputRepair ||
		info.ChannelSetting.PassThroughBodyEnabled || info.GetFinalRequestRelayFormat() != types.RelayFormatOpenAIResponses {
		return response, nil
	}
	replayable, ok := requestBody.(common.ReplayableBody)
	if !ok {
		return response, nil
	}
	if httpResponse.Body == nil {
		return response, nil
	}
	errorBody, readErr := io.ReadAll(httpResponse.Body)
	_ = httpResponse.Body.Close()
	if readErr != nil {
		httpResponse.Body = io.NopCloser(bytes.NewReader(errorBody))
		return response, nil
	}
	requestReader, readerErr := replayable.NewReader()
	if readerErr != nil {
		httpResponse.Body = io.NopCloser(bytes.NewReader(errorBody))
		return response, nil
	}
	originalRequest, requestReadErr := io.ReadAll(requestReader)
	_ = requestReader.Close()
	if requestReadErr != nil {
		httpResponse.Body = io.NopCloser(bytes.NewReader(errorBody))
		return response, nil
	}
	repairedRequest, repaired, repairErr := relaycommon.RepairToolOutputFromUpstreamError(originalRequest, errorBody, info)
	if repairErr != nil {
		return nil, repairErr
	}
	if repaired == 0 {
		httpResponse.Body = io.NopCloser(bytes.NewReader(errorBody))
		return response, nil
	}
	retryBody, retryCloser, bodyErr := relaycommon.NewOutboundJSONBody(repairedRequest)
	if bodyErr != nil {
		return nil, bodyErr
	}
	defer retryCloser.Close()
	return adaptor.DoRequest(c, info, retryBody)
}
