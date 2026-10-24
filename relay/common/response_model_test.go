package common

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestRewriteResponseModelKeepsClientAlias(t *testing.T) {
	info := &RelayInfo{
		OriginModelName: "public-alias",
		ChannelMeta: &ChannelMeta{
			UpstreamModelName: "provider-model",
			IsModelMapped:     true,
		},
	}
	body := []byte(`{"model":"provider-model","modelVersion":"provider-model-v1","response":{"model":"provider-model","modelVersion":"provider-model-v1"},"message":{"model":"provider-model"},"output":[{"model":"keep-nested"}]}`)

	patched, err := RewriteResponseModel(info, body)
	require.NoError(t, err)
	require.Equal(t, "public-alias", gjson.GetBytes(patched, "model").String())
	require.Equal(t, "public-alias", gjson.GetBytes(patched, "modelVersion").String())
	require.Equal(t, "public-alias", gjson.GetBytes(patched, "response.model").String())
	require.Equal(t, "public-alias", gjson.GetBytes(patched, "response.modelVersion").String())
	require.Equal(t, "public-alias", gjson.GetBytes(patched, "message.model").String())
	require.Equal(t, "keep-nested", gjson.GetBytes(patched, "output.0.model").String())
}

func TestResponseModelNormalizationPreservesMatchingAndAbsentMetadata(t *testing.T) {
	info := &RelayInfo{OriginModelName: "client-model", ChannelMeta: &ChannelMeta{}}
	for _, body := range []string{
		`{ "model": "client-model", "usage":{"total_tokens":7} }`,
		`{"output":[{"model":"tool-argument"}],"text":"provider-model","reasoning":{"text":"model-version"}}`,
	} {
		out, err := RewriteResponseModel(info, []byte(body))
		require.NoError(t, err)
		assert.Equal(t, body, string(out))
		assert.Nil(t, info.ResponseModelMismatch())
	}
	data := []byte(`{"session":{"model":"provider-model"},"response":{"model":"provider-model"},"usage":{"total_tokens":7}}`)
	out, err := RewriteResponseModel(info, data)
	require.NoError(t, err)
	assert.Equal(t, "client-model", gjson.GetBytes(out, "session.model").String())
	assert.Equal(t, "client-model", gjson.GetBytes(out, "response.model").String())
	assert.Equal(t, int64(7), gjson.GetBytes(out, "usage.total_tokens").Int())
	require.NotNil(t, info.ResponseModelMismatch())
	assert.Equal(t, []string{"provider-model"}, info.ResponseModelMismatch().ResponseModels)
}

func TestResponseModelAuditOnlyRecordsObservedMetadataAndResetsOnRetry(t *testing.T) {
	info := &RelayInfo{OriginModelName: "client-model", ChannelMeta: &ChannelMeta{UpstreamModelName: "mapped-target", IsModelMapped: true}}
	assert.Equal(t, "client-model", ClientResponseModel(info, "mapped-target"))
	assert.Nil(t, info.ResponseModelMismatch(), "a configured mapping is not evidence of an upstream response")
	info.ObserveUpstreamResponseJSON([]byte(`{"model":"provider-v2","response":{"model":"provider-v2"},"output":[{"model":"ignore"}]}`))
	snapshot := info.ResponseModelMismatch()
	require.NotNil(t, snapshot)
	assert.Equal(t, "client-model", snapshot.RequestedModel)
	assert.Equal(t, []string{"provider-v2"}, snapshot.ResponseModels)
	info.ObserveUpstreamResponseModel("provider-v3")
	assert.Equal(t, []string{"provider-v2"}, snapshot.ResponseModels, "previous log snapshots remain immutable")
	c, _ := gin.CreateTestContext(nil)
	info.InitChannelMeta(c)
	assert.Nil(t, info.ResponseModelMismatch(), "the next channel cannot inherit the previous attempt's models")
}

