package service

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestQuotaNotificationUsesFinalBalanceAndStoredCredentials(t *testing.T) {
	notificationLimitFixture(t)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	oldDB, oldClient, oldWorker := model.DB, httpClient, system_setting.WorkerUrl
	fetch := system_setting.GetFetchSetting()
	oldFetch := *fetch
	model.DB = db
	system_setting.WorkerUrl = ""
	fetch.EnableSSRFProtection = false
	t.Cleanup(func() {
		model.DB, httpClient, system_setting.WorkerUrl = oldDB, oldClient, oldWorker
		*fetch = oldFetch
		assert.NoError(t, sqlDB.Close())
	})
	require.NoError(t, db.AutoMigrate(&model.User{}))
	type delivery struct {
		body      []byte
		signature string
	}
	received := make(chan delivery, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		received <- delivery{body, r.Header.Get("X-Webhook-Signature")}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	httpClient = server.Client()
	for i, name := range []string{"wallet", "subscription"} {
		t.Run(name, func(t *testing.T) {
			id := 82000 + i
			key := fmt.Sprintf("%d:%s", id, dto.NotifyTypeQuotaExceed)
			notifyLimitStore.Delete(key)
			t.Cleanup(func() { notifyLimitStore.Delete(key) })
			fullSetting := dto.UserSetting{NotifyType: dto.NotifyTypeWebhook, WebhookUrl: server.URL, WebhookSecret: "local-fixture-secret"}
			raw, err := common.Marshal(fullSetting)
			require.NoError(t, err)
			require.NoError(t, db.Create(&model.User{Id: id, Username: name, AffCode: name, Setting: string(raw)}).Error)
			// Relay auth snapshots intentionally omit all transport credentials.
			info := &relaycommon.RelayInfo{UserId: id, UserQuota: 110, UserSetting: dto.UserSetting{NotifyType: dto.NotifyTypeWebhook}}
			if name == "wallet" {
				checkAndSendQuotaNotify(info, 10, 20)
			} else {
				info.SubscriptionId = 1
				info.SubscriptionAmountTotal = 200
				info.SubscriptionAmountUsedAfterPreConsume = 110
				info.SubscriptionPreConsumed = 20
				info.SubscriptionPostDelta = 10
				checkAndSendSubscriptionQuotaNotify(info)
			}
			select {
			case sent := <-received:
				var payload WebhookPayload
				require.NoError(t, common.Unmarshal(sent.body, &payload))
				assert.Equal(t, dto.NotifyTypeQuotaExceed, payload.Type)
				require.Len(t, payload.Values, 4)
				assert.Equal(t, logger.FormatQuota(80), payload.Values[1])
				assert.Contains(t, payload.Content, logger.FormatQuota(80))
				assert.NotContains(t, payload.Content, "{{value}}")
				assert.NotContains(t, payload.Content, "%!")
				assert.Equal(t, generateSignature(fullSetting.WebhookSecret, sent.body), sent.signature)
			case <-time.After(5 * time.Second):
				t.Fatal("quota notification was not delivered with stored credentials")
			}
		})
	}
}
