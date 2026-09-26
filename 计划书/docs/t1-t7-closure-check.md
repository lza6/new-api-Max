# 六份深挖文档闭环核对（t1-t7 · v1.3.44 批次）

> 生成：2026-09-27 · 目的：对 计划书/docs/ 下 t1-perf-deepdive / t2-t3-gap-options /
> t4-billing-quota-safety / t5-observability-deepdive / t6-plugin-market-safety / t7-ux-a11y-path
> 逐份核对开放项，回填文档状态；只改文档 + 审计记录，不改业务代码（业务项均已在此前版本闭环）。

## 核对结论

| 文档 | 开放项 | 核对证据 | 结论 |
|---|---|---|---|
| t1-perf-deepdive | 基准 10ms 目标 | perf-ledger 0011：overhead_p50=-28.27ms、并发20 -20.1ms、并发50 -20.6ms（warm 连接池） | ✅ 已闭环；文档「待授权重建环境」已过时，回填 |
| t2-t3-gap-options | T2-1 健康分参数化 | setting/operation_setting/channel_health_setting.go 已存在；channel_health_score.go:102/235 已接线 | ✅ 已闭环 |
| t2-t3-gap-options | T3-1 Web 防护维度 | allowed_paths/blocked_paths/ua_allowlist 已落地（v1.3.30 起）；ip_allowlist v1.3.43 新增 | ✅ 闭环；geo_mode 无 GeoIP 库明确不做 |
| t4-billing-quota-safety | 裸转换审计 | 3 处 int(float64(...)) 均为非计费（percentile 索引 / 图像像素尺寸 / 字节格式化）；计费转换全走 quota_math | ✅ 审计通过，追加 ledger 记录 |
| t5-observability-deepdive | 前端 upstream id 展示 | web/src/features/usage-logs/components/dialogs/details-dialog.tsx:787 已展示 + filter-bar 过滤 | ✅ 已闭环 |
| t6-plugin-market-safety | applyStructuredTaskProgress 验收 | service/task_polling_test.go:1033 TestApplyStructuredTaskProgressMergesWithoutClobbering（3 用例） | ✅ 已闭环 |
| t7-ux-a11y-path | a11y/断点/错误映射 | v1.3.40 已闭环（a11y-smoke + browser-e2e-ux 9 截图 + friendly-error-mapping 7/7） | ✅ 已闭环 |

## 明确不做（诚实边界）
- T3 geo_mode：无 GeoIP 库，读 X-Forwarded-For 简化版误判风险高 → 不做，保留为「后续可配」。
- T6 通用 webhook / 定时同步：属 T15 立项方向 B/C，非本批。

## 验证（本批执行）
- go build/vet PASS；go test ./service/ -run TestWebProtection + TaskProgress 相关 PASS
- web typecheck + build PASS（前述 v1.3.43 已验证）
