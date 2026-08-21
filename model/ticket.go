package model

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	TicketStatusWaitingAdmin = "waiting_admin"
	TicketStatusWaitingUser  = "waiting_user"
	TicketStatusClosed       = "closed"

	TicketSenderUser  = "user"
	TicketSenderAdmin = "admin"

	TicketTitleMaxLength                  = 120
	TicketContentMaxLength                = 4000
	TicketMessageLimit                    = 100
	TicketAttachmentMaxBytes        int64 = 5 * 1024 * 1024
	TicketAttachmentTotalMaxBytes   int64 = 50 * 1024 * 1024
	TicketAttachmentLimit                 = 20
	ticketSQLiteTransactionAttempts       = 4
	ticketMigrationBatchSize              = 500
	ticketActiveSlotIndexName             = "idx_ticket_user_active"
	ticketActiveSlotMigrationKey          = "TicketActiveSlotMigrationV1"
	ticketSystemActorId                   = 0
)

var (
	ErrTicketNotFound        = errors.New("ticket not found")
	ErrTicketActiveExists    = errors.New("an active ticket already exists")
	ErrTicketWaitingAdmin    = errors.New("ticket is waiting for an administrator reply")
	ErrTicketWaitingUser     = errors.New("ticket is waiting for the user reply")
	ErrTicketClosed          = errors.New("ticket is closed")
	ErrTicketMessageLimit    = errors.New("ticket message limit reached")
	ErrTicketAttachmentLimit = errors.New("ticket attachment storage limit reached")
	ErrTicketStateChanged    = errors.New("ticket state changed, please retry")
	ErrInvalidTicketStatus   = errors.New("invalid ticket status")
	ErrInvalidTicketSearch   = errors.New("invalid ticket search")
)

type Ticket struct {
	Id            int             `json:"id"`
	UserId        int             `json:"user_id" gorm:"index"`
	Title         string          `json:"title" gorm:"type:varchar(120);not null"`
	Status        string          `json:"status" gorm:"type:varchar(32);not null;index:idx_ticket_admin_queue,priority:1"`
	ActiveSlot    *int            `json:"-"`
	MessageCount  int             `json:"message_count" gorm:"not null"`
	LastMessageAt int64           `json:"last_message_at" gorm:"bigint;not null;index:idx_ticket_admin_queue,priority:2"`
	CreatedAt     int64           `json:"created_at" gorm:"bigint;autoCreateTime"`
	UpdatedAt     int64           `json:"updated_at" gorm:"bigint;autoUpdateTime"`
	ClosedAt      int64           `json:"closed_at" gorm:"bigint;not null"`
	ClosedBy      int             `json:"-"`
	User          *User           `json:"-" gorm:"foreignKey:UserId;-:migration"`
	Messages      []TicketMessage `json:"-" gorm:"foreignKey:TicketId;-:migration"`
}

// ticketActiveSlotIndex is only used by GORM's migrator. Keeping the unique
// index off Ticket lets AutoMigrate add active_slot before legacy rows are
// normalized, rather than creating the index too early.
type ticketActiveSlotIndex struct {
	UserId     int  `gorm:"column:user_id;uniqueIndex:idx_ticket_user_active,priority:1"`
	ActiveSlot *int `gorm:"column:active_slot;uniqueIndex:idx_ticket_user_active,priority:2"`
}

func (ticketActiveSlotIndex) TableName() string {
	return "tickets"
}

type TicketMessage struct {
	Id         int               `json:"id"`
	TicketId   int               `json:"-" gorm:"index"`
	SenderId   int               `json:"-"`
	SenderRole string            `json:"sender_role" gorm:"type:varchar(16);not null"`
	Content    string            `json:"content" gorm:"type:text;not null"`
	CreatedAt  int64             `json:"created_at" gorm:"bigint;autoCreateTime;index"`
	Attachment *TicketAttachment `json:"-" gorm:"foreignKey:MessageId;-:migration"`
}

