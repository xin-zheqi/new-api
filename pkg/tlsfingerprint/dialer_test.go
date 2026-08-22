package tlsfingerprint

import (
	"testing"

	utls "github.com/refraction-networking/utls"
	"github.com/stretchr/testify/require"
)

func TestNodeClaudeCodeProfileBuildsExpectedClientHello(t *testing.T) {
	spec := buildClientHelloSpecFromProfile(&Profile{
		EnableGREASE:  true,
		ALPNProtocols: []string{"h2", "http/1.1"},
	})
	require.Equal(t, uint16(utls.VersionTLS13), spec.TLSVersMax)
	require.Equal(t, uint16(utls.VersionTLS10), spec.TLSVersMin)
	require.Equal(t, uint16(0x1301), spec.CipherSuites[0])
	require.GreaterOrEqual(t, len(spec.Extensions), 15)
	var hasSNI, hasALPN, hasECH bool
	for _, extension := range spec.Extensions {
		switch extension.(type) {
		case *utls.SNIExtension:
			hasSNI = true
		case *utls.ALPNExtension:
			hasALPN = true
		case *utls.GREASEEncryptedClientHelloExtension:
			hasECH = true
		}
	}
	require.True(t, hasSNI)
	require.True(t, hasALPN)
	require.True(t, hasECH)
}
