package common

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"sync"
)

var topupGroupRatio = map[string]float64{
	"default": 1,
	"vip":     1,
	"svip":    1,
}
var topupGroupRatioMutex sync.RWMutex

func TopupGroupRatio2JSONString() string {
	topupGroupRatioMutex.RLock()
	defer topupGroupRatioMutex.RUnlock()
	jsonBytes, err := Marshal(topupGroupRatio)
	if err != nil {
		SysError("error marshalling topup group ratio: " + err.Error())
		return "{}"
	}
	return string(jsonBytes)
}

func parseTopupGroupRatio(jsonStr string) (map[string]float64, error) {
	trimmed := strings.TrimSpace(jsonStr)
	if GetJsonType(json.RawMessage(trimmed)) != "object" {
		return nil, fmt.Errorf("topup group ratio must be a JSON object")
	}

	var rawValues map[string]json.RawMessage
	if err := UnmarshalJsonStr(trimmed, &rawValues); err != nil {
		return nil, fmt.Errorf("failed to parse topup group ratio: %w", err)
	}
	if rawValues == nil {
		return nil, fmt.Errorf("topup group ratio must be a JSON object")
	}
	parsed := make(map[string]float64, len(rawValues))
	for name, rawValue := range rawValues {
		if GetJsonType(rawValue) != "number" {
			return nil, fmt.Errorf("topup group ratio %q must be a number", name)
		}
		var ratio float64
		if err := Unmarshal(rawValue, &ratio); err != nil {
			return nil, fmt.Errorf("failed to parse topup group ratio %q: %w", name, err)
		}
		if math.IsNaN(ratio) || math.IsInf(ratio, 0) || ratio < 0 {
			return nil, fmt.Errorf("topup group ratio %q must be a finite non-negative number", name)
		}
		parsed[name] = ratio
	}
	return parsed, nil
}

// ValidateTopupGroupRatioByJSONString validates an operator-supplied ratio
// map without changing the active configuration.
func ValidateTopupGroupRatioByJSONString(jsonStr string) error {
	_, err := parseTopupGroupRatio(jsonStr)
	return err
}

func UpdateTopupGroupRatioByJSONString(jsonStr string) error {
	parsed, err := parseTopupGroupRatio(jsonStr)
	if err != nil {
		return err
	}

	// Parse and validate before taking the lock. A malformed update must leave
	// the last known-good map intact rather than clearing it first.
	topupGroupRatioMutex.Lock()
	topupGroupRatio = parsed
	topupGroupRatioMutex.Unlock()
	return nil
}

func GetTopupGroupRatio(name string) float64 {
	topupGroupRatioMutex.RLock()
	defer topupGroupRatioMutex.RUnlock()
	ratio, ok := topupGroupRatio[name]
	if !ok {
		SysError("topup group ratio not found: " + name)
		return 1
	}
	return ratio
}