func TestResponseModelAuditBoundsDistinctNamesAndKeepsUTF8(t *testing.T) {
	info := &RelayInfo{OriginModelName: "client-model", ChannelMeta: &ChannelMeta{}}
	for _, model := range []string{"one", "two", "three", "four", "five", "six", "seven", "eight", "nine", "one"} {
		info.ObserveUpstreamResponseModel(model)
	}
	audit := info.ResponseModelMismatch()
	require.NotNil(t, audit)
	assert.Equal(t, []string{"one", "two", "three", "four", "five", "six", "seven", "eight"}, audit.ResponseModels)
	assert.True(t, audit.Truncated)
	info.ChannelMeta = &ChannelMeta{}
	info.ObserveUpstreamResponseModel(strings.Repeat("模型", 100))
	audit = info.ResponseModelMismatch()
	require.NotNil(t, audit)
	require.Len(t, audit.ResponseModels, 1)
	assert.True(t, utf8.ValidString(audit.ResponseModels[0]))
	assert.LessOrEqual(t, len(audit.ResponseModels[0]), 256)
	assert.True(t, audit.Truncated)
}

func TestRewriteResponseModelAlsoNormalizesUpstreamOnlyMapping(t *testing.T) {
	info := &RelayInfo{
		OriginModelName: "public-model",
		ChannelMeta: &ChannelMeta{
			UpstreamModelName: "provider-model",
			IsModelMapped:     false,
		},
	}
	body := []byte(`{"model":"provider-model"}`)

	patched, err := RewriteResponseModel(info, body)
	require.NoError(t, err)
	require.JSONEq(t, `{"model":"public-model"}`, string(patched))
	require.Equal(t, "public-model", ClientResponseModel(info, "provider-model"))
}

func TestRewriteResponseModelRejectsInvalidJSON(t *testing.T) {
	info := &RelayInfo{
		OriginModelName: "public-alias",
		ChannelMeta:     &ChannelMeta{IsModelMapped: true},
	}

	_, err := RewriteResponseModel(info, []byte(`{"model":`))
	require.Error(t, err)
}

func TestRewriteResponseModelPreservesEscapedMetadataAndOpaquePayloads(t *testing.T) {
	for _, tt := range []struct {
		name, body, want string
		observed         []string
	}{
		{"escaped_key", `{"\u006dodel":"private-model","output":[]}`, `{"model":"public-model","output":[]}`, []string{"private-model"}},
		{"escaped_envelope", `{"respon\u0073e":{"model":"private-model"}}`, `{"response":{"model":"public-model"}}`, []string{"private-model"}},
		{"opaque_tool_data", `{"type":"response.function_call_arguments.delta","delta":"{\"model\":\"private-model\"}"}`, `{"type":"response.function_call_arguments.delta","delta":"{\"model\":\"private-model\"}"}`, nil},
		{"null_metadata", `{"model":null,"usage":{"output_tokens":2}}`, `{"model":"public-model","usage":{"output_tokens":2}}`, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			info := &RelayInfo{OriginModelName: "public-model", ChannelMeta: &ChannelMeta{}}
			body := []byte(tt.body)
			out, err := RewriteResponseModel(info, body)
			require.NoError(t, err)
			assert.JSONEq(t, tt.want, string(out))
			assert.Equal(t, tt.body, string(body), "retry/source bytes must stay immutable")
			if tt.observed == nil {
				assert.Nil(t, info.ResponseModelMismatch())
			} else {
				require.NotNil(t, info.ResponseModelMismatch())
				assert.Equal(t, tt.observed, info.ResponseModelMismatch().ResponseModels)
			}
		})
	}
	info := &RelayInfo{OriginModelName: "public-model", ChannelMeta: &ChannelMeta{}}
	_, err := RewriteResponseModel(info, []byte(`{"delta":`))
	require.Error(t, err, "missing model metadata must not bypass JSON validation")
}
