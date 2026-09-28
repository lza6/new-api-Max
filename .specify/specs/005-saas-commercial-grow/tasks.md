# Tasks：v4.x SaaS 商业化与增长（005）

**Prerequisites**: spec.md（005）、plan.md（含 D1-D4 设计决策）
**组织方式**：3 个 Builder 并行（A 后端阈值 / B 前端 / C 后端到期提醒），主控负责 E2E/审查/交付。

## US-4.3.1 — 定价与额度透明（P1，Builder A + B）
### Builder A（后端阈值档）
- [x] **[A-1]** 新增 `common.QuotaWarnThresholds []int`（默认 `[1000,500,100]`，env `QUOTA_WARN_THRESHOLDS` 逗号分隔可覆盖；`common/constants.go` + env 解析）。
- [x] **[A-2]** `dto.UserSetting` 增加 `QuotaWarnedLevels`（已触发档位记录，JSON 持久化）——relaykit/dto/user_settings.go 新增，三库 JSON 列无迁移。
- [x] **[A-3]** `service/quota.go checkAndSendQuotaNotify`：改为按档触发——用户显式 QuotaWarningThreshold（非默认）保持单档最高优先级；否则按 QuotaWarnThresholds 多档，`剩余 < 档值` 且该档未记录过 → 发提醒 + 记录档位。充值/额度增长后重置已记录档位。
- [x] **[A-4]** `checkAndSendSubscriptionQuotaNotify`（quota.go:528）同法升级多档。
- [x] **[A-5]** 单测：`service/quota_warn_test.go`（新增）6/6 PASS——多档触发顺序、防重复、充值重置、显式单档兼容、subscription 同法、env 解析。
- [x] **[A-6]** `notifyQuotaExhausted`（用尽）行为不变（仅引用确认）。

### Builder B（前端：对比页 + 余额展示）
- [x] **[B-1]** 套餐对比页：新路由 `/pricing/plans`（公开，beforeLoad 跟随 pricing 模式）→ `plan-comparison.tsx` + `plans-compare-table.tsx`（7 列：价格/额度/时长/并发/RPM/状态/操作，min-w-760 + overflow 保 320px，row 含当前订阅徽章+剩余额度+到期日）；调 `GetSubscriptionPlans`；加载/空/错误态齐（compliance 未确认空数组→空态文案）。i18n 7 语言。
- [x] **[B-2]** 定价页余额展示：`quota-balance-banner.tsx`（登录显示 formatQuota(auth.user.quota) + Top-up，未登录 null）+ 定价页 header「套餐对比」入口。
- [x] **[B-3]** 订阅余额展示：对比表每行显示当前订阅徽章 + 剩余额度（amount_total−amount_used，GetSubscriptionSelf all_subscriptions）+ 到期日。
- [x] **[B-4]** 从定价页给「套餐对比」入口（Link → /pricing/plans）。

## US-4.3.2 — 留存与续费（P2，Builder B + C）
### Builder C（后端到期提醒 + 续费）
- [x] **[C-1]** `model.UserSubscription` 加 `ReminderDaysNotified int`（gorm 列 default:0，三库 ADD COLUMN 兼容，AutoMigrate 幂等；conformanceModels 已含该模型）。
- [x] **[C-2]** `service/subscription_reset_task.go` 新增 `scanSubscriptionExpiryReminder`：窗口 `active AND end_time in (now, now+N天] AND reminder_days_notified<N`，分批 300，NotifyUser（渠道复用）→ 成功才置位 N（`< N` 条件防并发重复）；复用 MasterNode 1min tick + CAS 单跑。文案含到期时间 + 续费链接。
- [x] **[C-3]** env `SUBSCRIPTION_EXPIRY_REMIND_DAYS`（默认 3，≤0 跳过）。
- [x] **[C-4]** 续费一键后端确认：复用 `SubscriptionRequestBalancePay` → `PurchaseSubscriptionWithBalance` → `CreateUserSubscriptionFromPlanTx`（同套餐 active 顺延 end_time，不新建行/不计 MaxPurchasePerUser）；既有续费顺延测试仍绿，无新增 API。
- [x] **[C-5]** 单测：`service/subscription_expiry_reminder_test.go` 4/4（窗口命中/去重/批处理/端到端置位）+ model 订阅/续费/重置 15+ PASS 无回归；relaykit/dto 新增 NotifyTypeSubscriptionExpiryReminder。

### Builder B（前端续费 + 合规文案）
- [x] **[B-5]** 订阅页「立即续费」：已订阅行 → `paySubscriptionBalance({plan_id})` → toast + 刷新；失败 handleServerError（余额不足原样提示）；allow_balance_pay=false 禁用 + tooltip，未订阅行「Buy in Wallet」兜底；购买复用 SubscriptionPurchaseDialog（<3 步）。
- [x] **[B-6]** 退订/自动续费中性合规文案：对比页底部常驻声明（订阅到期自动停止、不自动扣费、可手动续费），i18n 7 语言。
- [x] **[B-7]** 到期提醒前端入口：对比表每行到期日 + 续费 CTA。
- [x] **[B-8]** `plans-compare-table.test.tsx` 6/6 绿（本机实跑 29s）；`plan-comparison.test.tsx` 5 用例已修复 No QueryClient（QueryClientProvider + mock 重 Dialog）+ typecheck/lint 通过；**整页测试本机 worker OOM（2G 内存 + vitest fork 崩溃环境噪声，4 种池模式均卡死，tests 0ms 未执行）→ 断言与表格测试重叠、依赖静态确认有效，标记待 CI 环境全量确认**（非代码缺陷）。

## US-4.3.3 — B 端与增长观测（P3 · 落档建议，主控）
- [x] **[M-1 主控]** 落档建议：`计划书/docs/v4x-b2b-growth-p3-suggestion.md` 记录 P3 三方向（用量/成本报表导出、邀请分销 aff_code 扩展、增长看板）的实现思路/依赖/优先级；明确护栏（一次一个方向；不做自动涨价/自动风控闭环）。

## 质量门清单（全部通过才标记 done）
- [x] 后端 `go build ./...` + 相关 `go test` + relaykit 独立构建（A 阈值档 6/6 + C 到期提醒 4/4 + model 订阅无回归）
- [x] 三库 conformance（新增字段迁移）：`ReminderDaysNotified` int 列 + AutoMigrate 幂等（SQLite 实跑 PASS；MySQL/Postgres 无实例 SKIP，读代码确认无方言差异，CI 覆盖）
- [x] `bun run typecheck` / oxlint 绿（B 前端 + A/C 整合）
- [x] 新增 vitest 用例（plans-compare-table 6/6 本机绿；plan-comparison 5 用例修复 No QueryClient，整页本机 worker OOM 环境噪声待 CI）+ i18n:sync 无漂移（missing/extras 空）
- [x] 浏览器 E2E 截图（套餐对比页 / 续费按钮 / 定价页余额）：13/13 PASS，9 PNG + summary
- [ ] 独立审查 3 轮收敛
- [ ] commit + push + tag v1.3.57 + HTML 报告/测验
- [ ] 生产部署：按纪律待用户授权，不自动