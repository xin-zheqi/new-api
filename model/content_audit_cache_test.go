package model

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/contentaudit"
	"github.com/alicebob/miniredis/v2"
	"github.com/glebarez/sqlite"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func auditPolicyCacheFixture(t *testing.T, dialector gorm.Dialector) (*gorm.DB, *miniredis.Miniredis, *atomic.Int32) {
	t.Helper()
	t.Setenv("CRYPTO_SECRET", "audit-policy-cache-fixture-secret")
	db, err := gorm.Open(dialector, &gorm.Config{})
	require.NoError(t, err)
	connection, err := db.DB()
	require.NoError(t, err)
	connection.SetMaxOpenConns(4)
	require.NoError(t, db.AutoMigrate(&ContentAuditPolicy{}))
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr(), MaxRetries: -1})
	oldDB, oldRedis, oldEnabled := DB, common.RDB, common.RedisEnabled
	DB, common.RDB, common.RedisEnabled = db, client, true
	t.Cleanup(func() {
		DB, common.RDB, common.RedisEnabled = oldDB, oldRedis, oldEnabled
		assert.NoError(t, client.Close())
		assert.NoError(t, connection.Close())
	})
	queries := &atomic.Int32{}
	require.NoError(t, db.Callback().Query().After("gorm:query").Register("test:count_audit_reads", func(tx *gorm.DB) {
		if tx.Statement.Table == "content_audit_policies" {
			queries.Add(1)
		}
	}))
	return db, server, queries
}

func auditCachePolicy(enabled bool) contentaudit.Policy {
	return contentaudit.Policy{Enabled: enabled, Rules: []contentaudit.Rule{{
		ID: "one", Name: "private rule", Enabled: true, Pattern: "secret", Mode: "all", Action: "block",
	}}}
}

func TestContentAuditPolicyCacheDatabaseMatrix(t *testing.T) {
	cases := []struct {
		name string
		db   gorm.Dialector
	}{{"sqlite", sqlite.Open(filepath.Join(t.TempDir(), "cache.db"))}}
	if dsn := os.Getenv("TEST_MYSQL_DSN"); dsn != "" {
		cases = append(cases, struct {
			name string
			db   gorm.Dialector
		}{"mysql", mysql.Open(dsn)})
	}
	if dsn := os.Getenv("TEST_POSTGRES_DSN"); dsn != "" {
		cases = append(cases, struct {
			name string
			db   gorm.Dialector
		}{"postgres", postgres.Open(dsn)})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, server, queries := auditPolicyCacheFixture(t, tc.db)
			versionSQL := "SELECT version()"
			if tc.name == "sqlite" {
				versionSQL = "SELECT sqlite_version()"
			}
			var version string
			require.NoError(t, db.Raw(versionSQL).Scan(&version).Error)
			t.Logf("database version: %s", version)
			const userID = 62001
			require.NoError(t, db.Where("user_id IN ?", []int{userID, userID + 1}).Delete(&ContentAuditPolicy{}).Error)
			t.Cleanup(func() {
				assert.NoError(t, db.Where("user_id IN ?", []int{userID, userID + 1}).Delete(&ContentAuditPolicy{}).Error)
			})
			ctx := context.Background()
			row, err := GetContentAuditPolicyForRelay(ctx, userID)
			require.NoError(t, err)
			assert.False(t, row.Enabled)
			before := queries.Load()
			_, err = GetContentAuditPolicyForRelay(ctx, userID)
			require.NoError(t, err)
			assert.Equal(t, before, queries.Load(), "empty hit must avoid SQL")
			server.FastForward(time.Second)
			_, err = GetContentAuditPolicyForRelay(ctx, userID)
			require.NoError(t, err)
			server.FastForward(2 * time.Second)
			_, err = GetContentAuditPolicyForRelay(ctx, userID)
			require.NoError(t, err)
			assert.Equal(t, before+1, queries.Load(), "hits must not refresh the fixed expiry")

			// Independent client, same Redis: there is no node-local negative
			// snapshot that could survive a successful save on another node.
			otherNode := redis.NewClient(&redis.Options{Addr: server.Addr(), MaxRetries: -1})
			t.Cleanup(func() { assert.NoError(t, otherNode.Close()) })
			firstNode := common.RDB
			common.RDB = otherNode
			require.NoError(t, SaveContentAuditPolicy(ctx, userID, auditCachePolicy(true)))
			common.RDB = firstNode
			row, err = GetContentAuditPolicyForRelay(ctx, userID)
			require.NoError(t, err)
			assert.True(t, row.Enabled, "new enabled policy must immediately replace cached absence")
			before = queries.Load()
			_, err = GetContentAuditPolicyForRelay(ctx, userID)
			require.NoError(t, err)
			assert.Equal(t, before, queries.Load(), "active policy hits must also avoid SQL")
			state, err := server.Get(contentAuditPolicyCacheKey(userID))
			require.NoError(t, err)
			assert.NotContains(t, state, "private rule", "Redis must not contain decrypted rules")

			require.NoError(t, SaveContentAuditPolicy(ctx, userID, auditCachePolicy(false)))
			_, err = GetContentAuditPolicyForRelay(ctx, userID)
			require.NoError(t, err)
			before = queries.Load()
			_, err = GetContentAuditPolicyForRelay(ctx, userID)
			require.NoError(t, err)
			assert.Equal(t, before, queries.Load())
			row, err = GetContentAuditPolicy(ctx, userID)
			require.NoError(t, err)
			policy, err := DecodeContentAuditPolicy(row)
			require.NoError(t, err)
			assert.Equal(t, auditCachePolicy(false), policy, "settings must retain disabled rules")
			_, err = GetContentAuditPolicyForRelay(ctx, userID+1)
			require.NoError(t, err)
			assert.Equal(t, before+2, queries.Load(), "users must not share negative entries")

			// Redis failure must neither hide enabled rules nor turn a save into
			// a falsely acknowledged change while another node has old emptiness.
			server.SetError("ERR local Redis fixture unavailable")
			row, err = GetContentAuditPolicyForRelay(ctx, userID)
			require.NoError(t, err)
			assert.NotEmpty(t, row.EncryptedRules)
			err = SaveContentAuditPolicy(ctx, userID, auditCachePolicy(true))
			require.ErrorContains(t, err, "were not saved")
			row, err = GetContentAuditPolicy(ctx, userID)
			require.NoError(t, err)
			assert.False(t, row.Enabled)
			server.SetError("")
		})
	}
}

