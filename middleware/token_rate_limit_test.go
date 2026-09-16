package middleware

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestAllowTokenQBS(t *testing.T) {
	// qbs=2：同一秒最多 2 次，之后拒。
	allow := true
	for i := 0; i < 2; i++ {
		if !allowTokenQBS(101, 2) {
			allow = false
		}
	}
	assert.True(t, allow, "前2次应允许")
	assert.False(t, allowTokenQBS(101, 2), "第3次应拒绝")

	// 空桶后，等待回填：qbs=2/s，过 1s 应回填 2 个令牌。
	time.Sleep(1100 * time.Millisecond)
	assert.True(t, allowTokenQBS(101, 2), "回填后应允许")
}

func TestTokenConcurrencyStore(t *testing.T) {
	assert.True(t, acquireTokenConcurrency(202, 2))
	assert.True(t, acquireTokenConcurrency(202, 2))
	assert.False(t, acquireTokenConcurrency(202, 2), "并发达到上限应拒绝")

	releaseTokenConcurrency(202)
	assert.True(t, acquireTokenConcurrency(202, 2), "释放后可再次获取")

	// 清场
	releaseTokenConcurrency(202)
	releaseTokenConcurrency(202)
}

func TestTokenRateLimitConfigParsing(t *testing.T) {
	// 通过 model 层的 GetRateLimitConfig 验证解析（中间件复用同一结构）。
	cfg := parseTokenRateLimitConfigFromString(`{"rpm":60,"qbs":5,"concurrency":2}`)
	assert.Equal(t, 60, cfg.RPM)
	assert.Equal(t, 5, cfg.QBS)
	assert.Equal(t, 2, cfg.Concurrency)

	empty := parseTokenRateLimitConfigFromString("")
	assert.Zero(t, empty.RPM)
	assert.Zero(t, empty.QBS)
	assert.Zero(t, empty.Concurrency)
}
