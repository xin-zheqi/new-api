package controller

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSub2APIBalanceDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := channelBalanceTestDB(t, dialect)
			for _, tc := range []struct {
				name    string
				body    string
				status  int
				balance float64
				err     string
			}{
				{name: "remaining takes precedence", body: `{"balance":100,"remaining":12.5,"isValid":true}`, balance: 12.5},
				{name: "explicit zero remaining", body: `{"balance":100,"remaining":0}`, balance: 0},
				{name: "wallet fallback", body: `{"balance":42.5}`, balance: 42.5},
				{name: "null remaining falls back", body: `{"balance":12.5,"remaining":null}`, balance: 12.5},
				{name: "zero wallet", body: `{"balance":0}`, balance: 0},
				{name: "negative wallet", body: `{"balance":-1}`, balance: -1},
				{name: "negative remaining without subscription", body: `{"remaining":-2}`, balance: -2},
				{name: "daily subscription", body: `{"remaining":8,"subscription":{"daily_limit_usd":10}}`, balance: 8},
				{name: "weekly subscription", body: `{"remaining":8,"subscription":{"weekly_limit_usd":10}}`, balance: 8},
				{name: "monthly subscription", body: `{"remaining":8,"subscription":{"monthly_limit_usd":10}}`, balance: 8},
				{name: "explicit subscription limit is finite", body: `{"remaining":-1,"subscription":{"daily_limit_usd":0}}`, balance: -1},
				{name: "unlimited subscription", body: `{"remaining":-1,"subscription":{"daily_limit_usd":null,"weekly_limit_usd":null,"monthly_limit_usd":null}}`, err: "unlimited subscription"},
				{name: "unlimited subscription with wallet", body: `{"balance":100,"remaining":-1,"subscription":{}}`, err: "unlimited subscription"},
				{name: "invalid key", body: `{"balance":100,"isValid":false}`, err: "isValid=false"},
				{name: "missing balance", body: `{}`, err: "missing remaining and balance"},
				{name: "null balance", body: `{"balance":null,"remaining":null}`, err: "missing remaining and balance"},
				{name: "null response", body: `null`, err: "missing remaining and balance"},
				{name: "invalid JSON", body: `{`, err: "invalid usage response"},
				{name: "wrong balance type", body: `{"balance":"sk-sub2api-fixture"}`, err: "invalid usage response"},
				{name: "overflowing balance", body: `{"balance":1e999}`, err: "invalid usage response"},
				{name: "wrong validity type", body: `{"balance":12,"isValid":"true"}`, err: "invalid usage response"},
				{name: "HTTP rejection", body: `{"balance":12}`, status: http.StatusUnauthorized, err: "HTTP 401"},
				{name: "oversized response", body: strings.Repeat(" ", 300<<10), err: "response is too large"},
			} {
				t.Run(tc.name, func(t *testing.T) {
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						assert.Equal(t, http.MethodGet, r.Method)
						assert.Equal(t, "/gateway/v1/usage", r.URL.Path)
						assert.Equal(t, "Bearer sk-sub2api-fixture", r.Header.Get("Authorization"))
						if tc.status != 0 {
							w.WriteHeader(tc.status)
						}
						fmt.Fprint(w, tc.body)
					}))
					t.Cleanup(upstream.Close)
					channel := model.Channel{
						Type: constant.ChannelTypeSub2API, Name: "keep name", Key: " sk-sub2api-fixture ",
						BaseURL: common.GetPointer(upstream.URL + "/gateway/"), Status: common.ChannelStatusEnabled,
						Balance: 19, BalanceUpdatedTime: 123, UsedQuota: 77,
					}
					require.NoError(t, db.Create(&channel).Error)
					t.Cleanup(func() { assert.NoError(t, db.Delete(&channel).Error) })
					result, err := updateChannelBalance(&channel)
					var saved model.Channel
					require.NoError(t, db.First(&saved, channel.Id).Error)
					assert.Equal(t, channel.Key, saved.Key)
					assert.Equal(t, "keep name", saved.Name)
					assert.Equal(t, int64(77), saved.UsedQuota)
					assert.Equal(t, common.ChannelStatusEnabled, saved.Status)
					assert.Equal(t, saved.Balance, channel.Balance)
					assert.Equal(t, saved.BalanceUpdatedTime, channel.BalanceUpdatedTime)
					if tc.err != "" {
						require.ErrorContains(t, err, tc.err)
						assert.NotContains(t, err.Error(), "sk-sub2api-fixture")
						assert.Equal(t, float64(19), saved.Balance)
						assert.Equal(t, int64(123), saved.BalanceUpdatedTime)
						return
					}
					require.NoError(t, err)
					assert.Equal(t, tc.balance, result.Balance)
					assert.Empty(t, result.RawResponse)
					assert.Equal(t, tc.balance, saved.Balance)
					assert.Greater(t, saved.BalanceUpdatedTime, int64(123))
				})
			}

			t.Run("API and bulk preserve unlimited failed disabled and multi-key channels", func(t *testing.T) {
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch r.Header.Get("Authorization") {
					case "Bearer good":
						fmt.Fprint(w, `{"remaining":12.5}`)
					case "Bearer unlimited":
						fmt.Fprint(w, `{"remaining":-1,"subscription":{}}`)
					case "Bearer malformed":
						fmt.Fprint(w, `{}`)
					default:
						t.Errorf("disabled/multi-key channel was queried")
					}
				}))
				t.Cleanup(upstream.Close)
				channels := []model.Channel{
					{Key: "good", Status: common.ChannelStatusEnabled},
					{Key: "unlimited", Status: common.ChannelStatusEnabled},
					{Key: "malformed", Status: common.ChannelStatusEnabled},
					{Key: "disabled", Status: common.ChannelStatusManuallyDisabled},
					{Key: "multi", Status: common.ChannelStatusEnabled, ChannelInfo: model.ChannelInfo{IsMultiKey: true}},
				}
				for i := range channels {
					channels[i].Type, channels[i].BaseURL = constant.ChannelTypeSub2API, &upstream.URL
					channels[i].Balance, channels[i].BalanceUpdatedTime = 19, 123
					require.NoError(t, db.Create(&channels[i]).Error)
				}
				router := gin.New()
				router.GET("/api/channel/update_balance/:id", UpdateChannelBalance)
				router.GET("/api/channel/update_balance", UpdateAllChannelsBalance)
				for i, channel := range channels {
					if channel.Key == "disabled" {
						continue // Manual queries of disabled channels remain allowed.
					}
					recorder := httptest.NewRecorder()
					router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/channel/update_balance/%d", channel.Id), nil))
					assert.Equal(t, http.StatusOK, recorder.Code)
					var body map[string]any
					require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &body))
					assert.Equal(t, i == 0, body["success"])
					if i == 0 {
						assert.Equal(t, 12.5, body["balance"])
					} else {
						assert.NotEmpty(t, body["message"])
						assert.NotContains(t, body, "balance")
					}
				}
				previousInterval := common.RequestInterval
				common.RequestInterval = 0
				t.Cleanup(func() { common.RequestInterval = previousInterval })
				recorder := httptest.NewRecorder()
				router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/channel/update_balance", nil))
				assert.JSONEq(t, `{"success":true,"message":""}`, recorder.Body.String())
				for i, channel := range channels {
					var saved model.Channel
					require.NoError(t, db.First(&saved, channel.Id).Error)
					assert.Equal(t, channel.Status, saved.Status)
					if i == 0 {
						assert.Equal(t, 12.5, saved.Balance)
					} else {
						assert.Equal(t, float64(19), saved.Balance)
						assert.Equal(t, int64(123), saved.BalanceUpdatedTime)
					}
				}
			})

			t.Run("proxy and database write failure", func(t *testing.T) {
				proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					assert.Equal(t, "sub2api.invalid", r.URL.Host)
					assert.Equal(t, "/v1/usage", r.URL.Path)
					assert.Equal(t, "Bearer fixture", r.Header.Get("Authorization"))
					fmt.Fprint(w, `{"remaining":12.5}`)
				}))
				t.Cleanup(proxy.Close)
				t.Cleanup(func() { service.InvalidateProxyClient(proxy.URL) })
				settings, err := common.Marshal(map[string]string{"proxy": proxy.URL})
				require.NoError(t, err)
				channel := model.Channel{Type: constant.ChannelTypeSub2API, Key: "fixture", BaseURL: common.GetPointer("http://sub2api.invalid"), Setting: common.GetPointer(string(settings)), Balance: 19, BalanceUpdatedTime: 123}
				require.NoError(t, db.Create(&channel).Error)
				writeError := errors.New("balance write rejected")
				require.NoError(t, db.Callback().Update().Before("gorm:update").Register("reject_sub2api_balance_update", func(tx *gorm.DB) { tx.AddError(writeError) }))
				t.Cleanup(func() { assert.NoError(t, db.Callback().Update().Remove("reject_sub2api_balance_update")) })
				_, err = updateChannelBalance(&channel)
				require.ErrorIs(t, err, writeError)
				assert.Equal(t, float64(19), channel.Balance)
				assert.Equal(t, int64(123), channel.BalanceUpdatedTime)
				var saved model.Channel
				require.NoError(t, db.First(&saved, channel.Id).Error)
				assert.Equal(t, channel.Balance, saved.Balance)
				assert.Equal(t, channel.BalanceUpdatedTime, saved.BalanceUpdatedTime)
			})
		})
	}
}

func TestSub2APIBalanceRejectsInvalidConfiguration(t *testing.T) {
	for _, baseURL := range []string{"", "sub2api.invalid", "ftp://sub2api.invalid", "http://user:password@sub2api.invalid", "http://sub2api.invalid?key=secret", "http://sub2api.invalid#fragment"} {
		t.Run(baseURL, func(t *testing.T) {
			_, err := updateChannelSub2APIBalance(&model.Channel{BaseURL: &baseURL, Key: "fixture"})
			require.ErrorContains(t, err, "valid HTTP(S) base URL")
		})
	}
	_, err := updateChannelSub2APIBalance(&model.Channel{BaseURL: common.GetPointer("https://sub2api.invalid"), Key: " "})
	require.ErrorContains(t, err, "missing an API key")
}
