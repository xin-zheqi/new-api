package service

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/types"
)

type openAICyberPolicyEnvelope struct {
	Error    *types.OpenAIError `json:"error"`
	Response *struct {
		Error *types.OpenAIError `json:"error"`
	} `json:"response"`
}

// DetectOpenAICyberPolicy matches OpenAI's explicit cyber_policy error code.
// Error messages are intentionally ignored to avoid false positives.
func DetectOpenAICyberPolicy(payload []byte) (bool, string) {
	if len(payload) == 0 {
		return false, ""
	}

	var envelope openAICyberPolicyEnvelope
	if err := common.Unmarshal(payload, &envelope); err != nil {
		return false, ""
	}
	errorValue := envelope.Error
	if errorValue == nil && envelope.Response != nil {
		errorValue = envelope.Response.Error
	}
	if errorValue == nil {
		return false, ""
	}

	code := strings.TrimSpace(fmt.Sprint(errorValue.Code))
	if !strings.EqualFold(code, string(types.ErrorCodeCyberPolicy)) {
		return false, ""
	}
	return true, strings.TrimSpace(errorValue.Message)
}
