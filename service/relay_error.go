package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
)

// DecideRelayRetry is the single retry decision for relay attempts. The reason
// is recorded in the request policy decision events of the log details.
func DecideRelayRetry(c *gin.Context, err *types.NewAPIError, retryTimes int) PolicyDecision {
	if err == nil {
		return PolicyDecision{Action: "stop", Reason: "request_completed", Source: "system"}
	}
	// A committed stream must never be replayed on another channel. Client
	// cancellation and local content rejection also remain terminal.
	if err.GetErrorCode() == types.ErrorCodeClientRequestCanceled || errors.Is(err, context.Canceled) || (c != nil && c.Request != nil && errors.Is(c.Request.Context().Err(), context.Canceled)) {
		return PolicyDecision{Action: "stop", Reason: "client_canceled", Source: "system"}
	}
	if err.GetErrorCode() == types.ErrorCodeCyberPolicy {
		return PolicyDecision{Action: "stop", Reason: "local_rejection", Source: "system"}
	}
	if c != nil && c.Writer != nil && c.Writer.Written() {
		return PolicyDecision{Action: "stop", Reason: "response_started", Source: "system"}
	}
	crossGroupRetryAvailable := common.GetContextKeyBool(c, constant.ContextKeyTokenCrossGroupRetry) && !common.GetContextKeyBool(c, constant.ContextKeyCrossGroupExhausted)

	if ShouldSkipRetryAfterChannelAffinityFailure(c) {
		source := RequestPolicy(c).SessionModeSource
		if source == "" {
			source = "session_rule"
		}
		return PolicyDecision{Action: "stop", Reason: "strict_session", Source: source}
	}
	if GetChannelConstraints(c).SuppressesRetry() {
		return PolicyDecision{Action: "stop", Reason: "pinned_channel", Source: "channel_constraint"}
	}
	if types.IsChannelError(err) {
		if retryTimes > 0 || crossGroupRetryAvailable {
			return PolicyDecision{Action: "retry", Reason: "channel_error", Source: "system"}
		}
		return PolicyDecision{Action: "stop", Reason: "attempt_budget_exhausted", Source: "global"}
	}
	if types.IsSkipRetryError(err) {
		return PolicyDecision{Action: "stop", Reason: "non_retryable_error", Source: "system"}
	}
	if retryTimes <= 0 && !crossGroupRetryAvailable {
		return PolicyDecision{Action: "stop", Reason: "attempt_budget_exhausted", Source: "global"}
	}
	if c != nil {
		if _, pinned := c.Get("specific_channel_id"); pinned {
			return PolicyDecision{Action: "stop", Reason: "pinned_channel", Source: "channel_constraint"}
		}
	}
	code := err.StatusCode
	if code >= 200 && code < 300 {
		return PolicyDecision{Action: "stop", Reason: "system_retry_exclusion", Source: "system"}
	}
	if code < 100 || code > 599 {
		return PolicyDecision{Action: "retry", Reason: "unrecognized_status", Source: "system"}
	}
	if operation_setting.IsAlwaysSkipRetryCode(err.GetErrorCode()) {
		return PolicyDecision{Action: "stop", Reason: "system_retry_exclusion", Source: "system"}
	}
	if operation_setting.ShouldRetryByStatusCode(code) {
		return PolicyDecision{Action: "retry", Reason: "retry_status_matched", Source: "global"}
	}
	return PolicyDecision{Action: "stop", Reason: "status_not_retryable", Source: "global"}
}

func ShouldRetryRelayError(c *gin.Context, openaiErr *types.NewAPIError, retryTimes int) bool {
	return DecideRelayRetry(c, openaiErr, retryTimes).Action == "retry"
}

func ProcessChannelError(c *gin.Context, channelError types.ChannelError, err *types.NewAPIError, relayInfo *relaycommon.RelayInfo) {
	if err == nil {
		return
	}
	logger.LogError(c, fmt.Sprintf("channel error (channel #%d, status code: %d): %s", channelError.ChannelId, err.StatusCode, common.LocalLogPreview(err.MaskSensitiveErrorWithStatusCode())))
	if ShouldDisableChannel(err) && channelError.AutoBan {
		reason := err.MaskSensitiveErrorWithStatusCode()
		gopool.Go(func() {
			DisableChannel(channelError, reason)
		})
	}

	if constant.ErrorLogEnabled && types.IsRecordErrorLog(err) {
		userId := c.GetInt("id")
		tokenName := c.GetString("token_name")
		modelName := c.GetString("original_model")
		tokenId := c.GetInt("token_id")
		userGroup := c.GetString("group")
		other := make(map[string]interface{})
		if c.Request != nil && c.Request.URL != nil {
			other["request_path"] = c.Request.URL.Path
		}
		other["error_type"] = err.GetErrorType()
		other["error_code"] = err.GetErrorCode()
		other["status_code"] = err.StatusCode
		adminInfo := map[string]interface{}{"use_channel": c.GetStringSlice("use_channel")}
		if relayInfo != nil {
			if mismatch := relayInfo.ResponseModelMismatch(); mismatch != nil {
				adminInfo["response_model_mismatch"] = mismatch
			}
			if diagnostics := relayInfo.ConversionDiagnostics(); len(diagnostics) > 0 {
				adminInfo["conversion_diagnostics"] = diagnostics
			}
		}
		AppendChannelAffinityAdminInfo(c, adminInfo)
		other["admin_info"] = model.MergeLogRequestAdminInfo(c, adminInfo)
		AppendTaskPluginContextAuditInfo(c, other)
		startTime := common.GetContextKeyTime(c, constant.ContextKeyRequestStartTime)
		if startTime.IsZero() {
			startTime = time.Now()
		}
		useTimeSeconds := int(time.Since(startTime).Seconds())
		model.RecordErrorLog(c, userId, channelError.ChannelId, modelName, tokenName, err.MaskSensitiveErrorWithStatusCode(), tokenId, useTimeSeconds, common.GetContextKeyBool(c, constant.ContextKeyIsStream), userGroup, other)
	}
}
