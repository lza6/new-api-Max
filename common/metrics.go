package common

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// §4.1.4 轻量运行时指标（Prometheus 文本格式）。
// 用途：/metrics 端点（env METRICS_ENABLED=true 开放，默认关）输出请求量/延迟、
// 限流命中、封禁与事件总线计数。仅进程内计数，不落库；多实例各自独立（与既有
// 进程内语义一致）。直方图用固定桶，输出 _bucket/_sum/_count 供 Prometheus 聚合。

// metricBuckets 延迟直方图桶（秒）。
var metricBuckets = []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30}

var metricsState = struct {
	sync.Mutex
	counters map[string]map[string]int64 // name -> labelKey -> count
	hist     map[string][]int64          // name -> bucket count (len = buckets+1)
	histSum  map[string]float64
	histN    map[string]int64
}{counters: make(map[string]map[string]int64), hist: make(map[string][]int64), histSum: make(map[string]float64), histN: make(map[string]int64)}

func labelKey(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(labels[k])
	}
	return b.String()
}

// MetricsInc 增加带标签的计数器。
func MetricsInc(name string, labels map[string]string, delta int64) {
	if delta == 0 {
		return
	}
	metricsState.Lock()
	defer metricsState.Unlock()
	if metricsState.counters[name] == nil {
		metricsState.counters[name] = make(map[string]int64)
	}
	metricsState.counters[name][labelKey(labels)] += delta
}

// MetricsObserve 记录一次耗时观测到直方图（固定桶；le 桶为累计观测数）。
func MetricsObserve(name string, valueSeconds float64) {
	metricsState.Lock()
	defer metricsState.Unlock()
	if metricsState.hist[name] == nil {
		metricsState.hist[name] = make([]int64, len(metricBuckets)+1)
	}
	b := sort.SearchFloat64s(metricBuckets, valueSeconds)
	// Prometheus histogram 的 _bucket{le=X} 为累计（观测值 ≤ X）；故 ≥ 该桶全部 +1。
	for i := b; i < len(metricsState.hist[name]); i++ {
		metricsState.hist[name][i]++
	}
	metricsState.histSum[name] += valueSeconds
	metricsState.histN[name]++
}

// RenderPrometheusMetrics 渲染 Prometheus 文本格式（确定性排序）。
func RenderPrometheusMetrics() string {
	metricsState.Lock()
	defer metricsState.Unlock()
	var b strings.Builder

	names := make([]string, 0, len(metricsState.counters))
	for n := range metricsState.counters {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		fmt.Fprintf(&b, "# TYPE %s counter\n", n)
		keys := make([]string, 0, len(metricsState.counters[n]))
		for k := range metricsState.counters[n] {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if k == "" {
				fmt.Fprintf(&b, "%s %d\n", n, metricsState.counters[n][k])
			} else {
				fmt.Fprintf(&b, "%s{%s} %d\n", n, k, metricsState.counters[n][k])
			}
		}
	}

	hNames := make([]string, 0, len(metricsState.hist))
	for n := range metricsState.hist {
		hNames = append(hNames, n)
	}
	sort.Strings(hNames)
	for _, n := range hNames {
		fmt.Fprintf(&b, "# TYPE %s histogram\n", n)
		buckets := metricsState.hist[n]
		for i, le := range metricBuckets {
			fmt.Fprintf(&b, "%s_bucket{le=%q} %d\n", n, strconv.FormatFloat(le, 'g', -1, 64), buckets[i])
		}
		fmt.Fprintf(&b, "%s_bucket{le=\"+Inf\"} %d\n", n, buckets[len(buckets)-1])
		fmt.Fprintf(&b, "%s_sum %g\n", n, metricsState.histSum[n])
		fmt.Fprintf(&b, "%s_count %d\n", n, metricsState.histN[n])
	}
	return b.String()
}