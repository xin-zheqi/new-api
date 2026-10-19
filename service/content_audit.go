package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/contentaudit"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
)

// Only compiled rules enter this bounded cache; request text and matches never
// do. The encrypted policy hash changes on every save, also across instances.
var auditRuleCache = struct {
	sync.Mutex
	items map[[32]byte][]contentaudit.CompiledRule
	order [][32]byte
}{items: make(map[[32]byte][]contentaudit.CompiledRule)}

func contentAuditRules(ctx context.Context, userID int) ([]contentaudit.CompiledRule, error) {
	row, err := model.GetContentAuditPolicy(ctx, userID)
	if err != nil || !row.Enabled {
		return nil, err
	}
	key := sha256.Sum256([]byte(row.EncryptedRules))
	auditRuleCache.Lock()
	rules, ok := auditRuleCache.items[key]
	auditRuleCache.Unlock()
	if ok {
		return rules, nil
	}
	policy, err := model.DecodeContentAuditPolicy(row)
	if err != nil {
		return nil, err
	}
	rules, err = contentaudit.Compile(policy)
	if err != nil {
		return nil, err
	}
	auditRuleCache.Lock()
	defer auditRuleCache.Unlock()
	if _, exists := auditRuleCache.items[key]; !exists {
		if len(auditRuleCache.order) >= 64 {
			delete(auditRuleCache.items, auditRuleCache.order[0])
			auditRuleCache.order = auditRuleCache.order[1:]
		}
		auditRuleCache.order = append(auditRuleCache.order, key)
		auditRuleCache.items[key] = rules
	}
	return rules, nil
}

func auditError(message string, status int) *types.NewAPIError {
	return types.NewErrorWithStatusCode(errors.New(message), types.ErrorCode("content_audit"), status, types.ErrOptionWithSkipRetry())
}

// AuditRelayContent runs before billing and before retries. A replaced body
// becomes the sole replay source, including for passthrough channels.
func AuditRelayContent(c *gin.Context) *types.NewAPIError {
	if !common.GetContentAuditSettings().Enabled {
		return nil
	}
	if c.GetBool("content_audit_completed") {
		return nil
	}
	rules, err := contentAuditRules(c.Request.Context(), c.GetInt("id"))
	if err != nil {
		return auditError("content audit is temporarily unavailable", http.StatusServiceUnavailable)
	}
	if len(rules) == 0 {
		return nil
	}
	if c.Request.Method == http.MethodGet && !strings.HasSuffix(c.Request.URL.Path, "/responses") {
		return auditError("content audit does not support Realtime audio sessions; disable your audit policy before using this endpoint", http.StatusBadRequest)
	}
	if strings.Contains(c.Request.Header.Get("Content-Type"), "multipart/") {
		return auditMultipartContent(c, rules)
	}
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return auditError("cannot read request for content audit", http.StatusBadRequest)
	}
	if storage.Size() > contentaudit.MaxBodyBytes {
		return auditError(contentaudit.ErrLimit.Error(), http.StatusRequestEntityTooLarge)
	}
	body, err := storage.Bytes()
	if err != nil {
		return auditError("cannot read request for content audit", http.StatusBadRequest)
	}
	updated, result, err := contentaudit.InspectJSON(body, rules)
	if err != nil {
		if errors.Is(err, contentaudit.ErrLimit) {
			return auditError(err.Error(), http.StatusRequestEntityTooLarge)
		}
		return auditError("invalid JSON content for content audit", http.StatusBadRequest)
	}
	var metadata struct {
		Model string `json:"model"`
	}
	_ = common.Unmarshal(body, &metadata)
	if metadata.Model == "" {
		metadata.Model = c.GetString("original_model")
	}
	if metadata.Model == "" {
		metadata.Model = c.Param("model")
	}
	if apiErr := recordContentAuditResult(c, result, metadata.Model); apiErr != nil {
		return apiErr
	}
	if !bytes.Equal(updated, body) {
		if err := replaceAuditedBody(c, updated); err != nil {
			return auditError("cannot prepare audited request", http.StatusInternalServerError)
		}
	}
	c.Set("content_audit_completed", true)
	return nil
}

func replaceAuditedBody(c *gin.Context, data []byte) error {
	storage, err := common.CreateBodyStorage(data)
	if err != nil {
		return err
	}
	common.CleanupBodyStorage(c)
	c.Set(common.KeyRequestBody, nil)
	c.Set(common.KeyBodyStorage, storage)
	c.Request.Body = io.NopCloser(storage)
	c.Request.ContentLength = storage.Size()
	c.Request.Header.Del("Content-Length")
	// Any previously parsed form refers to the original body.
	c.Request.PostForm = nil
	c.Request.Form = nil
	if c.Request.MultipartForm != nil {
		_ = c.Request.MultipartForm.RemoveAll()
		c.Request.MultipartForm = nil
	}
	return nil
}

func recordContentAuditResult(c *gin.Context, result contentaudit.Result, modelName string) *types.NewAPIError {
	if result.Count == 0 {
		return nil
	}
	// Keep successful provider response bodies untouched; HTTP clients can
	// discover warnings via metadata without receiving private match content.
	c.Header("X-Content-Audit-Action", result.Action)
	c.Header("X-Content-Audit-Matches", strconv.Itoa(result.Count))
	row := model.ContentAuditRecord{UserID: c.GetInt("id"), CreatedAt: time.Now().Unix(), RequestID: clippedAuditMetadata(c.GetString(common.RequestIdKey), 128), TokenName: clippedAuditMetadata(c.GetString("token_name"), 255), Model: clippedAuditMetadata(modelName, 255), Endpoint: clippedAuditMetadata(c.Request.URL.Path, 255), Action: result.Action, MatchCount: result.Count}
	err := model.InsertContentAuditRecord(c.Request.Context(), row, result)
	if err != nil {
		common.SysError("content audit record could not be saved")
	}
	if result.Action == "block" {
		return auditError("命中自定义敏感内容 (custom sensitive content matched); request blocked by your content audit rules", http.StatusForbidden)
	}
	if err != nil {
		return auditError("content audit record could not be saved; request was not forwarded", http.StatusServiceUnavailable)
	}
	return nil
}

func clippedAuditMetadata(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return strings.ToValidUTF8(value[:limit-3], "") + "..."
}

// Cleanup continues while auditing is disabled. One bounded goroutine and a
// timed DELETE avoid per-record timers, unbounded queues and content caches.
func RunContentAuditRetention(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for {
		cleanupCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		if err := model.PurgeExpiredContentAuditRecords(cleanupCtx, time.Now()); err != nil {
			common.SysError("content audit retention cleanup failed")
		}
		if err := model.PruneOrphanContentAuditDetails(cleanupCtx); err != nil {
			common.SysError("content audit orphan cleanup failed")
		}
		cancel()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
