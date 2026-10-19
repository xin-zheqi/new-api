package controller

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/contentaudit"
	"github.com/gin-gonic/gin"
)

// No administrator override: every operation uses the authenticated owner's ID.
// These endpoints do not accept a user_id from query strings or JSON.
func contentAuditAvailable(c *gin.Context) bool {
	c.Header("Cache-Control", "no-store")
	if !common.GetContentAuditSettings().Enabled {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Content audit is disabled"})
		return false
	}
	return true
}

func GetContentAuditPolicy(c *gin.Context) {
	if !contentAuditAvailable(c) {
		return
	}
	row, err := model.GetContentAuditPolicy(c.Request.Context(), c.GetInt("id"))
	if err != nil {
		common.ApiError(c, errors.New("Unable to load audit rules"))
		return
	}
	policy, err := model.DecodeContentAuditPolicy(row)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"policy": policy, "retention_days": common.GetContentAuditSettings().RetentionDays, "max_rules": contentaudit.MaxRules})
}

func UpdateContentAuditPolicy(c *gin.Context) {
	if !contentAuditAvailable(c) {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 24<<10)
	var policy contentaudit.Policy
	if err := common.DecodeJson(c.Request.Body, &policy); err != nil {
		common.ApiError(c, errors.New("Invalid audit policy"))
		return
	}
	if err := model.SaveContentAuditPolicy(c.Request.Context(), c.GetInt("id"), policy); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func TestContentAuditRules(c *gin.Context) {
	if !contentAuditAvailable(c) {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 40<<10)
	var input struct {
		Policy contentaudit.Policy `json:"policy"`
		Text   string              `json:"text"`
	}
	if err := common.DecodeJson(c.Request.Body, &input); err != nil || len(input.Text) > 8192 {
		common.ApiError(c, errors.New("Use up to 8192 bytes of synthetic test text"))
		return
	}
	rules, err := contentaudit.Compile(input.Policy)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	body, err := common.Marshal(map[string]string{"input": input.Text})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	updated, result, err := contentaudit.InspectJSON(body, rules)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var output map[string]string
	if err := common.Unmarshal(updated, &output); err != nil {
		common.ApiError(c, err)
		return
	}
	// Preview is never saved and never sent to a model provider.
	common.ApiSuccess(c, gin.H{"result": result, "text": output["input"]})
}

func ListContentAuditRecords(c *gin.Context) {
	if !contentAuditAvailable(c) {
		return
	}
	filter := model.ContentAuditFilter{Keyword: c.Query("keyword"), Action: c.Query("action"), Page: 1, PageSize: 20}
	if len(filter.Keyword) > 128 {
		common.ApiError(c, errors.New("Search is too long"))
		return
	}
	if filter.Action != "" && filter.Action != "warn" && filter.Action != "replace" && filter.Action != "block" {
		common.ApiError(c, errors.New("Invalid audit action"))
		return
	}
	for _, param := range []struct {
		key    string
		target *int64
	}{{"start_timestamp", &filter.Start}, {"end_timestamp", &filter.End}} {
		if value := c.Query(param.key); value != "" {
			n, err := strconv.ParseInt(value, 10, 64)
			if err != nil || n < 0 {
				common.ApiError(c, errors.New("Invalid time range"))
				return
			}
			*param.target = n
		}
	}
	if filter.End > 0 && filter.Start > filter.End {
		common.ApiError(c, errors.New("Invalid time range"))
		return
	}
	if value := c.Query("page"); value != "" {
		page, err := strconv.Atoi(value)
		if err != nil || page < 1 || page > 1000 {
			common.ApiError(c, errors.New("Invalid page"))
			return
		}
		filter.Page = page
	}
	rows, total, err := model.ListContentAuditRecords(c.Request.Context(), c.GetInt("id"), filter)
	if err != nil {
		common.ApiError(c, errors.New("Unable to load audit records"))
		return
	}
	common.ApiSuccess(c, gin.H{"items": rows, "total": total})
}

func GetContentAuditRecord(c *gin.Context) {
	if !contentAuditAvailable(c) {
		return
	}
	details, err := model.GetContentAuditDetails(c.Request.Context(), c.GetInt("id"), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Audit record is unavailable or expired"})
		return
	}
	common.ApiSuccess(c, details)
}

func DeleteContentAuditRecords(c *gin.Context) {
	if !contentAuditAvailable(c) {
		return
	}
	if err := model.DeleteContentAuditRecords(c.Request.Context(), c.GetInt("id"), c.Param("id")); err != nil {
		common.ApiError(c, errors.New("Unable to delete audit records"))
		return
	}
	common.ApiSuccess(c, nil)
}
