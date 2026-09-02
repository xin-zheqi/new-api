package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestFrontendThemeSupportsClassicAndDefault(t *testing.T) {
	previousDB := model.DB
	previousOptionMap := common.OptionMap
	previousTheme := system_setting.GetThemeSettings().Frontend
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Option{}))
	model.DB = db
	common.OptionMap = map[string]string{}
	t.Cleanup(func() {
		model.DB = previousDB
		common.OptionMap = previousOptionMap
		system_setting.GetThemeSettings().Frontend = previousTheme
		system_setting.UpdateAndSyncTheme()
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			require.NoError(t, sqlDB.Close())
		}
	})

	gin.SetMode(gin.TestMode)
	for _, theme := range []string{"classic", "default"} {
		t.Run(theme, func(t *testing.T) {
			response := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(response)
			context.Request = httptest.NewRequest(
				http.MethodPut,
				"/api/option/",
				strings.NewReader(`{"key":"theme.frontend","value":"`+theme+`"}`),
			)

			UpdateOption(context)

			assert.Equal(t, http.StatusOK, response.Code)
			assert.JSONEq(t, `{"success":true,"message":""}`, response.Body.String())
			var option model.Option
			require.NoError(t, db.Where("key = ?", "theme.frontend").First(&option).Error)
			assert.Equal(t, theme, option.Value)
			assert.Equal(t, theme, common.GetTheme())

			statusResponse := httptest.NewRecorder()
			statusContext, _ := gin.CreateTestContext(statusResponse)
			statusContext.Request = httptest.NewRequest(http.MethodGet, "/api/status", nil)
			GetStatus(statusContext)
			var payload struct {
				Success bool           `json:"success"`
				Data    map[string]any `json:"data"`
			}
			require.NoError(t, common.Unmarshal(statusResponse.Body.Bytes(), &payload))
			assert.True(t, payload.Success)
			assert.Equal(t, theme, payload.Data["theme"])
		})
	}
}
