package controller

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	_ "golang.org/x/image/webp"
)

const (
	ticketImageMaxBytes     int64 = 5 * 1024 * 1024
	ticketRequestMaxBytes   int64 = 6 * 1024 * 1024
	ticketMultipartMemory   int64 = 512 * 1024
	ticketImageMaxDimension       = 8192
	ticketImageMaxPixels    int64 = 16_000_000
	ticketDefaultPageSize         = 20
	ticketMaxPageSize             = 50
)

var ticketAttachmentDir = "ticket-attachments"
var ticketImageDecodeSlots = make(chan struct{}, 2)

var (
	errTicketImageInvalid    = errors.New("image content is invalid")
	errTicketImageType       = errors.New("image type is invalid")
	errTicketImageDimensions = errors.New("image dimensions are too large")
	errTicketImageBusy       = errors.New("image decoder is busy")
)

type ticketRequestError struct {
	status  int
	code    string
	message string
}

func (e *ticketRequestError) Error() string {
	return e.message
}

type ticketAttachmentResponse struct {
	Id       int    `json:"id"`
	FileName string `json:"file_name"`
	MimeType string `json:"mime_type"`
	Size     int64  `json:"size"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
}

type ticketMessageResponse struct {
	Id         int                       `json:"id"`
	SenderRole string                    `json:"sender_role"`
	Content    string                    `json:"content"`
	CreatedAt  int64                     `json:"created_at"`
	Attachment *ticketAttachmentResponse `json:"attachment,omitempty"`
}

type ticketResponse struct {
	Id            int                     `json:"id"`
	UserId        int                     `json:"user_id"`
	Title         string                  `json:"title"`
	Status        string                  `json:"status"`
	CreatedAt     int64                   `json:"created_at"`
	UpdatedAt     int64                   `json:"updated_at"`
	LastMessageAt int64                   `json:"last_message_at"`
	ClosedAt      int64                   `json:"closed_at"`
	MessageCount  int                     `json:"message_count"`
	Username      string                  `json:"username,omitempty"`
	DisplayName   string                  `json:"display_name,omitempty"`
	Email         string                  `json:"email,omitempty"`
	Messages      []ticketMessageResponse `json:"messages,omitempty"`
}

func newTicketResponse(ticket *model.Ticket, includeMessages, includeUser bool) ticketResponse {
	response := ticketResponse{
		Id: ticket.Id, UserId: ticket.UserId, Title: ticket.Title, Status: ticket.Status,
		CreatedAt: ticket.CreatedAt, UpdatedAt: ticket.UpdatedAt,
		LastMessageAt: ticket.LastMessageAt, ClosedAt: ticket.ClosedAt,
		MessageCount: ticket.MessageCount,
	}
	if includeUser && ticket.User != nil {
		response.Username = ticket.User.Username
		response.DisplayName = ticket.User.DisplayName
		response.Email = ticket.User.Email
	}
	if !includeMessages {
		return response
	}
	response.Messages = make([]ticketMessageResponse, 0, len(ticket.Messages))
	for i := range ticket.Messages {
		message := ticketMessageResponse{
			Id: ticket.Messages[i].Id, SenderRole: ticket.Messages[i].SenderRole,
			Content: ticket.Messages[i].Content, CreatedAt: ticket.Messages[i].CreatedAt,
		}
		if ticket.Messages[i].Attachment != nil {
			attachment := ticket.Messages[i].Attachment
			message.Attachment = &ticketAttachmentResponse{
				Id: attachment.Id, FileName: attachment.FileName, MimeType: attachment.MimeType,
				Size: attachment.Size, Width: attachment.Width, Height: attachment.Height,
			}
		}
		response.Messages = append(response.Messages, message)
	}
	return response
}

func ticketPageQuery(c *gin.Context) (int, int) {
	page, err := strconv.Atoi(c.DefaultQuery("p", "1"))
	if err != nil || page < 1 {
		page = 1
	}
	pageSize, err := strconv.Atoi(c.DefaultQuery("page_size", strconv.Itoa(ticketDefaultPageSize)))
	if err != nil || pageSize < 1 {
		pageSize = ticketDefaultPageSize
	}
	if pageSize > ticketMaxPageSize {
		pageSize = ticketMaxPageSize
	}
	maxInt := int(^uint(0) >> 1)
	if page > maxInt/pageSize {
		page = 1
	}
	return page, pageSize
}

func ticketAttachmentPath(storageName string) (string, error) {
	if storageName == "" || filepath.Base(storageName) != storageName || strings.ContainsAny(storageName, `/\\`) {
		return "", errors.New("invalid attachment storage name")
	}
	root, err := filepath.Abs(ticketAttachmentDir)
	if err != nil {
		return "", err
	}
	path, err := filepath.Abs(filepath.Join(root, storageName))
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("attachment path escapes storage directory")
	}
	return path, nil
}

func removeTicketAttachmentFiles(storageNames []string) {
	for _, storageName := range storageNames {
		path, err := ticketAttachmentPath(storageName)
		if err != nil {
			common.SysError("invalid ticket attachment path during user deletion: " + err.Error())
			continue
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			common.SysError("failed to remove ticket attachment during user deletion: " + err.Error())
		}
	}
}

func ticketImageFormat(format string) (mimeType, extension string, ok bool) {
	switch format {
	case "jpeg":
		return "image/jpeg", ".jpg", true
	case "png":
		return "image/png", ".png", true
	case "webp":
		return "image/webp", ".webp", true
	default:
		return "", "", false
	}
}

func validateTicketImage(data []byte) (format, mimeType, extension string, width, height int, err error) {
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return "", "", "", 0, 0, errTicketImageInvalid
	}
	mimeType, extension, ok := ticketImageFormat(format)
	if !ok {
		return "", "", "", 0, 0, errTicketImageType
	}
	if config.Width < 1 || config.Height < 1 || config.Width > ticketImageMaxDimension || config.Height > ticketImageMaxDimension || int64(config.Width)*int64(config.Height) > ticketImageMaxPixels {
		return "", "", "", 0, 0, errTicketImageDimensions
	}
	select {
	case ticketImageDecodeSlots <- struct{}{}:
		defer func() { <-ticketImageDecodeSlots }()
	default:
		return "", "", "", 0, 0, errTicketImageBusy
	}
	_, decodedFormat, err := image.Decode(bytes.NewReader(data))
	if err != nil || decodedFormat != format {
		return "", "", "", 0, 0, errTicketImageInvalid
	}
	return format, mimeType, extension, config.Width, config.Height, nil
}

func parseTicketMultipart(c *gin.Context, includeTitle bool) (string, string, *model.TicketAttachment, string, error) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, ticketRequestMaxBytes)
	if err := c.Request.ParseMultipartForm(ticketMultipartMemory); err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			return "", "", nil, "", &ticketRequestError{status: http.StatusRequestEntityTooLarge, code: "ticket_request_too_large", message: "request must not exceed 6 MiB"}
		}
		return "", "", nil, "", &ticketRequestError{status: http.StatusBadRequest, code: "ticket_invalid_multipart", message: "invalid multipart request"}
	}
	form := c.Request.MultipartForm
	if form == nil {
		return "", "", nil, "", &ticketRequestError{status: http.StatusBadRequest, code: "ticket_invalid_multipart", message: "invalid multipart request"}
	}
	defer form.RemoveAll()

	allowedValues := map[string]bool{"content": true}
	if includeTitle {
		allowedValues["title"] = true
	}
	for key, values := range form.Value {
		if !allowedValues[key] || len(values) != 1 {
			return "", "", nil, "", &ticketRequestError{status: http.StatusBadRequest, code: "ticket_invalid_fields", message: "invalid ticket fields"}
		}
	}
	if len(form.Value["content"]) != 1 || (includeTitle && len(form.Value["title"]) != 1) {
		return "", "", nil, "", &ticketRequestError{status: http.StatusBadRequest, code: "ticket_missing_fields", message: "required ticket fields are missing"}
	}
	for key, files := range form.File {
		if key != "image" || len(files) > 1 {
			return "", "", nil, "", &ticketRequestError{status: http.StatusBadRequest, code: "ticket_image_count_invalid", message: "only one image is allowed"}
		}
	}

	title := ""
	var err error
	if includeTitle {
		title, err = model.NormalizeTicketTitle(form.Value["title"][0])
		if err != nil {
			return "", "", nil, "", &ticketRequestError{status: http.StatusBadRequest, code: "ticket_title_invalid", message: "title must be 1 to 120 characters of plain text"}
		}
	}
	content, err := model.NormalizeTicketContent(form.Value["content"][0])
	if err != nil {
		return "", "", nil, "", &ticketRequestError{status: http.StatusBadRequest, code: "ticket_content_invalid", message: "content must be 1 to 4000 characters of plain text"}
	}

	files := form.File["image"]
	if len(files) == 0 {
		return title, content, nil, "", nil
	}
	header := files[0]
	if header == nil || header.Size < 1 || header.Size > ticketImageMaxBytes {
		return "", "", nil, "", &ticketRequestError{status: http.StatusBadRequest, code: "ticket_image_size_invalid", message: "image must be between 1 byte and 5 MiB"}
	}
	disposition, dispositionParams, err := mime.ParseMediaType(header.Header.Get("Content-Disposition"))
	if err != nil || !strings.EqualFold(disposition, "form-data") || dispositionParams["name"] != "image" {
		return "", "", nil, "", &ticketRequestError{status: http.StatusBadRequest, code: "ticket_image_name_invalid", message: "invalid image file name"}
	}
	fileName := strings.TrimSpace(dispositionParams["filename"])
	if fileName == "" || !utf8.ValidString(fileName) || utf8.RuneCountInString(fileName) > 255 || strings.ContainsAny(fileName, `/\\`) {
		return "", "", nil, "", &ticketRequestError{status: http.StatusBadRequest, code: "ticket_image_name_invalid", message: "invalid image file name"}
	}
	for _, r := range fileName {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || unicode.Is(unicode.Zl, r) || unicode.Is(unicode.Zp, r) {
			return "", "", nil, "", &ticketRequestError{status: http.StatusBadRequest, code: "ticket_image_name_invalid", message: "invalid image file name"}
		}
	}
	originalExtension := strings.ToLower(filepath.Ext(fileName))
	if originalExtension != ".jpg" && originalExtension != ".jpeg" && originalExtension != ".jfif" && originalExtension != ".png" && originalExtension != ".webp" {
		return "", "", nil, "", &ticketRequestError{status: http.StatusBadRequest, code: "ticket_image_type_invalid", message: "only JPEG, PNG, and WebP images are supported"}
	}

	file, err := header.Open()
	if err != nil {
		return "", "", nil, "", err
	}
	data, readErr := io.ReadAll(io.LimitReader(file, ticketImageMaxBytes+1))
	closeErr := file.Close()
	if readErr != nil || closeErr != nil {
		return "", "", nil, "", errors.New("failed to read image")
	}
	if len(data) < 1 || int64(len(data)) > ticketImageMaxBytes {
		return "", "", nil, "", &ticketRequestError{status: http.StatusBadRequest, code: "ticket_image_size_invalid", message: "image must be between 1 byte and 5 MiB"}
	}
	format, mimeType, extension, width, height, err := validateTicketImage(data)
	if err != nil {
		status := http.StatusBadRequest
		code := "ticket_image_type_invalid"
		message := "image content is invalid"
		if errors.Is(err, errTicketImageDimensions) {
			code = "ticket_image_dimensions_invalid"
			message = "image dimensions are too large"
		} else if errors.Is(err, errTicketImageBusy) {
			status = http.StatusTooManyRequests
			code = "ticket_image_busy"
			message = "image processing is busy; try again shortly"
		}
		return "", "", nil, "", &ticketRequestError{status: status, code: code, message: message}
	}
	if (format == "jpeg" && originalExtension != ".jpg" && originalExtension != ".jpeg" && originalExtension != ".jfif") || (format != "jpeg" && originalExtension != extension) {
		return "", "", nil, "", &ticketRequestError{status: http.StatusBadRequest, code: "ticket_image_extension_mismatch", message: "image extension does not match its content"}
	}
	headerMediaType, _, err := mime.ParseMediaType(header.Header.Get("Content-Type"))
	if err != nil || !strings.EqualFold(headerMediaType, mimeType) {
		return "", "", nil, "", &ticketRequestError{status: http.StatusBadRequest, code: "ticket_image_mime_mismatch", message: "image content type does not match its content"}
	}

	if err := os.MkdirAll(ticketAttachmentDir, 0700); err != nil {
		return "", "", nil, "", err
	}
	if err := os.Chmod(ticketAttachmentDir, 0700); err != nil {
		return "", "", nil, "", err
	}
	storageName := uuid.NewString() + extension
	storedPath, err := ticketAttachmentPath(storageName)
	if err != nil {
		return "", "", nil, "", err
	}
	destination, err := os.OpenFile(storedPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return "", "", nil, "", err
	}
	_, writeErr := io.Copy(destination, bytes.NewReader(data))
	closeErr = destination.Close()
	if writeErr != nil || closeErr != nil {
		_ = os.Remove(storedPath)
		return "", "", nil, "", errors.New("failed to store image")
	}
	return title, content, &model.TicketAttachment{
		StorageName: storageName, FileName: fileName, MimeType: mimeType,
		Size: int64(len(data)), Width: width, Height: height,
	}, storedPath, nil
}

func writeTicketRequestError(c *gin.Context, err error) {
	var requestError *ticketRequestError
	if errors.As(err, &requestError) {
		c.JSON(requestError.status, gin.H{"success": false, "code": requestError.code, "message": requestError.message})
		return
	}
	common.SysError("ticket request failed: " + err.Error())
	c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "ticket_request_failed", "message": "ticket request failed"})
}

func writeTicketModelError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, model.ErrTicketNotFound):
		c.JSON(http.StatusNotFound, gin.H{"success": false, "code": "ticket_not_found", "message": "ticket not found"})
	case errors.Is(err, model.ErrTicketActiveExists):
		c.JSON(http.StatusConflict, gin.H{"success": false, "code": "ticket_active_exists", "message": "finish the current ticket before creating another one"})
	case errors.Is(err, model.ErrTicketWaitingAdmin):
		c.JSON(http.StatusConflict, gin.H{"success": false, "code": "ticket_waiting_admin", "message": "wait for an administrator reply before replying again"})
	case errors.Is(err, model.ErrTicketWaitingUser):
		c.JSON(http.StatusConflict, gin.H{"success": false, "code": "ticket_waiting_user", "message": "wait for the user reply before replying again"})
	case errors.Is(err, model.ErrTicketClosed):
		c.JSON(http.StatusConflict, gin.H{"success": false, "code": "ticket_closed", "message": "ticket is already closed"})
	case errors.Is(err, model.ErrTicketMessageLimit):
		c.JSON(http.StatusConflict, gin.H{"success": false, "code": "ticket_message_limit", "message": "ticket has reached the 100 message limit"})
	case errors.Is(err, model.ErrTicketStateChanged):
		c.JSON(http.StatusConflict, gin.H{"success": false, "code": "ticket_state_changed", "message": "ticket state changed; refresh and try again"})
	case errors.Is(err, model.ErrInvalidTicketStatus), errors.Is(err, model.ErrInvalidTicketSearch):
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "code": "ticket_invalid_filter", "message": "invalid ticket filter"})
	default:
		common.SysError("ticket operation failed: " + err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "ticket_operation_failed", "message": "ticket operation failed"})
	}
}

func GetSelfTickets(c *gin.Context) {
	page, pageSize := ticketPageQuery(c)
	tickets, total, activeTicketId, err := model.ListUserTickets(c.GetInt("id"), (page-1)*pageSize, pageSize)
	if err != nil {
		writeTicketModelError(c, err)
		return
	}
	items := make([]ticketResponse, 0, len(tickets))
	for i := range tickets {
		items = append(items, newTicketResponse(&tickets[i], false, false))
	}
	common.ApiSuccess(c, gin.H{
		"items": items, "total": total, "page": page, "page_size": pageSize,
		"active_ticket_id": activeTicketId,
	})
}

func CreateTicket(c *gin.Context) {
	if err := model.CheckCanCreateTicket(c.GetInt("id")); err != nil {
		writeTicketModelError(c, err)
		return
	}
	title, content, attachment, storedPath, err := parseTicketMultipart(c, true)
	if err != nil {
		writeTicketRequestError(c, err)
		return
	}
	ticket, err := model.CreateTicket(c.GetInt("id"), title, content, attachment)
	if err != nil {
		if storedPath != "" {
			_ = os.Remove(storedPath)
		}
		writeTicketModelError(c, err)
		return
	}
	common.ApiSuccess(c, newTicketResponse(ticket, true, false))
}

func GetTicket(c *gin.Context) {
	ticketId, err := strconv.Atoi(c.Param("id"))
	if err != nil || ticketId <= 0 {
		writeTicketModelError(c, model.ErrTicketNotFound)
		return
	}
	ticket, err := model.GetTicketForUser(ticketId, c.GetInt("id"))
	if err != nil {
		writeTicketModelError(c, err)
		return
	}
	common.ApiSuccess(c, newTicketResponse(ticket, true, false))
}

func ReplyTicket(c *gin.Context) {
	ticketId, err := strconv.Atoi(c.Param("id"))
	if err != nil || ticketId <= 0 {
		writeTicketModelError(c, model.ErrTicketNotFound)
		return
	}
	if err := model.CheckCanReplyTicketByUser(ticketId, c.GetInt("id")); err != nil {
		writeTicketModelError(c, err)
		return
	}
	_, content, attachment, storedPath, err := parseTicketMultipart(c, false)
	if err != nil {
		writeTicketRequestError(c, err)
		return
	}
	ticket, err := model.ReplyTicketByUser(ticketId, c.GetInt("id"), content, attachment)
	if err != nil {
		if storedPath != "" {
			_ = os.Remove(storedPath)
		}
		writeTicketModelError(c, err)
		return
	}
	common.ApiSuccess(c, newTicketResponse(ticket, true, false))
}

func ListAdminTickets(c *gin.Context) {
	page, pageSize := ticketPageQuery(c)
	userId := 0
	if value := strings.TrimSpace(c.Query("user_id")); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "code": "ticket_user_id_invalid", "message": "invalid user id"})
			return
		}
		userId = parsed
	}
	tickets, total, err := model.ListAdminTickets(model.TicketAdminFilter{
		Status: c.Query("status"), Keyword: c.Query("keyword"), UserId: userId,
	}, (page-1)*pageSize, pageSize)
	if err != nil {
		writeTicketModelError(c, err)
		return
	}
	items := make([]ticketResponse, 0, len(tickets))
	for i := range tickets {
		items = append(items, newTicketResponse(&tickets[i], false, true))
	}
	common.ApiSuccess(c, gin.H{"items": items, "total": total, "page": page, "page_size": pageSize})
}

func GetAdminTicket(c *gin.Context) {
	ticketId, err := strconv.Atoi(c.Param("id"))
	if err != nil || ticketId <= 0 {
		writeTicketModelError(c, model.ErrTicketNotFound)
		return
	}
	ticket, err := model.GetTicketForAdmin(ticketId)
	if err != nil {
		writeTicketModelError(c, err)
		return
	}
	common.ApiSuccess(c, newTicketResponse(ticket, true, true))
}

func ReplyAdminTicket(c *gin.Context) {
	ticketId, err := strconv.Atoi(c.Param("id"))
	if err != nil || ticketId <= 0 {
		writeTicketModelError(c, model.ErrTicketNotFound)
		return
	}
	if err := model.CheckCanReplyTicketByAdmin(ticketId, c.GetInt("id")); err != nil {
		writeTicketModelError(c, err)
		return
	}
	_, content, attachment, storedPath, err := parseTicketMultipart(c, false)
	if err != nil {
		writeTicketRequestError(c, err)
		return
	}
	ticket, err := model.ReplyTicketByAdmin(ticketId, c.GetInt("id"), content, attachment)
	if err != nil {
		if storedPath != "" {
			_ = os.Remove(storedPath)
		}
		writeTicketModelError(c, err)
		return
	}
	common.ApiSuccess(c, newTicketResponse(ticket, true, true))
}

func CloseAdminTicket(c *gin.Context) {
	ticketId, err := strconv.Atoi(c.Param("id"))
	if err != nil || ticketId <= 0 {
		writeTicketModelError(c, model.ErrTicketNotFound)
		return
	}
	ticket, err := model.CloseTicket(ticketId, c.GetInt("id"))
	if err != nil {
		writeTicketModelError(c, err)
		return
	}
	common.ApiSuccess(c, newTicketResponse(ticket, true, true))
}

func DownloadTicketAttachment(c *gin.Context) {
	ticketId, ticketErr := strconv.Atoi(c.Param("id"))
	attachmentId, attachmentErr := strconv.Atoi(c.Param("attachment_id"))
	if ticketErr != nil || attachmentErr != nil || ticketId <= 0 || attachmentId <= 0 {
		writeTicketModelError(c, model.ErrTicketNotFound)
		return
	}
	attachment, ownerId, err := model.GetTicketAttachment(ticketId, attachmentId)
	if err != nil || (c.GetInt("role") < common.RoleAdminUser && ownerId != c.GetInt("id")) {
		if err != nil && !errors.Is(err, model.ErrTicketNotFound) {
			common.SysError("ticket attachment lookup failed: " + err.Error())
		}
		writeTicketModelError(c, model.ErrTicketNotFound)
		return
	}
	path, err := ticketAttachmentPath(attachment.StorageName)
	if err != nil {
		common.SysError("invalid stored ticket attachment path: " + err.Error())
		writeTicketModelError(c, model.ErrTicketNotFound)
		return
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() != attachment.Size {
		writeTicketModelError(c, model.ErrTicketNotFound)
		return
	}
	root, rootErr := filepath.EvalSymlinks(ticketAttachmentDir)
	resolvedPath, pathErr := filepath.EvalSymlinks(path)
	if rootErr != nil || pathErr != nil {
		writeTicketModelError(c, model.ErrTicketNotFound)
		return
	}
	root, rootErr = filepath.Abs(root)
	resolvedPath, pathErr = filepath.Abs(resolvedPath)
	if rootErr != nil || pathErr != nil {
		writeTicketModelError(c, model.ErrTicketNotFound)
		return
	}
	relative, err := filepath.Rel(root, resolvedPath)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		writeTicketModelError(c, model.ErrTicketNotFound)
		return
	}
	file, err := os.Open(resolvedPath)
	if err != nil {
		writeTicketModelError(c, model.ErrTicketNotFound)
		return
	}
	defer file.Close()
	config, format, err := image.DecodeConfig(file)
	verifiedMime, extension, ok := ticketImageFormat(format)
	if err != nil || !ok || verifiedMime != attachment.MimeType || config.Width != attachment.Width || config.Height != attachment.Height {
		writeTicketModelError(c, model.ErrTicketNotFound)
		return
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		writeTicketModelError(c, model.ErrTicketNotFound)
		return
	}
	c.Header("Content-Type", verifiedMime)
	c.Header("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": "ticket-image" + extension}))
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Security-Policy", "sandbox")
	c.Header("Cross-Origin-Resource-Policy", "same-origin")
	c.Header("Cache-Control", "private, no-store")
	http.ServeContent(c.Writer, c.Request, fmt.Sprintf("ticket-image%s", extension), info.ModTime(), file)
}
