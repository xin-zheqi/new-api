package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUserSettingForCacheExcludesNotificationCredentials(t *testing.T) {
	raw, err := common.Marshal(dto.UserSetting{
		NotifyType:            dto.NotifyTypeWebhook,
		QuotaWarningThreshold: 123,
		QuotaWarningEnabled:   common.GetPointer(false),
		ChannelNotifyEnabled:  common.GetPointer(true),
		WebhookUrl:            "https://user:password@example.com/hook?token=secret",
		WebhookSecret:         "webhook-secret",
		NotificationEmail:     "private@example.com",
		BarkUrl:               "https://api.day.app/device-secret/{{content}}",
		GotifyUrl:             "https://user:password@gotify.example.com",
		GotifyToken:           "gotify-secret",
		BillingPreference:     "subscription",
		Language:              "zh",
	})
	require.NoError(t, err)

	cached := userSettingForCache(string(raw))
	var setting dto.UserSetting
	require.NoError(t, common.UnmarshalJsonStr(cached, &setting))

	assert.Equal(t, dto.NotifyTypeWebhook, setting.NotifyType)
	assert.Equal(t, float64(123), setting.QuotaWarningThreshold)
	require.NotNil(t, setting.QuotaWarningEnabled)
	assert.False(t, *setting.QuotaWarningEnabled)
	require.NotNil(t, setting.ChannelNotifyEnabled)
	assert.True(t, *setting.ChannelNotifyEnabled)
	assert.Equal(t, "subscription", setting.BillingPreference)
	assert.Equal(t, "zh", setting.Language)
	assert.Empty(t, setting.WebhookUrl)
	assert.Empty(t, setting.WebhookSecret)
	assert.Empty(t, setting.NotificationEmail)
	assert.Empty(t, setting.BarkUrl)
	assert.Empty(t, setting.GotifyUrl)
	assert.Empty(t, setting.GotifyToken)

	for _, secret := range []string{"password", "token=secret", "webhook-secret", "device-secret", "gotify-secret", "private@example.com"} {
		assert.NotContains(t, cached, secret)
	}
	assert.Equal(t, cached, userSettingForCache(cached))
}
