package controller

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type invoiceApplicationRequest struct {
	InvoiceTitle    string `json:"invoice_title"`
	TaxpayerId      string `json:"taxpayer_id"`
	BankName        string `json:"bank_name"`
	Remark          string `json:"remark"`
	SubscriptionIds []int  `json:"subscription_ids"`
	RedemptionIds   []int  `json:"redemption_ids"`
}

type invoiceRejectRequest struct {
	Reason string `json:"reason"`
}

type invoiceSettingsUpdateRequest struct {
	InvoiceEnabled                *bool `json:"invoice_enabled"`
	ApplicationDay                *int  `json:"application_day"`
	LookbackDays                  *int  `json:"lookback_days"`
	MonthlyLimit                  *int  `json:"monthly_limit"`
	SystemRechargeEnabled         *bool `json:"system_recharge_enabled"`
	RedemptionRechargeEnabled     *bool `json:"redemption_recharge_enabled"`
	SystemSubscriptionEnabled     *bool `json:"system_subscription_enabled"`
	RedemptionSubscriptionEnabled *bool `json:"redemption_subscription_enabled"`
}

const (
	invoiceApplicationRequestMaxBytes int64 = 64 * 1024
	invoiceSettingsRequestMaxBytes    int64 = 8 * 1024
	invoicePDFMaxBytes                int64 = 20 * 1024 * 1024
	invoicePDFRequestMaxBytes         int64 = invoicePDFMaxBytes + 1024*1024
	invoiceDefaultPageSize                  = 20
	invoiceMaxPageSize                      = 100
)

var invoiceStorageDir = "invoices"

