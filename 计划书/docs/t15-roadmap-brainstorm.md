# 专项分析 · T15 未来方向前瞻：计费/订阅/网关形态（头脑风暴，仅供立项参考）

> 定位：本文件是《下一步改进指南.md》T15 的**方案纵深**，先立项、后实现；不代替任务卡。
> 生成：2026-09-25 · 只读整理，未改业务代码。
> 原则：一切新能力必须复用现有 `common.*` / `quota_math` / `billingexpr` / `operation_setting` / i18n 机制；不发明新框架；立项前先有 dry-run 与回滚开关。

## 1. 三个候选方向（按风险/收益排序）

| 方向 | 现状证据 | 建议形态 | 风险 | 收益 |
|---|---|---|---|---|
| A. 订阅站点统计（specs/001） | `specs/001-subscription-site-stats/spec.md` 已存在（待办，未入库） | 定时拉取 + dry-run + 只读展示，先做计划书级 roadmap 再立项 | 低（只读聚合） | 站点运营透明度 |
| B. 通用 webhook 子系统 | `service/` 无通用 webhook 派发；`pkg/jsplugin` 已有任务轮询/结算 | 任务事件（状态变更/结算完成）→ 签名回调；幂等键=任务ID+事件；重试退避；默认关 | 中（回调可达性/SSRF） | 生态集成（企业通知/自动化） |
| C. `/v1/pricing` 公开定价路由 | 无（模型定价在管理端/计费表达式） | 只读快照：模型→价格（含表达式解析结果），鉴权或限流后开放 | 低 | 站点公开定价、客户端计价 |

## 2. 立项前置（每个方向必须满足）
1. 写 `计划书/ops/t15-task-cards.md`（或 `.specify/specs/`）任务卡，标「待优先级」；
2. 单测/回归：幂等、过期、重放、非法输入、默认关开关；
3. 涉 DB：三库矩阵；涉计费：`quota_math` 与 `billingexpr` 纪律；
4. 涉及外呼：webhook URL 白名单/协议白名单（沿用 marketplace https-only 模式），防 SSRF；
5. 全部先 dry-run + 灰度开关，线上验收需授权。

## 3. 与现有代码的衔接（锚点）
- 订阅：`model/subscription.go`（subscription_plans/user_subscriptions）、`web/src/features/subscriptions`、`specs/001`。
- 任务事件：`service/task_polling.go`、`service/task_plugin_audit.go`、`middleware/task_plugin.go`（已用 request-id，:1244/:1305）。
- 定价快照：`pkg/billingexpr`（表达式）、`setting/billing_setting/builtin_billing.go`、`web/src/features/model-pricing`。
- 定时基础设施：`main.go` 启动的 maintenance loops / `service/*cleanup.go` 模式（web_protection_cleanup.go:31-47 可作参照）。
- 外呼安全：`service/url_guard.go` SSRF 校验、marketplace URL 协议白名单（https only + 1MB + SHA-256）。

## 4. 反范围蔓延护栏
- 一次只做上述**一个**方向；不做「自动涨价/自动结算/自动风控」类闭环，先只读观测。
- webhook 只做「事件通知 + 幂等」，不做「回调驱动结算/退款」；任务结算语义仍以任务轮询为准。
- 公开定价只读，不开放修改；鉴权/限流沿用现有中间件族。

## 5. 验证（立项后）
- 后端：`go test ./service/... ./pkg/billingexpr/... ./relay/...`；涉 DB：三库矩阵。
- 前端：`cd web && bun run typecheck && bunx vitest run src/features/subscriptions src/features/task-plugins`。
- 浏览器/E2E：STAGING 真实截图入 `计划书/e2e-evidence/`；线上验收需授权。

## 立项状态（2026-09-27，v1.3.44 回填）
- ⚪ 三方向均为立项参考，未实施：A 订阅站点统计（specs/001 待办）、B 通用 webhook（需用户优先级）、C /v1/pricing 公开定价（需授权）。
- 纪律：新能力复用 common.*/quota_math/billingexpr/operation_setting/i18n；先 dry-run + 灰度开关；涉外呼走 URL 白名单防 SSRF。
