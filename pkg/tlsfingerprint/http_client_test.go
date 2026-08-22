package tlsfingerprint

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewHTTPClientKeepsProxyAndTransportScoped(t *testing.T) {
	client, err := NewHTTPClient("http://proxy.example:8080", 0)
	require.NoError(t, err)
	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)
	require.NotNil(t, transport.DialTLSContext)
	require.NotNil(t, transport.Proxy)

	upstreamURL, err := url.Parse("http://upstream.example/v1/messages")
	require.NoError(t, err)
	proxyURL, err := transport.Proxy(&http.Request{URL: upstreamURL})
	require.NoError(t, err)
	require.Equal(t, "proxy.example:8080", proxyURL.Host)
}

func TestNewHTTPClientRejectsProxyWithoutHost(t *testing.T) {
	_, err := NewHTTPClient("http://", 0)
	require.ErrorContains(t, err, "no host")
}
