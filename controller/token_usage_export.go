package controller

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

var tokenUsageExports = make(chan struct{}, 2)

func ExportTokenUsage(c *gin.Context) {
	select {
	case tokenUsageExports <- struct{}{}:
		defer func() { <-tokenUsageExports }()
	default:
		c.JSON(http.StatusTooManyRequests, gin.H{"success": false, "message": "Another export is running; try again shortly"})
		return
	}
	start, end := int64(0), time.Now().Unix()
	for _, param := range []struct {
		key   string
		value *int64
	}{{"start_timestamp", &start}, {"end_timestamp", &end}} {
		if value := c.Query(param.key); value != "" {
			parsed, err := strconv.ParseInt(value, 10, 64)
			if err != nil || parsed < 0 {
				common.ApiError(c, errors.New("Invalid time range"))
				return
			}
			if parsed != 0 {
				*param.value = parsed
			}
		}
	}
	if start > end || len(c.Query("keyword")) > 128 {
		common.ApiError(c, errors.New("Invalid export filters"))
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	rows, err := model.GetTokenUsageExport(ctx, c.GetInt("id"), c.Query("keyword"), start, end)
	if err != nil {
		common.ApiError(c, errors.New("Unable to export usage; narrow the time or name filter and try again"))
		return
	}
	data, err := service.BuildTokenUsageWorkbook(rows, start, end, strings.HasPrefix(c.Query("lang"), "zh"))
	if err != nil {
		common.ApiError(c, errors.New("Unable to create usage workbook"))
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Header("Content-Disposition", `attachment; filename="token-usage.xlsx"`)
	c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", data)
}
