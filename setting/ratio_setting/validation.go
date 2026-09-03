package ratio_setting

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

// ValidateRatioMapJSONString accepts only an object whose values are finite,
// non-negative numbers. Zero remains valid for free pricing configurations.
func ValidateRatioMapJSONString(jsonStr string) error {
	return validateRatioMapJSONString(jsonStr, "ratio setting")
}

func validateRatioMapJSONString(jsonStr, settingName string) error {
	trimmed := strings.TrimSpace(jsonStr)
	if common.GetJsonType(json.RawMessage(trimmed)) != "object" {
		return fmt.Errorf("%s must be a JSON object", settingName)
	}

	var values map[string]json.RawMessage
	if err := common.UnmarshalJsonStr(trimmed, &values); err != nil {
		return fmt.Errorf("invalid %s: %w", settingName, err)
	}
	if values == nil {
		return fmt.Errorf("%s must be a JSON object", settingName)
	}
	for name, rawValue := range values {
		if common.GetJsonType(rawValue) != "number" {
			return fmt.Errorf("%s value %q must be a number", settingName, name)
		}
		var value float64
		if err := common.Unmarshal(rawValue, &value); err != nil {
			return fmt.Errorf("invalid %s value %q: %w", settingName, name, err)
		}
		if value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
			return fmt.Errorf("%s value %q must be finite and non-negative", settingName, name)
		}
	}
	return nil
}

func ValidateNestedRatioMapJSONString(jsonStr string) error {
	trimmed := strings.TrimSpace(jsonStr)
	if common.GetJsonType(json.RawMessage(trimmed)) != "object" {
		return fmt.Errorf("nested ratio setting must be a JSON object")
	}

	var groups map[string]json.RawMessage
	if err := common.UnmarshalJsonStr(trimmed, &groups); err != nil {
		return fmt.Errorf("invalid nested ratio setting: %w", err)
	}
	if groups == nil {
		return fmt.Errorf("nested ratio setting must be a JSON object")
	}
	for group, rawValue := range groups {
		if err := validateRatioMapJSONString(string(rawValue), fmt.Sprintf("group ratio setting %q", group)); err != nil {
			return err
		}
	}
	return nil
}