type TicketAttachment struct {
	Id          int    `json:"id"`
	TicketId    int    `json:"-" gorm:"index"`
	MessageId   int    `json:"-" gorm:"uniqueIndex"`
	UploaderId  int    `json:"-"`
	StorageName string `json:"-" gorm:"type:varchar(64);not null;uniqueIndex"`
	FileName    string `json:"file_name" gorm:"type:varchar(255);not null"`
	MimeType    string `json:"mime_type" gorm:"type:varchar(32);not null"`
	Size        int64  `json:"size" gorm:"bigint;not null"`
	Width       int    `json:"width" gorm:"not null"`
	Height      int    `json:"height" gorm:"not null"`
	CreatedAt   int64  `json:"-" gorm:"bigint;autoCreateTime"`
}

type TicketAdminFilter struct {
	Status  string
	Keyword string
	UserId  int
}

// normalizeTicketActiveSlotIndex upgrades ticket rows created before the
// active-slot invariant was enforced. The most recently active ticket remains
// open for each user; older duplicates are system-closed and retain their full
// conversation history for audit purposes.
func normalizeTicketActiveSlotIndex() error {
	if err := DB.Clauses(clause.OnConflict{DoNothing: true}).
		Create(&Option{Key: ticketActiveSlotMigrationKey, Value: "pending"}).Error; err != nil {
		return err
	}

	closedDuplicates := 0
	err := DB.Transaction(func(tx *gorm.DB) error {
		// Claim only a pending marker. The UPDATE waits for a concurrent row
		// lock and re-evaluates this predicate afterwards, so another node sees
		// the running marker and exits instead of repeating the migration.
		claim := tx.Model(&Option{}).
			Where(commonKeyCol+" = ? AND value = ?", ticketActiveSlotMigrationKey, "pending").
			Update("value", "running")
		if claim.Error != nil {
			return claim.Error
		}
		if claim.RowsAffected == 0 {
			return nil
		}

		var maxTicketId int
		if err := tx.Model(&Ticket{}).Select("COALESCE(MAX(id), 0)").Scan(&maxTicketId).Error; err != nil {
			return err
		}
		if maxTicketId > 0 {
			activeStatuses := []string{TicketStatusWaitingAdmin, TicketStatusWaitingUser}
			var activeTickets []struct {
				Id     int
				UserId int
			}
			if err := tx.Model(&Ticket{}).
				Select("id", "user_id").
				Where("id <= ? AND status IN ?", maxTicketId, activeStatuses).
				Order("user_id ASC, last_message_at DESC, id DESC").
				Scan(&activeTickets).Error; err != nil {
				return err
			}

			winnerIds := make([]int, 0, len(activeTickets))
			duplicateIds := make([]int, 0)
			lastUserId := 0
			hasLastUser := false
			for _, ticket := range activeTickets {
				if !hasLastUser || ticket.UserId != lastUserId {
					winnerIds = append(winnerIds, ticket.Id)
					lastUserId = ticket.UserId
					hasLastUser = true
					continue
				}
				duplicateIds = append(duplicateIds, ticket.Id)
			}

			// Clear stale slots first so a winner can safely take slot 1 even
			// when an older or already-closed row previously occupied it.
			if err := tx.Model(&Ticket{}).
				Where("id <= ? AND status NOT IN ? AND active_slot IS NOT NULL", maxTicketId, activeStatuses).
				UpdateColumn("active_slot", nil).Error; err != nil {
				return err
			}
			now := common.GetTimestamp()
			for start := 0; start < len(duplicateIds); start += ticketMigrationBatchSize {
				end := min(start+ticketMigrationBatchSize, len(duplicateIds))
				result := tx.Model(&Ticket{}).
					Where("id IN ? AND status IN ?", duplicateIds[start:end], activeStatuses).
					Updates(map[string]interface{}{
						"status":      TicketStatusClosed,
						"active_slot": nil,
						"closed_at":   now,
						"closed_by":   ticketSystemActorId,
						"updated_at":  now,
					})
				if result.Error != nil {
					return result.Error
				}
				closedDuplicates += int(result.RowsAffected)
			}
			for start := 0; start < len(winnerIds); start += ticketMigrationBatchSize {
				end := min(start+ticketMigrationBatchSize, len(winnerIds))
				if err := tx.Model(&Ticket{}).
					Where("id IN ? AND status IN ?", winnerIds[start:end], activeStatuses).
					UpdateColumn("active_slot", 1).Error; err != nil {
					return err
				}
			}
		}

		complete := tx.Model(&Option{}).
			Where(commonKeyCol+" = ? AND value = ?", ticketActiveSlotMigrationKey, "running").
			Update("value", "completed")
		if complete.Error != nil {
			return complete.Error
		}
		if complete.RowsAffected != 1 {
			return errors.New("ticket active-slot migration lost its database lock")
		}
		return nil
	})
	if err != nil {
		return err
	}

	migrator := DB.Migrator()
	if !migrator.HasIndex(&Ticket{}, ticketActiveSlotIndexName) {
		if err := migrator.CreateIndex(&ticketActiveSlotIndex{}, ticketActiveSlotIndexName); err != nil {
			// A failed index build must be retried with normalization on restart.
			_ = DB.Model(&Option{}).Where(commonKeyCol+" = ?", ticketActiveSlotMigrationKey).Update("value", "pending").Error
			return err
		}
	}
	if closedDuplicates > 0 {
		common.SysLog(fmt.Sprintf("ticket active-slot migration system-closed %d duplicate tickets", closedDuplicates))
	}
	return nil
}

