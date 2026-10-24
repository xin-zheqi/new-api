package common

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// ClientResponseModel keeps response metadata consistent with the client's
// request, including when only the upstream gateway performs model mapping.
// Never use this presentation value for routing, token counting or billing.
func ClientResponseModel(info *RelayInfo, current string) string {
	if info != nil && info.OriginModelName != "" {
		return info.OriginModelName
	}
	return current
}

// ResponseModelMismatch is admin-only diagnostic metadata, not a relay error.
// It reports identifiers declared by the upstream, not proof of the actual
// model used to generate the answer.
type ResponseModelMismatch struct {
	RequestedModel string   `json:"requested_model"`
	ResponseModels []string `json:"response_models"`
	Truncated      bool     `json:"truncated,omitempty"`
}

// Kept on ChannelMeta so retries start with a fresh set of observations. The
// bounded list avoids per-chunk allocations and unbounded log growth; the lock
// also permits Realtime events and log snapshots to run concurrently.
type responseModelObservations struct {
	mu        sync.Mutex
	models    []string
	truncated bool
}

func (info *RelayInfo) ObserveUpstreamResponseModel(model string) {
	if info == nil || info.ChannelMeta == nil || info.OriginModelName == "" || model == "" || model == info.OriginModelName {
		return
	}
	state := &info.ChannelMeta.responseModels
	state.mu.Lock()
	defer state.mu.Unlock()
	if len(model) > 256 {
		model = model[:256]
		for !utf8.ValidString(model) {
			model = model[:len(model)-1]
		}
		state.truncated = true
	}
	for _, observed := range state.models {
		if observed == model {
			return
		}
	}
	if len(state.models) >= 8 {
		state.truncated = true
		return
	}
	state.models = append(state.models, strings.Clone(model))
}

func (info *RelayInfo) ResponseModelMismatch() *ResponseModelMismatch {
	if info == nil || info.ChannelMeta == nil {
		return nil
	}
	state := &info.ChannelMeta.responseModels
	state.mu.Lock()
	defer state.mu.Unlock()
	if len(state.models) == 0 {
		return nil
	}
	return &ResponseModelMismatch{
		RequestedModel: info.OriginModelName,
		ResponseModels: append([]string(nil), state.models...),
		Truncated:      state.truncated,
	}
}

var responseModelPaths = [...]string{"model", "modelVersion", "response.model", "response.modelVersion", "message.model", "message.modelVersion", "session.model"}

// ObserveUpstreamResponseJSON runs on raw upstream metadata before protocol
// conversion. It must not be called on locally synthesized model identifiers.
func (info *RelayInfo) ObserveUpstreamResponseJSON(data []byte) {
	if info == nil || info.ChannelMeta == nil || info.OriginModelName == "" {
		return
	}
	// Most SSE deltas have no model metadata. Skip parsing them altogether;
	// this also keeps binary audio and other non-JSON formats out of the audit.
	data = bytes.TrimSpace(data)
	if len(data) == 0 || data[0] != '{' || (!bytes.Contains(data, []byte(`"model"`)) && !bytes.Contains(data, []byte(`"modelVersion"`))) {
		return
	}
	for _, path := range responseModelPaths {
		if model := gjson.GetBytes(data, path); model.Type == gjson.String {
			info.ObserveUpstreamResponseModel(model.String())
		}
	}
}

// RewriteResponseModel rewrites only protocol response metadata. It
// deliberately patches known response locations instead of recursively
// replacing every JSON field named "model", because tool payloads and vendor
// extensions may contain unrelated model values. Gemini's native
// modelVersion field is included because it is the model identifier in that
// protocol. Missing metadata is not invented, and matching responses return
// their original bytes. Payload text, tools, usage and encrypted items stay intact.
func RewriteResponseModel(info *RelayInfo, data []byte) ([]byte, error) {
	clientModel := ClientResponseModel(info, "")
	if clientModel == "" || len(data) == 0 {
		return data, nil
	}
	if !gjson.ValidBytes(data) {
		return data, fmt.Errorf("invalid response JSON")
	}
	// Most stream deltas contain no model metadata. Retain validation above,
	// and fall back to path lookup for escapes (e.g. a Unicode-escaped key).
	if !bytes.Contains(data, []byte("model")) && bytes.IndexByte(data, '\\') < 0 {
		return data, nil
	}

	patched := data
	for _, path := range responseModelPaths {
		// GetBytes copies only the selected value. ParseBytes would copy the
		// entire response even when it already has the correct public model.
		model := gjson.GetBytes(data, path)
		if !model.Exists() {
			continue
		}
		if model.Type == gjson.String {
			info.ObserveUpstreamResponseModel(model.String())
			if model.String() == clientModel {
				continue
			}
		}
		var err error
		patched, err = sjson.SetBytes(patched, path, clientModel)
		if err != nil {
			return data, err
		}
	}
	return patched, nil
}
