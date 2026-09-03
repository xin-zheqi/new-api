package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadFromJsonStringPreservesLastKnownGoodMapOnError(t *testing.T) {
	values := NewRWMap[string, float64]()
	require.NoError(t, LoadFromJsonString(values, `{"model":1.5}`))

	err := LoadFromJsonString(values, `{"model":`)

	require.Error(t, err)
	assert.Equal(t, map[string]float64{"model": 1.5}, values.ReadAll())
}

func TestLoadFromJsonStringCallbackRunsOnlyAfterSuccessfulPublish(t *testing.T) {
	values := NewRWMap[string, int]()
	callbackCount := 0
	require.NoError(t, LoadFromJsonStringWithCallback(values, `{"value":1}`, func() {
		callbackCount++
	}))
	require.Error(t, LoadFromJsonStringWithCallback(values, `{"value":`, func() {
		callbackCount++
	}))

	assert.Equal(t, 1, callbackCount)
	assert.Equal(t, map[string]int{"value": 1}, values.ReadAll())
}
