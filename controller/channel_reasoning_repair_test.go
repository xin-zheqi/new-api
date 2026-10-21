package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The flag stays in the existing TEXT settings column: old rows default off,
// and updates must retain neighboring settings on all supported SQL engines.
func TestReasoningRepairSettingsDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := channelBalanceTestDB(t, dialect)
			channel := model.Channel{Name: "reasoning repair fixture", Type: 43, Key: "fixture", Setting: common.GetPointer(`{"thinking_to_content":true,"reject_empty_response":true}`)}
			require.NoError(t, db.Create(&channel).Error)
			assert.False(t, channel.GetSetting().ReasoningContentRepair)
			for _, enabled := range []bool{false, true, false} {
				var loaded model.Channel
				require.NoError(t, db.First(&loaded, channel.Id).Error)
				settings := loaded.GetSetting()
				settings.ReasoningContentRepair = enabled
				loaded.SetSetting(settings)
				require.NoError(t, loaded.ValidateSettings())
				require.NoError(t, db.Model(&loaded).Update("setting", loaded.Setting).Error)
				var saved model.Channel
				require.NoError(t, db.First(&saved, channel.Id).Error)
				assert.Equal(t, enabled, saved.GetSetting().ReasoningContentRepair)
				assert.True(t, saved.GetSetting().ThinkingToContent)
				assert.True(t, saved.GetSetting().RejectEmptyResponse)
			}
		})
	}
}
