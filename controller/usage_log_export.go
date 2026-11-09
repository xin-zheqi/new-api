package controller

import (
	"bufio"
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

var usageLogExports = make(chan struct{}, 2)

func ExportAllUsageLogs(c *gin.Context) {
	exportUsageLogs(c, false)
}

func ExportOwnUsageLogs(c *gin.Context) {
	exportUsageLogs(c, true)
}

func exportUsageLogs(c *gin.Context, own bool) {
	select {
	case usageLogExports <- struct{}{}:
		defer func() { <-usageLogExports }()
	default:
		c.JSON(http.StatusTooManyRequests, gin.H{"success": false, "message": "Another export is running; try again shortly"})
		return
	}
	logType, _ := strconv.Atoi(c.Query("type"))
	start, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	end, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	channel, _ := strconv.Atoi(c.Query("channel"))
	filter := model.UsageLogExportFilter{
		Type: logType, StartTimestamp: start, EndTimestamp: end,
		Username: c.Query("username"), TokenName: c.Query("token_name"),
		ModelName: c.Query("model_name"), Channel: channel, Group: c.Query("group"),
		RequestID: c.Query("request_id"), UpstreamRequestID: c.Query("upstream_request_id"),
	}
	if own {
		userID := c.GetInt("id")
		if userID <= 0 {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		filter.UserID = &userID
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Minute)
	defer cancel()
	// The browser creates the final spreadsheet. Only a bounded network buffer
	// exists on this server; neither export rows nor files are retained on disk.
	c.Header("Cache-Control", "no-store")
	c.Header("X-Accel-Buffering", "no")
	buffer := bufio.NewWriterSize(c.Writer, 64<<10)
	var count int64
	err := model.StreamUsageLogs(ctx, filter, func(row model.UsageLogExportRow) error {
		data, err := common.Marshal(row)
		if err != nil {
			return err
		}
		if count == 0 {
			c.Header("Content-Type", "application/x-ndjson; charset=utf-8")
		}
		if _, err := buffer.Write(data); err != nil {
			return err
		}
		if err := buffer.WriteByte('\n'); err != nil {
			return err
		}
		count++
		if count%1000 == 0 || buffer.Buffered() >= 60<<10 {
			if err := buffer.Flush(); err != nil {
				return err
			}
			c.Writer.Flush()
		}
		return ctx.Err()
	})
	if err != nil {
		// After streaming starts an ordinary JSON error would corrupt the
		// download. Omitting the terminal marker makes clients reject it.
		if count == 0 {
			common.ApiError(c, errors.New("Unable to export usage; narrow the time or name filter and try again"))
		}
		return
	}
	c.Header("Content-Type", "application/x-ndjson; charset=utf-8")
	complete, err := common.Marshal(struct {
		Complete bool  `json:"complete"`
		Count    int64 `json:"count"`
	}{Complete: true, Count: count})
	if err != nil {
		return
	}
	if _, err := buffer.Write(complete); err != nil {
		return
	}
	if err := buffer.WriteByte('\n'); err != nil {
		return
	}
	_ = buffer.Flush()
}
