package model

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/contentaudit"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// These tables intentionally use the main DB, not the general log pipeline.
// Generic log/admin APIs never receive the private rules or match details.
type ContentAuditPolicy struct {
	UserID         int    `gorm:"primaryKey;autoIncrement:false" json:"-"`
	Enabled        bool   `json:"enabled"`
	EncryptedRules string `gorm:"type:text" json:"-"`
}

type ContentAuditSequence struct {
	ID       int `gorm:"primaryKey;autoIncrement:false"`
	NextSlot int
}

type ContentAuditRecord struct {
	Slot       int    `gorm:"primaryKey;autoIncrement:false" json:"-"`
	ID         string `gorm:"size:36;uniqueIndex" json:"id"`
	UserID     int    `gorm:"index:idx_content_audit_owner_time,priority:1" json:"-"`
	CreatedAt  int64  `gorm:"index;index:idx_content_audit_owner_time,priority:2" json:"created_at"`
	RequestID  string `gorm:"size:128" json:"request_id"`
	TokenName  string `gorm:"size:255" json:"token_name"`
	Model      string `gorm:"size:255" json:"model"`
	Endpoint   string `gorm:"size:255" json:"endpoint"`
	Action     string `gorm:"size:16" json:"action"`
	MatchCount int    `json:"match_count"`
	// JSON escaping and base64 may exceed MySQL's 64 KiB TEXT limit even
	// for bounded snippets. GORM maps this size to MEDIUMTEXT there and TEXT
	// on SQLite/PostgreSQL, without a dialect-specific schema declaration.
	EncryptedDetails string `gorm:"size:262144" json:"-"`
}

const contentAuditDetailsDir = "records"

// contentAuditDetailsPath is deterministic so the database only needs to
// retain the searchable index. UUID validation also prevents a database value
// from escaping the per-user storage directory.
func contentAuditDetailsPath(userID int, id string) (string, error) {
	if userID <= 0 || common.ContentAuditDir == "" {
		return "", errors.New("content audit detail storage is not configured")
	}
	parsed, err := uuid.Parse(id)
	if err != nil {
		return "", errors.New("invalid content audit record id")
	}
	return filepath.Join(common.ContentAuditDir, contentAuditDetailsDir, strconv.Itoa(userID), parsed.String()+".enc"), nil
}

func writeContentAuditDetails(userID int, id, encrypted string) error {
	path, err := contentAuditDetailsPath(userID, id)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	if existing, err := os.ReadFile(path); err == nil {
		if string(existing) == encrypted {
			return nil
		}
		return errors.New("content audit detail file already contains different data")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".detail-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	cleanup := func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}
	defer cleanup()
	if err := temporary.Chmod(0600); err != nil {
		return err
	}
	if _, err := io.WriteString(temporary, encrypted); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		// A concurrent startup migration may have created the same deterministic
		// file. Accept it only when the ciphertext is identical.
		if existing, readErr := os.ReadFile(path); readErr == nil && string(existing) == encrypted {
			return nil
		}
		return err
	}
	return nil
}

func readContentAuditDetails(userID int, id string) (string, error) {
	path, err := contentAuditDetailsPath(userID, id)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func removeContentAuditDetails(userID int, id string) error {
	path, err := contentAuditDetailsPath(userID, id)
	if err != nil {
		return err
	}
	err = os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func GetContentAuditPolicy(ctx context.Context, userID int) (ContentAuditPolicy, error) {
	var policy ContentAuditPolicy
	err := DB.WithContext(ctx).Where("user_id = ?", userID).First(&policy).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ContentAuditPolicy{UserID: userID}, nil
	}
	return policy, err
}

func DecodeContentAuditPolicy(row ContentAuditPolicy) (contentaudit.Policy, error) {
	policy := contentaudit.Policy{Enabled: row.Enabled, Rules: []contentaudit.Rule{}}
	if row.EncryptedRules == "" {
		return policy, nil
	}
	data, err := common.OpenContentAudit(row.EncryptedRules, fmt.Sprintf("rules:%d", row.UserID))
	if err != nil {
		return policy, err
	}
	err = common.Unmarshal(data, &policy.Rules)
	return policy, err
}

func SaveContentAuditPolicy(ctx context.Context, userID int, policy contentaudit.Policy) error {
	if userID <= 0 {
		return errors.New("invalid user")
	}
	if _, err := contentaudit.Compile(policy); err != nil {
		return err
	}
	data, err := common.Marshal(policy.Rules)
	if err != nil {
		return err
	}
	sealed, err := common.SealContentAudit(data, fmt.Sprintf("rules:%d", userID))
	if err != nil {
		return err
	}
	row := ContentAuditPolicy{UserID: userID, Enabled: policy.Enabled, EncryptedRules: sealed}
	return DB.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "user_id"}}, DoUpdates: clause.AssignmentColumns([]string{"enabled", "encrypted_rules"})}).Create(&row).Error
}

