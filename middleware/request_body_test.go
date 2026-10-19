package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	appI18n "github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestBodyFailuresReportTheirCause(t *testing.T) {
	require.NoError(t, appI18n.Init())
	_, err := jsplugin.DefaultRegistry.Register(genericTaskPluginSource, jsplugin.Options{})
	require.NoError(t, err)
	t.Cleanup(func() { jsplugin.DefaultRegistry.Unregister("generic-entry-test") })
	routePlugin := compileTaskRoutePlugin(t, genericTaskPluginSource)
	previous := constant.MaxRequestBodyMB
	constant.MaxRequestBodyMB = 1
	t.Cleanup(func() { constant.MaxRequestBodyMB = previous })

	reached := func(c *gin.Context) { c.Status(http.StatusNoContent) }
	router := gin.New()
	router.Use(func(c *gin.Context) {
		defer common.CleanupBodyStorage(c)
		c.Next()
	})
	router.POST("/v1/responses", PinTaskPluginEndpoint(), PrepareTaskPluginEndpoint(), Distribute(), reached)
	router.POST("/v1/video/generations", PinTaskPluginEndpoint(), PrepareTaskPluginEndpoint(), Distribute(), reached)
	router.POST("/v1/chat/completions", Distribute(), reached)
	router.POST("/mj/submit/imagine", Distribute(), reached)
	router.POST("/v1/tasks/:key", PrepareTaskPluginSubmit(), reached)
	router.POST("/vendor/jobs", func(c *gin.Context) {
		c.Set(jsplugin.ContextKeyPinnedRoute, jsplugin.PinnedRoute{Plugin: routePlugin})
	}, PrepareTaskPluginRoute(), reached)
	oversized := `{"model":"fixture","prompt":"` + strings.Repeat("a", 1<<20) + `"}`
	for _, path := range []string{
		"/v1/responses", "/v1/video/generations", "/v1/chat/completions",
		"/mj/submit/imagine", "/v1/tasks/generic-entry-test", "/vendor/jobs",
	} {
		t.Run("oversized "+path, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(oversized))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Accept-Language", "en")
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			assert.Equal(t, http.StatusRequestEntityTooLarge, recorder.Code)
			assert.Contains(t, recorder.Body.String(), "request body exceeds 1 MB")
			assert.NotContains(t, recorder.Body.String(), "Invalid request: Invalid request:")
		})
	}
	for _, tc := range []struct {
		name    string
		path    string
		body    io.Reader
		message string
	}{
		{name: "interrupted shared protocol upload", path: "/v1/responses", body: iotest.ErrReader(io.ErrUnexpectedEOF), message: "unexpected EOF"},
		{name: "invalid shared protocol JSON", path: "/v1/responses", body: strings.NewReader(`{"model":`), message: "invalid JSON request body"},
		{name: "invalid relay JSON", path: "/v1/chat/completions", body: strings.NewReader(`{"model":`), message: "invalid JSON request body"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, tc.path, tc.body)
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Accept-Language", "en")
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			assert.Equal(t, http.StatusBadRequest, recorder.Code)
			assert.Contains(t, recorder.Body.String(), tc.message)
			assert.Equal(t, 1, strings.Count(recorder.Body.String(), "Invalid request:"))
		})
	}
}

func TestAudioValidationPreservesRequestBodyFailureAfterModelFallback(t *testing.T) {
	previous := constant.MaxRequestBodyMB
	constant.MaxRequestBodyMB = 1
	t.Cleanup(func() { constant.MaxRequestBodyMB = previous })
	for _, path := range []string{"/v1/audio/transcriptions", "/v1/audio/translations"} {
		for _, tc := range []struct {
			name string
			body io.Reader
			want error
		}{
			{name: "interrupted", body: iotest.ErrReader(io.ErrUnexpectedEOF), want: io.ErrUnexpectedEOF},
			{name: "oversized", body: strings.NewReader(strings.Repeat("x", (1<<20)+1)), want: common.ErrRequestBodyTooLarge},
		} {
			t.Run(path+"/"+tc.name, func(t *testing.T) {
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, path, tc.body)
				c.Request.Header.Set("Content-Type", "multipart/form-data; boundary=fixture")
				t.Cleanup(func() { common.CleanupBodyStorage(c) })
				// Audio routing deliberately defaults the model when the initial
				// read fails. The later relay validator must still see that cause.
				request, _, err := getModelRequest(c)
				require.NoError(t, err)
				assert.Equal(t, "whisper-1", request.Model)
				_, err = helper.GetAndValidateRequest(c, types.RelayFormatOpenAIAudio)
				require.ErrorIs(t, err, tc.want)
				assert.NotErrorIs(t, err, http.ErrBodyReadAfterClose)
			})
		}
	}
}
