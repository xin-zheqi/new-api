package service

import (
	"bytes"
	"io"
	"mime"
	"mime/multipart"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/contentaudit"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
)

// File bytes and multipart headers are never inspected/replaced. Only documented
// text fields are processed. Rebuilding keeps each part's original MIME headers.
func auditMultipartContent(c *gin.Context, rules []contentaudit.CompiledRule) *types.NewAPIError {
	_, parameters, err := mime.ParseMediaType(c.Request.Header.Get("Content-Type"))
	if err != nil {
		return auditError("invalid multipart content", http.StatusBadRequest)
	}
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return auditError("cannot read multipart content", http.StatusBadRequest)
	}
	// Keep the same explicit envelope cap as JSON: never silently bypass audit.
	if storage.Size() > contentaudit.MaxBodyBytes {
		return auditError(contentaudit.ErrLimit.Error(), http.StatusRequestEntityTooLarge)
	}
	reader, err := storage.NewReader()
	if err != nil {
		return auditError("cannot read multipart content", http.StatusBadRequest)
	}
	defer reader.Close()
	input := multipart.NewReader(reader, parameters["boundary"])
	var output bytes.Buffer
	writer := multipart.NewWriter(&output)
	fields := map[string]string{}
	parts := []struct {
		header map[string][]string
		body   []byte
		name   string
		text   bool
	}{}
	for {
		part, err := input.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return auditError("invalid multipart content", http.StatusBadRequest)
		}
		data, err := io.ReadAll(io.LimitReader(part, contentaudit.MaxBodyBytes+1))
		part.Close()
		if err != nil || len(data) > contentaudit.MaxBodyBytes {
			return auditError(contentaudit.ErrLimit.Error(), http.StatusRequestEntityTooLarge)
		}
		name := part.FormName()
		isText := part.FileName() == "" && (name == "prompt" || name == "input")
		if isText {
			if _, exists := fields[name]; exists {
				return auditError("duplicate multipart text fields are not supported by content audit", http.StatusBadRequest)
			}
			fields[name] = string(data)
		}
		parts = append(parts, struct {
			header map[string][]string
			body   []byte
			name   string
			text   bool
		}{part.Header, data, name, isText})
		if len(parts) > 128 {
			return auditError(contentaudit.ErrLimit.Error(), http.StatusRequestEntityTooLarge)
		}
	}
	body, err := common.Marshal(fields)
	if err != nil {
		return auditError("invalid multipart text", http.StatusBadRequest)
	}
	updated, result, err := contentaudit.InspectJSON(body, rules)
	if err != nil {
		return auditError(contentaudit.ErrLimit.Error(), http.StatusRequestEntityTooLarge)
	}
	modelName := ""
	for _, part := range parts {
		if part.name == "model" {
			modelName = string(part.body)
		}
	}
	if apiErr := recordContentAuditResult(c, result, modelName); apiErr != nil {
		return apiErr
	}
	if result.Action == "replace" {
		if err := common.Unmarshal(updated, &fields); err != nil {
			return auditError("invalid audited multipart text", http.StatusBadRequest)
		}
		for _, part := range parts {
			out, err := writer.CreatePart(part.header)
			if err != nil {
				return auditError("cannot prepare audited multipart request", http.StatusInternalServerError)
			}
			data := part.body
			if value, ok := fields[part.name]; ok && part.text {
				data = []byte(value)
			}
			if _, err := out.Write(data); err != nil {
				return auditError("cannot prepare audited multipart request", http.StatusInternalServerError)
			}
		}
		if err := writer.Close(); err != nil {
			return auditError("cannot prepare audited multipart request", http.StatusInternalServerError)
		}
		if output.Len() > contentaudit.MaxBodyBytes {
			return auditError(contentaudit.ErrLimit.Error(), http.StatusRequestEntityTooLarge)
		}
		if err := replaceAuditedBody(c, output.Bytes()); err != nil {
			return auditError("cannot prepare audited multipart request", http.StatusInternalServerError)
		}
		c.Request.Header.Set("Content-Type", writer.FormDataContentType())
	}
	c.Set("content_audit_completed", true)
	return nil
}
