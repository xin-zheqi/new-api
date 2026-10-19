package xunfei

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestXunfeiResponseHasOutputOrUsage(t *testing.T) {
	tests := []struct {
		name string
		body string
		want bool
	}{
		{
			name: "input usage alone does not release an empty response",
			body: `{"payload":{"choices":{"text":[{"content":""}]},"usage":{"text":{"prompt_tokens":12,"completion_tokens":0,"total_tokens":12}}}}`,
		},
		{
			name: "missing output counter does not release an empty response",
			body: `{"payload":{"choices":{"text":[{"content":" \n "}]},"usage":{"text":{"prompt_tokens":12,"total_tokens":12}}}}`,
		},
		{
			name: "output usage releases the response",
			body: `{"payload":{"choices":{"text":[]},"usage":{"text":{"prompt_tokens":12,"completion_tokens":3,"total_tokens":15}}}}`,
			want: true,
		},
		{
			name: "text without usage releases the response",
			body: `{"payload":{"choices":{"text":[{"content":"hello"}]}}}`,
			want: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var response XunfeiChatResponse
			require.NoError(t, common.UnmarshalJsonStr(test.body, &response))
			assert.Equal(t, test.want, xunfeiResponseHasOutputOrUsage(response))
		})
	}
}
