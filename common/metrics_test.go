package common

import (
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resetMetrics() {
	metricsState.Lock()
	defer metricsState.Unlock()
	metricsState.counters = make(map[string]map[string]int64)
	metricsState.hist = make(map[string][]int64)
	metricsState.histSum = make(map[string]float64)
	metricsState.histN = make(map[string]int64)
	metricsState.gauges = make(map[string]map[string]float64)
}

// TestMetricsCounterAndLabels 锁定 §4.1.4：带标签计数器累加 + 无标签 + 确定性渲染。
func TestMetricsCounterAndLabels(t *testing.T) {
	resetMetrics()
	MetricsInc("http_requests_total", map[string]string{"method": "GET", "status": "200"}, 1)
	MetricsInc("http_requests_total", map[string]string{"method": "GET", "status": "200"}, 1)
	MetricsInc("http_requests_total", map[string]string{"method": "POST", "status": "429"}, 1)
	MetricsInc("plain_counter", nil, 3)

	out := RenderPrometheusMetrics()
	require.Contains(t, out, "http_requests_total{method=GET,status=200} 2")
	require.Contains(t, out, "http_requests_total{method=POST,status=429} 1")
	require.Contains(t, out, "plain_counter 3")
}

// TestMetricsHistogram 锁定：直方图桶/_sum/_count 输出正确。
func TestMetricsHistogram(t *testing.T) {
	resetMetrics()
	MetricsObserve("http_request_duration_seconds", 0.05) // bucket[0] (<0.1)
	MetricsObserve("http_request_duration_seconds", 0.3)  // 0.05 落入 le=0.1；0.3 落入 le=0.5（累计含 0.05）
	MetricsObserve("http_request_duration_seconds", 30.0) // +Inf bucket (last)

	out := RenderPrometheusMetrics()
	require.Contains(t, out, `http_request_duration_seconds_bucket{le="0.1"} 1`)
	require.Contains(t, out, `http_request_duration_seconds_bucket{le="0.5"} 2`)
	require.Contains(t, out, `http_request_duration_seconds_bucket{le="+Inf"} 3`)
	require.Contains(t, out, "http_request_duration_seconds_count 3")
}

// TestMetricsConcurrent 锁定：并发写入不 panic、计数准确。
func TestMetricsConcurrent(t *testing.T) {
	resetMetrics()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			MetricsInc("c", map[string]string{"k": "v"}, 1)
			MetricsObserve("h", 0.2)
		}(i)
	}
	wg.Wait()
	out := RenderPrometheusMetrics()
	assert.Contains(t, out, "c{k=v} 100")
	assert.Contains(t, out, "h_count 100")
}

// TestMetricsDeterministic 锁定：渲染输出键序稳定（防测试/观测抖动）。
func TestMetricsDeterministic(t *testing.T) {
	resetMetrics()
	MetricsInc("z", map[string]string{"b": "2", "a": "1"}, 1)
	MetricsInc("a", map[string]string{"x": "1"}, 1)
	out1 := RenderPrometheusMetrics()
	out2 := RenderPrometheusMetrics()
	assert.Equal(t, out1, out2)
	assert.Equal(t, strings.Index(out1, "a{"), strings.Index(out2, "a{"))
}

// TestMetricsGaugeRender B3-1：gauge 渲染为 # TYPE ... gauge，带标签；Reset 后清空。
func TestMetricsGaugeRender(t *testing.T) {
	resetMetrics()
	MetricsSetGauge("channel_health_score", map[string]string{"channel_id": "7"}, 88.5)
	MetricsSetGauge("consume_log_queue_depth", nil, 12)
	out := RenderPrometheusMetrics()
	require.Contains(t, out, "# TYPE channel_health_score gauge")
	require.Contains(t, out, "channel_health_score{channel_id=7} 88.5")
	require.Contains(t, out, "# TYPE consume_log_queue_depth gauge")
	require.Contains(t, out, "consume_log_queue_depth 12")

	MetricsResetGauges()
	out2 := RenderPrometheusMetrics()
	assert.NotContains(t, out2, "channel_health_score", "gauges must be cleared after reset")
}

// TestMetricsGaugeProviderInvoked B3-1：渲染前调用 provider 刷新派生指标。
func TestMetricsGaugeProviderInvoked(t *testing.T) {
	resetMetrics()
	called := 0
	prev := MetricsGaugeProvider
	t.Cleanup(func() { MetricsGaugeProvider = prev })
	MetricsGaugeProvider = func() {
		called++
		MetricsResetGauges()
		MetricsSetGauge("channel_circuit_state", map[string]string{"channel_id": "3"}, 2)
	}
	out := RenderPrometheusMetrics()
	assert.Equal(t, 1, called)
	assert.Contains(t, out, "channel_circuit_state{channel_id=3} 2")
}
