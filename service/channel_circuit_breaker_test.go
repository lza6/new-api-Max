package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func withCircuitBreaker(t *testing.T, enabled bool) {
	t.Helper()
	resetCircuitTableForTest()
	SetCircuitBreakerEnabled(&enabled)
	t.Cleanup(func() {
		resetCircuitTableForTest()
		SetCircuitBreakerEnabled(nil)
	})
}

func tripBreaker(t *testing.T, ch int, class RelayErrorClass) {
	t.Helper()
	for i := 0; i < circuitFailureThreshold; i++ {
		RegisterChannelCircuitFailure(ch, class)
	}
}

// TestCircuitBreakerOpensAfterConsecutiveFailures 连续失败达阈值 → Open。
func TestCircuitBreakerOpensAfterConsecutiveFailures(t *testing.T) {
	withCircuitBreaker(t, true)
	const ch = 42

	for i := 0; i < circuitFailureThreshold-1; i++ {
		assert.False(t, RegisterChannelCircuitFailure(ch, ErrClassServerError), "fail #%d must not open yet", i+1)
		assert.True(t, ChannelCircuitAllows(ch), "channel must remain selectable below threshold")
	}
	assert.True(t, RegisterChannelCircuitFailure(ch, ErrClassServerError), "threshold-reaching failure must open the breaker")
	state, failures := GetChannelCircuitState(ch)
	assert.Equal(t, CircuitOpen, state)
	assert.Equal(t, circuitFailureThreshold, failures)
}

// TestCircuitBreakerIgnoresClientFaults 我方参数错误不计入熔断。
func TestCircuitBreakerIgnoresClientFaults(t *testing.T) {
	withCircuitBreaker(t, true)
	for i := 0; i < circuitFailureThreshold+3; i++ {
		RegisterChannelCircuitFailure(7, ErrClassBadRequest)
	}
	state, _ := GetChannelCircuitState(7)
	assert.Equal(t, CircuitClosed, state, "bad_request must never trip the breaker")
	assert.True(t, AcquireCircuitProbe(7))
}

// TestChannelCircuitAllowsIsPurePredicate §审查 C1：纯判定不得消费探测令牌，
// 多次调用结果一致（过滤阶段每候选/每轮多次调用）。
func TestChannelCircuitAllowsIsPurePredicate(t *testing.T) {
	withCircuitBreaker(t, true)
	const ch = 9
	tripBreaker(t, ch, ErrClassTimeout)

	// Open 未到期：纯判定恒 false（从候选剔除），且不改变内部状态。
	for i := 0; i < 5; i++ {
		assert.False(t, ChannelCircuitAllows(ch), "open (not expired) must be excluded from candidates")
	}

	// 拨到已到期（半开候选）。
	circuitMu.Lock()
	circuitTable[ch].openedAt = time.Now().Add(-circuitOpenDuration - time.Second)
	circuitMu.Unlock()

	// 纯判定恒 true，且**不**消耗探测：连续多次仍为 true，探测位未被占用。
	for i := 0; i < 5; i++ {
		assert.True(t, ChannelCircuitAllows(ch), "half-open candidate must remain allowed by pure predicate")
	}
	circuitMu.Lock()
	probing := circuitTable[ch].probing
	circuitMu.Unlock()
	assert.False(t, probing, "pure predicate must not consume the probe token")
}

// TestAcquireCircuitProbeSingleFlight §审查 C1：探测令牌只被真正发请求处消费一次。
func TestAcquireCircuitProbeSingleFlight(t *testing.T) {
	withCircuitBreaker(t, true)
	const ch = 10
	tripBreaker(t, ch, ErrClassTimeout)

	// Open 未到期：拒绝（且不消费）。
	require.False(t, AcquireCircuitProbe(ch))
	require.False(t, AcquireCircuitProbe(ch))

	// 到期 → 半开：只有第一个拿到令牌。
	circuitMu.Lock()
	circuitTable[ch].openedAt = time.Now().Add(-circuitOpenDuration - time.Second)
	circuitMu.Unlock()

	assert.True(t, AcquireCircuitProbe(ch), "first acquirer gets the probe")
	assert.False(t, AcquireCircuitProbe(ch), "concurrent acquirers must be rejected during probe")

	// 探测成功 → 关闭。
	RegisterChannelCircuitSuccess(ch)
	assert.True(t, AcquireCircuitProbe(ch))
	state, failures := GetChannelCircuitState(ch)
	assert.Equal(t, CircuitClosed, state)
	assert.Equal(t, 0, failures)
}

// TestCircuitBreakerHalfOpenProbeFailureReopens 半开探测失败 → 重新 Open。
func TestCircuitBreakerHalfOpenProbeFailureReopens(t *testing.T) {
	withCircuitBreaker(t, true)
	const ch = 11
	tripBreaker(t, ch, ErrClassAuth)
	circuitMu.Lock()
	circuitTable[ch].openedAt = time.Now().Add(-circuitOpenDuration - time.Second)
	circuitMu.Unlock()
	require.True(t, AcquireCircuitProbe(ch)) // 放行探测

	assert.True(t, RegisterChannelCircuitFailure(ch, ErrClassAuth), "probe failure must re-open")
	assert.False(t, ChannelCircuitAllows(ch) && AcquireCircuitProbe(ch))
}

// TestCircuitBreakerDisabledIsNoop 开关关闭时零行为变化。
func TestCircuitBreakerDisabledIsNoop(t *testing.T) {
	withCircuitBreaker(t, false)
	for i := 0; i < circuitFailureThreshold+5; i++ {
		assert.False(t, RegisterChannelCircuitFailure(5, ErrClassServerError))
	}
	assert.True(t, ChannelCircuitAllows(5))
	assert.True(t, AcquireCircuitProbe(5))
}

// TestCircuitBreakerConcurrentSafe 并发记录/判定无竞态（-race 下跑）。
func TestCircuitBreakerConcurrentSafe(t *testing.T) {
	withCircuitBreaker(t, true)
	done := make(chan struct{})
	for g := 0; g < 8; g++ {
		go func(id int) {
			defer func() { done <- struct{}{} }()
			for i := 0; i < 200; i++ {
				RegisterChannelCircuitFailure(id, ErrClassServerError)
				_ = ChannelCircuitAllows(id)
				_ = AcquireCircuitProbe(id)
				RegisterChannelCircuitSuccess(id)
				_, _ = GetChannelCircuitState(id)
			}
		}(g)
	}
	for g := 0; g < 8; g++ {
		<-done
	}
}