type invoiceApplicationUserResponse struct {
	Id          int    `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
	Identity    string `json:"identity"`
}

type invoiceApplicationAdminResponse struct {
	model.InvoiceApplication
	User *invoiceApplicationUserResponse `json:"user,omitempty"`
}

func newInvoiceApplicationAdminResponse(application model.InvoiceApplication) invoiceApplicationAdminResponse {
	response := invoiceApplicationAdminResponse{InvoiceApplication: application}
	if application.User == nil {
		return response
	}
	response.User = &invoiceApplicationUserResponse{
		Id: application.User.Id, Username: application.User.Username,
		DisplayName: application.User.DisplayName, Email: application.User.Email,
		Identity: application.User.Identity,
	}
	return response
}

func ensureInvoiceWindow(settings model.InvoiceSettings, now time.Time) error {
	if !settings.Enabled {
		return errors.New("invoice center is disabled")
	}
	if now.Day() != settings.ApplicationDay {
		return fmt.Errorf("invoice applications are accepted on day %d of each month", settings.ApplicationDay)
	}
	return nil
}

func loadCurrentInvoiceSettings(c *gin.Context) (model.InvoiceSettings, bool) {
	settings, err := model.LoadInvoiceSettings()
	if err != nil {
		common.SysError("failed to load current invoice settings: " + err.Error())
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": "invoice settings are temporarily unavailable"})
		return model.InvoiceSettings{}, false
	}
	return settings, true
}

func invoicePageQuery(c *gin.Context) (int, int) {
	page, err := strconv.Atoi(c.DefaultQuery("p", "1"))
	if err != nil || page < 1 {
		page = 1
	}
	pageSize, err := strconv.Atoi(c.DefaultQuery("size", strconv.Itoa(invoiceDefaultPageSize)))
	if err != nil || pageSize < 1 || pageSize > invoiceMaxPageSize {
		pageSize = invoiceDefaultPageSize
	}
	maxInt := int(^uint(0) >> 1)
	if page > maxInt/pageSize {
		page = 1
	}
	return page, pageSize
}

func writeInvoiceModelError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, model.ErrInvoiceApplicationNotFound):
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "invoice application not found"})
	case errors.Is(err, model.ErrInvoiceApplicationState), errors.Is(err, model.ErrInvoicePDFRequired):
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": err.Error()})
	case errors.Is(err, model.ErrInvalidInvoiceFilter):
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
	case errors.Is(err, model.ErrInvoiceIdentityRequired):
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": err.Error()})
	default:
		common.SysError("invoice operation failed: " + err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "invoice operation failed"})
	}
}

func GetSelfInvoiceCenter(c *gin.Context) {
	settings, ok := loadCurrentInvoiceSettings(c)
	if !ok {
		return
	}
	if !settings.Enabled {
		common.ApiErrorMsg(c, "invoice center is disabled")
		return
	}
	userId := c.GetInt("id")
	page, pageSize := invoicePageQuery(c)
	identityEligible := true
	subscriptions, err := model.GetInvoiceEligibleSubscriptions(userId, settings)
	if errors.Is(err, model.ErrInvoiceIdentityRequired) {
		identityEligible = false
		subscriptions = []model.InvoiceEligibleSubscription{}
	} else if err != nil {
		writeInvoiceModelError(c, err)
		return
	}
	applications, applicationsTotal, err := model.ListUserInvoiceApplications(userId, (page-1)*pageSize, pageSize)
	if err != nil {
		writeInvoiceModelError(c, err)
		return
	}
	now := time.Now()
	currentMonth := now.Format("2006-01")
	activeApplications, err := model.CountActiveUserInvoiceApplicationsInMonth(userId, currentMonth)
	if err != nil {
		writeInvoiceModelError(c, err)
		return
	}
	remainingApplications := int64(settings.MonthlyLimit) - activeApplications
	if remainingApplications < 0 {
		remainingApplications = 0
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": gin.H{
		"application_day":        settings.ApplicationDay,
		"application_open":       now.Day() == settings.ApplicationDay,
		"identity_eligible":      identityEligible,
		"lookback_days":          settings.LookbackDays,
		"monthly_limit":          settings.MonthlyLimit,
		"remaining_applications": remainingApplications,
		"subscriptions":          subscriptions,
		"applications":           applications,
		"applications_total":     applicationsTotal,
		"page":                   page,
		"size":                   pageSize,
	}})
}

func CreateInvoiceApplication(c *gin.Context) {
	settings, ok := loadCurrentInvoiceSettings(c)
	if !ok {
		return
	}
	if err := ensureInvoiceWindow(settings, time.Now()); err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	// Check identity eligibility before validating form fields so unauthorized
	// callers cannot learn or bypass invoice requirements with malformed input.
	if _, eligibilityErr := model.GetInvoiceEligibleSubscriptions(c.GetInt("id"), settings); errors.Is(eligibilityErr, model.ErrInvoiceIdentityRequired) {
		writeInvoiceModelError(c, eligibilityErr)
		return
	}
	var request invoiceApplicationRequest
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, invoiceApplicationRequestMaxBytes)
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"success": false, "message": "invoice application is too large"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid invoice application"})
		return
	}
	if strings.TrimSpace(request.TaxpayerId) == "" {
		common.ApiErrorMsg(c, "taxpayer id is required")
		return
	}
	application, err := model.CreateInvoiceApplication(c.GetInt("id"), settings, model.InvoiceApplicationInput{
		InvoiceTitle: request.InvoiceTitle, TaxpayerId: request.TaxpayerId,
		BankName: request.BankName, Remark: request.Remark,
	}, request.SubscriptionIds, request.RedemptionIds)
	if err != nil {
		if model.IsInvoiceRequestError(err) {
			common.ApiErrorMsg(c, err.Error())
		} else {
			writeInvoiceModelError(c, err)
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "invoice application submitted", "data": application})
}

func ListInvoiceApplications(c *gin.Context) {
	page, pageSize := invoicePageQuery(c)
	userId := 0
	if value := strings.TrimSpace(c.Query("user_id")); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid invoice user id"})
			return
		}
		userId = parsed
	}
	applications, total, err := model.ListInvoiceApplications(model.InvoiceAdminFilter{
		Status: strings.TrimSpace(c.Query("status")), Keyword: c.Query("keyword"), UserId: userId,
	}, (page-1)*pageSize, pageSize)
	if err != nil {
		writeInvoiceModelError(c, err)
		return
	}
	responses := make([]invoiceApplicationAdminResponse, 0, len(applications))
	for i := range applications {
		responses = append(responses, newInvoiceApplicationAdminResponse(applications[i]))
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true, "message": "", "data": responses,
		"total": total, "page": page, "size": pageSize,
	})
}

func UpdateInvoiceSettings(c *gin.Context) {
	var request invoiceSettingsUpdateRequest
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, invoiceSettingsRequestMaxBytes)
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid invoice settings"})
		return
	}
	if request.InvoiceEnabled == nil || request.ApplicationDay == nil || request.LookbackDays == nil || request.MonthlyLimit == nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "all invoice settings are required"})
		return
	}
	if *request.ApplicationDay < 1 || *request.ApplicationDay > 28 || *request.LookbackDays < 1 || *request.LookbackDays > 3650 || *request.MonthlyLimit < 1 || *request.MonthlyLimit > 31 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invoice settings are outside the allowed range"})
		return
	}
	current := model.GetInvoiceSettings()
	systemRecharge := current.SystemRechargeEnabled
	redemptionRecharge := current.RedemptionRechargeEnabled
	systemSubscription := current.SystemSubscriptionEnabled
	redemptionSubscription := current.RedemptionSubscriptionEnabled
	if request.SystemRechargeEnabled != nil {
		systemRecharge = *request.SystemRechargeEnabled
	}
	if request.RedemptionRechargeEnabled != nil {
		redemptionRecharge = *request.RedemptionRechargeEnabled
	}
	if request.SystemSubscriptionEnabled != nil {
		systemSubscription = *request.SystemSubscriptionEnabled
	}
	if request.RedemptionSubscriptionEnabled != nil {
		redemptionSubscription = *request.RedemptionSubscriptionEnabled
	}
	if err := model.UpdateOptionsBulk(map[string]string{
		"InvoiceEnabled":                       strconv.FormatBool(*request.InvoiceEnabled),
		"InvoiceApplicationDay":                strconv.Itoa(*request.ApplicationDay),
		"InvoiceLookbackDays":                  strconv.Itoa(*request.LookbackDays),
		"InvoiceMonthlyLimit":                  strconv.Itoa(*request.MonthlyLimit),
		"InvoiceSystemRechargeEnabled":         strconv.FormatBool(systemRecharge),
		"InvoiceRedemptionRechargeEnabled":     strconv.FormatBool(redemptionRecharge),
		"InvoiceSystemSubscriptionEnabled":     strconv.FormatBool(systemSubscription),
		"InvoiceRedemptionSubscriptionEnabled": strconv.FormatBool(redemptionSubscription),
	}); err != nil {
		common.SysError("failed to update invoice settings: " + err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "failed to update invoice settings"})
		return
	}
	recordManageAudit(c, "invoice.settings_update", nil)
	c.JSON(http.StatusOK, gin.H{"success": true, "message": ""})
}

func invoiceApplicationPath(id int) string {
	return filepath.Join(invoiceStorageDir, fmt.Sprintf("invoice-%d-%s.pdf", id, uuid.NewString()))
}

func secureInvoicePath(storedPath string) (string, error) {
	if storedPath == "" || !strings.EqualFold(filepath.Ext(storedPath), ".pdf") {
		return "", errors.New("invalid invoice PDF path")
	}
	root, err := filepath.Abs(invoiceStorageDir)
	if err != nil {
		return "", err
	}
	path, err := filepath.Abs(storedPath)
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("invoice PDF path escapes storage directory")
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	resolvedPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	resolvedRelative, err := filepath.Rel(resolvedRoot, resolvedPath)
	if err != nil || resolvedRelative == ".." || strings.HasPrefix(resolvedRelative, ".."+string(filepath.Separator)) {
		return "", errors.New("invoice PDF symbolic link escapes storage directory")
	}
	if filepath.Clean(relative) != filepath.Clean(resolvedRelative) {
		return "", errors.New("invoice PDF symbolic links are not allowed")
	}
	return path, nil
}

func normalizeInvoicePDFName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || name != filepath.Base(name) || strings.ContainsAny(name, `/\\`) || !strings.EqualFold(filepath.Ext(name), ".pdf") {
		return "", errors.New("invalid invoice PDF file name")
	}
	if !utf8.ValidString(name) || utf8.RuneCountInString(name) > 255 {
		return "", errors.New("invalid invoice PDF file name")
	}
	for _, r := range name {
		if r == '<' || r == '>' || r == '"' || r == '\'' || r == ';' || unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || unicode.Is(unicode.Zl, r) || unicode.Is(unicode.Zp, r) {
			return "", errors.New("invalid invoice PDF file name")
		}
	}
	return name, nil
}

func validateInvoicePDF(data []byte) error {
	if len(data) < 16 || !bytes.HasPrefix(data, []byte("%PDF-")) {
		return errors.New("uploaded file is not a valid PDF")
	}
	footerStart := len(data) - 2048
	if footerStart < 0 {
		footerStart = 0
	}
	if !bytes.Contains(data[footerStart:], []byte("%%EOF")) {
		return errors.New("uploaded PDF is incomplete")
	}
	if containsUnsafePDFName(data) {
		return errors.New("uploaded PDF contains active or embedded content")
	}
	return nil
}

func containsUnsafePDFName(data []byte) bool {
	for index := 0; index < len(data); index++ {
		if data[index] != '/' {
			continue
		}
		var name [64]byte
		nameLength := 0
		cursor := index + 1
		for cursor < len(data) && !isPDFNameDelimiter(data[cursor]) {
			value := data[cursor]
			if value == '#' && cursor+2 < len(data) {
				high, highOK := pdfHexNibble(data[cursor+1])
				low, lowOK := pdfHexNibble(data[cursor+2])
				if highOK && lowOK {
					value = high<<4 | low
					cursor += 3
				} else {
					cursor++
				}
			} else {
				cursor++
			}
			if nameLength < len(name) {
				if value >= 'A' && value <= 'Z' {
					value += 'a' - 'A'
				}
				name[nameLength] = value
				nameLength++
			}
		}
		index = cursor - 1
		switch string(name[:nameLength]) {
		case "javascript", "js", "launch", "embeddedfile", "filespec",
			"richmedia", "openaction", "aa", "xfa", "submitform",
			"importdata", "gotoe", "gotor", "uri", "rendition", "movie",
			"sound", "3d", "encrypt":
			return true
		}
	}
	return false
}

func isPDFNameDelimiter(value byte) bool {
	switch value {
	case 0, '\t', '\n', '\f', '\r', ' ', '(', ')', '<', '>', '[', ']', '{', '}', '/', '%':
		return true
	default:
		return false
	}
}

func pdfHexNibble(value byte) (byte, bool) {
	switch {
	case value >= '0' && value <= '9':
		return value - '0', true
	case value >= 'a' && value <= 'f':
		return value - 'a' + 10, true
	case value >= 'A' && value <= 'F':
		return value - 'A' + 10, true
	default:
		return 0, false
	}
}

func storeInvoicePDF(id int, data []byte) (string, error) {
	if err := os.MkdirAll(invoiceStorageDir, 0700); err != nil {
		return "", err
	}
	if err := os.Chmod(invoiceStorageDir, 0700); err != nil {
		return "", err
	}
	path := invoiceApplicationPath(id)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return "", err
	}
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		_ = os.Remove(path)
		return "", errors.New("failed to store invoice PDF")
	}
	return path, nil
}

func removeInvoicePDF(storedPath string) {
	if storedPath == "" {
		return
	}
	path, err := secureInvoicePath(storedPath)
	if err != nil {
		common.SysError("refused to remove invalid invoice PDF path: " + err.Error())
		return
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		common.SysError("failed to remove invoice PDF: " + err.Error())
	}
}

func UploadInvoicePDF(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid invoice id"})
		return
	}
	application, err := model.GetInvoicePDF(id, 0)
	if err != nil {
		writeInvoiceModelError(c, err)
		return
	}
	if application.Status != model.InvoiceApplicationStatusPending {
		writeInvoiceModelError(c, model.ErrInvoiceApplicationState)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, invoicePDFRequestMaxBytes)
	if err := c.Request.ParseMultipartForm(1024 * 1024); err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"success": false, "message": "PDF upload must not exceed 20 MB"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid PDF upload"})
		return
	}
	form := c.Request.MultipartForm
	if form == nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid PDF upload"})
		return
	}
	defer form.RemoveAll()
	if len(form.Value) != 0 || len(form.File) != 1 || len(form.File["file"]) != 1 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "upload exactly one PDF file"})
		return
	}
	header := form.File["file"][0]
	if header == nil || header.Size <= 0 || header.Size > invoicePDFMaxBytes {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "PDF must be between 1 byte and 20 MB"})
		return
	}
	fileName, err := normalizeInvoicePDFName(header.Filename)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	mediaType, _, err := mime.ParseMediaType(header.Header.Get("Content-Type"))
	if err != nil || !strings.EqualFold(mediaType, "application/pdf") {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "PDF content type is invalid"})
		return
	}
	file, err := header.Open()
	if err != nil {
		writeInvoiceModelError(c, err)
		return
	}
	data, readErr := io.ReadAll(io.LimitReader(file, invoicePDFMaxBytes+1))
	closeErr := file.Close()
	if readErr != nil || closeErr != nil || len(data) == 0 || int64(len(data)) > invoicePDFMaxBytes {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "failed to read invoice PDF"})
		return
	}
	if err := validateInvoicePDF(data); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	path, err := storeInvoicePDF(id, data)
	if err != nil {
		writeInvoiceModelError(c, err)
		return
	}
	oldPath, err := model.ReplaceInvoicePDF(id, path, fileName)
	if err != nil {
		removeInvoicePDF(path)
		writeInvoiceModelError(c, err)
		return
	}
	removeInvoicePDF(oldPath)
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "invoice PDF uploaded"})
}

func DeleteInvoicePDF(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid invoice id"})
		return
	}
	oldPath, err := model.ClearInvoicePDF(id)
	if err != nil {
		writeInvoiceModelError(c, err)
		return
	}
	removeInvoicePDF(oldPath)
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "invoice PDF deleted"})
}

func CompleteInvoiceApplication(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid invoice id"})
		return
	}
	if err := model.CompleteInvoiceApplication(id); err != nil {
		writeInvoiceModelError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "invoice application completed"})
}

func RejectInvoiceApplication(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid invoice id"})
		return
	}
	var request invoiceRejectRequest
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, invoiceApplicationRequestMaxBytes)
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid invoice rejection"})
		return
	}
	oldPath, err := model.RejectInvoiceApplication(id, c.GetInt("id"), request.Reason)
	if err != nil {
		if model.IsInvoiceRequestError(err) {
			common.ApiErrorMsg(c, err.Error())
		} else {
			writeInvoiceModelError(c, err)
		}
		return
	}
	removeInvoicePDF(oldPath)
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "invoice application rejected"})
}

func serveInvoicePDF(c *gin.Context, application *model.InvoiceApplication) {
	path, err := secureInvoicePath(application.PDFPath)
	if err != nil {
		common.SysError("invalid stored invoice PDF path: " + err.Error())
		c.Status(http.StatusNotFound)
		return
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > invoicePDFMaxBytes {
		c.Status(http.StatusNotFound)
		return
	}
	fileName, err := normalizeInvoicePDFName(application.PDFName)
	if err != nil {
		fileName = fmt.Sprintf("invoice-%d.pdf", application.Id)
	}
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Security-Policy", "sandbox")
	c.Header("Cross-Origin-Resource-Policy", "same-origin")
	c.Header("Cache-Control", "private, no-store")
	c.Header("Content-Type", "application/pdf")
	c.FileAttachment(path, fileName)
}

func DownloadInvoicePDF(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		c.Status(http.StatusNotFound)
		return
	}
	application, err := model.GetInvoicePDF(id, c.GetInt("id"))
	if err != nil || application.Status != model.InvoiceApplicationStatusCompleted || application.PDFPath == "" {
		c.Status(http.StatusNotFound)
		return
	}
	serveInvoicePDF(c, application)
}

func DownloadAdminInvoicePDF(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		c.Status(http.StatusNotFound)
		return
	}
	application, err := model.GetInvoicePDF(id, 0)
	if err != nil || application.PDFPath == "" {
		c.Status(http.StatusNotFound)
		return
	}
	serveInvoicePDF(c, application)
}
