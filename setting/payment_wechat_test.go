package setting

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEffectiveWechatUnitPricePrefersConfiguredValueAndFallsBackSafely(t *testing.T) {
	original := WechatUnitPrice
	t.Cleanup(func() { WechatUnitPrice = original })

	WechatUnitPrice = 8.2
	assert.Equal(t, 8.2, EffectiveWechatUnitPrice(6.9))

	WechatUnitPrice = 0
	assert.Equal(t, 6.9, EffectiveWechatUnitPrice(6.9))

	WechatUnitPrice = math.NaN()
	assert.Equal(t, DefaultWechatUnitPrice, EffectiveWechatUnitPrice(math.Inf(1)))

	WechatUnitPrice = MaxWechatUnitPrice + 1
	assert.Equal(t, 6.9, EffectiveWechatUnitPrice(6.9))
}

func TestEffectiveWechatMinTopUpPrefersConfiguredValueAndFallsBackSafely(t *testing.T) {
	original := WechatMinTopUp
	t.Cleanup(func() { WechatMinTopUp = original })

	WechatMinTopUp = 5
	assert.Equal(t, 5, EffectiveWechatMinTopUp(2))

	WechatMinTopUp = 0
	assert.Equal(t, 2, EffectiveWechatMinTopUp(2))

	WechatMinTopUp = MaxWechatMinTopUp + 1
	assert.Equal(t, DefaultWechatMinTopUp, EffectiveWechatMinTopUp(0))
}
