package controller

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateInvoicePDFRejectsIncompleteAndActiveContent(t *testing.T) {
	validPDFs := [][]byte{
		[]byte("%PDF-1.4\n1 0 obj\n<< /Type /Catalog >>\nendobj\n%%EOF\n"),
		[]byte("%PDF-1.5\n1 0 obj\n<< /Type /ObjStm /Filter /FlateDecode >>\nstream\nopaque\nendstream\n%%EOF\n"),
	}
	for _, data := range validPDFs {
		require.NoError(t, validateInvoicePDF(data))
	}

	tests := []struct {
		name string
		data []byte
	}{
		{name: "wrong header", data: []byte("not-a-pdf-document%%EOF")},
		{name: "missing EOF", data: []byte("%PDF-1.4\n1 0 obj\n<<>>\nendobj")},
		{name: "javascript", data: []byte("%PDF-1.4\n/OpenAction<</JavaScript(alert)>>\n%%EOF")},
		{name: "escaped javascript name", data: []byte("%PDF-1.4\n/Open#41ction<</Java#53cript(alert)>>\n%%EOF")},
		{name: "additional action", data: []byte("%PDF-1.4\n/AA<</O 1 0 R>>\n%%EOF")},
		{name: "embedded file", data: []byte("%PDF-1.4\n/Type /EmbeddedFile\n%%EOF")},
		{name: "encrypted objects", data: []byte("%PDF-1.7\ntrailer <</Encrypt 4 0 R>>\n%%EOF")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Error(t, validateInvoicePDF(test.data))
		})
	}
}

func TestSecureInvoicePathAndDownloadHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	originalDir := invoiceStorageDir
	invoiceStorageDir = filepath.Join(t.TempDir(), "invoices")
	t.Cleanup(func() { invoiceStorageDir = originalDir })
	require.NoError(t, os.MkdirAll(invoiceStorageDir, 0700))

	path := filepath.Join(invoiceStorageDir, "invoice-1-test.pdf")
	data := []byte("%PDF-1.4\n1 0 obj\n<<>>\nendobj\n%%EOF\n")
	require.NoError(t, os.WriteFile(path, data, 0600))
	resolved, err := secureInvoicePath(path)
	require.NoError(t, err)
	assert.Equal(t, path, resolved)
	_, err = secureInvoicePath(filepath.Join(invoiceStorageDir, "..", "outside.pdf"))
	require.Error(t, err)

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/user/invoice/1/download", nil)
	serveInvoicePDF(context, &model.InvoiceApplication{
		Id: 1, PDFPath: path, PDFName: `unsafe"name.pdf`,
	})
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, data, recorder.Body.Bytes())
	assert.Equal(t, "nosniff", recorder.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "sandbox", recorder.Header().Get("Content-Security-Policy"))
	assert.Equal(t, "same-origin", recorder.Header().Get("Cross-Origin-Resource-Policy"))
	assert.Equal(t, "private, no-store", recorder.Header().Get("Cache-Control"))
	assert.Equal(t, "application/pdf", recorder.Header().Get("Content-Type"))
	assert.Contains(t, recorder.Header().Get("Content-Disposition"), "attachment")
	assert.NotContains(t, recorder.Header().Get("Content-Disposition"), "unsafe")
}

func TestServeInvoicePDFRejectsSymbolicLinks(t *testing.T) {
	gin.SetMode(gin.TestMode)
	root := t.TempDir()
	originalDir := invoiceStorageDir
	invoiceStorageDir = filepath.Join(root, "invoices")
	t.Cleanup(func() { invoiceStorageDir = originalDir })
	require.NoError(t, os.MkdirAll(invoiceStorageDir, 0700))

	outsidePath := filepath.Join(root, "outside.pdf")
	require.NoError(t, os.WriteFile(outsidePath, []byte("%PDF-1.4\n%%EOF\n"), 0600))
	linkedPath := filepath.Join(invoiceStorageDir, "invoice-1-linked.pdf")
	if err := os.Symlink(outsidePath, linkedPath); err != nil {
		t.Skipf("symbolic links are unavailable: %v", err)
	}

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/user/invoice/1/download", nil)
	serveInvoicePDF(context, &model.InvoiceApplication{
		Id: 1, PDFPath: linkedPath, PDFName: "invoice.pdf",
	})
	assert.Equal(t, http.StatusNotFound, context.Writer.Status())
	assert.Empty(t, recorder.Body.Bytes())
}