func normalizeTicketText(value string, maxLength int, multiline bool) (string, error) {
	if !utf8.ValidString(value) {
		return "", errors.New("text must be valid UTF-8")
	}
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	if multiline {
		value = strings.NewReplacer("\u2028", "\n", "\u2029", "\n").Replace(value)
	}
	value = strings.TrimSpace(value)
	length := utf8.RuneCountInString(value)
	if length < 1 || length > maxLength {
		return "", errors.New("text length is out of range")
	}
	for _, r := range value {
		// Ticket text is rendered in multiple clients and may be reused by
		// future rich-text surfaces. Reject markup delimiters at the storage
		// boundary instead of relying on every renderer to escape them.
		if r == '<' || r == '>' {
			return "", errors.New("text contains unsupported markup characters")
		}
		if unicode.Is(unicode.Cf, r) || unicode.Is(unicode.Zl, r) || unicode.Is(unicode.Zp, r) {
			return "", errors.New("text contains unsupported format characters")
		}
		if !unicode.IsControl(r) {
			continue
		}
		if multiline && (r == '\n' || r == '\t') {
			continue
		}
		return "", errors.New("text contains unsupported control characters")
	}
	return value, nil
}

func NormalizeTicketTitle(value string) (string, error) {
	return normalizeTicketText(value, TicketTitleMaxLength, false)
}

func NormalizeTicketContent(value string) (string, error) {
	return normalizeTicketText(value, TicketContentMaxLength, true)
}

func validateTicketAttachment(attachment *TicketAttachment) error {
	if attachment == nil {
		return nil
	}
	if attachment.Size < 1 || attachment.Size > TicketAttachmentMaxBytes {
		return ErrTicketAttachmentLimit
	}
	return nil
}

func isTicketSQLiteLockError(err error) bool {
	if err == nil || !common.UsingMainDatabase(common.DatabaseTypeSQLite) {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "sqlite_busy") ||
		strings.Contains(message, "sqlite_locked") ||
		strings.Contains(message, "database is locked") ||
		strings.Contains(message, "database table is locked")
}

