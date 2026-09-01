package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitTaskStoresChannelKeyFingerprint(t *testing.T) {
	const key = "gemini-secret-key"
	task := InitTask(constant.TaskPlatform("gemini"), &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType: constant.ChannelTypeGemini,
			ApiKey:      key,
		},
	})

	assert.Empty(t, task.PrivateData.Key)
	assert.Equal(t, common.Sha256([]byte(key)), task.PrivateData.KeyFingerprint)
	data, err := task.PrivateData.Value()
	require.NoError(t, err)
	serialized, ok := data.([]byte)
	require.True(t, ok)
	assert.NotContains(t, string(serialized), key)
}

func TestTaskPrivateDataResolvesChannelKey(t *testing.T) {
	channel := &Channel{
		Key: "first-key\nselected-key",
		ChannelInfo: ChannelInfo{
			IsMultiKey: true,
		},
	}

	t.Run("matching fingerprint", func(t *testing.T) {
		privateData := TaskPrivateData{KeyFingerprint: common.Sha256([]byte("selected-key"))}
		assert.Equal(t, "selected-key", privateData.ResolveChannelKey(channel))
	})

	t.Run("legacy plaintext key", func(t *testing.T) {
		privateData := TaskPrivateData{Key: "historical-key"}
		assert.Equal(t, "historical-key", privateData.ResolveChannelKey(channel))
	})

	t.Run("rotated key fallback", func(t *testing.T) {
		privateData := TaskPrivateData{KeyFingerprint: common.Sha256([]byte("removed-key"))}
		resolved := privateData.ResolveChannelKey(channel)
		assert.Contains(t, []string{"first-key", "selected-key"}, resolved)
	})
}
