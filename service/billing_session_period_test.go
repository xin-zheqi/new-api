package service

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestBillingSessionRejectsExhaustedPeriodicKeyWithZeroEstimate(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "period-billing.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Token{}))
	previousDB := model.DB
	previousRedis := common.RedisEnabled
	mainType, logType := common.MainDatabaseType(), common.LogDatabaseType()
	model.DB = db
	common.RedisEnabled = false
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, logType)
	t.Cleanup(func() {
		model.DB = previousDB
		common.RedisEnabled = previousRedis
		common.SetDatabaseTypes(mainType, logType)
	})

	token := model.Token{
		UserId: 1, Key: "period-zero-estimate", Name: "period-zero-estimate",
		Status: common.TokenStatusEnabled, ExpiredTime: -1,
		UnlimitedQuota: true, QuotaPeriod: model.QuotaPeriodDay,
		QuotaPeriodLimit: 100, QuotaPeriodUsed: 100,
		QuotaPeriodStart: model.QuotaPeriodStart(model.QuotaPeriodDay, time.Now()),
	}
	require.NoError(t, db.Create(&token).Error)
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(nil)
	session := &BillingSession{
		relayInfo: &relaycommon.RelayInfo{
			TokenId: token.Id, TokenKey: token.Key,
			TokenQuotaPeriod: model.QuotaPeriodDay,
		},
		funding: &quotaBoundaryFunding{},
	}

	apiErr := session.preConsume(ctx, 0)
	require.NotNil(t, apiErr)
	require.Equal(t, types.ErrorCodePreConsumeTokenQuotaFailed, apiErr.GetErrorCode())
	require.Zero(t, session.preConsumedQuota)
}