func runTicketTransaction(operation func(*gorm.DB) error) error {
	for attempt := 0; attempt < ticketSQLiteTransactionAttempts; attempt++ {
		err := DB.Transaction(operation)
		if !isTicketSQLiteLockError(err) {
			return err
		}
		if attempt+1 < ticketSQLiteTransactionAttempts {
			time.Sleep(time.Duration(attempt+1) * 5 * time.Millisecond)
		}
	}
	return ErrTicketStateChanged
}

func CheckCanCreateTicket(userId int) error {
	if userId <= 0 {
		return ErrTicketNotFound
	}
	var count int64
	if err := DB.Model(&Ticket{}).Where("user_id = ? AND active_slot = ?", userId, 1).Limit(1).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return ErrTicketActiveExists
	}
	return nil
}

func checkCanReplyTicket(ticketId, actorId int, actorRole string) error {
	if ticketId <= 0 || actorId <= 0 {
		return ErrTicketNotFound
	}
	var ticket Ticket
	query := DB.Select("status", "message_count").Where("id = ?", ticketId)
	if actorRole == TicketSenderUser {
		query = query.Where("user_id = ?", actorId)
	} else if actorRole != TicketSenderAdmin {
		return ErrTicketNotFound
	}
	if err := query.First(&ticket).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrTicketNotFound
		}
		return err
	}
	if ticket.Status == TicketStatusClosed {
		return ErrTicketClosed
	}
	if actorRole == TicketSenderUser && ticket.Status != TicketStatusWaitingUser {
		return ErrTicketWaitingAdmin
	}
	if actorRole == TicketSenderAdmin && ticket.Status != TicketStatusWaitingAdmin {
		return ErrTicketWaitingUser
	}
	if ticket.MessageCount >= TicketMessageLimit {
		return ErrTicketMessageLimit
	}
	return nil
}

func CheckCanReplyTicketByUser(ticketId, userId int) error {
	return checkCanReplyTicket(ticketId, userId, TicketSenderUser)
}

func CheckCanReplyTicketByAdmin(ticketId, adminId int) error {
	return checkCanReplyTicket(ticketId, adminId, TicketSenderAdmin)
}

func CreateTicket(userId int, title, content string, attachment *TicketAttachment) (*Ticket, error) {
	if userId <= 0 {
		return nil, ErrTicketNotFound
	}
	var err error
	title, err = NormalizeTicketTitle(title)
	if err != nil {
		return nil, err
	}
	content, err = NormalizeTicketContent(content)
	if err != nil {
		return nil, err
	}
	if err := validateTicketAttachment(attachment); err != nil {
		return nil, err
	}

	now := common.GetTimestamp()
	activeSlot := 1
	ticket := Ticket{
		UserId: userId, Title: title, Status: TicketStatusWaitingAdmin,
		ActiveSlot: &activeSlot, MessageCount: 1, LastMessageAt: now,
		CreatedAt: now, UpdatedAt: now,
	}
	var created Ticket
	err = runTicketTransaction(func(tx *gorm.DB) error {
		var user User
		if err := lockForUpdate(tx).Select("id").Where("id = ?", userId).First(&user).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrTicketNotFound
			}
			return err
		}

		var active Ticket
		err := tx.Select("id").Where("user_id = ? AND active_slot = ?", userId, 1).First(&active).Error
		if err == nil {
			return ErrTicketActiveExists
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		attemptTicket := ticket
		if err := tx.Create(&attemptTicket).Error; err != nil {
			return err
		}
		message := TicketMessage{
			TicketId: attemptTicket.Id, SenderId: userId, SenderRole: TicketSenderUser,
			Content: content, CreatedAt: now,
		}
		if err := tx.Create(&message).Error; err != nil {
			return err
		}
		if attachment != nil {
			attemptAttachment := *attachment
			attemptAttachment.TicketId = attemptTicket.Id
			attemptAttachment.MessageId = message.Id
			attemptAttachment.UploaderId = userId
			attemptAttachment.CreatedAt = now
			if err := tx.Create(&attemptAttachment).Error; err != nil {
				return err
			}
		}
		return getTicketDetail(tx, attemptTicket.Id, userId, false, &created)
	})
	if err != nil {
		if !errors.Is(err, ErrTicketActiveExists) {
			var count int64
			if lookupErr := DB.Model(&Ticket{}).Where("user_id = ? AND active_slot = ?", userId, 1).Count(&count).Error; lookupErr == nil && count > 0 {
				return nil, ErrTicketActiveExists
			}
		}
		return nil, err
	}
	return &created, nil
}

