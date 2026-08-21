package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func boolPointer(value bool) *bool {
	return &value
}

func stringPointer(value string) *string {
	return &value
}

func TestNormalizeOptionUpdateValueValidatesManagedSettings(t *testing.T) {
	tests := []struct {
		name      string
		key       string
		value     string
		expected  string
		shouldErr bool
	}{
		{name: "mall enabled", key: "payment_setting.mall_enabled", value: " TRUE ", expected: "true"},
		{name: "mall enabled invalid", key: "payment_setting.mall_enabled", value: "1", shouldErr: true},
		{name: "external mall URL", key: "MallURL", value: " https://shop.example.com/buy ", expected: "https://shop.example.com/buy"},
		{name: "same-host mall URL", key: "MallURL", value: "https://api.example.com/shop", shouldErr: true},
		{name: "same-host mall URL with multiple trailing dots", key: "MallURL", value: "https://API.EXAMPLE.COM../shop", shouldErr: true},
		{name: "same IDN mall URL", key: "MallURL", value: "https://ａｐｉ.example.com/shop", shouldErr: true},
		{name: "insecure mall URL", key: "MallURL", value: "http://shop.example.com", shouldErr: true},
		{name: "invoice enabled", key: "InvoiceEnabled", value: "False", expected: "false"},
		{name: "invoice enabled invalid", key: "InvoiceEnabled", value: "yes", shouldErr: true},
		{name: "invoice day lower bound", key: "InvoiceApplicationDay", value: "1", expected: "1"},
		{name: "invoice day upper bound", key: "InvoiceApplicationDay", value: "28", expected: "28"},
		{name: "invoice day too high", key: "InvoiceApplicationDay", value: "29", shouldErr: true},
		{name: "invoice lookback canonical", key: "InvoiceLookbackDays", value: "0090", expected: "90"},
		{name: "invoice lookback too high", key: "InvoiceLookbackDays", value: "3651", shouldErr: true},
		{name: "invoice monthly limit", key: "InvoiceMonthlyLimit", value: "31", expected: "31"},
		{name: "invoice monthly limit zero", key: "InvoiceMonthlyLimit", value: "0", shouldErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual, err := normalizeOptionUpdateValue(test.key, test.value, "api.example.com")
			if test.shouldErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.expected, actual)
		})
	}
}

func TestNormalizeMallSettingsUpdateKeepsThePairConsistent(t *testing.T) {
	tests := []struct {
		name      string
		request   mallSettingsUpdateRequest
		expected  string
		shouldErr bool
	}{
		{
			name: "enabled with external URL",
			request: mallSettingsUpdateRequest{
				MallEnabled: boolPointer(true),
				MallURL:     stringPointer(" https://shop.example.com/buy "),
			},
			expected: "https://shop.example.com/buy",
		},
		{
			name: "disabled with empty URL",
			request: mallSettingsUpdateRequest{
				MallEnabled: boolPointer(false),
				MallURL:     stringPointer(""),
			},
			expected: "",
		},
		{
			name: "disabled clears an invalid legacy URL",
			request: mallSettingsUpdateRequest{
				MallEnabled: boolPointer(false),
				MallURL:     stringPointer("javascript:alert(1)"),
			},
			expected: "",
		},
		{
			name: "enabled without URL",
			request: mallSettingsUpdateRequest{
				MallEnabled: boolPointer(true),
				MallURL:     stringPointer(""),
			},
			shouldErr: true,
		},
		{
			name: "enabled with same-host URL",
			request: mallSettingsUpdateRequest{
				MallEnabled: boolPointer(true),
				MallURL:     stringPointer("https://api.example.com/shop"),
			},
			shouldErr: true,
		},
		{
			name:      "missing fields",
			request:   mallSettingsUpdateRequest{},
			shouldErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			values, err := normalizeMallSettingsUpdate(test.request, "api.example.com")
			if test.shouldErr {
				require.Error(t, err)
				assert.Nil(t, values)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, strconv.FormatBool(*test.request.MallEnabled), values["payment_setting.mall_enabled"])
			assert.Equal(t, test.expected, values["MallURL"])
		})
	}
}

func TestUpdateOptionRejectsPartialMallUpdates(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, key := range []string{"MallURL", "payment_setting.mall_enabled"} {
		t.Run(key, func(t *testing.T) {
			body, err := common.Marshal(OptionUpdateRequest{Key: key, Value: "true"})
			require.NoError(t, err)
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			context.Request = httptest.NewRequest(http.MethodPut, "/api/option/", bytes.NewReader(body))

			UpdateOption(context)

			assert.Equal(t, http.StatusOK, recorder.Code)
			var response struct {
				Success bool   `json:"success"`
				Message string `json:"message"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			assert.False(t, response.Success)
			assert.Contains(t, response.Message, "专用接口")
		})
	}
}
