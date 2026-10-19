package helper

import (
	"strings"
	"testing"
)

func BenchmarkResponseFramingDetection(b *testing.B) {
	// Pretty-printed Responses payloads contain many lines, but framing is
	// determined by the first non-empty line, regardless of the output size.
	body := []byte("{\n  \"output\": [\n" + strings.Repeat("    {},\n", 2048) + "    {}\n  ]\n}")
	b.ReportAllocs()
	for b.Loop() {
		if looksLikeEventStream(body) {
			b.Fatal("JSON was classified as SSE")
		}
	}
}
