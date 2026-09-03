package ratio_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRatioUpdatesRejectInvalidValuesWithoutReplacingActiveConfiguration(t *testing.T) {
	original := ImageRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, UpdateImageRatioByJSONString(original))
	})

	require.NoError(t, UpdateImageRatioByJSONString(`{"image-model":1.25}`))
	for _, invalid := range []string{
		`{"image-model":-1}`,
		`{"image-model":null}`,
		`{"image-model":"1"}`,
		`{"image-model":`,
		`[]`,
	} {
		require.Error(t, UpdateImageRatioByJSONString(invalid))
		assert.Equal(t, map[string]float64{"image-model": 1.25}, GetImageRatioCopy())
	}
}

func TestNestedRatioValidationRejectsInvalidInnerValues(t *testing.T) {
	require.NoError(t, ValidateNestedRatioMapJSONString(`{"vip":{"default":0.9}}`))
	require.Error(t, ValidateNestedRatioMapJSONString(`{"vip":{"default":-0.1}}`))
	require.Error(t, ValidateNestedRatioMapJSONString(`{"vip":null}`))
}
