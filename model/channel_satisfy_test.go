package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsChannelEnabledForAnyGroupModelDBBatchesGroupsAndNormalizedModel(t *testing.T) {
	truncateTables(t)
	originalMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		common.MemoryCacheEnabled = originalMemoryCacheEnabled
	})

	require.NoError(t, DB.Create(&Ability{
		Group:     "vip",
		Model:     "gpt-4o-gizmo-*",
		ChannelId: 7,
		Enabled:   true,
	}).Error)
	require.NoError(t, DB.Create(&Ability{
		Group:     "default",
		Model:     "plain-model",
		ChannelId: 9,
		Enabled:   true,
	}).Error)

	assert.True(t, IsChannelEnabledForAnyGroupModel([]string{"default", "vip"}, "gpt-4o-gizmo-custom", 7))
	assert.True(t, IsChannelEnabledForAnyGroupModel([]string{"default"}, "plain-model", 9))
	assert.False(t, IsChannelEnabledForAnyGroupModel([]string{"default"}, "gpt-4o-gizmo-custom", 7))
	assert.False(t, IsChannelEnabledForAnyGroupModel([]string{"default", "vip"}, "gpt-4o-gizmo-custom", 8))
}
