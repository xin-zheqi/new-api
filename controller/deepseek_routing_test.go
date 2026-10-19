package controller

import (
	"os"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeepSeekModifierRoutingDatabaseMatrix(t *testing.T) {
	for _, dialect := range []struct{ kind, env string }{{"sqlite", ""}, {"mysql", "TEST_MYSQL_DSN"}, {"postgres", "TEST_POSTGRES_DSN"}} {
		t.Run(dialect.kind, func(t *testing.T) {
			dsn := os.Getenv(dialect.env)
			if dialect.env != "" && dsn == "" {
				t.Skipf("%s not configured", dialect.env)
			}
			db := requestPolicyRoutingDB(t, dialect.kind, dsn)
			channel := model.Channel{Id: 930001, Type: constant.ChannelTypeDeepSeek, Name: "modifier-routing-test", Models: "deepseek-flash", Group: "modifier-test", Status: common.ChannelStatusEnabled}
			require.NoError(t, db.Create(&channel).Error)
			require.NoError(t, channel.AddAbilities(db))
			t.Cleanup(func() {
				require.NoError(t, db.Where("channel_id = ?", channel.Id).Delete(&model.Ability{}).Error)
				require.NoError(t, db.Delete(&channel).Error)
			})
			for _, cached := range []bool{false, true} {
				common.MemoryCacheEnabled = cached
				model.InitChannelCache()
				for _, name := range []string{"deepseek-flash@effort:max", "deepseek-flash@thinking:on"} {
					selected, err := model.GetRandomSatisfiedChannel("modifier-test", name, 0, nil)
					require.NoError(t, err)
					require.NotNil(t, selected, "cache=%t model=%s", cached, name)
					assert.Equal(t, channel.Id, selected.Id)
				}
				selected, err := model.GetRandomSatisfiedChannel("modifier-test", "deepseek-pro@effort:max", 0, nil)
				require.NoError(t, err)
				assert.Nil(t, selected, "modifiers must not bypass model eligibility")
			}
		})
	}
}