func InsertContentAuditRecord(ctx context.Context, row ContentAuditRecord, details contentaudit.Result) error {
	// Time-ordered IDs keep newest hits ahead of older rows within the same second.
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}
	row.ID = id.String()
	data, err := common.Marshal(details)
	if err != nil {
		return err
	}
	encryptedDetails, err := common.SealContentAudit(data, fmt.Sprintf("hits:%d:%s", row.UserID, row.ID))
	if err != nil {
		return err
	}
	if err := writeContentAuditDetails(row.UserID, row.ID, encryptedDetails); err != nil {
		return err
	}
	// The encrypted match payload lives in a private file so deleting a record
	// releases its large storage allocation immediately. Keep the column for
	// backwards-compatible reads and one-time migration of old rows.
	row.EncryptedDetails = ""
	// The sequence lock allocates a unique slot across instances. Slots are no
	// longer wrapped or globally capped; retention cleanup is the only lifetime
	// policy for audit records.
	if err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		sequence := ContentAuditSequence{ID: 1}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&sequence).Error; err != nil {
			return err
		}
		if err := lockForUpdate(tx).First(&sequence, 1).Error; err != nil {
			return err
		}
		// Databases upgraded from the old wrapped ring may still have a
		// sequence value below the largest persisted slot. Advance it before
		// allocation so removing the global cap cannot overwrite old records.
		var maxSlot int
		if err := tx.Model(&ContentAuditRecord{}).Select("COALESCE(MAX(slot), 0)").Scan(&maxSlot).Error; err != nil {
			return err
		}
		if sequence.NextSlot < maxSlot {
			sequence.NextSlot = maxSlot
		}
		row.Slot = sequence.NextSlot + 1
		if err := tx.Model(&ContentAuditSequence{}).Where("id = ?", 1).Update("next_slot", row.Slot).Error; err != nil {
			return err
		}
		// UpdateAll omits GORM creation timestamps on conflict. A reused slot
		// must receive the new hit's time, otherwise a fresh hit can expire.
		columns := []string{"id", "user_id", "created_at", "request_id", "token_name", "model", "endpoint", "action", "match_count", "encrypted_details"}
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "slot"}}, DoUpdates: clause.AssignmentColumns(columns)}).Create(&row).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		_ = removeContentAuditDetails(row.UserID, row.ID)
		return err
	}
	return nil
}

// MigrateContentAuditDetailsToFiles moves legacy encrypted payloads out of
// the database without decrypting them. It is safe to retry after interruption
// because each update is conditional on the original ciphertext.
func MigrateContentAuditDetailsToFiles(ctx context.Context) error {
	if DB == nil || common.ContentAuditDir == "" {
		return nil
	}
	for {
		var rows []ContentAuditRecord
		if err := DB.WithContext(ctx).Where("encrypted_details <> ''").Order("slot ASC").Limit(100).Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		for _, row := range rows {
			if err := writeContentAuditDetails(row.UserID, row.ID, row.EncryptedDetails); err != nil {
				return err
			}
			if err := DB.WithContext(ctx).Model(&ContentAuditRecord{}).
				Where("slot = ? AND encrypted_details = ?", row.Slot, row.EncryptedDetails).
				Update("encrypted_details", "").Error; err != nil {
				return err
			}
		}
	}
}

type ContentAuditFilter struct {
	Start, End      int64
	Action, Keyword string
	Page, PageSize  int
}

const contentAuditDeleteBatchSize = 256

