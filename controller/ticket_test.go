package controller

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func encodedTicketImage(t *testing.T, format string, width, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	var buffer bytes.Buffer
	var err error
	switch format {
	case "png":
		err = png.Encode(&buffer, img)
	case "jpeg":
		err = jpeg.Encode(&buffer, img, &jpeg.Options{Quality: 80})
	case "gif":
		err = gif.Encode(&buffer, img, nil)
	default:
		t.Fatalf("unsupported test image format %q", format)
	}
	require.NoError(t, err)
	return buffer.Bytes()
}

func ticketMultipartContext(t *testing.T, fileName, mimeType string, data []byte) *gin.Context {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("title", "API request failed"))
	require.NoError(t, writer.WriteField("content", "The upstream returned an error."))
	if fileName != "" {
		header := textproto.MIMEHeader{}
		header.Set("Content-Disposition", `form-data; name="image"; filename="`+fileName+`"`)
		header.Set("Content-Type", mimeType)
		part, err := writer.CreatePart(header)
		require.NoError(t, err)
		_, err = part.Write(data)
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest(http.MethodPost, "/api/ticket", &body)
	context.Request.Header.Set("Content-Type", writer.FormDataContentType())
	return context
}

func TestValidateTicketImageAllowsOnlyFullyDecodableBoundedFormats(t *testing.T) {
	tests := []struct {
		name     string
		format   string
		mimeType string
	}{
		{name: "png", format: "png", mimeType: "image/png"},
		{name: "jpeg", format: "jpeg", mimeType: "image/jpeg"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			format, mimeType, _, width, height, err := validateTicketImage(encodedTicketImage(t, test.format, 8, 6))
			require.NoError(t, err)
			assert.Equal(t, test.format, format)
			assert.Equal(t, test.mimeType, mimeType)
			assert.Equal(t, 8, width)
			assert.Equal(t, 6, height)
		})
	}
	webpData, err := base64.StdEncoding.DecodeString("UklGRrIBAABXRUJQVlA4TKUBAAAvSsAYAA8w//M///MfeJAkbXvaSG7m8Q3GfYSBJekwQztm/IcZlgwnmWImn2BK7aFmBtnVir6q//8VOkFE/xm4baTIu8c48ArEo6+B3zFKYln3pqClSCKX0begFTAXFOLXHSyF8cCNcZEG4OywuA4KVVfJCiArU7GAgJI8+lJP/OKMT/fBAjevg1cYB7YVkFuWga2lyPi5I0HFy5YTpWIHg0RZpkniRVW9odHAKOwosWuOGdxIyn2OvaCDvhg/we6TwadPBPbqBV58MsLmMJ8yZnOWk8SRz4N+QoyPL+MnamzMvcE1rHNEr91F9GKZPVUcS9w7PhhH36suB9qPeYb/oLk6cuTiJ0wOK3m5h1cKjW6EVZCYMK7dxcKCBdgP9HkKr9gkAO2P8GKZGWVdIAatQa+1IDpt6qyorVwdy01xdW8Jkfk6xjEXmVQQ+HQdFr6OKhIN34dXWq0+0qr6EJSCeeVLH9+gvGTLyqM65PQ44ihzlTXxQKjKbAvshXgir7Lil9w4L2bvMycmjQcqXaMCO6BlY28i+FOLzbfI1vEqxAhotocAAA==")
	require.NoError(t, err)
	format, mimeType, _, width, height, err := validateTicketImage(webpData)
	require.NoError(t, err)
	assert.Equal(t, "webp", format)
	assert.Equal(t, "image/webp", mimeType)
	assert.Equal(t, 75, width)
	assert.Equal(t, 100, height)

	_, _, _, _, _, err = validateTicketImage(encodedTicketImage(t, "gif", 8, 6))
	require.ErrorIs(t, err, errTicketImageType)

	truncated := encodedTicketImage(t, "png", 8, 6)
	truncated = truncated[:len(truncated)/2]
	_, _, _, _, _, err = validateTicketImage(truncated)
	require.ErrorIs(t, err, errTicketImageInvalid)

	_, _, _, _, _, err = validateTicketImage(encodedTicketImage(t, "png", ticketImageMaxDimension+1, 1))
	require.ErrorIs(t, err, errTicketImageDimensions)
}

func TestValidateTicketImageRejectsWhenDecodeConcurrencyIsFull(t *testing.T) {
	for i := 0; i < cap(ticketImageDecodeSlots); i++ {
		ticketImageDecodeSlots <- struct{}{}
	}
	t.Cleanup(func() {
		for len(ticketImageDecodeSlots) > 0 {
			<-ticketImageDecodeSlots
		}
	})

	_, _, _, _, _, err := validateTicketImage(encodedTicketImage(t, "png", 2, 2))
	require.ErrorIs(t, err, errTicketImageBusy)
}

