package service

import (
	"context"
	"io"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/contentaudit"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestAuditCachedPolicyPreservesActionsAndImmediateUpdates(t *testing.T) {
	for _, action := range []string{"warn", "replace", "block"} {
		t.Run(action, func(t *testing.T) {
			auditServiceFixture(t, action)
			server := miniredis.RunT(t)
			client := redis.NewClient(&redis.Options{Addr: server.Addr(), MaxRetries: -1})
			oldClient, oldEnabled := common.RDB, common.RedisEnabled
			common.RDB, common.RedisEnabled = client, true
			t.Cleanup(func() {
				common.RDB, common.RedisEnabled = oldClient, oldEnabled
				assert.NoError(t, client.Close())
			})
			var policyReads atomic.Int32
			require.NoError(t, model.DB.Callback().Query().After("gorm:query").Register("test:service_audit_policy_reads", func(tx *gorm.DB) {
				if tx.Statement.Table == "content_audit_policies" {
					policyReads.Add(1)
				}
			}))
			policy := contentaudit.Policy{Enabled: false, Rules: []contentaudit.Rule{{
				ID: "one", Name: "private rule", Enabled: true, Pattern: "secret", Mode: "all", Action: action, Replacement: "safe",
			}}}
			require.NoError(t, model.SaveContentAuditPolicy(context.Background(), 7, policy))
			// A cached inactive policy must not consume, validate, or rewrite the
			// request body; ordinary protocol validation still owns that work.
			for i := 0; i < 2; i++ {
				c := auditContext(strings.NewReader("not JSON"))
				require.Nil(t, AuditRelayContent(c))
				body, err := io.ReadAll(c.Request.Body)
				require.NoError(t, err)
				assert.Equal(t, "not JSON", string(body))
			}
			assert.EqualValues(t, 1, policyReads.Load())

			policy.Enabled = true
			require.NoError(t, model.SaveContentAuditPolicy(context.Background(), 7, policy))
			for i := 0; i < 2; i++ {
				c := auditContext(strings.NewReader(`{"model":"public","input":"secret"}`))
				err := AuditRelayContent(c)
				if action == "block" {
					require.NotNil(t, err)
					assert.Equal(t, 403, err.StatusCode)
				} else {
					require.Nil(t, err)
				}
				assert.Equal(t, action, c.Writer.Header().Get("X-Content-Audit-Action"))
				assert.Equal(t, "1", c.Writer.Header().Get("X-Content-Audit-Matches"))
				storage, storageErr := common.GetBodyStorage(c)
				require.NoError(t, storageErr)
				body, readErr := storage.Bytes()
				require.NoError(t, readErr)
				if action == "replace" {
					assert.JSONEq(t, `{"model":"public","input":"safe"}`, string(body))
				} else {
					assert.JSONEq(t, `{"model":"public","input":"secret"}`, string(body))
				}
				common.CleanupBodyStorage(c)
			}
			assert.EqualValues(t, 2, policyReads.Load(), "second enabled request must reuse the policy")
			records, total, err := model.ListContentAuditRecords(context.Background(), 7, model.ContentAuditFilter{Page: 1, PageSize: 20})
			require.NoError(t, err)
			assert.EqualValues(t, 2, total)
			require.Len(t, records, 2)
			for _, record := range records {
				assert.Equal(t, action, record.Action)
				assert.Equal(t, 1, record.MatchCount)
			}

			// Saving an enabled policy must also replace the prior compiled rules,
			// not just invalidate transitions between disabled and enabled.
			policy.Rules[0].Pattern = "changed"
			require.NoError(t, model.SaveContentAuditPolicy(context.Background(), 7, policy))
			unchanged := auditContext(strings.NewReader(`{"model":"public","input":"secret"}`))
			require.Nil(t, AuditRelayContent(unchanged))
			assert.Empty(t, unchanged.Writer.Header().Get("X-Content-Audit-Action"))
			common.CleanupBodyStorage(unchanged)
			changed := auditContext(strings.NewReader(`{"model":"public","input":"changed"}`))
			changedErr := AuditRelayContent(changed)
			if action == "block" {
				require.NotNil(t, changedErr)
				assert.Equal(t, 403, changedErr.StatusCode)
			} else {
				require.Nil(t, changedErr)
			}
			assert.Equal(t, action, changed.Writer.Header().Get("X-Content-Audit-Action"))
			changedStorage, err := common.GetBodyStorage(changed)
			require.NoError(t, err)
			changedBody, err := changedStorage.Bytes()
			require.NoError(t, err)
			if action == "replace" {
				assert.JSONEq(t, `{"model":"public","input":"safe"}`, string(changedBody))
			} else {
				assert.JSONEq(t, `{"model":"public","input":"changed"}`, string(changedBody))
			}
			common.CleanupBodyStorage(changed)
			assert.EqualValues(t, 3, policyReads.Load(), "an active edit must refresh SQL once and then reuse the new policy")

			policy.Enabled = false
			require.NoError(t, model.SaveContentAuditPolicy(context.Background(), 7, policy))
			c := auditContext(strings.NewReader("not JSON after disable"))
			require.Nil(t, AuditRelayContent(c))
			body, err := io.ReadAll(c.Request.Body)
			require.NoError(t, err)
			assert.Equal(t, "not JSON after disable", string(body))
			assert.Empty(t, c.Writer.Header().Get("X-Content-Audit-Action"))
			assert.EqualValues(t, 4, policyReads.Load(), "disabling rules must invalidate the enabled policy immediately")
		})
	}
}
