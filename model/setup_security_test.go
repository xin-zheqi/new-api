package model

import (
	"errors"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestInitializeSystemAllowsOnlyOneConcurrentRootCreation(t *testing.T) {
	originalDB := DB
	t.Cleanup(func() { DB = originalDB })

	db, err := gorm.Open(sqlite.Open("file:setup-security?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Setup{}, &User{}, &Option{}))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	DB = db

	start := make(chan struct{})
	results := make(chan error, 2)
	var wait sync.WaitGroup
	for _, username := range []string{"first-root", "second-root"} {
		username := username
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			results <- InitializeSystem(&User{
				Username: username,
				Password: "hashed-password",
				Role:     common.RoleRootUser,
				Status:   common.UserStatusEnabled,
			}, nil, "test", 1)
		}()
	}
	close(start)
	wait.Wait()
	close(results)

	successes, alreadyCompleted := 0, 0
	for result := range results {
		switch {
		case result == nil:
			successes++
		case errors.Is(result, ErrSetupCompleted):
			alreadyCompleted++
		default:
			require.NoError(t, result)
		}
	}
	assert.Equal(t, 1, successes)
	assert.Equal(t, 1, alreadyCompleted)

	var rootCount int64
	require.NoError(t, db.Model(&User{}).Where("role = ?", common.RoleRootUser).Count(&rootCount).Error)
	assert.EqualValues(t, 1, rootCount)
	var setupCount int64
	require.NoError(t, db.Model(&Setup{}).Count(&setupCount).Error)
	assert.EqualValues(t, 1, setupCount)
}