func TestParseTicketMultipartStoresValidatedImageWithPrivatePermissions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	originalDir := ticketAttachmentDir
	ticketAttachmentDir = filepath.Join(t.TempDir(), "ticket-attachments")
	t.Cleanup(func() { ticketAttachmentDir = originalDir })

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("title", "API request failed"))
	require.NoError(t, writer.WriteField("content", "The upstream returned an error."))
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", `form-data; name="image"; filename="evidence.png"`)
	header.Set("Content-Type", "image/png")
	part, err := writer.CreatePart(header)
	require.NoError(t, err)
	_, err = part.Write(encodedTicketImage(t, "png", 4, 3))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest(http.MethodPost, "/api/ticket", &body)
	context.Request.Header.Set("Content-Type", writer.FormDataContentType())
	title, content, attachment, storedPath, err := parseTicketMultipart(context, true)
	require.NoError(t, err)
	assert.Equal(t, "API request failed", title)
	assert.Equal(t, "The upstream returned an error.", content)
	require.NotNil(t, attachment)
	assert.Equal(t, "image/png", attachment.MimeType)
	assert.Equal(t, 4, attachment.Width)
	assert.Equal(t, 3, attachment.Height)
	assert.NotContains(t, storedPath, attachment.FileName)
	info, err := os.Stat(storedPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
}

func TestParseTicketMultipartAcceptsJFIFAndRejectsDisguisedOrOversizedFiles(t *testing.T) {
	gin.SetMode(gin.TestMode)
	originalDir := ticketAttachmentDir
	ticketAttachmentDir = filepath.Join(t.TempDir(), "ticket-attachments")
	t.Cleanup(func() { ticketAttachmentDir = originalDir })

	context := ticketMultipartContext(t, "evidence.jfif", "image/jpeg", encodedTicketImage(t, "jpeg", 3, 2))
	_, _, attachment, storedPath, err := parseTicketMultipart(context, true)
	require.NoError(t, err)
	require.NotNil(t, attachment)
	assert.Equal(t, "image/jpeg", attachment.MimeType)
	assert.Equal(t, ".jpg", filepath.Ext(storedPath))

	tests := []struct {
		name     string
		fileName string
		mimeType string
		data     []byte
		code     string
	}{
		{name: "jpeg disguised as png", fileName: "fake.png", mimeType: "image/png", data: encodedTicketImage(t, "jpeg", 2, 2), code: "ticket_image_extension_mismatch"},
		{name: "content type mismatch", fileName: "evidence.png", mimeType: "image/jpeg", data: encodedTicketImage(t, "png", 2, 2), code: "ticket_image_mime_mismatch"},
		{name: "gif rejected", fileName: "animation.gif", mimeType: "image/gif", data: encodedTicketImage(t, "gif", 2, 2), code: "ticket_image_type_invalid"},
		{name: "svg rejected", fileName: "payload.svg", mimeType: "image/svg+xml", data: []byte(`<svg><script>alert(1)</script></svg>`), code: "ticket_image_type_invalid"},
		{name: "empty file", fileName: "empty.png", mimeType: "image/png", data: nil, code: "ticket_image_size_invalid"},
		{name: "path file name", fileName: "../payload.png", mimeType: "image/png", data: encodedTicketImage(t, "png", 2, 2), code: "ticket_image_name_invalid"},
		{name: "oversized file", fileName: "large.png", mimeType: "image/png", data: make([]byte, ticketImageMaxBytes+1), code: "ticket_image_size_invalid"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			context := ticketMultipartContext(t, test.fileName, test.mimeType, test.data)
			_, _, _, _, err := parseTicketMultipart(context, true)
			var requestError *ticketRequestError
			require.ErrorAs(t, err, &requestError)
			assert.Equal(t, test.code, requestError.code)
		})
	}
}

func TestParseTicketMultipartRejectsDuplicateAndUnknownFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name      string
		buildBody func(*testing.T, *multipart.Writer)
		code      string
	}{
		{
			name: "duplicate content",
			buildBody: func(t *testing.T, writer *multipart.Writer) {
				require.NoError(t, writer.WriteField("title", "Title"))
				require.NoError(t, writer.WriteField("content", "First"))
				require.NoError(t, writer.WriteField("content", "Second"))
			},
			code: "ticket_invalid_fields",
		},
		{
			name: "unknown file field",
			buildBody: func(t *testing.T, writer *multipart.Writer) {
				require.NoError(t, writer.WriteField("title", "Title"))
				require.NoError(t, writer.WriteField("content", "Content"))
				part, err := writer.CreateFormFile("document", "payload.png")
				require.NoError(t, err)
				_, err = part.Write(encodedTicketImage(t, "png", 2, 2))
				require.NoError(t, err)
			},
			code: "ticket_image_count_invalid",
		},
		{
			name: "duplicate image",
			buildBody: func(t *testing.T, writer *multipart.Writer) {
				require.NoError(t, writer.WriteField("title", "Title"))
				require.NoError(t, writer.WriteField("content", "Content"))
				for i := 0; i < 2; i++ {
					part, err := writer.CreateFormFile("image", "payload.png")
					require.NoError(t, err)
					_, err = part.Write(encodedTicketImage(t, "png", 2, 2))
					require.NoError(t, err)
				}
			},
			code: "ticket_image_count_invalid",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var body bytes.Buffer
			writer := multipart.NewWriter(&body)
			test.buildBody(t, writer)
			require.NoError(t, writer.Close())
			context, _ := gin.CreateTestContext(httptest.NewRecorder())
			context.Request = httptest.NewRequest(http.MethodPost, "/api/ticket", &body)
			context.Request.Header.Set("Content-Type", writer.FormDataContentType())
			_, _, _, _, err := parseTicketMultipart(context, true)
			var requestError *ticketRequestError
			require.ErrorAs(t, err, &requestError)
			assert.Equal(t, test.code, requestError.code)
		})
	}
}

