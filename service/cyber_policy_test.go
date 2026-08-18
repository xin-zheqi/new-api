package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectOpenAICyberPolicy(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		hit     bool
		message string
	}{
		{name: "top level error", payload: `{"error":{"code":"cyber_policy","message":"blocked"}}`, hit: true, message: "blocked"},
		{name: "responses failed event", payload: `{"type":"response.failed","response":{"error":{"code":"Cyber_Policy","message":"  denied  "}}}`, hit: true, message: "denied"},
		{name: "message alone is not enough", payload: `{"error":{"code":"content_policy","message":"cyber policy warning"}}`},
		{name: "other error", payload: `{"error":{"code":"server_error","message":"failed"}}`},
		{name: "invalid json", payload: `{`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			hit, message := DetectOpenAICyberPolicy([]byte(test.payload))
			assert.Equal(t, test.hit, hit)
			if test.hit {
				require.Equal(t, test.message, message)
			}
		})
	}
}
