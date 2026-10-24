package common

import (
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDisabledFieldsKeepFilteringAndPassthroughContracts(t *testing.T) {
	old := model_setting.GetGlobalSettings().PassThroughRequestEnabled
	model_setting.GetGlobalSettings().PassThroughRequestEnabled = false
	t.Cleanup(func() { model_setting.GetGlobalSettings().PassThroughRequestEnabled = old })
	for _, tt := range []struct {
		name, body, want string
		settings         dto.ChannelOtherSettings
		unchanged        bool
	}{
		{"nested_only", `{"input":[{"content":"service_tier","metadata":{"service_tier":"flex"}}]}`, "", dto.ChannelOtherSettings{}, true},
		{"null_controlled_field", `{"service_tier":null,"model":"public"}`, `{"model":"public"}`, dto.ChannelOtherSettings{}, false},
		{"escaped_controlled_key", `{"service_\u0074ier":"flex","model":"public"}`, `{"model":"public"}`, dto.ChannelOtherSettings{}, false},
		{"explicit_false_store", `{"store":false,"model":"public"}`, `{"model":"public"}`, dto.ChannelOtherSettings{DisableStore: true}, false},
		{"stream_options_keep_usage", `{"stream_options":{"include_obfuscation":false,"include_usage":true}}`, `{"stream_options":{"include_usage":true}}`, dto.ChannelOtherSettings{}, false},
		{"all_allowed", `{ "service_tier":"flex", "inference_geo":"eu", "speed":"fast", "store":false, "safety_identifier":"id", "stream_options":{"include_obfuscation":false} }`, "", dto.ChannelOtherSettings{AllowServiceTier: true, AllowInferenceGeo: true, AllowSpeed: true, AllowSafetyIdentifier: true, AllowIncludeObfuscation: true}, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			body := []byte(tt.body)
			out, err := RemoveDisabledFields(body, tt.settings, false)
			require.NoError(t, err)
			if tt.unchanged {
				assert.Equal(t, tt.body, string(out))
			} else {
				assert.JSONEq(t, tt.want, string(out))
			}
			assert.Equal(t, tt.body, string(body), "filtering must preserve the retry source")
		})
	}
}