func replyTicket(ticketId, actorId int, actorRole, content string, attachment *TicketAttachment) (*Ticket, error) {
	if ticketId <= 0 || actorId <= 0 {
		return nil, ErrTicketNotFound
	}
	content, err := NormalizeTicketContent(content)
	if err != nil {
		return nil, err
	}
	if err := validateTicketAttachment(attachment); err != nil {
		return nil, err
	}
	expectedStatus := TicketStatusWaitingAdmin
	nextStatus := TicketStatusWaitingUser
	if actorRole == TicketSenderUser {
		expectedStatus = TicketStatusWaitingUser
		nextStatus = TicketStatusWaitingAdmin
	} else if actorRole != TicketSenderAdmin {
		return nil, ErrTicketNotFound
	}

	now := common.GetTimestamp()
	var replied Ticket
	err = runTicketTransaction(func(tx *gorm.DB) error {
		var owner Ticket
		if err := tx.Select("user_id").Where("id = ?", ticketId).First(&owner).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrTicketNotFound
			}
			return err
		}
		if actorRole == TicketSenderUser && owner.UserId != actorId {
			return ErrTicketNotFound
		}
		var user User
		if err := lockForUpdate(tx).Select("id").Where("id = ?", owner.UserId).First(&user).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrTicketNotFound
			}
			return err
		}
		var ticket Ticket
		query := lockForUpdate(tx).Where("id = ? AND user_id = ?", ticketId, owner.UserId)
		if err := query.First(&ticket).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrTicketNotFound
			}
			return err
		}
		if ticket.Status == TicketStatusClosed {
			return ErrTicketClosed
		}
		if ticket.Status != expectedStatus {
			if actorRole == TicketSenderUser {
				return ErrTicketWaitingAdmin
			}
			return ErrTicketWaitingUser
		}
		if ticket.MessageCount >= TicketMessageLimit {
			return ErrTicketMessageLimit
		}
		if attachment != nil {
			var usage struct {
				Count      int64
				TotalBytes int64
			}
			if err := tx.Model(&TicketAttachment{}).
				Select("COUNT(*) AS count, COALESCE(SUM(size), 0) AS total_bytes").
				Where("ticket_id = ?", ticket.Id).Scan(&usage).Error; err != nil {
				return err
			}
			if usage.Count < 0 || usage.Count >= TicketAttachmentLimit ||
				usage.TotalBytes < 0 || usage.TotalBytes > TicketAttachmentTotalMaxBytes ||
				attachment.Size > TicketAttachmentTotalMaxBytes-usage.TotalBytes {
				return ErrTicketAttachmentLimit
			}
		}

		result := tx.Model(&Ticket{}).
			Where("id = ? AND status = ? AND message_count < ?", ticket.Id, expectedStatus, TicketMessageLimit).
			Updates(map[string]interface{}{
				"status":          nextStatus,
				"message_count":   gorm.Expr("message_count + 1"),
				"last_message_at": now,
				"updated_at":      now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrTicketStateChanged
		}
		message := TicketMessage{
			TicketId: ticket.Id, SenderId: actorId, SenderRole: actorRole,
			Content: content, CreatedAt: now,
		}
		if err := tx.Create(&message).Error; err != nil {
			return err
		}
		if attachment != nil {
			attemptAttachment := *attachment
			attemptAttachment.TicketId = ticket.Id
			attemptAttachment.MessageId = message.Id
			attemptAttachment.UploaderId = actorId
			attemptAttachment.CreatedAt = now
			if err := tx.Create(&attemptAttachment).Error; err != nil {
				return err
			}
		}
		return getTicketDetail(tx, ticket.Id, 0, actorRole == TicketSenderAdmin, &replied)
	})
	if err != nil {
		return nil, err
	}
	return &replied, nil
}

