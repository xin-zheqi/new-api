package controller

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestUsageLogExportHTTP(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	previousDB, previousType := model.LOG_DB, common.LogDatabaseType()
	model.LOG_DB = db
	common.SetLogDatabaseType(common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		model.LOG_DB = previousDB
		common.SetLogDatabaseType(previousType)
		require.NoError(t, sqlDB.Close())
	})
	require.NoError(t, db.Create(&[]model.Log{
		{UserId: 10, Type: model.LogTypeConsume, Username: "alice", TokenName: "token", ModelName: "model", CreatedAt: 100, Other: `{"cache_tokens":3,"admin_info":{"secret":"private"}}`, Content: "private log content", Ip: "private ip"},
		{UserId: 20, Type: model.LogTypeConsume, Username: "bob", TokenName: "other", CreatedAt: 200},
		{UserId: 10, Type: model.LogTypeError, Username: "alice", TokenName: "error", Content: "private error"},
	}).Error)
	for _, tc := range []struct {
		name    string
		own     bool
		userID  int
		query   string
		want    []string
		wantNum int
	}{
		{"admin", false, 10, "", []string{"bob", "alice"}, 2},
		{"self cannot export another user", true, 10, "?username=bob&channel=999", []string{"alice"}, 1},
		{"error-only selection is empty", false, 10, "?type=5", nil, 0},
		{"request filters preserved", false, 10, "?token_name=token&start_timestamp=100&end_timestamp=100", []string{"alice"}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Set("id", tc.userID)
			c.Request = httptest.NewRequest(http.MethodGet, "/api/log/export"+tc.query, nil)
			if tc.own {
				ExportOwnUsageLogs(c)
			} else {
				ExportAllUsageLogs(c)
			}
			assert.Equal(t, http.StatusOK, recorder.Code)
			assert.Contains(t, recorder.Header().Get("Content-Type"), "application/x-ndjson")
			assert.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))
			assert.Equal(t, "no", recorder.Header().Get("X-Accel-Buffering"))
			var names []string
			var complete bool
			var count int
			scanner := bufio.NewScanner(strings.NewReader(recorder.Body.String()))
			for scanner.Scan() {
				var value map[string]interface{}
				require.NoError(t, common.Unmarshal(scanner.Bytes(), &value))
				if value["complete"] == true {
					complete = true
					count = int(value["count"].(float64))
					assert.False(t, scanner.Scan(), "completion marker must be final")
					break
				}
				require.Len(t, value, 9, "only spreadsheet columns may leave the server")
				names = append(names, value["username"].(string))
			}
			require.NoError(t, scanner.Err())
			assert.True(t, complete)
			assert.Equal(t, tc.wantNum, count)
			assert.Equal(t, tc.want, names)
			assert.NotContains(t, recorder.Body.String(), "private")
			assert.NotContains(t, recorder.Body.String(), "admin_info")
		})
	}
	t.Run("cancelled request never reports a complete export", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/log/export", nil).WithContext(ctx)
		ExportAllUsageLogs(c)
		assert.NotContains(t, recorder.Body.String(), `"complete":true`)
	})
	t.Run("unauthenticated self scope is rejected", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/log/self/export", nil)
		ExportOwnUsageLogs(c)
		assert.Equal(t, http.StatusUnauthorized, recorder.Code)
		assert.NotContains(t, recorder.Body.String(), "alice")
	})
}
