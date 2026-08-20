package common

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateSessionSecret(t *testing.T) {
	tests := []struct {
		name      string
		secret    string
		wantError bool
	}{
		{name: "empty", secret: "", wantError: true},
		{name: "known placeholder", secret: "random_string", wantError: true},
		{name: "too short", secret: strings.Repeat("a", SessionSecretMinLength-1), wantError: true},
		{name: "minimum length", secret: strings.Repeat("a", SessionSecretMinLength)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateSessionSecret(test.secret)
			if test.wantError {
				require.Error(t, err)
				return
			}
			assert.NoError(t, err)
		})
	}
}
