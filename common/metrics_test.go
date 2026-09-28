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
	MetricsObserve("http_request_duration_seconds", 0.3)  // bucket[1] (<0.25 -> index2? 0.3 -> bucket 0.5 le -> idx2)
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