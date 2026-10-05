package task_pricing_setting

import (
	"testing"

	"github.com/lza6/new-api-Max/setting/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskPricingDefaultsAndOptionUpdate(t *testing.T) {
	original := GetCopy()
	t.Cleanup(func() {
		UpdateTaskPricingSetting(func(s *TaskPricingSetting) { *s = original })
	})
	assert.InDelta(t, 1.666667, SoraSizeRatio("1792x1024"), 0.000001)
	assert.InDelta(t, 2.333333, VertexResolutionRatio("veo-3.1-fast-generate-preview", "4K"), 0.000001)
	require.NoError(t, config.UpdateConfigFromMap(&taskPricingSetting, map[string]string{"sora_size_ratio": `{"1792x1024":2}`}))
	assert.Equal(t, 2.0, SoraSizeRatio("1792x1024"))
	assert.Equal(t, 1.0, SoraSizeRatio("720x1280"))
}

func TestVertexResolutionRatioPrefersMostSpecificModelPattern(t *testing.T) {
	original := GetCopy()
	t.Cleanup(func() {
		UpdateTaskPricingSetting(func(s *TaskPricingSetting) { *s = original })
	})
	// Write through the publish helper: readers (VertexResolutionRatio) read the
	// immutable snapshot, so a raw master mutation must republish to be visible.
	UpdateTaskPricingSetting(func(s *TaskPricingSetting) {
		s.VertexResolution4K = map[string]float64{
			"veo-3.1":                       1.5,
			"veo-3.1-fast-generate":         2.333333,
			"veo-3.1-fast-generate-preview": 3,
		}
	})

	assert.Equal(t, 3.0, VertexResolutionRatio("veo-3.1-fast-generate-preview", "4K"))
	assert.Equal(t, 2.333333, VertexResolutionRatio("veo-3.1-fast-generate", "4k"))
	assert.Equal(t, 1.5, VertexResolutionRatio("veo-3.1-generate", "4K"))
}
