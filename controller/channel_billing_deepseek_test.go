package controller

import (
	"errors"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"io"
	"math"
	"net/http"
	"strings"
	"testing"
)

func TestGetDeepSeekBalanceUSD(t *testing.T) {
	tests := []struct {
		name            string
		responseJSON    string
		usdExchangeRate float64
		want            float64
		wantErrContains string
	}{
		{
			name:            "prefers USD when USD precedes CNY",
			responseJSON:    `{"balance_infos":[{"currency":"USD","total_balance":"12.50"},{"currency":"CNY","total_balance":"73.00"}]}`,
			usdExchangeRate: 7.3,
			want:            12.5,
		},
		{
			name:            "prefers USD when CNY precedes USD",
			responseJSON:    `{"balance_infos":[{"currency":"CNY","total_balance":"73.00"},{"currency":"USD","total_balance":"12.50"}]}`,
			usdExchangeRate: 7.3,
			want:            12.5,
		},
		{
			name:            "converts CNY when USD is absent",
			responseJSON:    `{"balance_infos":[{"currency":"CNY","total_balance":"73.00"}]}`,
			usdExchangeRate: 7.3,
			want:            10,
		},
		{
			name:            "converts CNY when USD balance is zero",
			responseJSON:    `{"balance_infos":[{"currency":"USD","total_balance":"0.00"},{"currency":"CNY","total_balance":"73.00"}]}`,
			usdExchangeRate: 7.3,
			want:            10,
		},
		{
			name:            "converts CNY when zero USD follows CNY",
			responseJSON:    `{"balance_infos":[{"currency":"CNY","total_balance":"73.00"},{"currency":"USD","total_balance":"0.00"}]}`,
			usdExchangeRate: 7.3,
			want:            10,
		},
		{
			name:            "returns zero when USD balance is zero and CNY is absent",
			responseJSON:    `{"balance_infos":[{"currency":"USD","total_balance":"0.00"}]}`,
			usdExchangeRate: 7.3,
			want:            0,
		},
		{
			name:            "returns error when USD and CNY are absent",
			responseJSON:    `{"balance_infos":[{"currency":"EUR","total_balance":"10.00"}]}`,
			usdExchangeRate: 7.3,
			wantErrContains: "currency USD or CNY not found",
		},
		{
			name:            "returns USD parse error instead of falling back to CNY",
			responseJSON:    `{"balance_infos":[{"currency":"USD","total_balance":"invalid"},{"currency":"CNY","total_balance":"73.00"}]}`,
			usdExchangeRate: 7.3,
			wantErrContains: "invalid syntax",
		},
		{
			name:            "rejects NaN USD balance",
			responseJSON:    `{"balance_infos":[{"currency":"USD","total_balance":"NaN"}]}`,
			usdExchangeRate: 7.3,
			wantErrContains: "USD balance must be finite",
		},
		{
			name:            "rejects negative USD balance",
			responseJSON:    `{"balance_infos":[{"currency":"USD","total_balance":"-1.00"}]}`,
			usdExchangeRate: 7.3,
			wantErrContains: "USD balance must be non-negative",
		},
		{
			name:            "rejects positive infinity CNY balance",
			responseJSON:    `{"balance_infos":[{"currency":"CNY","total_balance":"+Inf"}]}`,
			usdExchangeRate: 7.3,
			wantErrContains: "CNY balance must be finite",
		},
		{
			name:            "rejects negative CNY balance",
			responseJSON:    `{"balance_infos":[{"currency":"CNY","total_balance":"-7.30"}]}`,
			usdExchangeRate: 7.3,
			wantErrContains: "CNY balance must be non-negative",
		},
		{
			name:            "returns error for non-positive CNY exchange rate",
			responseJSON:    `{"balance_infos":[{"currency":"CNY","total_balance":"73.00"}]}`,
			usdExchangeRate: 0,
			wantErrContains: "USD exchange rate must be greater than zero",
		},
		{
			name:            "rejects NaN CNY exchange rate",
			responseJSON:    `{"balance_infos":[{"currency":"CNY","total_balance":"73.00"}]}`,
			usdExchangeRate: math.NaN(),
			wantErrContains: "USD exchange rate must be finite",
		},
		{
			name:            "rejects positive infinity CNY exchange rate",
			responseJSON:    `{"balance_infos":[{"currency":"CNY","total_balance":"73.00"}]}`,
			usdExchangeRate: math.Inf(1),
			wantErrContains: "USD exchange rate must be finite",
		},
		{
			name:            "rejects CNY conversion overflow",
			responseJSON:    `{"balance_infos":[{"currency":"CNY","total_balance":"1.7976931348623157e+308"}]}`,
			usdExchangeRate: math.SmallestNonzeroFloat64,
			wantErrContains: "converted USD balance must be finite",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var response DeepSeekUsageResponse
			require.NoError(t, common.Unmarshal([]byte(test.responseJSON), &response))

			balance, err := getDeepSeekBalanceUSD(response, test.usdExchangeRate)
			if test.wantErrContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), test.wantErrContains)
				return
			}

			require.NoError(t, err)
			assert.InDelta(t, test.want, balance, 1e-12)
		})
	}
}

