package common_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/jinzhu/copier"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeepCopyNilSource(t *testing.T) {
	got, err := common.DeepCopy[dto.OpenAIResponsesRequest](nil)
	require.Error(t, err)
	assert.Nil(t, got)
}

// RawMessage is opaque here: copying must not parse, normalize, or re-encode it.
func TestDeepCopyOpaqueBytes(t *testing.T) {
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"nil", nil},
		{"empty", []byte{}},
		{"binary", []byte{0, 255, 128, '\n', '\r', 0}},
		{"json", []byte(` { "encrypted_content": "opaque+/=\\x\u4e2d", "n":9007199254740993, "unknown":[false,null,0] } `)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			src := &struct {
				Raw  json.RawMessage
				Data []byte
			}{json.RawMessage(bytes.Clone(tc.data)), bytes.Clone(tc.data)}
			got, err := common.DeepCopy(src)
			require.NoError(t, err)
			assert.Equal(t, src, got)
			assert.Equal(t, tc.data == nil, got.Raw == nil)
			assert.Equal(t, tc.data == nil, got.Data == nil)
			if len(tc.data) > 0 {
				got.Raw[0] ^= 0xff
				got.Data[0] ^= 0xff
				assert.Equal(t, json.RawMessage(tc.data), src.Raw)
				assert.Equal(t, tc.data, src.Data)
			}
		})
	}
}

func TestDeepCopyNestedBuffersAndRetryIsolation(t *testing.T) {
	type namedBytes []byte // Other named slice types retain copier's existing path.
	type payload struct {
		Raw       json.RawMessage
		Bytes     []byte
		Pointer   *json.RawMessage
		Map       map[string]json.RawMessage
		List      []json.RawMessage
		Interface any
		Named     namedBytes
	}
	// Spare capacity makes sharing visible even when a caller only appends.
	raw := make(json.RawMessage, 3, 64)
	copy(raw, `123`)
	src := &payload{
		Raw: raw, Bytes: []byte{0, 1, 255}, Pointer: &raw,
		Map: map[string]json.RawMessage{"raw": raw}, List: []json.RawMessage{raw},
		Interface: map[string]any{"raw": raw, "bytes": []byte{2, 3}}, Named: namedBytes{4, 5},
	}
	first, err := common.DeepCopy(src)
	require.NoError(t, err)
	retry, err := common.DeepCopy(src)
	require.NoError(t, err)
	assert.Equal(t, src, first)
	first.Raw[0] = '9'
	first.Bytes[0] = 8
	(*first.Pointer)[1] = '8'
	first.Map["raw"][2] = '7'
	first.List[0][0] = '6'
	first.Interface.(map[string]any)["raw"].(json.RawMessage)[0] = '5'
	first.Interface.(map[string]any)["bytes"].([]byte)[0] = 4
	first.Named[0] = 3
	assert.Equal(t, json.RawMessage(`123`), src.Raw)
	assert.Equal(t, src, retry, "channel-local edits must not contaminate the next attempt")
	first.Raw = append(first.Raw, 'x')
	src.Raw = append(src.Raw, 'y')
	assert.Equal(t, byte('x'), first.Raw[3])
	assert.Equal(t, json.RawMessage(`123`), retry.Raw)
}

