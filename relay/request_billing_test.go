package relay

import (
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestEmptySensitiveDictionaryPreservesOutputBudgetReservation(t *testing.T) {
	oldDB, oldRedis, oldCount := model.DB, common.RedisEnabled, constant.CountToken
	oldEnabled, oldPrompt, oldWords := setting.CheckSensitiveEnabled, setting.CheckSensitiveOnPromptEnabled, setting.SensitiveWords
	oldReserve := common.PreConsumedQuota
	oldRatios := ratio_setting.ModelRatio2JSONString()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "billing.db")), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() {
		model.DB, common.RedisEnabled, constant.CountToken = oldDB, oldRedis, oldCount
		setting.CheckSensitiveEnabled, setting.CheckSensitiveOnPromptEnabled, setting.SensitiveWords = oldEnabled, oldPrompt, oldWords
		common.PreConsumedQuota = oldReserve
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldRatios))
		_ = sqlDB.Close()
	})
	model.DB, common.RedisEnabled, constant.CountToken = db, false, false
	common.PreConsumedQuota = 100
	setting.CheckSensitiveEnabled, setting.CheckSensitiveOnPromptEnabled = true, true
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"relay-perf-budget":1}`))
	require.NoError(t, db.AutoMigrate(&model.User{}))
	require.NoError(t, db.Create(&model.User{Id: 7, Username: "perf-fixture", Quota: 0}).Error)
	for _, tt := range []struct {
		name, body string
		request    dto.Request
	}{
		{"chat", `{"max_tokens":128,"max_completion_tokens":256}`, &dto.GeneralOpenAIRequest{}},
		{"responses", `{"max_output_tokens":256}`, &dto.OpenAIResponsesRequest{}},
		{"claude", `{"max_tokens":256}`, &dto.ClaudeRequest{}},
		{"gemini", `{"generationConfig":{"maxOutputTokens":256}}`, &dto.GeminiChatRequest{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			require.NoError(t, common.Unmarshal([]byte(tt.body), tt.request))
			for _, words := range []string{"unused-marker", ""} {
				setting.SensitiveWordsFromString(words)
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
				info := &relaycommon.RelayInfo{Request: tt.request, UserId: 7, OriginModelName: "relay-perf-budget", UserGroup: "default", UsingGroup: "default"}
				info.UserSetting.BillingPreference = "wallet_only"
				apiErr := PrepareRequestBilling(c, info)
				require.NotNil(t, apiErr, "empty wallet must still fail pre-consume")
				assert.Equal(t, types.ErrorCodeInsufficientUserQuota, apiErr.GetErrorCode())
				assert.Equal(t, 356, info.PriceData.QuotaToPreConsume, "the output budget must survive skipping prompt extraction")
			}
		})
	}
}
