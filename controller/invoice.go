package controller

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type invoiceApplicationRequest struct {
	InvoiceTitle    string `json:"invoice_title"`
	SubscriptionIds []int  `json:"subscription_ids"`
}

func invoiceApplicationDay() int {
	common.OptionMapRWMutex.RLock()
	value := common.OptionMap["InvoiceApplicationDay"]
	common.OptionMapRWMutex.RUnlock()
	day, err := strconv.Atoi(value)
	if err != nil || day < 1 || day > 28 {
		return 25
	}
	return day
}

func invoiceOptionInt(key string, fallback int) int {
	common.OptionMapRWMutex.RLock()
	value := common.OptionMap[key]
	common.OptionMapRWMutex.RUnlock()
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func invoiceEnabled() bool {
	common.OptionMapRWMutex.RLock()
	enabled := common.OptionMap["InvoiceEnabled"] != "false"
	common.OptionMapRWMutex.RUnlock()
	return enabled
}

func ensureInvoiceWindow() error {
	if !invoiceEnabled() {
		return errors.New("invoice center is disabled")
	}
	if time.Now().Day() != invoiceApplicationDay() {
		return fmt.Errorf("invoice applications are accepted on day %d of each month", invoiceApplicationDay())
	}
	return nil
}

func GetSelfInvoiceCenter(c *gin.Context) {
	if !invoiceEnabled() {
		common.ApiErrorMsg(c, "invoice center is disabled")
		return
	}
	user, err := model.GetUserById(c.GetInt("id"), false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !model.IsInvoiceEligibleIdentity(user.Identity) {
		common.ApiErrorMsg(c, "invoice center is only available for university or enterprise users")
		return
	}
	subscriptions, err := model.GetInvoiceEligibleSubscriptions(user.Id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	applications, err := model.ListUserInvoiceApplications(user.Id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": gin.H{
		"application_day": invoiceApplicationDay(),
		"lookback_days":   invoiceOptionInt("InvoiceLookbackDays", 90),
		"monthly_limit":   invoiceOptionInt("InvoiceMonthlyLimit", 1),
		"subscriptions":   subscriptions, "applications": applications,
	}})
}

func CreateInvoiceApplication(c *gin.Context) {
	if err := ensureInvoiceWindow(); err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	user, err := model.GetUserById(c.GetInt("id"), false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !model.IsInvoiceEligibleIdentity(user.Identity) {
		common.ApiErrorMsg(c, "invoice center is only available for university or enterprise users")
		return
	}
	var request invoiceApplicationRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiErrorMsg(c, "invalid invoice application")
		return
	}
	application, err := model.CreateInvoiceApplication(user.Id, request.InvoiceTitle, request.SubscriptionIds)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "invoice application submitted", "data": application})
}

func ListInvoiceApplications(c *gin.Context) {
	if !invoiceEnabled() {
		common.ApiErrorMsg(c, "invoice center is disabled")
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("p", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	applications, total, err := model.ListInvoiceApplications((page-1)*limit, limit)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": applications, "total": total})
}

func invoiceApplicationPath(id int) string {
	return filepath.Join("invoices", fmt.Sprintf("invoice-%d-%s.pdf", id, uuid.NewString()))
}

func UploadInvoicePDF(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "invalid invoice id")
		return
	}
	application, err := model.GetInvoiceApplication(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	header, err := c.FormFile("file")
	if err != nil || header.Size <= 0 || header.Size > 20*1024*1024 {
		common.ApiErrorMsg(c, "PDF must be between 1 byte and 20 MB")
		return
	}
	if !strings.EqualFold(filepath.Ext(header.Filename), ".pdf") {
		common.ApiErrorMsg(c, "only PDF files are supported")
		return
	}
	if err := os.MkdirAll("invoices", 0755); err != nil {
		common.ApiError(c, err)
		return
	}
	path := invoiceApplicationPath(id)
	if err := c.SaveUploadedFile(header, path); err != nil {
		common.ApiError(c, err)
		return
	}
	content, err := os.ReadFile(path)
	if err != nil || len(content) < 4 || string(content[:4]) != "%PDF" {
		_ = os.Remove(path)
		common.ApiErrorMsg(c, "uploaded file is not a valid PDF")
		return
	}
	if application.PDFPath != "" {
		_ = os.Remove(application.PDFPath)
	}
	if err := model.SaveInvoicePDF(id, path, filepath.Base(header.Filename)); err != nil {
		_ = os.Remove(path)
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "invoice PDF uploaded"})
}

func DeleteInvoicePDF(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorMsg(c, "invalid invoice id")
		return
	}
	application, err := model.GetInvoiceApplication(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if application.PDFPath != "" {
		_ = os.Remove(application.PDFPath)
	}
	if err := model.ClearInvoicePDF(id); err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "invoice PDF deleted"})
}

func CompleteInvoiceApplication(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorMsg(c, "invalid invoice id")
		return
	}
	application, err := model.GetInvoiceApplication(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if application.PDFPath == "" {
		common.ApiErrorMsg(c, "upload an invoice PDF before completing the application")
		return
	}
	if err := model.CompleteInvoiceApplication(id); err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "invoice application completed"})
}

func DownloadInvoicePDF(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorMsg(c, "invalid invoice id")
		return
	}
	application, err := model.GetInvoiceApplication(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if application.UserId != c.GetInt("id") || application.Status != model.InvoiceApplicationStatusCompleted || application.PDFPath == "" {
		c.Status(http.StatusNotFound)
		return
	}
	if _, err := os.Stat(application.PDFPath); errors.Is(err, os.ErrNotExist) {
		c.Status(http.StatusNotFound)
		return
	}
	c.FileAttachment(application.PDFPath, application.PDFName)
}
