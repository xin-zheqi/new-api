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
			assert.False(t, channel.GetSetting().ToolOutputRepair)
			for _, flags := range []struct{ reasoning, tool bool }{{false, false}, {true, false}, {false, true}, {true, true}, {false, false}} {
				var loaded model.Channel
				require.NoError(t, db.First(&loaded, channel.Id).Error)
				settings := loaded.GetSetting()
				settings.ReasoningContentRepair = flags.reasoning
				settings.ToolOutputRepair = flags.tool
				loaded.SetSetting(settings)
				require.NoError(t, loaded.ValidateSettings())
				require.NoError(t, db.Model(&loaded).Update("setting", loaded.Setting).Error)
				var saved model.Channel
				require.NoError(t, db.First(&saved, channel.Id).Error)
				assert.Equal(t, flags.reasoning, saved.GetSetting().ReasoningContentRepair)
				assert.Equal(t, flags.tool, saved.GetSetting().ToolOutputRepair)
				assert.True(t, saved.GetSetting().ThinkingToContent)
				assert.True(t, saved.GetSetting().RejectEmptyResponse)
			}
		})
	}
}
