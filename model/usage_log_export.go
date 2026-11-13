package model

import (
	"context"
	"database/sql"
	"math"

	"github.com/QuantumNous/new-api/common"
)

// UsageLogExportFilter mirrors the log list filters. UserID is mandatory for
// self-service exports; administrators leave it nil to export across users.
type UsageLogExportFilter struct {
	UserID            *int
	Type              int
	StartTimestamp    int64
	EndTimestamp      int64
	Username          string
	TokenName         string
	ModelName         string
	Channel           int
	Group             string
	RequestID         string
	UpstreamRequestID string
}

// UsageLogExportRow deliberately excludes content and administrative metadata.
// Quota stays in its stored unit so the frontend uses the same currency display
// configuration as the usage log table, without changing billing calculations.
type UsageLogExportRow struct {
	Username         string  `json:"username"`
	TokenName        string  `json:"token_name"`
	ModelName        string  `json:"model_name"`
	PromptTokens     int64   `json:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
	CacheReadTokens  float64 `json:"cache_read_tokens"`
	CacheWriteTokens float64 `json:"cache_write_tokens"`
	Quota            int64   `json:"quota"`
	CreatedAt        int64   `json:"created_at"`
	IsStream         bool    `json:"is_stream"`
	RequestPath      string  `json:"request_path"`
	// Preserve the stored units: first response is milliseconds, total time is
	// seconds. A missing/invalid legacy first-response metric stays unavailable.
	FirstResponseTimeMS *float64 `json:"first_response_time_ms"`
	UseTime             int64    `json:"use_time"`
}

// StreamUsageLogs reads one database cursor instead of counting and repeatedly
// paging through logs. The callback provides backpressure and cancellation
// closes the cursor, keeping server memory bounded without temporary files.
func StreamUsageLogs(ctx context.Context, filter UsageLogExportFilter, emit func(UsageLogExportRow) error) error {
	// Legacy rows may have a NULL type; only explicit error rows are excluded.
	tx := LOG_DB.WithContext(ctx).Model(&Log{}).Where("(logs.type <> ? OR logs.type IS NULL)", LogTypeError)
	if filter.UserID != nil {
		tx = tx.Where("logs.user_id = ?", *filter.UserID)
	}
	if filter.Type != LogTypeUnknown {
		tx = tx.Where("logs.type = ?", filter.Type)
	}
	var err error
	if tx, err = applyExplicitLogTextFilter(tx, "logs.model_name", filter.ModelName); err != nil {
		return err
	}
	if filter.UserID == nil {
		if tx, err = applyExplicitLogTextFilter(tx, "logs.username", filter.Username); err != nil {
			return err
		}
		if filter.Channel != 0 {
			tx = tx.Where("logs.channel_id = ?", filter.Channel)
		}
	}
	if filter.TokenName != "" {
		tx = tx.Where("logs.token_name = ?", filter.TokenName)
	}
	if filter.RequestID != "" {
		tx = tx.Where("logs.request_id = ?", filter.RequestID)
	}
	if filter.UpstreamRequestID != "" {
		tx = tx.Where("logs.upstream_request_id = ?", filter.UpstreamRequestID)
	}
	if filter.StartTimestamp != 0 {
		tx = tx.Where("logs.created_at >= ?", filter.StartTimestamp)
	}
	if filter.EndTimestamp != 0 {
		tx = tx.Where("logs.created_at <= ?", filter.EndTimestamp)
	}
	if filter.Group != "" {
		tx = tx.Where("logs."+logGroupCol+" = ?", filter.Group)
	}
	order := "logs.created_at desc, logs.id desc"
	if filter.UserID != nil {
		order = "logs.id desc"
	}
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		order = clickHouseLogOrder("logs.")
	}
	rows, err := tx.Select("logs.username, logs.token_name, logs.model_name, logs.prompt_tokens, logs.completion_tokens, logs.quota, logs.created_at, logs.other, logs.is_stream, logs.use_time").Order(order).Rows()
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return err
		}
		// Nullable legacy columns export as empty/zero, just like the list DTO.
		var username, tokenName, modelName, other sql.NullString
		var promptTokens, completionTokens, quota, createdAt, useTime sql.NullInt64
		var isStream sql.NullBool
		if err := rows.Scan(&username, &tokenName, &modelName, &promptTokens, &completionTokens, &quota, &createdAt, &other, &isStream, &useTime); err != nil {
			return err
		}
		row := UsageLogExportRow{
			Username: username.String, TokenName: tokenName.String, ModelName: modelName.String,
			PromptTokens: promptTokens.Int64, CompletionTokens: completionTokens.Int64,
			Quota: quota.Int64, CreatedAt: createdAt.Int64,
			IsStream: isStream.Bool, UseTime: useTime.Int64,
		}
		var metadata struct {
			Read              common.RawMessage `json:"cache_tokens"`
			Write             common.RawMessage `json:"cache_creation_tokens"`
			Write5m           common.RawMessage `json:"cache_creation_tokens_5m"`
			Write1h           common.RawMessage `json:"cache_creation_tokens_1h"`
			RequestPath       common.RawMessage `json:"request_path"`
			FirstResponseTime common.RawMessage `json:"frt"`
		}
		if other.String != "" && common.UnmarshalJsonStr(other.String, &metadata) == nil {
			// Decode only public request facts and keep malformed legacy fields
			// independent, so they cannot discard valid timing/cache values.
			_ = common.Unmarshal(metadata.RequestPath, &row.RequestPath)
			var firstResponseTime *float64
			if common.Unmarshal(metadata.FirstResponseTime, &firstResponseTime) == nil && firstResponseTime != nil && *firstResponseTime >= 0 {
				row.FirstResponseTimeMS = firstResponseTime
			}
			// Match the table's per-field number validation. One malformed legacy
			// counter must not discard the other valid counters or their decimals.
			var counts [4]float64
			for i, raw := range [...]common.RawMessage{metadata.Read, metadata.Write, metadata.Write5m, metadata.Write1h} {
				if len(raw) == 0 || common.Unmarshal(raw, &counts[i]) != nil || counts[i] < 0 {
					counts[i] = 0
				}
			}
			row.CacheReadTokens = counts[0]
			row.CacheWriteTokens = counts[1]
			// Split counters already comprise the total; never add both forms.
			if counts[2] > 0 || counts[3] > 0 {
				row.CacheWriteTokens = counts[2] + counts[3]
				// Two finite legacy counters can still overflow when added. The
				// table treats non-finite counts as zero; keep the export writable.
				if math.IsInf(row.CacheWriteTokens, 0) {
					row.CacheWriteTokens = 0
				}
			}
		}
		if err := emit(row); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return ctx.Err()
}
