package tlsfingerprint

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"
)

// NewHTTPClient returns a client whose TLS ClientHello follows the captured
// Node.js 24.x Claude Code profile. The transport is deliberately isolated
// from service.GetHttpClient so other providers keep their existing behavior.
func NewHTTPClient(proxyURL string, timeout time.Duration) (*http.Client, error) {
	profile := &Profile{
		Name:          "nodejs-24-claude-code",
		EnableGREASE:  true,
		ALPNProtocols: []string{"h2", "http/1.1"},
	}
	var dialTLS func(ctx context.Context, network, addr string) (net.Conn, error)
	if proxyURL == "" {
		dialTLS = NewDialer(profile, nil).DialTLSContext
	} else {
		parsed, err := url.Parse(proxyURL)
		if err != nil {
			return nil, err
		}
		switch parsed.Scheme {
		case "http", "https":
			dialTLS = NewHTTPProxyDialer(profile, parsed).DialTLSContext
		case "socks5", "socks5h":
			dialTLS = NewSOCKS5ProxyDialer(profile, parsed).DialTLSContext
		default:
			return nil, fmt.Errorf("unsupported Claude Code proxy scheme %q", parsed.Scheme)
		}
	}
	transport := &http.Transport{
		ForceAttemptHTTP2: true,
		MaxIdleConns:      100,
		IdleConnTimeout:   90 * time.Second,
		DialTLSContext:    dialTLS,
	}
	client := &http.Client{Transport: transport}
	if timeout > 0 {
		client.Timeout = timeout
	}
	return client, nil
}