func TestContentAuditCacheDoesNotRepublishReadBeforeSave(t *testing.T) {
	db, _, _ := auditPolicyCacheFixture(t, sqlite.Open(filepath.Join(t.TempDir(), "late-read.db")))
	read := make(chan struct{})
	resume := make(chan struct{})
	var once sync.Once
	const userID = 62002
	require.NoError(t, db.Callback().Query().After("gorm:query").Register("test:pause_old_read", func(tx *gorm.DB) {
		if tx.Statement.Table == "content_audit_policies" {
			once.Do(func() { close(read); <-resume })
		}
	}))
	finished := make(chan error, 1)
	go func() { _, err := GetContentAuditPolicyForRelay(context.Background(), userID); finished <- err }()
	<-read
	err := SaveContentAuditPolicy(context.Background(), userID, auditCachePolicy(true))
	close(resume)
	require.NoError(t, err)
	require.NoError(t, <-finished)
	row, err := GetContentAuditPolicyForRelay(context.Background(), userID)
	require.NoError(t, err)
	assert.True(t, row.Enabled)
}

func TestContentAuditCacheDoesNotCacheDatabaseFailure(t *testing.T) {
	db, server, _ := auditPolicyCacheFixture(t, sqlite.Open(filepath.Join(t.TempDir(), "read-error.db")))
	queryErr := errors.New("fixture policy read failed")
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register("test:fail_policy_read", func(tx *gorm.DB) { tx.AddError(queryErr) }))
	_, err := GetContentAuditPolicyForRelay(context.Background(), 62003)
	require.ErrorIs(t, err, queryErr)
	assert.False(t, server.Exists(contentAuditPolicyCacheKey(62003)))
	require.NoError(t, db.Callback().Query().Remove("test:fail_policy_read"))
	require.NoError(t, SaveContentAuditPolicy(context.Background(), 62003, auditCachePolicy(true)))
	row, err := GetContentAuditPolicyForRelay(context.Background(), 62003)
	require.NoError(t, err)
	assert.True(t, row.Enabled)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = GetContentAuditPolicyForRelay(ctx, 62003)
	require.ErrorIs(t, err, context.Canceled)
}

func TestContentAuditSaveFinalInvalidationFailureIsExplicit(t *testing.T) {
	db, server, _ := auditPolicyCacheFixture(t, sqlite.Open(filepath.Join(t.TempDir(), "save-error.db")))
	require.NoError(t, db.Callback().Create().After("gorm:commit_or_rollback_transaction").Register("test:lose_redis_after_commit", func(tx *gorm.DB) {
		if tx.Statement.Table == "content_audit_policies" {
			server.SetError("ERR lost Redis acknowledgement")
		}
	}))
	err := SaveContentAuditPolicy(context.Background(), 62004, auditCachePolicy(true))
	require.ErrorContains(t, err, "were saved, but")
	server.SetError("")
	row, err := GetContentAuditPolicyForRelay(context.Background(), 62004)
	require.NoError(t, err)
	assert.True(t, row.Enabled, "uncleared barrier must fall back to SQL")
	state, err := server.Get(contentAuditPolicyCacheKey(62004))
	require.NoError(t, err)
	assert.Equal(t, "updating", state)
}