func ReplyTicketByUser(ticketId, userId int, content string, attachment *TicketAttachment) (*Ticket, error) {
	return replyTicket(ticketId, userId, TicketSenderUser, content, attachment)
}

func ReplyTicketByAdmin(ticketId, adminId int, content string, attachment *TicketAttachment) (*Ticket, error) {
	return replyTicket(ticketId, adminId, TicketSenderAdmin, content, attachment)
}

func CloseTicket(ticketId, adminId int) (*Ticket, error) {
	if ticketId <= 0 || adminId <= 0 {
		return nil, ErrTicketNotFound
	}
	now := common.GetTimestamp()
	var closed Ticket
	err := runTicketTransaction(func(tx *gorm.DB) error {
		var owner Ticket
		if err := tx.Select("user_id").Where("id = ?", ticketId).First(&owner).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrTicketNotFound
			}
			return err
		}
		var user User
		if err := lockForUpdate(tx).Select("id").Where("id = ?", owner.UserId).First(&user).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrTicketNotFound
			}
			return err
		}
		var ticket Ticket
		if err := lockForUpdate(tx).Where("id = ? AND user_id = ?", ticketId, owner.UserId).First(&ticket).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrTicketNotFound
			}
			return err
		}
		if ticket.Status == TicketStatusClosed {
			return ErrTicketClosed
		}
		result := tx.Model(&Ticket{}).
			Where("id = ? AND status IN ?", ticket.Id, []string{TicketStatusWaitingAdmin, TicketStatusWaitingUser}).
			Updates(map[string]interface{}{
				"status":      TicketStatusClosed,
				"active_slot": nil,
				"closed_at":   now,
				"closed_by":   adminId,
				"updated_at":  now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrTicketStateChanged
		}
		return getTicketDetail(tx, ticket.Id, 0, true, &closed)
	})
	if err != nil {
		return nil, err
	}
	return &closed, nil
}

func ticketDetailQuery(query *gorm.DB) *gorm.DB {
	return query.
		Preload("Messages", func(tx *gorm.DB) *gorm.DB {
			return tx.Order("created_at ASC, id ASC")
		}).
		Preload("Messages.Attachment")
}

func getTicketDetail(tx *gorm.DB, ticketId, userId int, includeUser bool, ticket *Ticket) error {
	query := ticketDetailQuery(tx).Where("id = ?", ticketId)
	if userId > 0 {
		query = query.Where("user_id = ?", userId)
	}
	if includeUser {
		query = query.Preload("User", func(preload *gorm.DB) *gorm.DB {
			return preload.Select("id", "username", "display_name", "email")
		})
	}
	if err := query.First(ticket).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrTicketNotFound
		}
		return err
	}
	return nil
}

func GetTicketForUser(ticketId, userId int) (*Ticket, error) {
	var ticket Ticket
	err := getTicketDetail(DB, ticketId, userId, false, &ticket)
	return &ticket, err
}

func GetTicketForAdmin(ticketId int) (*Ticket, error) {
	var ticket Ticket
	err := getTicketDetail(DB, ticketId, 0, true, &ticket)
	return &ticket, err
}

