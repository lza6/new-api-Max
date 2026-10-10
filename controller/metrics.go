package controller

import (
	"runtime"
	"strconv"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/model"
	"github.com/lza6/new-api-Max/service"
)

// RegisterMetricsGaugeProvider B3-1：把派生型瞬时指标（渠道健康分 / 熔断状态 /
// 消费日志队列深度）注入 common 的 Prometheus 渲染器。每次 /metrics 渲染前调用，
// 保证 gauge 反映实时快照而非陈旧值。
//
// 在 main 启动时调用一次。
func RegisterMetricsGaugeProvider() {
	common.MetricsGaugeProvider = func() {
		common.MetricsResetGauges()

		// 资源类指标放最前面：**不依赖 DB、任何情况下都能输出**。
		// 目的是让"跑三天之后变慢"从靠猜变成可观测（指南 §12.2.2）。
		setRuntimeResourceGauges()
		setBackgroundLoopHeartbeatGauges()

		// 队列深度/批量落库指标（进程内原子量，无 DB 依赖）。
		queueDepth, lastBatch, failures, retries := model.GetConsumeLogFlusherMetrics()
		common.MetricsSetGauge("consume_log_queue_depth", nil, float64(queueDepth))
		common.MetricsSetGauge("consume_log_last_batch_size", nil, float64(lastBatch))
		common.MetricsSetGauge("consume_log_flush_failures_total", nil, float64(failures))
		common.MetricsSetGauge("consume_log_flush_retries_total", nil, float64(retries))

		// T7 中继一致性自检命中计数。
		if snap := service.RelayAuditSnapshot(); len(snap) > 0 {
			for check, n := range snap {
				common.MetricsSetGauge("relay_audit_findings_total", map[string]string{"check": check}, float64(n))
			}
		}
		// T10 策略引擎命中计数。
		if snap := service.PolicySnapshot(); len(snap) > 0 {
			common.MetricsSetGauge("policy_engine_eval_total", nil, float64(service.PolicyEvalTotal()))
			for k, n := range snap {
				common.MetricsSetGauge("policy_decision_total", map[string]string{"decision": k}, float64(n))
			}
		}

		// G3 响应缓存效果度量（与「实验功能」页显示的是同一组数字）。
		for k, v := range ResponseCacheMetrics() {
			common.MetricsSetGauge(k, nil, v)
		}
		// G3 工具抽屉收益度量。
		for k, v := range ToolDrawerMetrics() {
			common.MetricsSetGauge(k, nil, v)
		}

		// 渠道健康分 + 熔断状态。查询失败（DB 未就绪等）时静默跳过，不影响计数/直方图输出。
		var ids []int
		if err := model.DB.Model(&model.Channel{}).Pluck("id", &ids).Error; err != nil {
			return
		}
		circuitStateValue := map[service.CircuitState]float64{
			service.CircuitClosed:   0,
			service.CircuitHalfOpen: 1,
			service.CircuitOpen:     2,
		}
		for _, id := range ids {
			labels := map[string]string{"channel_id": strconv.Itoa(id)}
			snap := service.GetChannelHealthSnapshot(id)
			common.MetricsSetGauge("channel_health_score", labels, snap.Score)
			common.MetricsSetGauge("channel_health_success_rate", labels, snap.SuccessRate)
			common.MetricsSetGauge("channel_health_p95_latency_ms", labels, float64(snap.P95LatencyMs))

			state, fails := service.GetChannelCircuitState(id)
			common.MetricsSetGauge("channel_circuit_state", labels, circuitStateValue[state])
			common.MetricsSetGauge("channel_circuit_consecutive_failures", labels, float64(fails))

			cooling := 0.0
			if snap.CoolingDown {
				cooling = 1
			}
			common.MetricsSetGauge("channel_cooling_down", labels, cooling)
		}
	}
}

// setRuntimeResourceGauges 输出进程与运行时资源指标。
//
// 为什么需要：项目此前**没有任何**进程内存/goroutine 指标，判断"跑久会不会变慢"
// 只能靠猜。这里用 `runtime.ReadMemStats`（已在 performance.go 有先例）与
// `runtime.NumGoroutine`，成本约几十微秒、只在 /metrics 渲染时执行。
//
// 指标含义（判读方式）：
//   - process_goroutines 持续上升不回落 → 大概率有 goroutine 泄漏（例如未 stop 的 ticker）；
//   - process_heap_objects 持续上升而 process_memory_alloc_bytes 也在涨 → 大概率有
//     "只加不减"的 map/slice（本批次已在修若干处，见台账 §G10）；
//   - gc_cycles_total 与 gc_pause_total_seconds 的**增速**上升 → GC 压力变大。
func setRuntimeResourceGauges() {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	common.MetricsSetGauge("process_goroutines", nil, float64(runtime.NumGoroutine()))
	common.MetricsSetGauge("process_memory_alloc_bytes", nil, float64(mem.Alloc))
	common.MetricsSetGauge("process_memory_heap_bytes", nil, float64(mem.HeapAlloc))
	common.MetricsSetGauge("process_memory_sys_bytes", nil, float64(mem.Sys))
	common.MetricsSetGauge("process_memory_heap_objects", nil, float64(mem.HeapObjects))
	common.MetricsSetGauge("process_memory_stack_bytes", nil, float64(mem.StackInuse))
	common.MetricsSetGauge("process_gc_cycles_total", nil, float64(mem.NumGC))
	common.MetricsSetGauge("process_gc_pause_total_seconds", nil, float64(mem.PauseTotalNs)/1e9)

	// DB 连接池：等待计数/等待时长增长说明池子偏小或被慢查询占满。
	if model.DB == nil {
		return
	}
	sqlDB, err := model.DB.DB()
	if err != nil {
		return
	}
	stats := sqlDB.Stats()
	common.MetricsSetGauge("db_max_open_connections", nil, float64(stats.MaxOpenConnections))
	common.MetricsSetGauge("db_open_connections", nil, float64(stats.OpenConnections))
	common.MetricsSetGauge("db_in_use_connections", nil, float64(stats.InUse))
	common.MetricsSetGauge("db_idle_connections", nil, float64(stats.Idle))
	common.MetricsSetGauge("db_wait_count_total", nil, float64(stats.WaitCount))
	common.MetricsSetGauge("db_wait_duration_seconds_total", nil, stats.WaitDuration.Seconds())
}

// setBackgroundLoopHeartbeatGauges 输出各常驻后台 loop 的「上次执行时刻」Unix 秒。
//
// 用途：某个 loop 卡死（heartbeat 不再前进）时一眼可见 —— 否则只能靠翻日志猜。
// 值为 0 表示该 loop 自进程启动以来还没跑过（或在本版本尚未接心跳）。
func setBackgroundLoopHeartbeatGauges() {
	for name, atMs := range common.LoopHeartbeatSnapshot() {
		common.MetricsSetGauge("background_loop_last_run_timestamp_seconds",
			map[string]string{"loop": name}, float64(atMs)/1000)
	}
}