func TestContentAuditOverlappingSavesInvalidateInterveningRead(t *testing.T) {
	db, _, _ := auditPolicyCacheFixture(t, sqlite.Open(filepath.Join(t.TempDir(), "overlap.db")))
	paused := make(chan struct{})
	resume := make(chan struct{})
	var once sync.Once
	const userID = 62005
	require.NoError(t, db.Callback().Create().Before("gorm:begin_transaction").Register("test:pause_enabled_save", func(tx *gorm.DB) {
		row, ok := tx.Statement.Dest.(*ContentAuditPolicy)
		if ok && row.Enabled {
			once.Do(func() { close(paused); <-resume })
		}
	}))
	finished := make(chan error, 1)
	go func() { finished <- SaveContentAuditPolicy(context.Background(), userID, auditCachePolicy(true)) }()
	<-paused
	disabledErr := SaveContentAuditPolicy(context.Background(), userID, auditCachePolicy(false))
	// A second save may clear the first writer's barrier. Refill here to prove
	// the first writer's final unconditional invalidation removes that entry.
	row, readErr := GetContentAuditPolicyForRelay(context.Background(), userID)
	close(resume)
	enabledErr := <-finished
	require.NoError(t, disabledErr)
	require.NoError(t, readErr)
	assert.False(t, row.Enabled)
	require.NoError(t, enabledErr)
	row, err := GetContentAuditPolicyForRelay(context.Background(), userID)
	require.NoError(t, err)
	assert.True(t, row.Enabled)
}

func TestContentAuditCanceledSaveStillInvalidatesCache(t *testing.T) {
	db, server, _ := auditPolicyCacheFixture(t, sqlite.Open(filepath.Join(t.TempDir(), "canceled.db")))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, db.Callback().Create().After("gorm:commit_or_rollback_transaction").Register("test:cancel_after_commit", func(tx *gorm.DB) {
		if tx.Statement.Table == "content_audit_policies" {
			cancel()
		}
	}))
	require.NoError(t, SaveContentAuditPolicy(ctx, 62006, auditCachePolicy(true)))
	assert.False(t, server.Exists(contentAuditPolicyCacheKey(62006)), "cleanup must not inherit caller cancellation")
	row, err := GetContentAuditPolicyForRelay(context.Background(), 62006)
	require.NoError(t, err)
	assert.True(t, row.Enabled)
}

func TestContentAuditInvalidCachedPolicyFallsBackWithoutHidingRules(t *testing.T) {
	db, server, queries := auditPolicyCacheFixture(t, sqlite.Open(filepath.Join(t.TempDir(), "corrupt.db")))
	const userID = 62007
	require.NoError(t, SaveContentAuditPolicy(context.Background(), userID, auditCachePolicy(true)))
	for _, cached := range []string{`policy:{}`, `policy:{"enabled":false}`, `policy:broken`, "policy:" + strings.Repeat("x", contentAuditCacheMaxBytes), "loading:another-reader", "updating"} {
		server.Set(contentAuditPolicyCacheKey(userID), cached)
		before := queries.Load()
		row, err := GetContentAuditPolicyForRelay(context.Background(), userID)
		require.NoError(t, err)
		assert.True(t, row.Enabled)
		assert.Equal(t, before+1, queries.Load())
	}
	server.Del(contentAuditPolicyCacheKey(userID))
	require.NoError(t, db.Model(&ContentAuditPolicy{}).Where("user_id = ?", userID).Update("encrypted_rules", "invalid-ciphertext").Error)
	row, err := GetContentAuditPolicyForRelay(context.Background(), userID)
	require.NoError(t, err)
	_, err = DecodeContentAuditPolicy(row)
	require.Error(t, err)
	assert.False(t, server.Exists(contentAuditPolicyCacheKey(userID)), "decryption failure cannot populate the policy cache")
}

func TestContentAuditPolicyCacheWithRealRedis(t *testing.T) {
	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("TEST_REDIS_ADDR not configured")
	}
	_, _, queries := auditPolicyCacheFixture(t, sqlite.Open(filepath.Join(t.TempDir(), "redis.db")))
	client := redis.NewClient(&redis.Options{Addr: addr, MaxRetries: -1})
	t.Cleanup(func() { assert.NoError(t, client.Close()) })
	const userID = 62008
	ctx := context.Background()
	require.NoError(t, client.Ping(ctx).Err())
	require.NoError(t, client.Del(ctx, contentAuditPolicyCacheKey(userID)).Err())
	t.Cleanup(func() { assert.NoError(t, client.Del(ctx, contentAuditPolicyCacheKey(userID)).Err()) })
	common.RDB = client
	_, err := GetContentAuditPolicyForRelay(ctx, userID)
	require.NoError(t, err)
	before := queries.Load()
	_, err = GetContentAuditPolicyForRelay(ctx, userID)
	require.NoError(t, err)
	assert.Equal(t, before, queries.Load())
	require.NoError(t, SaveContentAuditPolicy(ctx, userID, auditCachePolicy(true)))
	row, err := GetContentAuditPolicyForRelay(ctx, userID)
	require.NoError(t, err)
	assert.True(t, row.Enabled)
	before = queries.Load()
	row, err = GetContentAuditPolicyForRelay(ctx, userID)
	require.NoError(t, err)
	assert.Equal(t, before, queries.Load())
	decoded, err := DecodeContentAuditPolicy(row)
	require.NoError(t, err)
	assert.Equal(t, auditCachePolicy(true), decoded)
}