func ListUserTickets(userId, offset, limit int) ([]Ticket, int64, *int, error) {
	if userId <= 0 {
		return nil, 0, nil, ErrTicketNotFound
	}
	if offset < 0 {
		offset = 0
	}
	if limit < 1 || limit > 50 {
		limit = 20
	}
	query := DB.Model(&Ticket{}).Where("user_id = ?", userId)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, nil, err
	}
	var tickets []Ticket
	if err := query.Order("last_message_at DESC, id DESC").Offset(offset).Limit(limit).Find(&tickets).Error; err != nil {
		return nil, 0, nil, err
	}
	var active Ticket
	err := DB.Select("id").Where("user_id = ? AND active_slot = ?", userId, 1).First(&active).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return tickets, total, nil, nil
	}
	if err != nil {
		return nil, 0, nil, err
	}
	return tickets, total, &active.Id, nil
}

func escapeTicketLike(value string) string {
	value = strings.ReplaceAll(value, "!", "!!")
	value = strings.ReplaceAll(value, "%", "!%")
	value = strings.ReplaceAll(value, "_", "!_")
	return "%" + strings.ToLower(value) + "%"
}

func ListAdminTickets(filter TicketAdminFilter, offset, limit int) ([]Ticket, int64, error) {
	if offset < 0 {
		offset = 0
	}
	if limit < 1 || limit > 50 {
		limit = 20
	}
	status := strings.ToLower(strings.TrimSpace(filter.Status))
	if status != "" && status != TicketStatusWaitingAdmin && status != TicketStatusWaitingUser && status != TicketStatusClosed {
		return nil, 0, ErrInvalidTicketStatus
	}
	keyword := strings.TrimSpace(filter.Keyword)
	if keyword != "" {
		normalizedKeyword, err := normalizeTicketText(keyword, 100, false)
		if err != nil {
			return nil, 0, ErrInvalidTicketSearch
		}
		keyword = normalizedKeyword
	}
	buildQuery := func() *gorm.DB {
		query := DB.Model(&Ticket{}).Joins("LEFT JOIN users u ON u.id = tickets.user_id")
		if status != "" {
			query = query.Where("tickets.status = ?", status)
		}
		if filter.UserId > 0 {
			query = query.Where("tickets.user_id = ?", filter.UserId)
		}
		if keyword != "" {
			pattern := escapeTicketLike(keyword)
			conditions := "(LOWER(tickets.title) LIKE ? ESCAPE '!' OR LOWER(u.username) LIKE ? ESCAPE '!' OR LOWER(u.display_name) LIKE ? ESCAPE '!' OR LOWER(u.email) LIKE ? ESCAPE '!')"
			args := []interface{}{pattern, pattern, pattern, pattern}
			if id, err := strconv.Atoi(keyword); err == nil && id > 0 {
				conditions = strings.TrimSuffix(conditions, ")") + " OR tickets.id = ? OR tickets.user_id = ?)"
				args = append(args, id, id)
			}
			query = query.Where(conditions, args...)
		}
		return query
	}
	var total int64
	if err := buildQuery().Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var tickets []Ticket
	err := buildQuery().
		Select("tickets.*").
		Preload("User", func(tx *gorm.DB) *gorm.DB {
			return tx.Select("id", "username", "display_name", "email")
		}).
		Order("tickets.last_message_at DESC, tickets.id DESC").
		Offset(offset).Limit(limit).Find(&tickets).Error
	return tickets, total, err
}

func GetTicketAttachment(ticketId, attachmentId int) (*TicketAttachment, int, error) {
	if ticketId <= 0 || attachmentId <= 0 {
		return nil, 0, ErrTicketNotFound
	}
	var attachment TicketAttachment
	err := DB.Where("id = ? AND ticket_id = ?", attachmentId, ticketId).First(&attachment).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, 0, ErrTicketNotFound
	}
	if err != nil {
		return nil, 0, err
	}
	var ticket Ticket
	if err := DB.Select("user_id").Where("id = ?", ticketId).First(&ticket).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, 0, ErrTicketNotFound
		}
		return nil, 0, err
	}
	return &attachment, ticket.UserId, nil
}
