package model

import (
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newLogRequestTestContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	req.RemoteAddr = "203.0.113.9:1234"
	req.Header.Set("Authorization", "Bearer sk-secret")
	req.Header.Set("X-Custom-Header", "visible-value")
	req.Header.Set("User-Agent", "new-api-test")
	c.Request = req
	c.Set("username", "log-user")
	c.Set(common.RequestIdKey, "req-test")
	return c
}

func TestIsClickHouseDSN(t *testing.T) {
	cases := []struct {
		dsn  string
		want bool
	}{
		{"clickhouse://default:pass@localhost:9000/logs", true},
		{"tcp://localhost:9000/logs", true},
		{"http://localhost:8123/logs", true},
		{"https://localhost:8443/logs", true},
		{"postgres://root:pass@localhost:5432/db", false},
		{"postgresql://root:pass@localhost:5432/db", false},
		{"root:pass@tcp(localhost:3306)/db", false},
		{"local", false},
		{"", false},
	}
	for _, c := range cases {
		assert.Equalf(t, c.want, isClickHouseDSN(c.dsn), "dsn=%q", c.dsn)
	}
}

func TestNormalizeClickHouseDSN(t *testing.T) {
	// https without secure gets secure=true appended
	normalized := normalizeClickHouseDSN("https://default:pass@localhost:8443/logs")
	assert.Contains(t, normalized, "secure=true")
	assert.True(t, strings.HasPrefix(normalized, "https://"))

	// https that already specifies secure is left untouched
	assert.Equal(t,
		"https://localhost:8443/logs?secure=false",
		normalizeClickHouseDSN("https://localhost:8443/logs?secure=false"),
	)

	// non-https schemes are returned verbatim
	assert.Equal(t, "clickhouse://localhost:9000/logs", normalizeClickHouseDSN("clickhouse://localhost:9000/logs"))
	assert.Equal(t, "tcp://localhost:9000/logs", normalizeClickHouseDSN("tcp://localhost:9000/logs"))
}

func TestChooseDBRejectsClickHouseForMainDatabase(t *testing.T) {
	original, had := os.LookupEnv("SQL_DSN")
	t.Cleanup(func() {
		if had {
			require.NoError(t, os.Setenv("SQL_DSN", original))
		} else {
			require.NoError(t, os.Unsetenv("SQL_DSN"))
		}
	})
	require.NoError(t, os.Setenv("SQL_DSN", "clickhouse://default:pass@localhost:9000/logs"))

	db, dbType, err := chooseDB("SQL_DSN", false)
	require.Error(t, err)
	assert.Nil(t, db)
	assert.Equal(t, common.DatabaseType(""), dbType)
	assert.Contains(t, err.Error(), "does not support ClickHouse")
}

func TestClickHouseLogTTLExpression(t *testing.T) {
	assert.Equal(t, "", clickHouseLogTTLExpression(0))
	assert.Equal(t, "", clickHouseLogTTLExpression(-5))
	assert.Equal(t, "toDateTime(created_at) + INTERVAL 30 DAY DELETE", clickHouseLogTTLExpression(30))
}

func TestClickHouseLogTTLClause(t *testing.T) {
	assert.Equal(t, "", clickHouseLogTTLClause(0))
	assert.Equal(t, "\nTTL toDateTime(created_at) + INTERVAL 7 DAY DELETE", clickHouseLogTTLClause(7))
}

func TestClickHouseLogCreateTableSQL(t *testing.T) {
	withoutTTL := clickHouseLogCreateTableSQL(0)
	assert.Contains(t, withoutTTL, "CREATE TABLE IF NOT EXISTS logs")
	assert.Contains(t, withoutTTL, "ENGINE = MergeTree()")
	assert.Contains(t, withoutTTL, "PARTITION BY toYYYYMM(toDateTime(created_at))")
	assert.Contains(t, withoutTTL, "ORDER BY (created_at, request_id)")
	assert.NotContains(t, withoutTTL, "TTL ")

	withTTL := clickHouseLogCreateTableSQL(30)
	assert.Contains(t, withTTL, "ORDER BY (created_at, request_id)")
	assert.Contains(t, withTTL, "TTL toDateTime(created_at) + INTERVAL 30 DAY DELETE")
}

func TestClickHouseCreateTableHasTTL(t *testing.T) {
	assert.True(t, clickHouseCreateTableHasTTL("CREATE TABLE logs (...)\nTTL toDateTime(created_at) + INTERVAL 30 DAY DELETE"))
	assert.True(t, clickHouseCreateTableHasTTL("CREATE TABLE logs (...) TTL toDateTime(created_at)"))
	assert.False(t, clickHouseCreateTableHasTTL("CREATE TABLE logs (...)\nORDER BY (created_at, request_id)"))
}

