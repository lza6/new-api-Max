# 审计验证账本（Audit Ledger）
> 目的：记录已执行的审计/测试/基准及其范围与结论，避免重复无意义重跑；改动相关区域时优先引用。
> 追加规范：每次新增一行 [date] 范围/命令/结论/证据路径。

## 条目
- 2026-09-20 | 并发+完成指标 | 本地 E2E(5 并发)+线上验收(7/7) | PASS | 计划书/e2e-evidence/concurrency-live-metrics-e2e.json, prod-v1.2.35-live-metrics.json
- 2026-09-20 | 延迟基准 60 对（直连 vs 网关）| async-flush | 并发50 p50 240→119ms | 计划书/e2e-evidence/paired-latency-bench-async-flush-v1.2.34.json
- 2026-09-20 | Phase A 审计（4 节点）| 见 计划书/audit/*.md（运行中）
- 2026-09-20 | Phase A 审计 | 4 节点（closure-matrix/sql-db/contract-errors/resilience-observability）| 见 计划书/audit/*.md | A1: 15 节点 13D/1P/1M；A2: 13 条(P1×4)；A3: drift×1 + 映射缺口×3；A4: 无 P0
- 2026-09-20 | SQL 索引/保留修复 | F2/F3/F4（logs 复合索引 + task_events 索引与保留）| 待三库验证 | 修复后勿重复跑 A2 全量，重点复核变更区域
