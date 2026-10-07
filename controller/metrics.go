package controller

import (
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
