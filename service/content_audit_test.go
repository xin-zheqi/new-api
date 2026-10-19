package service

import (
	"bytes"
	"context"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/contentaudit"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func auditServiceFixture(t *testing.T, action string) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "audit.db")), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	oldDB := model.DB
	oldSettings := common.GetContentAuditSettings()
	oldContentAuditDir := common.ContentAuditDir
	model.DB = db
	common.ContentAuditDir = filepath.Join(t.TempDir(), "content-audit")
	t.Setenv("CRYPTO_SECRET", "test-audit-persistent-secret")
	require.NoError(t, common.ApplyContentAuditSettings(`{"enabled":true,"retention_days":7}`))
	require.NoError(t, db.AutoMigrate(&model.ContentAuditPolicy{}, &model.ContentAuditSequence{}, &model.ContentAuditRecord{}))
	t.Cleanup(func() {
		model.DB = oldDB
		common.ContentAuditDir = oldContentAuditDir
		encoded, _ := common.Marshal(oldSettings)
		_ = common.ApplyContentAuditSettings(string(encoded))
		_ = sqlDB.Close()
	})
	require.NoError(t, model.SaveContentAuditPolicy(context.Background(), 7, contentaudit.Policy{Enabled: true, Rules: []contentaudit.Rule{{ID: "one", Name: "secret", Enabled: true, Pattern: "secret", Mode: "all", Action: action, Replacement: "safe"}}}))
}
func auditContext(body io.Reader) *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", body)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("id", 7)
	return c
}
func TestAuditReplacementIsOnlyReplaySource(t *testing.T) {
	auditServiceFixture(t, "replace")
	c := auditContext(strings.NewReader(`{"model":"model","input":"secret secret"}`))
	defer common.CleanupBodyStorage(c)
	_, err := common.GetBodyStorage(c)
	require.NoError(t, err)
	require.Nil(t, AuditRelayContent(c))
	for _, label := range []string{"passthrough", "retry"} {
		storage, err := common.GetBodyStorage(c)
		require.NoError(t, err, label)
		reader, err := storage.NewReader()
		require.NoError(t, err, label)
		body, err := io.ReadAll(reader)
		require.NoError(t, err, label)
		_ = reader.Close()
		assert.JSONEq(t, `{"model":"model","input":"safe safe"}`, string(body), label)
	}
	require.Nil(t, AuditRelayContent(c))
	records, total, err := model.ListContentAuditRecords(context.Background(), 7, model.ContentAuditFilter{Page: 1, PageSize: 20})
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	details, err := model.GetContentAuditDetails(context.Background(), 7, records[0].ID)
	require.NoError(t, err)
	assert.Equal(t, 2, details.Count)
	_, err = model.GetContentAuditDetails(context.Background(), 8, records[0].ID)
	require.Error(t, err)
}
func TestAuditDisabledHasNoDatabaseOrBodyDependency(t *testing.T) {
	original := common.GetContentAuditSettings()
	oldDB := model.DB
	require.NoError(t, common.ApplyContentAuditSettings(`{"enabled":false,"retention_days":7}`))
	model.DB = nil
	t.Cleanup(func() {
		model.DB = oldDB
		encoded, _ := common.Marshal(original)
		_ = common.ApplyContentAuditSettings(string(encoded))
	})
	c := auditContext(strings.NewReader("not JSON"))
	require.Nil(t, AuditRelayContent(c))
	assert.False(t, c.GetBool("content_audit_completed"))
}
func TestAuditBlockReturns403AndDoesNotModifyBody(t *testing.T) {
	auditServiceFixture(t, "block")
	c := auditContext(strings.NewReader(`{"input":"secret"}`))
	defer common.CleanupBodyStorage(c)
	err := AuditRelayContent(c)
	require.NotNil(t, err)
	assert.Equal(t, 403, err.StatusCode)
	assert.Contains(t, err.Error(), "命中自定义敏感内容")
}
func TestMultipartReplacementDoesNotRewriteSameNamedFile(t *testing.T) {
	auditServiceFixture(t, "replace")
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("prompt", "secret"))
	file, err := writer.CreateFormFile("prompt", "image.bin")
	require.NoError(t, err)
	_, err = file.Write([]byte("secret file bytes"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	c := auditContext(&body)
	c.Request.Header.Set("Content-Type", writer.FormDataContentType())
	defer common.CleanupBodyStorage(c)
	require.Nil(t, AuditRelayContent(c))
	require.NoError(t, c.Request.ParseMultipartForm(1<<20))
	defer c.Request.MultipartForm.RemoveAll()
	assert.Equal(t, "safe", c.Request.FormValue("prompt"))
	reader, err := c.Request.MultipartForm.File["prompt"][0].Open()
	require.NoError(t, err)
	defer reader.Close()
	actual, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Equal(t, "secret file bytes", string(actual))
}
func TestAuditRetentionHardDeletesExpiredRowsWhileDisabled(t *testing.T) {
	auditServiceFixture(t, "warn")
	now := time.Now()
	require.NoError(t, model.InsertContentAuditRecord(context.Background(), model.ContentAuditRecord{UserID: 7, CreatedAt: now.Add(-8 * 24 * time.Hour).Unix(), Action: "warn"}, contentaudit.Result{Count: 1}))
	require.NoError(t, common.ApplyContentAuditSettings(`{"enabled":false,"retention_days":7}`))
	require.NoError(t, model.PurgeExpiredContentAuditRecords(context.Background(), now))
	var count int64
	require.NoError(t, model.DB.Unscoped().Model(&model.ContentAuditRecord{}).Count(&count).Error)
	assert.Zero(t, count)
}
