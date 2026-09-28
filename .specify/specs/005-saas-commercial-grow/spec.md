# Feature Specification: v4.x SaaS 商业化与增长（005）

> Spec-Kit Phase 2 · 对应指南 §4.3.1（P1 定价与额度透明）/ §4.3.2（P2 留存与续费）/ §4.3.3（P3 落档建议）
> 状态：SPEC → SIMPLIFY → PLAN（规划中，待 Builder 启动）

## Problem Statement

v4.x SaaS 商业化路径上存在三个增长缺口：

1. **定价与额度透明不足（P1）**：额度预警只有单阈值（`QuotaRemindThreshold` 或用户
   `QuotaWarningThreshold`），没有「阈值档」分级提醒；钱包/订阅余额未在关键页（定价页/
   费用明细）展示，用户在触发扣费前看不到剩余额度；无套餐对比页。
2. **留存与续费手段缺（P2）**：订阅到期前无提醒任务（`ExpireDueSubscriptions` 只在到期
   时才处理）；无续费一键入口；退订/自动续费提示缺少合规文案（需产品拍板，不做法务判断）。
3. **B 端与增长观测（P3）**：API 用量/成本报表导出、邀请分销（aff_code 已有基础）、公开
   站点统计扩展增长看板——按护栏落档为建议，不实现。

## User Stories

### US-4.3.1：定价与额度透明（P1）

As a 平台用户
I want 在触发扣费前看到预估费用与剩余额度，额度用尽/临近时收到分级提醒
So that 我对消费有心理预期，不会突然被扣费/断服。

**Acceptance Criteria:**
- [ ] 额度预警升级「阈值档」：默认多档（如剩余 1000/500/100），可配置；触发时按档位发提醒（复用 NotifyUser 渠道选择）
- [ ] 钱包/订阅余额在关键页展示：定价页（或费用明细 CostDetailPanel）显示当前用户剩余额度/订阅余额
- [ ] 套餐对比页：列出在售套餐（价格/额度/并发/时长），支持对比
- [ ] 提交前预估费用：已有 CostDetailPanel 覆盖，本次不作重复
- [ ] 阈值档可配置（j环境变量或系统设置，默认值合理）

### US-4.3.2：留存与续费（P2）

As a 订阅用户
I want 套餐到期前收到提醒，并能在 3 步内一键续费
So that 不因忘记续费而断服。

**Acceptance Criteria:**
- [ ] 订阅到期前 N 天提醒：周期任务扫描快到期订阅 → 复用 NotifyUser（邮箱/webhook 渠道）+ 可选站内
- [ ] 到期前提醒天数可配置（系统设置/env，默认如 3 天）
- [ ] 续费一键：订阅页「立即续费」按钮 → 复用余额购买/支付链路 → 成功反馈（<3 步）
- [ ] 退订/自动续费提示合规文案：中性声明（"可在设置中关闭自动续费"等），标注需产品拍板、不做法务判断
- [ ] 文案 i18n 7 语言（en/zh/zh-TW/fr/ru/ja/vi）

### US-4.3.3：B 端与增长观测（P3 · 落档建议）

As a 运营者
I want 用量/成本报表导出、邀请分销、增长看板的能力被记录为建议
So that 治理层知道这是下一步方向，同时不被本轮实现范围蔓延。

**Acceptance Criteria:**
- [ ] 落档建议文档：`计划书/docs/` 或 `workflow_status.md` 记录 P3 三个方向的实现思路/依赖/优先级
- [ ] 明确护栏：一次只做一个方向；不做自动涨价/自动风控闭环
- [ ] 不实现 P3 代码（除非与 P1/P2 重叠且为最小必要）

## Non-Functional Requirements

- **Performance**: 提醒周期任务批处理（复用现有 subscription_reset 周期任务节奏，勿每请求扫库）
- **Compatibility**: 三库（SQLite/MySQL/PostgreSQL）兼容；新增模型/迁移必须三库验证
- **i18n**: 所有新增前端文案 7 语言无漂移（bun run i18n:sync）
- **Security**: 通知内容不含密钥；续费路径走既有鉴权（余额购买已有）；不新增支付绕过
- **UI**: 沿用设计系统 tokens（§4.2.3 已建 -shadow-*/--elevation-*）；移动端 320px+

## Success Metrics

- 后端：`go build ./...`、相关单测（提醒阈值/到期扫描/续费）
- 前端：`bun run typecheck`、新增 vitest 用例、i18n:sync 无漂移
- 浏览器 E2E：套餐对比页渲染、续费按钮路径、定价页余额展示
- 独立审查收敛

## Out of Scope

- 不实现 P3 报表/分销/看板
- 不做法务判断：退订/自动续费文案用中性声明，标注"需产品拍板"
- 不做自动涨价/自动风控
- 本批不部署生产（按用户部署纪律）

## 任务分工（规划）
- Builder A（P1 后端）：额度预警阈值档升级（default 多档 + env 可配置）+ 单测
- Builder B（P1 前端 + P2 前端）：套餐对比页 + 定价页/详情余额展示 + 续费一键 + 到期提醒前置 UI + 合规文案 + i18n 7 语言
- Builder C（P2 后端）：到期前 N 天提醒周期任务 + 可配置 + 单测
- 主控：E2E 截图 + 独立审查循环 + 交付（commit/push/tag + HTML 报告）