package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSafeMallURLAllowsOnlyHTTPSWithoutCredentials(t *testing.T) {
	tests := []struct {
		name            string
		value           string
		applicationHost string
		expected        string
	}{
		{name: "cross-origin https", value: "https://shop.example.com/buy", applicationHost: "api.example.com", expected: "https://shop.example.com/buy"},
		{name: "same host", value: "https://api.example.com/shop", applicationHost: "api.example.com"},
		{name: "same host with application port", value: "https://api.example.com/shop", applicationHost: "api.example.com:3000"},
		{name: "http", value: "http://shop.example.com/buy", applicationHost: "api.example.com"},
		{name: "javascript", value: "javascript:alert(1)", applicationHost: "api.example.com"},
		{name: "credentials", value: "https://user:password@shop.example.com/buy", applicationHost: "api.example.com"},
		{name: "relative", value: "/buy", applicationHost: "api.example.com"},
		{name: "missing application host", value: "https://shop.example.com/buy"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expected, safeMallURL(test.value, test.applicationHost))
		})
	}
}
