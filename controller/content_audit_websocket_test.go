package controller

import (
	"context"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/contentaudit"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"net/http"
	"path/filepath"
	"testing"
)

func TestContentAuditResponsesWebSocketPerTurnBeforeBilling(t *testing.T) {
	incoming := make(chan string, 2)
	fixture := newResponsesWSBillingTest(t, `tier("request", 2000)`, func(ws *websocket.Conn, _ *http.Request) {
		for {
			_, body, err := ws.ReadMessage()
			if err != nil {
				return
			}
			incoming <- string(body)
			if err := ws.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.completed","response":{"id":"audit_response","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}}`)); err != nil {
				return
			}
		}
	})
	old := common.GetContentAuditSettings()
	oldContentAuditDir := common.ContentAuditDir
	common.ContentAuditDir = filepath.Join(t.TempDir(), "content-audit")
	t.Setenv("CRYPTO_SECRET", "audit-test-persistent")
	require.NoError(t, common.ApplyContentAuditSettings(`{"enabled":true,"retention_days":7}`))
	t.Cleanup(func() {
		common.ContentAuditDir = oldContentAuditDir
		encoded, _ := common.Marshal(old)
		_ = common.ApplyContentAuditSettings(string(encoded))
	})
	require.NoError(t, model.DB.AutoMigrate(&model.ContentAuditPolicy{}, &model.ContentAuditRecord{}, &model.ContentAuditSequence{}))
	policy := contentaudit.Policy{Enabled: true, Rules: []contentaudit.Rule{{ID: "privacy", Name: "Privacy", Enabled: true, Pattern: "secret", Mode: "all", Action: "replace", Replacement: "safe"}}}
	require.NoError(t, model.SaveContentAuditPolicy(context.Background(), fixture.user.Id, policy))
	// Wrapped response.create is what previously allowed middleware to cache
	// the envelope instead of the actual audited turn body.
	require.NoError(t, fixture.client.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.create","response":{"model":"ws-billing","input":"secret"}}`)))
	event := readResponsesWSTestEvent(t, fixture.client)
	require.Equal(t, "response.completed", event["type"])
	observed := <-incoming
	assert.Contains(t, observed, `"input":"safe"`)
	assert.NotContains(t, observed, "secret")
	policy.Rules[0].Action = "block"
	require.NoError(t, model.SaveContentAuditPolicy(context.Background(), fixture.user.Id, policy))
	require.NoError(t, fixture.client.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.create","model":"ws-billing","input":"secret"}`)))
	event = readResponsesWSTestEvent(t, fixture.client)
	assert.Equal(t, "error", event["type"])
	assert.EqualValues(t, 403, event["status"])
	fixture.closeAndWait(t)
	assert.Empty(t, incoming, "blocked turn must not contact upstream")
	assertResponsesWSAccounting(t, fixture, []int{1000})
	_, total, err := model.ListContentAuditRecords(context.Background(), fixture.user.Id, model.ContentAuditFilter{Page: 1, PageSize: 20})
	require.NoError(t, err)
	assert.EqualValues(t, 2, total)
}
