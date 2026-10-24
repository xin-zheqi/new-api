package contentaudit

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestAuditKeepsResponsesCiphertextOpaque(t *testing.T) {
	const body = `{"input":[{"type":"reasoning","id":"rs_1","encrypted_content":"secret+/=","summary":[]},{"type":"compaction","id":"cmp_1","encrypted_content":"secret+/="},{"type":"message","role":"assistant","id":"amsg_1","encrypted_content":"secret+/=","content":[{"type":"output_text","text":"answer"}]},{"role":"user","content":"secret"}]}`
	rules, err := Compile(Policy{Rules: []Rule{auditRule("replace", "secret", "safe")}})
	require.NoError(t, err)
	updated, result, err := InspectJSON([]byte(body), rules)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Count, "ciphertext is not audit text")
	assert.Equal(t, "safe", gjson.GetBytes(updated, "input.3.content").String())
	for _, path := range []string{"input.0", "input.1", "input.2"} {
		assert.JSONEq(t, gjson.Get(body, path).Raw, gjson.GetBytes(updated, path).Raw)
	}
}