func TestClickHouseLogOrder(t *testing.T) {
	assert.Equal(t, "created_at desc, request_id desc", clickHouseLogOrder(""))
	assert.Equal(t, "logs.created_at desc, logs.request_id desc", clickHouseLogOrder("logs."))
}

func TestEnsureLogRequestId(t *testing.T) {
	empty := &Log{}
	ensureLogRequestId(empty)
	assert.NotEmpty(t, empty.RequestId, "empty request id should be backfilled")

	existing := &Log{RequestId: "fixed-request-id"}
	ensureLogRequestId(existing)
	assert.Equal(t, "fixed-request-id", existing.RequestId, "existing request id must be preserved")

	assert.NotPanics(t, func() { ensureLogRequestId(nil) })
}

func TestAssignDisplayLogIds(t *testing.T) {
	logs := []*Log{{}, {}, {}}

	assignDisplayLogIds(logs, 0)
	assert.Equal(t, []int{1, 2, 3}, []int{logs[0].Id, logs[1].Id, logs[2].Id})

	assignDisplayLogIds(logs, 20)
	assert.Equal(t, []int{21, 22, 23}, []int{logs[0].Id, logs[1].Id, logs[2].Id})

	assert.NotPanics(t, func() { assignDisplayLogIds(nil, 0) })
}

func TestBuildLogRequestAdminInfoStoresUserAgentOnly(t *testing.T) {
	c := newLogRequestTestContext()

	adminInfo := BuildLogRequestAdminInfo(c)

	assert.Equal(t, "203.0.113.9", adminInfo["request_ip"])
	assert.Equal(t, "new-api-test", adminInfo["user_agent"])
	assert.NotContains(t, adminInfo, "request_headers")
}

func TestRecordConsumeLogStoresRequestContextOnlyForAdminView(t *testing.T) {
	truncateTables(t)
	c := newLogRequestTestContext()

	RecordConsumeLog(c, 42, RecordConsumeLogParams{
		ChannelId:      1,
		ModelName:      "gpt-test",
		TokenName:      "test-token",
		Quota:          10,
		Content:        "test consume",
		TokenId:        7,
		Other:          map[string]interface{}{"source": "unit-test"},
		UseTimeSeconds: 1,
	})

	var log Log
	require.NoError(t, LOG_DB.Where("user_id = ? AND type = ?", 42, LogTypeConsume).First(&log).Error)
	assert.Equal(t, "203.0.113.9", log.Ip)
	adminOther, err := common.StrToMap(log.Other)
	require.NoError(t, err)
	adminInfo, ok := adminOther["admin_info"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "203.0.113.9", adminInfo["request_ip"])
	assert.Equal(t, "new-api-test", adminInfo["user_agent"])
	assert.NotContains(t, adminInfo, "request_headers")

	userLogs, total, err := GetUserLogs(42, LogTypeConsume, 0, 0, "", "", 0, 10, "", "", "")
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, userLogs, 1)
	userOther, err := common.StrToMap(userLogs[0].Other)
	require.NoError(t, err)
	assert.NotContains(t, userOther, "admin_info")
	assert.Equal(t, "unit-test", userOther["source"])
}

func TestRecordSystemLogWithRequestContextStoresIPAndUserAgent(t *testing.T) {
	truncateTables(t)
	c := newLogRequestTestContext()
	c.Request.Header.Set(common.InternalRealIPHeader, "198.51.100.77")

	RecordSystemLogWithRequestContext(c, 42, "用户签到，获得额度 $1.00", "user.checkin", map[string]interface{}{
		"quota": 500000,
	})

	var log Log
	require.NoError(t, LOG_DB.Where("user_id = ? AND type = ?", 42, LogTypeSystem).First(&log).Error)
	assert.Equal(t, "198.51.100.77", log.Ip)

	adminOther, err := common.StrToMap(log.Other)
	require.NoError(t, err)
	adminInfo, ok := adminOther["admin_info"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "198.51.100.77", adminInfo["request_ip"])
	assert.Equal(t, "new-api-test", adminInfo["user_agent"])

	op, ok := adminOther["op"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "user.checkin", op["action"])

	userLogs, total, err := GetUserLogs(42, LogTypeSystem, 0, 0, "", "", 0, 10, "", "", "")
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, userLogs, 1)
	userOther, err := common.StrToMap(userLogs[0].Other)
	require.NoError(t, err)
	assert.NotContains(t, userOther, "admin_info")
	assert.Contains(t, userOther, "op")
}
