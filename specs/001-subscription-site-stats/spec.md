# 特性规格：站点订阅运营统计（T15-A）

**分支**: `main`（v1.3.46 批次）| **创建**: 2026-09-27 | **状态**: Implemented (v1.3.46)

**输入**: 《下一步改进指南.md》T15-A「订阅站点统计 — specs/001」

## 用户场景与测试（必然项）

### 用户故事 1 — 站点运营查看订阅概况（P1）
公开访问者可在定价页看到站点订阅运营聚合（生效中/总数/档位/临期/新增），
无需登录；仅聚合计数，不暴露任何用户/订单明细。

**独立测试**：`GET /v1/stats/subscriptions` 返回 `{success, data:{total_plans, total_subscriptions, active_subscriptions, expiring_soon_7d, new_last_30d, by_plan[]}}`。

**验收场景**：
1. Given 站点已有 2 个档位与 5 条订阅（4 active），When 访问端点，Then 返回正确聚合。
2. Given 响应体，Then 不得包含 user_id / start_time / end_time 等用户或订单字段。

### 用户故事 2 — 只读不落库（P2）
端点只做聚合查询，不得新增表、不得写任何记录。

**独立测试**：运行端点前后 `user_subscriptions`/`subscription_plans` 行数与内容不变。

## 需求

### 功能需求
- **FR-001**: `GET /v1/stats/subscriptions` 公开可用（无鉴权），挂 `CriticalRateLimit` 轻量限流，落在 `/v1` 前缀（跳过 Web 防护）。
- **FR-002**: 聚合字段：档位总数、订阅总数、生效中、7 天内到期、30 天内新增、按档位（id/title/total/active）。
- **FR-003**: 统计口径与现有 `user_subscriptions.status (active/expired/cancelled)`、`end_time`、`created_at` 一致。
- **FR-004**: 不新增 schema、不经由计费/额度路径、不触发外部网络。
- **FR-005**: 查询全走 GORM，SQLite/MySQL/PostgreSQL 三库兼容。
- **FR-006**: 聚合失败返回错误码而非降级假数据。

### 关键实体
- **UserSubscription**: `plan_id`, `status`, `end_time`, `created_at`。
- **SubscriptionPlan**: `id`, `title`（用于 by_plan 展示）。

## 成功标准
- **SC-001**: 端点响应时间 < 200ms（本地 SQLite）。
- **SC-002**: controller 单测覆盖聚合口径与「无明细泄露」。
- **SC-003**: 前端定价页渲染只读统计卡（真实浏览器冒烟）。

## 假设
- 站点规模：订阅量级 < 10 万条，聚合 COUNT 代价可忽略；规模增大后加缓存/定时快照（GPS 项）。
- 公开统计不视作敏感数据；若站点需要私有化，可通过运营决策移除该公开路由（代码已隔离）。
- 依赖既有 `/v1` 公开路由组（`router/relay-router.go`）。

## 实现记录（v1.3.46）
- `model/subscription_stats.go`: `GetSiteSubscriptionStats()`（GORM 聚合，无 schema 变更）。
- `controller/site_stats.go`: `GetSiteSubscriptionStats`。
- `router/relay-router.go`: `GET /v1/stats/subscriptions`。
- `web/src/features/pricing/components/site-subscription-stats-card.tsx`: 定价页公开只读统计卡（+i18n 7 语言）。
- 测试：`controller/site_stats_test.go`（聚合 + 无泄露）。