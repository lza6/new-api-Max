package service

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestClassifyHTTPStatus 依据 aisix "cooldown 与 retryability 解耦"模型,
// 覆盖全部错误类与边界输入。
func TestClassifyHTTPStatus(t *testing.T) {
	testCases := []struct {
		name       string
		status     int
		retryAfter string
		wantRetry  bool
		wantCool   bool
		wantClass  RelayErrorClass
	}{
		{name: "2xx success", status: 200, wantClass: ErrClassOK},
		{name: "204 success", status: 204, wantClass: ErrClassOK},
		{name: "301 redirect", status: 301, wantClass: ErrClassBadRequest},
		{name: "401 auth", status: http.StatusUnauthorized, wantCool: true, wantClass: ErrClassAuth},
		{name: "403 auth", status: http.StatusForbidden, wantCool: true, wantClass: ErrClassAuth},
		{name: "407 proxy auth", status: http.StatusProxyAuthRequired, wantCool: true, wantClass: ErrClassAuth},
		{name: "429 rate limited", status: http.StatusTooManyRequests, wantRetry: true, wantCool: true, wantClass: ErrClassRateLimited},
		{name: "500 server error", status: http.StatusInternalServerError, wantRetry: true, wantCool: true, wantClass: ErrClassServerError},
		{name: "502 server error", status: http.StatusBadGateway, wantRetry: true, wantCool: true, wantClass: ErrClassServerError},
		{name: "503 server error", status: http.StatusServiceUnavailable, wantRetry: true, wantCool: true, wantClass: ErrClassServerError},
		{name: "408 request timeout", status: http.StatusRequestTimeout, wantCool: true, wantClass: ErrClassTimeout},
		{name: "400 bad request", status: http.StatusBadRequest, wantClass: ErrClassBadRequest},
		{name: "422 client error", status: http.StatusUnprocessableEntity, wantClass: ErrClassBadRequest},
		{name: "network error no response", status: 0, wantCool: true, wantClass: ErrClassTimeout},
		{name: "negative status", status: -1, wantCool: true, wantClass: ErrClassTimeout},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			retry, cool, class := ClassifyHTTPStatus(tc.status, tc.retryAfter)
			assert.Equal(t, tc.wantRetry, retry, "retryable")
			assert.Equal(t, tc.wantCool, cool, "needCooldown")
			assert.Equal(t, tc.wantClass, class, "class")
		})
	}
}

// TestRetryAfterCooldownSeconds 数字 Retry-After 解析与上限钳制。
func TestRetryAfterCooldownSeconds(t *testing.T) {
	testCases := []struct {
		name       string
		retryAfter string
		cap        time.Duration
		want       time.Duration
	}{
		{name: "empty", retryAfter: "", want: 0},
		{name: "zero", retryAfter: "0", want: 0},
		{name: "negative", retryAfter: "-5", want: 0},
		{name: "garbage", retryAfter: "abc", want: 0},
		{name: "valid small", retryAfter: "120", want: 2 * time.Minute},
		{name: "valid under cap", retryAfter: "30", cap: time.Minute, want: 30 * time.Second},
		{name: "exceeds cap clamps", retryAfter: "3600", cap: time.Minute, want: time.Minute},
		{name: "default cap 15min", retryAfter: "999999", want: DefaultCooldownCap},
		{name: "whitespace trimmed", retryAfter: " 45 ", want: 45 * time.Second},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := RetryAfterCooldown(tc.retryAfter, tc.cap)
			assert.Equal(t, tc.want, got, "cooldown duration")
		})
	}
}

// TestRetryAfterCooldownHTTPDate HTTP-date 格式的 Retry-After 解析。
func TestRetryAfterCooldownHTTPDate(t *testing.T) {
	future := time.Now().Add(90 * time.Second).UTC().Format(http.TimeFormat)
	got := RetryAfterCooldown(future, time.Minute)
	require.Positive(t, got)
	require.LessOrEqual(t, got, time.Minute)

	past := time.Now().Add(-time.Hour).UTC().Format(http.TimeFormat)
	assert.Zero(t, RetryAfterCooldown(past, time.Minute), "expired date must yield zero")
}