// Exercise the eight request DTOs used by relay handlers. The original and a
// second attempt must survive mutations to raw fields, nested history and
// explicit zero/false pointers in the first channel's private copy.
func TestDeepCopyRelayRequests(t *testing.T) {
	t.Run("responses", func(t *testing.T) {
		checkRequestCopy(t, `{"model":"public","input":[{"type":"reasoning","encrypted_content":"opaque+/=","summary":[]},{"type":"function_call","call_id":"call_1","name":"read","arguments":"{}"},{"type":"function_call_output","call_id":"call_1","output":"中文"}],"tools":[{"type":"function","name":"read","parameters":{"type":"object","properties":{}}}],"max_output_tokens":0,"stream":false,"temperature":0,"store":false,"reasoning":{"effort":"high"},"prompt_cache_key":"stable","prompt_cache_retention":"24h"}`, func(r *dto.OpenAIResponsesRequest) {
			require.NotNil(t, r.MaxOutputTokens)
			assert.Zero(t, *r.MaxOutputTokens)
			require.NotNil(t, r.Stream)
			assert.False(t, *r.Stream)
			r.Input[0] = ' '
			r.Tools[0] = ' '
			r.Reasoning.Effort = "low"
			*r.MaxOutputTokens = 32
			*r.Stream = true
			*r.Temperature = 1
			r.PromptCacheRetention[1] = 'X'
		})
	})
	t.Run("chat", func(t *testing.T) {
		checkRequestCopy(t, `{"model":"public","messages":[{"role":"assistant","content":"answer","reasoning_content":"native reasoning"},{"role":"user","content":[{"type":"text","text":"question"}]}],"max_tokens":0,"stream":false,"temperature":0,"response_format":{"type":"json_schema","json_schema":{"name":"test","schema":{"type":"object"}}},"metadata":{"large":9007199254740993},"enable_thinking":true}`, func(r *dto.GeneralOpenAIRequest) {
			r.Messages[1].Content.([]any)[0].(map[string]any)["text"] = "changed"
			r.Messages[0].Content = "changed"
			r.Metadata[0] = ' '
			r.ResponseFormat.JsonSchema[0] = ' '
			*r.MaxTokens = 32
			*r.Stream = true
		})
	})
	t.Run("claude", func(t *testing.T) {
		checkRequestCopy(t, `{"model":"public","system":[{"type":"text","text":"system","cache_control":{"type":"ephemeral"}}],"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"native thought","signature":"opaque-signature"}]}],"max_tokens":0,"stream":false,"tools":[{"name":"read","input_schema":{"type":"object"}}],"metadata":{"user_id":"fixture"},"cache_control":{"type":"ephemeral"}}`, func(r *dto.ClaudeRequest) {
			r.Messages[0].Content.([]any)[0].(map[string]any)["signature"] = "changed"
			r.System.([]any)[0].(map[string]any)["text"] = "changed"
			r.Tools.([]any)[0].(map[string]any)["name"] = "changed"
			r.CacheControl[0] = ' '
			*r.MaxTokens = 32
			*r.Stream = true
		})
	})
	t.Run("gemini", func(t *testing.T) {
		checkRequestCopy(t, `{"contents":[{"role":"user","parts":[{"text":"question"},{"inlineData":{"mimeType":"image/png","data":"AAEC"}}]}],"generationConfig":{"maxOutputTokens":0,"temperature":0},"systemInstruction":{"parts":[{"text":"system"}]},"tools":[{"functionDeclarations":[{"name":"read","parameters":{"type":"OBJECT"}}]}],"cachedContent":"cachedContents/fixture"}`, func(r *dto.GeminiChatRequest) {
			r.Contents[0].Parts[0].Text = "changed"
			r.SystemInstructions.Parts[0].Text = "changed"
			r.Tools[0] = ' '
			*r.GenerationConfig.MaxOutputTokens = 32
		})
	})
	t.Run("image", func(t *testing.T) {
		checkRequestCopy(t, `{"model":"public","prompt":"image","n":1,"stream":false,"watermark":false,"images":[{"image_url":"data:image/png;base64,AAEC"}],"vendor_payload":{"large":9007199254740993}}`, func(r *dto.ImageRequest) {
			r.Images[0] = ' '
			r.Extra["vendor_payload"][0] = ' '
			*r.N = 2
			*r.Stream = true
			*r.Watermark = true
		})
	})
	t.Run("audio", func(t *testing.T) {
		checkRequestCopy(t, `{"model":"public","input":"speech","voice":"alloy","speed":0,"metadata":{"large":9007199254740993},"ref_audio":"AAEC"}`, func(r *dto.AudioRequest) {
			r.Metadata[0] = ' '
			r.RefAudio[1] = 'X'
			*r.Speed = 1
		})
	})
	t.Run("embedding", func(t *testing.T) {
		checkRequestCopy(t, `{"model":"public","input":[[1,2],[3,4]],"dimensions":0,"temperature":0,"seed":0}`, func(r *dto.EmbeddingRequest) {
			r.Input.([]any)[0].([]any)[0] = float64(9)
			*r.Dimensions = 128
			*r.Temperature = 1
			*r.Seed = 1
		})
	})
	t.Run("rerank", func(t *testing.T) {
		checkRequestCopy(t, `{"model":"public","query":"question","documents":[{"text":"document","metadata":{"id":1}}],"top_n":0,"return_documents":false,"max_chunk_per_doc":0}`, func(r *dto.RerankRequest) {
			r.Documents[0].(map[string]any)["metadata"].(map[string]any)["id"] = float64(2)
			*r.TopN = 1
			*r.ReturnDocuments = true
			*r.MaxChunkPerDoc = 1
		})
	})
}