// Mock only the provider response; the matrix uses real database connections.
type deepSeekBalanceTransport struct {
	t      *testing.T
	body   string
	status int
}

func (fixture deepSeekBalanceTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	assert.Equal(fixture.t, "https://api.deepseek.com/user/balance", r.URL.String())
	assert.Equal(fixture.t, http.MethodGet, r.Method)
	assert.Equal(fixture.t, "Bearer fixture-only", r.Header.Get("Authorization"))
	return &http.Response{StatusCode: fixture.status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(fixture.body)), Request: r}, nil
}

func TestDeepSeekBalanceDatabaseMatrix(t *testing.T) {
	if service.GetHttpClient() == nil {
		service.InitHttpClient()
	}
	client := service.GetHttpClient()
	originalTransport, originalRate := client.Transport, operation_setting.USDExchangeRate
	t.Cleanup(func() {
		client.Transport = originalTransport
		operation_setting.USDExchangeRate = originalRate
	})
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := channelBalanceTestDB(t, dialect)
			for _, tc := range []struct {
				name, body, wantError string
				want, rate            float64
				status                int
				writeFailure          bool
			}{
				{name: "USD preferred", body: `{"balance_infos":[{"currency":"CNY","total_balance":"73"},{"currency":"USD","total_balance":"12.5"}]}`, rate: 7.3, want: 12.5},
				{name: "CNY converted", body: `{"balance_infos":[{"currency":"CNY","total_balance":"73"}]}`, rate: 7.3, want: 10},
				{name: "zero USD falls back", body: `{"balance_infos":[{"currency":"USD","total_balance":"0"},{"currency":"CNY","total_balance":"73"}]}`, rate: 7.3, want: 10},
				{name: "actual zero persists", body: `{"balance_infos":[{"currency":"USD","total_balance":"0"}]}`, rate: 7.3, want: 0},
				{name: "invalid rate preserves balance", body: `{"balance_infos":[{"currency":"CNY","total_balance":"73"}]}`, rate: 0, wantError: "exchange rate must be greater than zero"},
				{name: "missing balance preserves balance", body: `{}`, rate: 7.3, wantError: "currency USD or CNY not found"},
				{name: "nonfinite balance preserves balance", body: `{"balance_infos":[{"currency":"USD","total_balance":"NaN"}]}`, rate: 7.3, wantError: "USD balance must be finite"},
				{name: "rejected query preserves balance", body: `{}`, status: http.StatusUnauthorized, rate: 7.3, wantError: "status code: 401"},
				{name: "failed write preserves cached balance", body: `{"balance_infos":[{"currency":"CNY","total_balance":"73"}]}`, rate: 7.3, writeFailure: true, wantError: "DeepSeek balance write rejected"},
			} {
				t.Run(tc.name, func(t *testing.T) {
					operation_setting.USDExchangeRate = tc.rate
					status := tc.status
					if status == 0 {
						status = http.StatusOK
					}
					client.Transport = deepSeekBalanceTransport{t: t, body: tc.body, status: status}
					channel := model.Channel{Type: constant.ChannelTypeDeepSeek, Key: "fixture-only", Name: "keep name", Status: common.ChannelStatusEnabled, Balance: 73, BalanceUpdatedTime: 123, UsedQuota: 77}
					require.NoError(t, db.Create(&channel).Error)
					if tc.writeFailure {
						require.NoError(t, db.Callback().Update().Before("gorm:update").Register("reject_deepseek_balance_update", func(tx *gorm.DB) { tx.AddError(errors.New("DeepSeek balance write rejected")) }))
						t.Cleanup(func() { assert.NoError(t, db.Callback().Update().Remove("reject_deepseek_balance_update")) })
					}
					result, err := updateChannelBalance(&channel)
					var saved model.Channel
					require.NoError(t, db.First(&saved, channel.Id).Error)
					assert.Equal(t, "keep name", saved.Name)
					assert.Equal(t, "fixture-only", saved.Key)
					assert.Equal(t, common.ChannelStatusEnabled, saved.Status)
					assert.Equal(t, int64(77), saved.UsedQuota)
					if tc.wantError != "" {
						require.ErrorContains(t, err, tc.wantError)
						assert.Equal(t, float64(73), saved.Balance)
						assert.Equal(t, int64(123), saved.BalanceUpdatedTime)
					} else {
						require.NoError(t, err)
						assert.InDelta(t, tc.want, result.Balance, 1e-12)
						assert.InDelta(t, tc.want, saved.Balance, 1e-12)
						assert.Greater(t, saved.BalanceUpdatedTime, int64(123))
					}
					assert.Equal(t, saved.Balance, channel.Balance)
					assert.Equal(t, saved.BalanceUpdatedTime, channel.BalanceUpdatedTime)
				})
			}
		})
	}
}
