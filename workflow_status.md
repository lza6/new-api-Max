# B5-1 解释性日志卡（Facts + Inference 分离）— 任务状态

## 任务契约
- 目标：日志详情回答「为什么扣这么多钱」「为什么选这个渠道」。
- 范围：后端 `service/log_info_generate.go`（text_quota 调用链）+ 前端 `usage-logs` details-dialog 费用解释卡。
- 锚点：`other` 字段机制、`appendBillingExplain`（facts，已实现）、`matched_tier`（tiered 已有）。

## 现状（已侦察确认）
- `appendBillingExplain(other, summary)` 已在 text_quota.go:482 调用，仅写 `explain.facts`（无 inferences）。
- `textQuotaSummary` 无 tier/定价基价字段 → facts 缺 tier_matched；inferences 需从 tieredResult 与 use_channel 推导。
- `InjectTieredBillingInfo` 已写 `matched_tier` 等 public 字段；前端已渲染 tiered 卡片。
- 前端 `ExplainBreakdown` 组件已存在（渲染 facts + inferences），types/i18n 已备好 → 前端主要缺口是事实性 check 测试。

## 待办（本次 Batch-5）
- [x] 后端：
  - [x] `textQuotaSummary` 增加 `FixedPriceTier`、`BillingUnit`、`MatchedTier` 字段
  - [x] `PostTextConsumeQuota` 在 tiered settle 后回填这些字段
  - [x] `appendBillingExplain` 扩展（facts 增加 tier_matched/fixed_price/billing_unit/channels_considered；inferences 增加阶梯价/按请求计费推断）
  - [x] `channels_considered` = context 中记录的本请求经过渠道数（组合候选 + 普通选中，去重，零额外计算）
  - [x] 单测扩展 `TestAppendBillingExplain*`（普通/tiered/fixed 三场景）
- [x] 前端：
  - [x] `ExplainBreakdown` 事实性测试（`dialogs/__tests__/explain-breakdown.test.tsx`，5 用例）

## 验收
- [x] 后端单测：`go test ./service/ -run TestAppendBillingExplain`（3 用例 PASS）
- [x] 前端：typecheck PASS；`explain-breakdown.test.tsx` 5 用例 PASS；改动文件无 lint error
- [x] 全仓库 `go build ./...` + `relaykit` 独立编译 PASS；`go vet service/controller/model/constant` PASS
- [x] 敏感检查：explain 内容仅由纯事实 label/value 与预置文本推断组成，写死在代码中，不读任何渠道 key/上游 URL 字段 → 静态确认满足
- [ ] E2E：真实/模拟请求 → 日志详情费用解释卡数据正确（截图）；老日志无 JS 报错
- [ ] 独立审查（code-reviewer）

## 关键文件
- `service/text_quota.go`（textQuotaSummary + PostTextConsumeQuota）
- `service/billing_usage.go`（appendBillingExplain）
- `web/src/features/usage-logs/components/dialogs/details-dialog.tsx`（ExplainBreakdown）
