package model

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupTicketTest(t *testing.T) {
	t.Helper()
	require.NoError(t, DB.AutoMigrate(&Ticket{}, &TicketMessage{}, &TicketAttachment{}))
	require.NoError(t, DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&TicketAttachment{}).Error)
	require.NoError(t, DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&TicketMessage{}).Error)
	require.NoError(t, DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&Ticket{}).Error)
}

func createTicketTestUser(t *testing.T, prefix string) User {
	t.Helper()
	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	user := User{
		Username: prefix + suffix, Password: "password", Status: common.UserStatusEnabled,
		Role: common.RoleCommonUser, AffCode: prefix + suffix,
	}
	require.NoError(t, DB.Create(&user).Error)
	return user
}

func TestTicketStateMachineAndOwnership(t *testing.T) {
	setupTicketTest(t)
	user := createTicketTestUser(t, "ticket-user-")
	other := createTicketTestUser(t, "ticket-other-")
	admin := createTicketTestUser(t, "ticket-admin-")

	ticket, err := CreateTicket(user.Id, "Cannot call the API", "Please help me diagnose this request.", nil)
	require.NoError(t, err)
	assert.Equal(t, TicketStatusWaitingAdmin, ticket.Status)
	assert.Equal(t, 1, ticket.MessageCount)
	require.Len(t, ticket.Messages, 1)
	assert.Equal(t, TicketSenderUser, ticket.Messages[0].SenderRole)

	_, err = CreateTicket(user.Id, "Second ticket", "This must be rejected.", nil)
	require.ErrorIs(t, err, ErrTicketActiveExists)
	_, err = GetTicketForUser(ticket.Id, other.Id)
	require.ErrorIs(t, err, ErrTicketNotFound)
	_, err = ReplyTicketByUser(ticket.Id, other.Id, "I should not see this.", nil)
	require.ErrorIs(t, err, ErrTicketNotFound)
	_, err = ReplyTicketByUser(ticket.Id, user.Id, "Sending twice should fail.", nil)
	require.ErrorIs(t, err, ErrTicketWaitingAdmin)

	ticket, err = ReplyTicketByAdmin(ticket.Id, admin.Id, "Please provide the request ID.", nil)
	require.NoError(t, err)
	assert.Equal(t, TicketStatusWaitingUser, ticket.Status)
	assert.Equal(t, 2, ticket.MessageCount)
	_, err = ReplyTicketByAdmin(ticket.Id, admin.Id, "A second admin reply is not allowed.", nil)
	require.ErrorIs(t, err, ErrTicketWaitingUser)

	ticket, err = ReplyTicketByUser(ticket.Id, user.Id, "The request ID is req-123.", nil)
	require.NoError(t, err)
	assert.Equal(t, TicketStatusWaitingAdmin, ticket.Status)
	assert.Equal(t, 3, ticket.MessageCount)

	ticket, err = CloseTicket(ticket.Id, admin.Id)
	require.NoError(t, err)
	assert.Equal(t, TicketStatusClosed, ticket.Status)
	assert.Zero(t, ticket.ActiveSlot)
	assert.NotZero(t, ticket.ClosedAt)
	_, err = ReplyTicketByUser(ticket.Id, user.Id, "Closed tickets cannot be reopened.", nil)
	require.ErrorIs(t, err, ErrTicketClosed)

	second, err := CreateTicket(user.Id, "A new issue", "This is allowed after closure.", nil)
	require.NoError(t, err)
	assert.NotEqual(t, ticket.Id, second.Id)

	items, total, activeId, err := ListUserTickets(user.Id, 0, 50)
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	require.Len(t, items, 2)
	require.NotNil(t, activeId)
	assert.Equal(t, second.Id, *activeId)
}

