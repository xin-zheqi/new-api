package common

import (
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type failedUploadBody struct {
	closed bool
}

func (body *failedUploadBody) Read([]byte) (int, error) {
	if body.closed {
		return 0, http.ErrBodyReadAfterClose
	}
	return 0, io.ErrUnexpectedEOF
}

func (body *failedUploadBody) Close() error {
	body.closed = true
	return nil
}

func TestGetRequestBodyPreservesReadFailure(t *testing.T) {
	previous := constant.MaxRequestBodyMB
	constant.MaxRequestBodyMB = 1
	t.Cleanup(func() { constant.MaxRequestBodyMB = previous })
	previousCache := GetDiskCacheConfig()
	t.Cleanup(func() { SetDiskCacheConfig(previousCache) })
	for _, disk := range []bool{false, true} {
		cachePath := t.TempDir()
		SetDiskCacheConfig(DiskCacheConfig{Enabled: disk, ThresholdMB: 0, MaxSizeMB: 16, Path: cachePath})
		for _, tc := range []struct {
			name string
			body io.ReadCloser
			want error
		}{
			{name: "interrupted upload", body: &failedUploadBody{}, want: io.ErrUnexpectedEOF},
			{name: "oversized upload", body: io.NopCloser(strings.NewReader(strings.Repeat("x", (1<<20)+1))), want: ErrRequestBodyTooLarge},
		} {
			t.Run(fmt.Sprintf("disk=%t/%s", disk, tc.name), func(t *testing.T) {
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", tc.body)
				c.Request.ContentLength = 2 << 20
				t.Cleanup(func() { CleanupBodyStorage(c) })
				_, firstErr := GetRequestBody(c)
				require.ErrorIs(t, firstErr, tc.want)
				_, nextErr := GetBodyStorage(c)
				require.ErrorIs(t, nextErr, tc.want)
				assert.Equal(t, firstErr.Error(), nextErr.Error())
				assert.NotErrorIs(t, nextErr, http.ErrBodyReadAfterClose)
				err := filepath.WalkDir(cachePath, func(path string, entry fs.DirEntry, err error) error {
					if err != nil {
						return err
					}
					assert.True(t, entry.IsDir(), "failed upload left a cache file: %s", path)
					return nil
				})
				require.NoError(t, err)
			})
		}
	}
}

func TestGetRequestBodyCanReplaySuccessfulUpload(t *testing.T) {
	previousCache := GetDiskCacheConfig()
	t.Cleanup(func() { SetDiskCacheConfig(previousCache) })
	for _, disk := range []bool{false, true} {
		t.Run(fmt.Sprintf("disk=%t", disk), func(t *testing.T) {
			SetDiskCacheConfig(DiskCacheConfig{Enabled: disk, ThresholdMB: 0, MaxSizeMB: 16, Path: t.TempDir()})
			const payload = `{"model":"fixture","stream":true}`
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(payload))
			c.Request.Header.Set("Content-Type", "application/json")
			t.Cleanup(func() { CleanupBodyStorage(c) })
			for range 2 {
				var decoded struct {
					Model  string `json:"model"`
					Stream bool   `json:"stream"`
				}
				require.NoError(t, UnmarshalBodyReusable(c, &decoded))
				assert.Equal(t, "fixture", decoded.Model)
				assert.True(t, decoded.Stream)
				replayed, err := io.ReadAll(c.Request.Body)
				require.NoError(t, err)
				assert.Equal(t, payload, string(replayed))
			}
			storage, err := GetBodyStorage(c)
			require.NoError(t, err)
			assert.Equal(t, disk, storage.IsDisk(), "both memory and disk replay paths must be exercised")
		})
	}
}
