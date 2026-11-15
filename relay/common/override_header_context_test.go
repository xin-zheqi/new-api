package common

import (
	"maps"
	"testing"

	commonroot "github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildParamOverrideContextRequestHeadersRemainIsolated(t *testing.T) {
	for _, tt := range []struct {
		name    string
		headers map[string]string
		want    map[string]interface{}
	}{
		{name: "nil", want: map[string]interface{}{}},
		{name: "empty", headers: map[string]string{}, want: map[string]interface{}{}},
		{
			name: "normalized and filtered",
			headers: map[string]string{
				" X-Trace ":                     " trace-123 \t",
				"AUTHORIZATION":                 " Bearer {api_key} ",
				" ":                             "no-name",
				"X-Empty":                       " \t",
				commonroot.InternalRealIPHeader: "203.0.113.10",
			},
			want: map[string]interface{}{"x-trace": "trace-123", "authorization": "Bearer {api_key}"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			original := maps.Clone(tt.headers)
			info := &RelayInfo{RequestHeaders: tt.headers}
			ctx := BuildParamOverrideContext(info)
			headers, ok := ctx[paramOverrideContextRequestHeaders].(map[string]interface{})
			require.True(t, ok)
			require.NotNil(t, headers)
			assert.Equal(t, tt.want, headers)

			headers["x-trace"] = "request-local"
			assert.Equal(t, original, info.RequestHeaders)
			next := BuildParamOverrideContext(info)
			assert.Equal(t, tt.want, next[paramOverrideContextRequestHeaders], "later contexts must not reuse the mutable map")
		})
	}
}

func TestBuildParamOverrideContextPreservesHeaderOverrideSnapshots(t *testing.T) {
	for _, runtime := range []bool{false, true} {
		name := "channel headers"
		if runtime {
			name = "runtime headers"
		}
		t.Run(name, func(t *testing.T) {
			source := map[string]interface{}{
				" Authorization ":               " Bearer {api_key} ",
				" X-From-Client ":               " {client_header:X-Trace} ",
				" X-Count ":                     0,
				"X-False":                       false,
				" * ":                           "",
				" re:^x-trace- ":                " ",
				"regex:^x-client-":              "",
				" ":                             "no-name",
				"X-Empty":                       " \t",
				commonroot.InternalRealIPHeader: "203.0.113.10",
			}
			original := maps.Clone(source)
			info := &RelayInfo{
				UseRuntimeHeadersOverride: runtime,
				RuntimeHeadersOverride:    map[string]interface{}{"X-Ignored-Runtime": "ignored"},
				ChannelMeta: &ChannelMeta{
					HeadersOverride: source,
				},
			}
			if runtime {
				info.RuntimeHeadersOverride = source
				info.HeadersOverride = map[string]interface{}{"X-Ignored-Channel": "ignored"}
			}
			want := map[string]interface{}{
				"authorization": "Bearer {api_key}", "x-from-client": "{client_header:X-Trace}",
				"x-count": "0", "x-false": "false", "*": "", "re:^x-trace-": "", "regex:^x-client-": "",
			}
			ctx := BuildParamOverrideContext(info)
			headers, ok := ctx[paramOverrideContextHeaderOverride].(map[string]interface{})
			require.True(t, ok)
			assert.Equal(t, want, headers)
			headers["x-count"] = "local-change"
			delete(headers, "authorization")
			assert.Equal(t, original, source, "operations must not edit channel or runtime source headers")

			// A retry sees the current source, not the previous context snapshot.
			source[" X-Count "] = 1
			info.RetryIndex = 1
			want["x-count"] = "1"
			next := BuildParamOverrideContext(info)
			assert.Equal(t, want, next[paramOverrideContextHeaderOverride])
			assert.Equal(t, "local-change", headers["x-count"])
			assert.NotContains(t, headers, "authorization")
		})
	}
}

func TestParamOverrideContextStillSanitizesHeadersAfterMutation(t *testing.T) {
	info := &RelayInfo{ChannelMeta: &ChannelMeta{HeadersOverride: map[string]interface{}{"X-Static": "keep"}}}
	ctx := BuildParamOverrideContext(info)
	headers, ok := ctx[paramOverrideContextHeaderOverride].(map[string]interface{})
	require.True(t, ok)
	// Context consumers may add values after the initial snapshot. The final
	// synchronization must still enforce internal-header filtering and copying.
	headers[" X-Added "] = " added "
	headers[commonroot.InternalRealIPHeader] = "203.0.113.10"
	headers["X-Empty"] = " \t"
	syncRuntimeHeaderOverrideFromContext(info, ctx)
	require.True(t, info.UseRuntimeHeadersOverride)
	assert.Equal(t, map[string]interface{}{"x-static": "keep", "x-added": "added"}, info.RuntimeHeadersOverride)
	headers[" X-Added "] = "late mutation"
	assert.Equal(t, "added", info.RuntimeHeadersOverride["x-added"])
	assert.Equal(t, map[string]interface{}{"X-Static": "keep"}, info.HeadersOverride)
}