// deleteContentAuditRecordsByQuery removes index rows in bounded batches and
// unlinks their detail files after each successful database delete. The
// metadata stays searchable in SQL, while large match payloads never need to
// be loaded into one unbounded slice during a bulk delete.
func deleteContentAuditRecordsByQuery(ctx context.Context, query *gorm.DB) error {
	var firstErr error
	for {
		var rows []ContentAuditRecord
		if err := query.Session(&gorm.Session{}).
			Select("slot, id, user_id").
			Order("slot ASC").Limit(contentAuditDeleteBatchSize).Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return firstErr
		}
		slots := make([]int, len(rows))
		for i := range rows {
			slots[i] = rows[i].Slot
		}
		if err := DB.WithContext(ctx).Where("slot IN ?", slots).Delete(&ContentAuditRecord{}).Error; err != nil {
			return err
		}
		for _, row := range rows {
			if err := removeContentAuditDetails(row.UserID, row.ID); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
}

// PruneOrphanContentAuditDetails repairs the only failure window that cannot
// be handled by the row query: a process crash after the database row is
// deleted but before its detail file is unlinked. It intentionally scans only
// the private audit directory and removes temporary files from interrupted
// atomic writes as well.
func PruneOrphanContentAuditDetails(ctx context.Context) error {
	root := filepath.Join(common.ContentAuditDir, contentAuditDetailsDir)
	users, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var firstErr error
	for _, userEntry := range users {
		if !userEntry.IsDir() {
			continue
		}
		userID, err := strconv.Atoi(userEntry.Name())
		if err != nil || userID <= 0 {
			continue
		}
		entries, err := os.ReadDir(filepath.Join(root, userEntry.Name()))
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		ids := make([]string, 0, len(entries))
		paths := make(map[string]string, len(entries))
		for _, entry := range entries {
			path := filepath.Join(root, userEntry.Name(), entry.Name())
			if entry.IsDir() {
				continue
			}
			if strings.HasPrefix(entry.Name(), ".detail-") {
				if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) && firstErr == nil {
					firstErr = err
				}
				continue
			}
			if filepath.Ext(entry.Name()) != ".enc" {
				continue
			}
			id := strings.TrimSuffix(entry.Name(), ".enc")
			if _, err := uuid.Parse(id); err != nil {
				continue
			}
			ids = append(ids, id)
			paths[id] = path
		}
		for start := 0; start < len(ids); start += contentAuditDeleteBatchSize {
			end := start + contentAuditDeleteBatchSize
			if end > len(ids) {
				end = len(ids)
			}
			var existing []string
			if err := DB.WithContext(ctx).Model(&ContentAuditRecord{}).Where("user_id = ? AND id IN ?", userID, ids[start:end]).Pluck("id", &existing).Error; err != nil {
				return err
			}
			keep := make(map[string]struct{}, len(existing))
			for _, id := range existing {
				keep[id] = struct{}{}
			}
			for _, id := range ids[start:end] {
				if _, ok := keep[id]; ok {
					continue
				}
				if err := os.Remove(paths[id]); err != nil && !errors.Is(err, os.ErrNotExist) && firstErr == nil {
					firstErr = err
				}
			}
		}
	}
	return firstErr
}

func ListContentAuditRecords(ctx context.Context, userID int, filter ContentAuditFilter) ([]ContentAuditRecord, int64, error) {
	rows := []ContentAuditRecord{}
	tx := DB.WithContext(ctx).Model(&ContentAuditRecord{}).Omit("encrypted_details").Where("user_id = ? AND created_at >= ?", userID, time.Now().Add(-time.Duration(common.GetContentAuditSettings().RetentionDays)*24*time.Hour).Unix())
	if filter.Start > 0 {
		tx = tx.Where("created_at >= ?", filter.Start)
	}
	if filter.End > 0 {
		tx = tx.Where("created_at <= ?", filter.End)
	}
	if filter.Action != "" {
		tx = tx.Where("action = ?", filter.Action)
	}
	if filter.Keyword != "" {
		pattern := tokenUsageNameLikePattern(filter.Keyword)
		tx = tx.Where("(request_id LIKE ? ESCAPE '!' OR token_name LIKE ? ESCAPE '!' OR model LIKE ? ESCAPE '!')", pattern, pattern, pattern)
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := tx.Order("created_at DESC, id DESC").Offset((filter.Page - 1) * filter.PageSize).Limit(filter.PageSize).Find(&rows).Error
	return rows, total, err
}

func GetContentAuditDetails(ctx context.Context, userID int, id string) (contentaudit.Result, error) {
	var row ContentAuditRecord
	var result contentaudit.Result
	cutoff := time.Now().Add(-time.Duration(common.GetContentAuditSettings().RetentionDays) * 24 * time.Hour).Unix()
	if err := DB.WithContext(ctx).Where("id = ? AND user_id = ? AND created_at >= ?", id, userID, cutoff).First(&row).Error; err != nil {
		return result, err
	}
	encryptedDetails := row.EncryptedDetails
	var err error
	if encryptedDetails == "" {
		encryptedDetails, err = readContentAuditDetails(userID, row.ID)
		if err != nil {
			return result, err
		}
	}
	data, err := common.OpenContentAudit(encryptedDetails, fmt.Sprintf("hits:%d:%s", userID, id))
	if err != nil {
		return result, err
	}
	err = common.Unmarshal(data, &result)
	return result, err
}

func DeleteContentAuditRecords(ctx context.Context, userID int, id string) error {
	tx := DB.WithContext(ctx).Where("user_id = ?", userID)
	if id != "" {
		tx = tx.Where("id = ?", id)
	}
	return deleteContentAuditRecordsByQuery(ctx, tx) // No DeletedAt: physical DELETE, not soft deletion.
}

func PurgeExpiredContentAuditRecords(ctx context.Context, now time.Time) error {
	cutoff := now.Add(-time.Duration(common.GetContentAuditSettings().RetentionDays) * 24 * time.Hour).Unix()
	return deleteContentAuditRecordsByQuery(ctx, DB.WithContext(ctx).Where("created_at < ?", cutoff))
}
