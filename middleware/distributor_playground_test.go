package middleware

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlaygroundRoutingPreservesValidationAndBodyReplay(t *testing.T) {
	previousCache := common.GetDiskCacheConfig()
	t.Cleanup(func() { common.SetDiskCacheConfig(previousCache) })
	for _, disk := range []bool{false, true} {
		t.Run(fmt.Sprintf("disk=%t", disk), func(t *testing.T) {
			common.SetDiskCacheConfig(common.DiskCacheConfig{Enabled: disk, ThresholdMB: 0, MaxSizeMB: 16, Path: t.TempDir()})
			for _, tc := range []struct {
				name, body, group, wantError, relayError string
			}{
				{name: "explicit group", body: `{"model":"fixture","group":"teaching","messages":[{"role":"user","content":"hello"}]}`, group: "teaching"},
				{name: "omitted group", body: `{"model":"fixture","messages":[{"role":"user","content":"hello"}]}`},
				{name: "null group", body: `{"model":"fixture","group":null,"messages":[{"role":"user","content":"hello"}]}`},
				{name: "escaped model key and nested model", body: `{"mo\u0064el":"fixture","group":"teaching","metadata":{"model":"nested"},"messages":[{"role":"user","content":"hello"}]}`, group: "teaching"},
				{name: "malformed JSON", body: `{"model":"fixture",`, wantError: "invalid JSON request body"},
				{name: "duplicate model", body: `{"model":"fixture","model":"other"}`, wantError: "model must be provided once"},
				{name: "escaped duplicate model takes precedence over bad group", body: `{"model":"fixture","mo\u0064el":"other","group":42}`, wantError: "model must be provided once"},
				{name: "bad model takes precedence over bad group", body: `{"model":42,"group":false}`, wantError: "field model must be a string"},
				{name: "bad group type", body: `{"model":"fixture","group":42}`, wantError: "field group must be a string"},
				{name: "overlong group", body: `{"model":"fixture","group":"` + strings.Repeat("a", model.MaxTokenGroupConfigLength+1) + `"}`, wantError: "field group is too long"},
				{name: "overlong model", body: `{"model":"` + strings.Repeat("a", model.MaxRelayModelNameLength+1) + `"}`, wantError: "field model is too long"},
				{name: "invalid chat field still reaches DTO validation", body: `{"model":"fixture","group":"teaching","messages":42}`, group: "teaching", relayError: "cannot unmarshal number"},
				{name: "missing messages still reaches relay validation", body: `{"model":"fixture"}`, relayError: "field messages is required"},
			} {
				t.Run(tc.name, func(t *testing.T) {
					c, _ := gin.CreateTestContext(httptest.NewRecorder())
					c.Request = httptest.NewRequest(http.MethodPost, "/pg/chat/completions", strings.NewReader(tc.body))
					c.Request.Header.Set("Content-Type", "application/json")
					t.Cleanup(func() { common.CleanupBodyStorage(c) })

					route, shouldSelect, err := getModelRequest(c)
					if tc.wantError != "" {
						require.EqualError(t, err, tc.wantError)
						assert.Nil(t, route)
						assert.False(t, shouldSelect)
						return
					}
					require.NoError(t, err)
					require.NotNil(t, route)
					assert.True(t, shouldSelect)
					assert.Equal(t, "fixture", route.Model)
					assert.Equal(t, tc.group, route.Group)
					assert.Equal(t, tc.group, common.GetContextKeyString(c, constant.ContextKeyTokenGroup))
					replayed, err := io.ReadAll(c.Request.Body)
					require.NoError(t, err)
					assert.Equal(t, tc.body, string(replayed))

					// Preserve the existing distributor DTO decode and later chat
					// validation after the routing read has consumed the same body.
					var playground dto.PlayGroundRequest
					require.NoError(t, common.UnmarshalBodyReusable(c, &playground))
					assert.Equal(t, route.Model, playground.Model)
					assert.Equal(t, route.Group, playground.Group)
					request, err := helper.GetAndValidateRequest(c, types.RelayFormatOpenAI)
					if tc.relayError != "" {
						require.ErrorContains(t, err, tc.relayError)
						return
					}
					require.NoError(t, err)
					chat, ok := request.(*dto.GeneralOpenAIRequest)
					require.True(t, ok)
					assert.Equal(t, route.Model, chat.Model)
					require.Len(t, chat.Messages, 1)
					assert.Equal(t, "hello", chat.Messages[0].StringContent())
					storage, err := common.GetBodyStorage(c)
					require.NoError(t, err)
					assert.Equal(t, disk, storage.IsDisk())
				})
			}
		})
	}
}

func TestPlaygroundFormRoutingPreservesGroupAndBodyReplay(t *testing.T) {
	const body = "model=fixture&group=teaching%2Fclass"
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/pg/chat/completions", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	t.Cleanup(func() { common.CleanupBodyStorage(c) })

	route, shouldSelect, err := getModelRequest(c)
	require.NoError(t, err)
	require.NotNil(t, route)
	assert.True(t, shouldSelect)
	assert.Equal(t, "fixture", route.Model)
	assert.Equal(t, "teaching/class", route.Group)
	assert.Equal(t, route.Group, common.GetContextKeyString(c, constant.ContextKeyTokenGroup))
	var playground dto.PlayGroundRequest
	require.NoError(t, common.UnmarshalBodyReusable(c, &playground))
	assert.Equal(t, route.Model, playground.Model)
	assert.Equal(t, route.Group, playground.Group)
	replayed, err := io.ReadAll(c.Request.Body)
	require.NoError(t, err)
	assert.Equal(t, body, string(replayed))
}
