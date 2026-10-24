package helper

import (
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModelMappingConfigurationChangesPreserveRouting(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	for _, tc := range []struct {
		name, raw, want, errorText string
		mapped                     bool
	}{
		{"first channel", `{"public":"upstream-a"}`, "upstream-a", "", true},
		{"changed channel", `{"public":"upstream-b"}`, "upstream-b", "", true},
		{"removed mapping", `{}`, "public", "", false},
		{"restored mapping", `{"public":"upstream-a"}`, "upstream-a", "", true},
		{"chained mapping", `{"public":"middle","middle":"upstream-c"}`, "upstream-c", "", true},
		{"identity mapping", `{"public":"public"}`, "public", "", false},
		{"invalid edited config", `{"public":`, "", "unmarshal_model_mapping_failed", false},
		{"invalid field type", `{"public":42}`, "", "unmarshal_model_mapping_failed", false},
		{"cycle", `{"public":"middle","middle":"public"}`, "", "model_mapping_contains_cycle", false},
		{"corrected config", `{"public":"upstream-b"}`, "upstream-b", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c.Set("model_mapping", tc.raw)
			info := &relaycommon.RelayInfo{OriginModelName: "public", ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "public"}}
			request := &dto.OpenAIResponsesRequest{Model: "public", Input: []byte(`[{"role":"user","content":"unchanged"}]`)}
			err := ModelMappedHelper(c, info, request)
			if tc.errorText != "" {
				require.EqualError(t, err, tc.errorText)
				assert.Equal(t, "public", request.Model)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.want, request.Model)
				assert.Equal(t, tc.want, info.UpstreamModelName)
				assert.Equal(t, tc.mapped, info.IsModelMapped)
			}
			assert.Equal(t, "public", info.OriginModelName, "the client model used for billing metadata must not change")
			assert.Equal(t, `[{"role":"user","content":"unchanged"}]`, string(request.Input))
		})
	}
}

func TestModelMappingConcurrentRequestsKeepDistinctAliases(t *testing.T) {
	for _, model := range []string{"public-a", "public-b", "unmapped"} {
		t.Run(model, func(t *testing.T) {
			t.Parallel()
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Set("model_mapping", `{"public-a":"upstream-a","public-b":"upstream-b"}`)
			info := &relaycommon.RelayInfo{OriginModelName: model, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: model}}
			request := &dto.GeneralOpenAIRequest{Model: model}
			require.NoError(t, ModelMappedHelper(c, info, request))
			want := map[string]string{"public-a": "upstream-a", "public-b": "upstream-b", "unmapped": "unmapped"}[model]
			assert.Equal(t, want, request.Model)
			assert.Equal(t, model, info.OriginModelName)
		})
	}
}

// Exercise a large, valid configuration through the public mapping behavior;
// an optional cache must never become a new configuration-size restriction.
func TestModelMappingLargeConfigurationRemainsSupported(t *testing.T) {
	mapping := make(map[string]string, 2000)
	for i := range 2000 {
		mapping[fmt.Sprintf("public-model-%04d", i)] = fmt.Sprintf("upstream-model-release-%04d", i)
	}
	raw, err := common.Marshal(mapping)
	require.NoError(t, err)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("model_mapping", string(raw))
	info := &relaycommon.RelayInfo{OriginModelName: "public-model-1999", ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "public-model-1999"}}
	request := &dto.OpenAIResponsesRequest{Model: info.OriginModelName}
	require.NoError(t, ModelMappedHelper(c, info, request))
	assert.Equal(t, "upstream-model-release-1999", request.Model)
}

func BenchmarkModelMappingAcrossRequests(b *testing.B) {
	for _, size := range []int{1, 200} {
		b.Run(fmt.Sprint(size), func(b *testing.B) {
			mapping := make(map[string]string, size)
			for i := range size {
				mapping[fmt.Sprintf("public-model-%03d", i)] = fmt.Sprintf("upstream-model-%03d", i)
			}
			raw, err := common.Marshal(mapping)
			require.NoError(b, err)
			config := string(raw)
			b.ReportAllocs()
			for b.Loop() {
				// A new context models distinct incoming requests, not a same-request hit.
				c := &gin.Context{}
				c.Set("model_mapping", config)
				info := &relaycommon.RelayInfo{OriginModelName: "public-model-000", ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "public-model-000"}}
				request := &dto.OpenAIResponsesRequest{Model: info.OriginModelName}
				if err := ModelMappedHelper(c, info, request); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
