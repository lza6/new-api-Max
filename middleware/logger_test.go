package middleware

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestSlowRequestThresholdMs 锁定 §4.1.4：SLOW_REQUEST_THRESHOLD_MS 解析默认/合法/非法值。
func TestSlowRequestThresholdMs(t *testing.T) {
	t.Setenv("SLOW_REQUEST_THRESHOLD_MS", "250")
	assert.Equal(t, int64(250), slowRequestThresholdMs())

	t.Setenv("SLOW_REQUEST_THRESHOLD_MS", "not-a-number")
	assert.Equal(t, int64(3000), slowRequestThresholdMs())

	t.Setenv("SLOW_REQUEST_THRESHOLD_MS", "")
	assert.Equal(t, int64(3000), slowRequestThresholdMs())

	t.Setenv("SLOW_REQUEST_THRESHOLD_MS", "0")
	assert.Equal(t, int64(0), slowRequestThresholdMs())
}