func TestTicketMessageLimitIsEnforcedBeforeInsert(t *testing.T) {
	setupTicketTest(t)
	user := createTicketTestUser(t, "ticket-limit-")
	admin := createTicketTestUser(t, "ticket-limit-admin-")
	ticket, err := CreateTicket(user.Id, "Long conversation", "Initial message", nil)
	require.NoError(t, err)
	require.NoError(t, DB.Model(&Ticket{}).Where("id = ?", ticket.Id).Update("message_count", TicketMessageLimit).Error)

	_, err = ReplyTicketByAdmin(ticket.Id, admin.Id, "This reply must not be inserted.", nil)
	require.ErrorIs(t, err, ErrTicketMessageLimit)
	var count int64
	require.NoError(t, DB.Model(&TicketMessage{}).Where("ticket_id = ?", ticket.Id).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func TestTicketTextValidationRejectsVisualControlCharacters(t *testing.T) {
	_, err := NormalizeTicketTitle("safe\u202Egnp.exe")
	require.Error(t, err)
	_, err = NormalizeTicketTitle("safe\u200Btitle")
	require.Error(t, err)
	_, err = NormalizeTicketTitle("line\u2028separator")
	require.Error(t, err)
	_, err = NormalizeTicketContent("body\x00text")
	require.Error(t, err)
	_, err = NormalizeTicketTitle(strings.Repeat("a", TicketTitleMaxLength+1))
	require.Error(t, err)

	content, err := NormalizeTicketContent("first\u2028second\r\nthird")
	require.NoError(t, err)
	assert.Equal(t, "first\nsecond\nthird", content)
}

func TestTicketAdminListFiltersWithoutExposingOtherRows(t *testing.T) {
	setupTicketTest(t)
	user := createTicketTestUser(t, "ticket-search-")
	other := createTicketTestUser(t, "ticket-search-other-")
	literal := createTicketTestUser(t, "ticket-search-literal-")
	first, err := CreateTicket(user.Id, "Provider timeout", "The upstream request timed out.", nil)
	require.NoError(t, err)
	_, err = CreateTicket(other.Id, "Billing question", "Please explain this charge.", nil)
	require.NoError(t, err)
	literalTicket, err := CreateTicket(literal.Id, "Usage is 100% lower_case", "Literal wildcard characters.", nil)
	require.NoError(t, err)

	items, total, err := ListAdminTickets(TicketAdminFilter{Keyword: "  Provider  ", Status: TicketStatusWaitingAdmin}, 0, 20)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, items, 1)
	assert.Equal(t, first.Id, items[0].Id)
	require.NotNil(t, items[0].User)
	assert.Equal(t, user.Username, items[0].User.Username)
	assert.Empty(t, items[0].User.Password)

	userItems, userTotal, _, err := ListUserTickets(user.Id, 0, 20)
	require.NoError(t, err)
	assert.Equal(t, int64(1), userTotal)
	require.Len(t, userItems, 1)
	assert.Equal(t, first.Id, userItems[0].Id)

	for _, keyword := range []string{"%", "_"} {
		items, total, err = ListAdminTickets(TicketAdminFilter{Keyword: keyword}, 0, 20)
		require.NoError(t, err)
		assert.Equal(t, int64(1), total)
		require.Len(t, items, 1)
		assert.Equal(t, literalTicket.Id, items[0].Id)
	}
	for _, keyword := range []string{"bad\x00query", "bad\u202Equery", "bad\u200Bquery"} {
		_, _, err = ListAdminTickets(TicketAdminFilter{Keyword: keyword}, 0, 20)
		require.ErrorIs(t, err, ErrInvalidTicketSearch)
	}
}

func TestTicketMigrationHasNoForeignKeysAndEnforcesOneActiveSlot(t *testing.T) {
	originalDB := DB
	originalMainType := common.MainDatabaseType()
	t.Cleanup(func() {
		DB = originalDB
		common.SetMainDatabaseType(originalMainType)
		initCol()
	})

	dbPath := filepath.Join(t.TempDir(), "tickets.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	initCol()
	require.NoError(t, db.AutoMigrate(&User{}, &UserOAuthBinding{}, &Ticket{}, &TicketMessage{}, &TicketAttachment{}))

	for _, table := range []string{"tickets", "ticket_messages", "ticket_attachments"} {
		var foreignKeys []struct{ Id int }
		require.NoError(t, db.Raw("PRAGMA foreign_key_list("+table+")").Scan(&foreignKeys).Error)
		assert.Empty(t, foreignKeys, "unexpected foreign key on %s", table)
	}
	require.True(t, db.Migrator().HasIndex(&Ticket{}, "idx_ticket_user_active"))

	user := User{Username: "migration-user", Password: "password", AffCode: "migration-user"}
	require.NoError(t, db.Create(&user).Error)
	closed := []Ticket{
		{UserId: user.Id, Title: "closed one", Status: TicketStatusClosed, MessageCount: 1, LastMessageAt: 1},
		{UserId: user.Id, Title: "closed two", Status: TicketStatusClosed, MessageCount: 1, LastMessageAt: 2},
	}
	require.NoError(t, db.Create(&closed).Error)
	activeSlot := 1
	active := Ticket{UserId: user.Id, Title: "active", Status: TicketStatusWaitingAdmin, ActiveSlot: &activeSlot, MessageCount: 1, LastMessageAt: 3}
	require.NoError(t, db.Create(&active).Error)
	message := TicketMessage{TicketId: active.Id, SenderId: user.Id, SenderRole: TicketSenderUser, Content: "message"}
	require.NoError(t, db.Create(&message).Error)
	require.NoError(t, db.Create(&TicketAttachment{
		TicketId: active.Id, MessageId: message.Id, UploaderId: user.Id,
		StorageName: "opaque-storage.png", FileName: "image.png", MimeType: "image/png", Size: 10, Width: 1, Height: 1,
	}).Error)
	err = db.Create(&Ticket{UserId: user.Id, Title: "duplicate active", Status: TicketStatusWaitingAdmin, ActiveSlot: &activeSlot, MessageCount: 1, LastMessageAt: 4}).Error
	require.Error(t, err)
	storageNames, err := HardDeleteUserByIdWithTicketAttachments(user.Id)
	require.NoError(t, err)
	assert.Equal(t, []string{"opaque-storage.png"}, storageNames)
	for _, table := range []interface{}{&Ticket{}, &TicketMessage{}, &TicketAttachment{}} {
		var count int64
		require.NoError(t, db.Model(table).Count(&count).Error)
		assert.Zero(t, count)
	}

	orphanSlot := 1
	orphan := Ticket{UserId: user.Id, Title: "orphan", Status: TicketStatusWaitingAdmin, ActiveSlot: &orphanSlot, MessageCount: 1, LastMessageAt: 5}
	require.NoError(t, db.Create(&orphan).Error)
	_, err = ReplyTicketByAdmin(orphan.Id, 1234, "must reject missing owner", nil)
	require.ErrorIs(t, err, ErrTicketNotFound)
	_, err = CloseTicket(orphan.Id, 1234)
	require.ErrorIs(t, err, ErrTicketNotFound)
	var messageCount int64
	require.NoError(t, db.Model(&TicketMessage{}).Where("ticket_id = ?", orphan.Id).Count(&messageCount).Error)
	assert.Zero(t, messageCount)
}

func TestSoftDeleteUserRemovesTicketsAndReturnsAttachmentNames(t *testing.T) {
	setupTicketTest(t)
	user := createTicketTestUser(t, "ticket-soft-delete-")
	ticket, err := CreateTicket(user.Id, "Delete my account", "Remove this ticket too.", nil)
	require.NoError(t, err)
	message := ticket.Messages[0]
	require.NoError(t, DB.Create(&TicketAttachment{
		TicketId: ticket.Id, MessageId: message.Id, UploaderId: user.Id,
		StorageName: "soft-delete-attachment.png", FileName: "attachment.png",
		MimeType: "image/png", Size: 10, Width: 1, Height: 1,
	}).Error)

	storageNames, err := DeleteUserByIdWithTicketAttachments(user.Id)
	require.NoError(t, err)
	assert.Equal(t, []string{"soft-delete-attachment.png"}, storageNames)

	var deletedUser User
	require.NoError(t, DB.Unscoped().First(&deletedUser, user.Id).Error)
	assert.True(t, deletedUser.DeletedAt.Valid)
	for _, table := range []interface{}{&Ticket{}, &TicketMessage{}, &TicketAttachment{}} {
		var count int64
		require.NoError(t, DB.Model(table).Count(&count).Error)
		assert.Zero(t, count)
	}
}

func TestConcurrentTicketCreationKeepsSingleActiveTicket(t *testing.T) {
	originalDB := DB
	originalMainType := common.MainDatabaseType()
	t.Cleanup(func() {
		DB = originalDB
		common.SetMainDatabaseType(originalMainType)
		initCol()
	})

	dsn := fmt.Sprintf("file:%s?_busy_timeout=30000&_journal_mode=WAL", filepath.Join(t.TempDir(), "concurrent.db"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	initCol()
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(8)
	require.NoError(t, db.AutoMigrate(&User{}, &Ticket{}, &TicketMessage{}, &TicketAttachment{}))
	user := User{Username: "concurrent-user", Password: "password", AffCode: "concurrent-user"}
	require.NoError(t, db.Create(&user).Error)

	start := make(chan struct{})
	results := make(chan error, 8)
	var wait sync.WaitGroup
	for i := 0; i < 8; i++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			<-start
			_, createErr := CreateTicket(user.Id, fmt.Sprintf("Concurrent ticket %d", index), "Only one request may win.", nil)
			results <- createErr
		}(i)
	}
	close(start)
	wait.Wait()
	close(results)

	successes := 0
	for createErr := range results {
		if createErr == nil {
			successes++
			continue
		}
		assert.True(t, errors.Is(createErr, ErrTicketActiveExists) || strings.Contains(strings.ToLower(createErr.Error()), "locked"), createErr)
	}
	assert.Equal(t, 1, successes)
	var activeCount int64
	require.NoError(t, db.Model(&Ticket{}).Where("user_id = ? AND active_slot = ?", user.Id, 1).Count(&activeCount).Error)
	assert.Equal(t, int64(1), activeCount)
	var ticketCount int64
	require.NoError(t, db.Model(&Ticket{}).Where("user_id = ?", user.Id).Count(&ticketCount).Error)
	assert.Equal(t, int64(1), ticketCount)
}
