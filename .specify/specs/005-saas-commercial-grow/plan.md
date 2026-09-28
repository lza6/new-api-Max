# Plan：v4.x SaaS 商业化与增长（005）

## 阶段划分（依赖顺序）
- **Phase 1【Spec/计划】**：spec.md + plan.md + tasks.md（本批）→ 质量门：文档。
- **Phase 2【并行实现 · 3 Builder】**：
  - 2a Builder A（P1 后端）：`service/quota.go` + `service/user_notify.go` 额度预警阈值档升级
  - 2b Builder B（P1+P2 前端）：套餐对比页 + 余额展示 + 续费一键 + 合规文案 + i18n
  - 2c Builder C（P2 后端）：`service/subscription_reset_task.go` 到期前 N 天提醒周期任务
- **Phase 3【E2E/截图】**：套餐对比页渲染、续费按钮、定价页余额展示（主控，依赖 2b 完成）
- **Phase 4【审查循环】**：独立 Critic 六维审查 → 修复 → 复验（最多 3 轮）
- **Phase 5【质量门/交付】**：typecheck/lint/vitest/i18n → commit → push → tag v1.3.57 → HTML 报告（生产部署待授权）

## 关键设计决策

### D1 · 额度预警「阈值档」（Builder A 核心）
- 现状：单阈值 `common.QuotaRemindThreshold=1000`；用户 `QuotaWarningThreshold`（默认新用户 80%）覆盖全局。
- 升级目标：**多档位分级提醒**。默认档位 `[1000, 500, 100]`（绝对额度）或 `[80%, 50%, 20%]`（比例，取大者语义）。env `QUOTA_WARN_THRESHOLDS=1000,500,100` 可配置。
- 防重复：每档触发一次（用用户设置中的 `QuotaWarnedLevels` 记录已触发档位，消费后不重置；充值后降档可重置），复用 `CheckNotificationLimit` 限频兜底。
- 兼容：用户显式设置 `QuotaWarningThreshold`（非默认）时，作为**最高优先级单档**（保持既有行为，不破坏存量设置）；未设置时用多档默认。
- 订阅额度（`checkAndSendSubscriptionQuotaNotify`）同样适用多档。

### D2 · 到期前 N 天提醒（Builder C 核心）
- 复用 `StartSubscriptionQuotaResetTask` 的 1 分钟 tick 节奏（MasterNode 单跑），新任务 `scanSubscriptionExpiryReminder`：批量查 `status=active AND end_time 在 [now, now+N天] AND 未提醒` → NotifyUser（复用渠道）。
- 防重复：`UserSubscription.reminder_days_notified` int 字段（新增迁移，三库兼容），记录已提醒天数（如 3）；N 天可配 env `SUBSCRIPTION_EXPIRY_REMIND_DAYS=3`。
- 注意：本批新增模型字段 → 必须三库迁移验证（AGENTS.md 三库铁则）。

### D3 · 合规文案边界（P2 前端）
- 退订/自动续费提示用**中性声明**：不声称法律效力、不替用户做自动续费决策。示例："订阅到期后自动停止，不自动扣费；可在订阅页手动续费。"标注「需产品拍板，不做法务判断」于文档。
- 续费一键：订阅页「立即续费」→ 复用 `SubscriptionRequestBalancePay`（余额购买已有）+ 支付链接兜底 → 成功 toast。步数 <3。

### D4 · 套餐对比页（P1 前端）
- 复用 `GetSubscriptionPlans`（公开 API 已有）+ 定价页排版 tokens（§4.2.3）。对比维度：价格/额度/并发/RPM/时长。移动端可横向滚动或折叠。

## 依赖关系
- 2a（后端阈值）与 2c（后端到期提醒）不同文件，可并行；都只动 service/，注意不覆盖彼此（A: quota.go/user_notify.go；C: subscription_reset_task.go + model/subscription.go）。
- 2b 前端依赖 2a/2c 的 API 形状（若前端需展示多档/提醒配置则需约定——本批前端不新增配置 UI，仅消费余额/续费/对比），故 2b 可与后端并行，联调在 Phase 3。
- Phase 3 依赖 2b；Phase 4 依赖 3；Phase 5 依赖 3/4。

## 并行机会
- A / B / C 三个 Builder 并行（文件不重叠：A service/quota.go；B web/src/features/pricing + subscriptions；C service/subscription_reset_task.go + model/subscription.go）。

## 风险与缓解
| 风险 | 缓解 |
|---|---|
| 阈值档升级破坏存量单阈值行为 | 用户显式设置仍为最高优先级单档；仅默认路径变多档 |
| 新增字段三库迁移 | model/subscription.go 加字段 + 三库 conformance（db-conformance.ps1 / CI 已有） |
| 到期提醒误发/漏发 | reminder_days_notified 去重 + 周期任务批处理 + 通知限频 |
| 合规文案越界 | 中性声明 + 文档标注需产品拍板；不写"自动续费"承诺 |
| 前端改动大 | 严格复用既有组件（subscriptions table / pricing 排版），只加对比页/余额展示/按钮 |
| 生产部署 | 不自动部署，用户授权后 deploy.sh |

## 质量门禁
- `cd web && bun run typecheck` exit 0
- `cd web && bunx oxlint` 改动文件无 error
- `cd web && bunx vitest run`（新增用例）+ i18n:sync 无漂移
- 后端 `go build ./...` + `go test`（quota 阈值档、到期提醒扫描、续费）+ relaykit 独立构建
- 三库 conformance 通过（本批有模型字段新增，必须跑）
- 浏览器 E2E：套餐对比页/续费按钮/余额展示截图
- 独立审查 3 轮收敛或明确阻塞
- commit + push + tag v1.3.57 + HTML 报告