func checkRequestCopy[T any](t *testing.T, input string, mutate func(*T)) {
	t.Helper()
	var src T
	require.NoError(t, common.UnmarshalJsonStr(input, &src))
	before, err := common.Marshal(src)
	require.NoError(t, err)
	var legacy T
	require.NoError(t, copier.CopyWithOption(&legacy, &src, copier.Option{DeepCopy: true, IgnoreEmpty: true}))
	got, err := common.DeepCopy(&src)
	require.NoError(t, err)
	assert.Equal(t, &src, got, "all fields, including explicit zero/false, must survive")
	assert.Equal(t, &legacy, got, "the optimized path must preserve existing behavior")
	encoded, err := common.Marshal(got)
	require.NoError(t, err)
	assert.Equal(t, before, encoded)
	retry, err := common.DeepCopy(&src)
	require.NoError(t, err)
	mutate(got)
	assert.NotEqual(t, &src, got, "fixture must exercise an actual edit")
	assert.Equal(t, &legacy, &src, "the original must remain intact")
	assert.Equal(t, &src, retry, "another channel's copy must remain intact")
}

func TestDeepCopyConcurrentResponses(t *testing.T) {
	zero, disabled := 0, false
	src := &dto.OpenAIResponsesRequest{
		Input:               json.RawMessage(`[{"type":"reasoning","encrypted_content":"opaque+/=","summary":[]}]`),
		ReasoningConversion: &dto.ReasoningConversionState{Mode: "enabled", BudgetTokens: &zero, IncludeThoughts: &disabled},
	}
	for _, channel := range []string{"openai", "deepseek", "claude", "gemini"} {
		t.Run(channel, func(t *testing.T) {
			t.Parallel()
			got, err := common.DeepCopy(src)
			require.NoError(t, err)
			assert.Equal(t, src, got)
			got.Input[0] = ' '
			got.ReasoningConversion.Mode = channel
			*got.ReasoningConversion.BudgetTokens = 128
			*got.ReasoningConversion.IncludeThoughts = true
			assert.Equal(t, byte('['), src.Input[0])
			assert.Equal(t, "enabled", src.ReasoningConversion.Mode)
			assert.Zero(t, zero)
			assert.False(t, disabled)
		})
	}
}

func BenchmarkDeepCopyResponses(b *testing.B) {
	for _, size := range []int{0, 16 << 10, 256 << 10, 1 << 20} {
		src := &dto.OpenAIResponsesRequest{
			Model: "public", Input: json.RawMessage(`[{"role":"user","content":"` + strings.Repeat("x", size) + `"}]`),
			Reasoning: &dto.Reasoning{Effort: "high"},
		}
		b.Run(fmt.Sprintf("%d/bulk", size), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(src.Input)))
			for b.Loop() {
				_, err := common.DeepCopy(src)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
		b.Run(fmt.Sprintf("%d/legacy", size), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(src.Input)))
			for b.Loop() {
				var dst dto.OpenAIResponsesRequest
				if err := copier.CopyWithOption(&dst, src, copier.Option{DeepCopy: true, IgnoreEmpty: true}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
