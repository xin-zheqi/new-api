package tlsfingerprint

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/QuantumNous/new-api/common"
)

// NewHTTPClient returns a client whose TLS ClientHello follows the captured
// Node.js 24.x Claude Code profile. The transport is deliberately isolated
// from service.GetHttpClient so other providers keep their existing behavior.
func NewHTTPClient(proxyURL string, timeout time.Duration) (*http.Client, error) {
	profile := &Profile{
		Name:               "nodejs-24-claude-code",
		EnableGREASE:       true,
		ALPNProtocols:      []string{"h2", "http/1.1"},
		InsecureSkipVerify: common.TLSInsecureSkipVerify,
	}
	var dialTLS func(ctx context.Context, network, addr string) (net.Conn, error)
	var dialContext func(ctx context.Context, network, addr string) (net.Conn, error)
	var proxyForHTTP func(*http.Request) (*url.URL, error)
	if proxyURL == "" {
		dialTLS = NewDialer(profile, nil).DialTLSContext
	} else {
		parsed, err := url.Parse(proxyURL)
		if err != nil {
			return nil, err
		}
		if parsed.Hostname() == "" {
			return nil, fmt.Errorf("Claude Code proxy URL has no host")
		}
		switch parsed.Scheme {
		case "http", "https":
			dialTLS = NewHTTPProxyDialer(profile, parsed).DialTLSContext
			if parsed.Scheme == "http" {
				// HTTPS requests use the custom CONNECT dialer above. Plain HTTP
				// requests still need the standard absolute-form proxy path.
				proxyForHTTP = http.ProxyURL(parsed)
			}
		case "socks5", "socks5h":
			socksDialer := NewSOCKS5ProxyDialer(profile, parsed)
			dialContext = socksDialer.DialContext
			dialTLS = socksDialer.DialTLSContext
		default:
			return nil, fmt.Errorf("unsupported Claude Code proxy scheme %q", parsed.Scheme)
		}
	}
	transport := &http.Transport{
		ForceAttemptHTTP2: true,
		MaxIdleConns:      100,
		IdleConnTimeout:   90 * time.Second,
		DialContext:       dialContext,
		DialTLSContext:    dialTLS,
		Proxy:             proxyForHTTP,
	}
	client := &http.Client{Transport: transport}
	if timeout > 0 {
		client.Timeout = timeout
	}
	return client, nil
}
