package common

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestJitterDurationBounds 4.2.5：抖动结果必须落在 [d*(1-ratio), d*(1+ratio)] 内，
// 且恒为正；ratio=0 时不变；非正 d 原样返回。
func TestJitterDurationBounds(t *testing.T) {
	const d = 10 * time.Second
	const ratio = 0.1
	lo := time.Duration(float64(d) * (1 - ratio))
	hi := time.Duration(float64(d) * (1 + ratio))

	// 多次采样都应落在带内且有正数。
	sawBelow, sawAbove := false, false
	for range 200 {
		got := JitterDuration(d, ratio)
		assert.Greater(t, got, time.Duration(0), "抖动结果必须为正")
		assert.GreaterOrEqual(t, got, lo, "不得低于下界")
		assert.LessOrEqual(t, got, hi, "不得高于上界")
		if got < d {
			sawBelow = true
		}
		if got > d {
			sawAbove = true
		}
	}
	assert.True(t, sawBelow && sawAbove, "抖动应在 d 两侧都出现（非恒定偏移）")

	// ratio=0：恒等于 d。
	assert.Equal(t, d, JitterDuration(d, 0))
	// 非正 d：原样返回。
	assert.Equal(t, time.Duration(0), JitterDuration(0, ratio))
	assert.Equal(t, time.Duration(-5), JitterDuration(-5, ratio))
}
