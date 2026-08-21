package service

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestResolveWaffoPancakeTradeNoPropagatesDatabaseFailure(t *testing.T) {
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "closed.db")), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, err = ResolveWaffoPancakeTradeNo(&WaffoPancakeWebhookEvent{Data: WaffoPancakeWebhookData{
		OrderMerchantExternalID: "db-failure-trade",
	}})
	require.Error(t, err)
	require.False(t, errors.Is(err, model.ErrTopUpNotFound), "database failures must not be collapsed into not-found")
}
