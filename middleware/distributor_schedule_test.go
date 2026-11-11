package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDistributePinnedScheduleRestrictsSubmissionsButKeepsExistingTaskReads(t *testing.T) {
	require.NoError(t, i18n.Init())
	database := setupTokenAuthTestDB(t)
	require.NoError(t, database.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.Log{}))
	originalMemory := common.MemoryCacheEnabled
	t.Cleanup(func() { common.MemoryCacheEnabled = originalMemory })
	require.NoError(t, database.Create(&model.User{Id: 941001, Username: "schedule-read-admin", Password: "fixture", Role: common.RoleRootUser, Status: common.UserStatusEnabled, Group: "default"}).Error)
	require.NoError(t, database.Create(&model.Token{UserId: 941001, Key: "schedulepinnedadmin", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true}).Error)
	future := time.Now().UTC().Add(12 * time.Minute)
	schedule := &dto.ChannelSchedule{Timezone: "UTC", Periods: []dto.ChannelSchedulePeriod{{Start: future.Format("15:04"), End: future.Add(time.Minute).Format("15:04")}}}
	enabled := &model.Channel{Name: "closed-window", Key: "fixture", Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Group: "default", Models: "schedule-model"}
	enabled.SetSetting(dto.ChannelSettings{ChannelSchedule: schedule})
	require.NoError(t, enabled.Insert())
	disabled := &model.Channel{Name: "manual-disabled", Key: "fixture", Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusManuallyDisabled, Group: "default", Models: "schedule-model"}
	disabled.SetSetting(dto.ChannelSettings{ChannelSchedule: schedule})
	require.NoError(t, disabled.Insert())

	for _, memory := range []bool{false, true} {
		t.Run(fmt.Sprintf("memory=%v", memory), func(t *testing.T) {
			common.MemoryCacheEnabled = memory
			model.InitChannelCache()
			for _, tc := range []struct {
				name      string
				method    string
				path      string
				channelID int
				status    int
				reached   bool
			}{
				{"video task read remains accessible", http.MethodGet, "/v1/videos/task-existing", enabled.Id, http.StatusNoContent, true},
				{"legacy video task read remains accessible", http.MethodGet, "/v1/video/generations/task-existing", enabled.Id, http.StatusNoContent, true},
				{"new chat request remains restricted", http.MethodPost, "/v1/chat/completions", enabled.Id, http.StatusForbidden, false},
				{"manual disable still blocks video read", http.MethodGet, "/v1/videos/task-existing", disabled.Id, http.StatusForbidden, false},
				{"manual disable still blocks legacy video read", http.MethodGet, "/v1/video/generations/task-existing", disabled.Id, http.StatusForbidden, false},
			} {
				t.Run(tc.name, func(t *testing.T) {
					router := gin.New()
					reached := false
					router.Use(TokenAuth(), Distribute())
					router.Handle(tc.method, tc.path, func(c *gin.Context) {
						reached = true
						assert.Equal(t, tc.channelID, common.GetContextKeyInt(c, constant.ContextKeyChannelId))
						c.Status(http.StatusNoContent)
					})
					request := httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{"model":"schedule-model","messages":[{"role":"user","content":"hello"}]}`))
					request.Header.Set("Content-Type", "application/json")
					request.Header.Set("Authorization", fmt.Sprintf("Bearer sk-schedulepinnedadmin-%d", tc.channelID))
					recorder := httptest.NewRecorder()
					router.ServeHTTP(recorder, request)
					assert.Equal(t, tc.status, recorder.Code, recorder.Body.String())
					assert.Equal(t, tc.reached, reached)
				})
			}
		})
	}
}