func TestTicketResponseDoesNotExposeSenderOrStorageIdentifiers(t *testing.T) {
	response := newTicketResponse(&model.Ticket{
		Id: 1, UserId: 2, Title: "Question", Status: model.TicketStatusWaitingUser,
		User: &model.User{Username: "alice", Password: "password-hash", AccessToken: func() *string { value := "secret-token"; return &value }()},
		Messages: []model.TicketMessage{{
			Id: 3, SenderId: 99, SenderRole: model.TicketSenderAdmin, Content: "Reply",
			Attachment: &model.TicketAttachment{
				Id: 4, UploaderId: 99, StorageName: "server-secret.png", FileName: "evidence.png", MimeType: "image/png", Size: 10,
			},
		}},
	}, true, true)
	payload, err := common.Marshal(response)
	require.NoError(t, err)
	serialized := string(payload)
	assert.Contains(t, serialized, "alice")
	assert.NotContains(t, serialized, "sender_id")
	assert.NotContains(t, serialized, "uploader")
	assert.NotContains(t, serialized, "server-secret")
	assert.NotContains(t, serialized, "password-hash")
	assert.NotContains(t, serialized, "secret-token")
	assert.NotContains(t, strings.ToLower(serialized), "file_path")
}

func TestDownloadTicketAttachmentEnforcesOwnershipAndSecurityHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	originalDB := model.DB
	originalMainType := common.MainDatabaseType()
	originalDir := ticketAttachmentDir
	t.Cleanup(func() {
		model.DB = originalDB
		common.SetMainDatabaseType(originalMainType)
		ticketAttachmentDir = originalDir
	})

	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "tickets.db")), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	require.NoError(t, db.AutoMigrate(&model.Ticket{}, &model.TicketMessage{}, &model.TicketAttachment{}))
	ticketAttachmentDir = filepath.Join(t.TempDir(), "ticket-attachments")
	require.NoError(t, os.MkdirAll(ticketAttachmentDir, 0700))

	data := encodedTicketImage(t, "png", 4, 3)
	storageName := "8db55d31-f506-48eb-a1f0-9520ce10de49.png"
	path, err := ticketAttachmentPath(storageName)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0600))
	ticket := model.Ticket{UserId: 101, Title: "Private ticket", Status: model.TicketStatusWaitingAdmin, MessageCount: 1, LastMessageAt: 1}
	require.NoError(t, db.Create(&ticket).Error)
	message := model.TicketMessage{TicketId: ticket.Id, SenderId: 101, SenderRole: model.TicketSenderUser, Content: "Private message"}
	require.NoError(t, db.Create(&message).Error)
	attachment := model.TicketAttachment{
		TicketId: ticket.Id, MessageId: message.Id, UploaderId: 101,
		StorageName: storageName, FileName: `untrusted"name.png`, MimeType: "image/png",
		Size: int64(len(data)), Width: 4, Height: 3,
	}
	require.NoError(t, db.Create(&attachment).Error)

	perform := func(userId, role int) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(recorder)
		context.Request = httptest.NewRequest(http.MethodGet, "/api/ticket/1/attachment/1", nil)
		context.Params = gin.Params{{Key: "id", Value: strconv.Itoa(ticket.Id)}, {Key: "attachment_id", Value: strconv.Itoa(attachment.Id)}}
		context.Set("id", userId)
		context.Set("role", role)
		DownloadTicketAttachment(context)
		return recorder
	}

	denied := perform(202, common.RoleCommonUser)
	assert.Equal(t, http.StatusNotFound, denied.Code)
	assert.Contains(t, denied.Body.String(), `"code":"ticket_not_found"`)

	owner := perform(101, common.RoleCommonUser)
	assert.Equal(t, http.StatusOK, owner.Code)
	assert.Equal(t, data, owner.Body.Bytes())
	assert.Equal(t, "nosniff", owner.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "sandbox", owner.Header().Get("Content-Security-Policy"))
	assert.Equal(t, "same-origin", owner.Header().Get("Cross-Origin-Resource-Policy"))
	assert.Equal(t, "private, no-store", owner.Header().Get("Cache-Control"))
	assert.Equal(t, "image/png", owner.Header().Get("Content-Type"))
	assert.NotContains(t, owner.Header().Get("Content-Disposition"), "untrusted")

	admin := perform(303, common.RoleAdminUser)
	assert.Equal(t, http.StatusOK, admin.Code)
	assert.Equal(t, data, admin.Body.Bytes())
}
