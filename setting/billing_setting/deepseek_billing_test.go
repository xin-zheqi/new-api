package billing_setting_test

import (
	"testing"

	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// DeepSeek prices are peak/off-peak expressions in USD per 1M tokens; peak is
// twice off-peak. The same usage settles to the same quota from a Chat,
// Responses or Anthropic response, and the Search/Fetch routing names cost
// $0.75 per 1,000 requests with no token price. Which tier runs depends on the clock; both are pinned here.
func TestDeepSeekBuiltinBilling(t *testing.T) {
	settings := config.GlobalConfig.Get("billing_setting").(*billing_setting.BillingSetting)
	saved := *settings
	savedRatios, savedPrices := ratio_setting.ModelRatio2JSONString(), ratio_setting.ModelPrice2JSONString()
	t.Cleanup(func() {
		*settings = saved
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(savedRatios))
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(savedPrices))
	})
	*settings = billing_setting.BillingSetting{BillingMode: map[string]string{}, BillingExpr: map[string]string{}}
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{}`))
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{}`))

	// 1000 cache-miss + 2000 cache-hit input tokens and 500 output tokens.
	chat := &dto.Usage{PromptTokens: 3000, CompletionTokens: 500, PromptTokensDetails: dto.InputTokenDetails{CachedTokens: 2000}}
	responses := &dto.Usage{}
	service.ApplyResponsesUsage(responses, &dto.Usage{InputTokens: 3000, OutputTokens: 500, InputTokensDetails: &dto.InputTokenDetails{CachedTokens: 2000}})
	anthropic := &dto.Usage{PromptTokens: 1000, CompletionTokens: 500, PromptTokensDetails: dto.InputTokenDetails{CachedTokens: 2000}, UsageSemantic: "anthropic"}
	quota := func(expression string, usage *dto.Usage) billingexpr.TieredResult {
		params := service.BuildTieredTokenParams(usage, usage.UsageSemantic == "anthropic", billingexpr.UsedVars(expression))
		result, err := billingexpr.ComputeTieredQuota(&billingexpr.BillingSnapshot{
			ExprString: expression, ExprHash: billingexpr.ExprHashString(expression), GroupRatio: 1, QuotaPerUnit: 500000,
		}, params)
		require.NoError(t, err)
		return result
	}
	for model, want := range map[string]map[string]int{
		// flash: 1000*0.15 + 2000*0.003 + 500*0.6 = $0.000456 off-peak.
		"deepseek-flash":               {"off_peak": 228, "peak": 456},
		"deepseek-v4-flash":            {"off_peak": 228, "peak": 456},
		"deepseek-v4-flash-vision-exp": {"off_peak": 228, "peak": 456},
		// pro: 1000*0.66 + 2000*0.022 + 500*1.98 = $0.001694 off-peak.
		"deepseek-v4-pro": {"off_peak": 847, "peak": 1694},
	} {
		t.Run(model, func(t *testing.T) {
			assert.Equal(t, billing_setting.BillingModeTieredExpr, billing_setting.GetBillingMode(model))
			expression, ok := billing_setting.GetBillingExpr(model)
			require.True(t, ok)
			for name, usage := range map[string]*dto.Usage{"chat": chat, "responses": responses, "anthropic": anthropic} {
				result := quota(expression, usage)
				require.Contains(t, want, result.MatchedTier, name)
				assert.Equal(t, want[result.MatchedTier], result.ActualQuotaAfterGroup, name)
			}
		})
	}
	for _, model := range []string{"deepseek-search", "deepseek-fetch"} {
		expression, ok := billing_setting.GetBillingExpr(model)
		require.True(t, ok, model)
		result := quota(expression, &dto.Usage{})
		assert.Equal(t, 375, result.ActualQuotaAfterGroup, model)
	}
}
