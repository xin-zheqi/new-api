package common

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
)

// Use fixed protocol payloads to measure local relay work, independently of
// network/model latency. Keep benchmarks separate from correctness assertions.
func BenchmarkResponseModelRelay(b *testing.B) {
	for _, fixture := range []struct{ name, body string }{
		{"text_delta", `{"type":"response.output_text.delta","sequence_number":3,"delta":"Hello from the model."}`},
		{"plain_delta", `{"type":"response.output_text.delta","sequence_number":3,"delta":"Hello there."}`},
		{"large_tool_delta", `{"type":"response.function_call_arguments.delta","delta":"` + strings.Repeat(`code line\n`, 4096) + `"}`},
		{"completed", `{"model":"public-model","output":[{"content":[{"type":"output_text","text":"` + strings.Repeat("A code explanation. ", 4096) + `"}]}]}`},
	} {
		b.Run(fixture.name, func(b *testing.B) {
			body := []byte(fixture.body)
			info := &RelayInfo{OriginModelName: "public-model", ChannelMeta: &ChannelMeta{}}
			b.SetBytes(int64(len(body)))
			b.ReportAllocs()
			for b.Loop() {
				if _, err := RewriteResponseModel(info, body); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkRequestDisabledFields(b *testing.B) {
	input := `"input":[{"role":"user","content":"` + strings.Repeat("Review this code and tool output. ", 8192) + `"}]`
	for _, fixture := range []struct {
		name, body string
		settings   dto.ChannelOtherSettings
	}{
		{"no_controlled_fields", `{` + input + `}`, dto.ChannelOtherSettings{}},
		{"service_tier", `{"service_tier":"flex",` + input + `}`, dto.ChannelOtherSettings{}},
		{"all_allowed", `{` + input + `}`, dto.ChannelOtherSettings{AllowServiceTier: true, AllowInferenceGeo: true, AllowSpeed: true, AllowSafetyIdentifier: true, AllowIncludeObfuscation: true}},
	} {
		b.Run(fixture.name, func(b *testing.B) {
			body := []byte(fixture.body)
			b.SetBytes(int64(len(body)))
			b.ReportAllocs()
			for b.Loop() {
				if _, err := RemoveDisabledFields(body, fixture.settings, false); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
