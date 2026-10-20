# 钱相关链路只读审计（钱包/充值/订阅/兑换码/计费表达式）

审计日期：2026-10-12
范围：TopUp / SubscriptionPlan / UserSubscription / SubscriptionOrder / Redemption / 额度预警 / `pkg/billingexpr/`
性质：**只读审计**，未修改任何文件，未调用任何真实支付或充值接口。

---

## 结论一句话

核心记账正确性整体扎实（额度换算全部走 `common/quota_math.go`、回调有行锁 + 状态机幂等、兑换码用 CAS），未发现可直接算错钱的 P0；但发现 **1 个 P1 真缺陷（TOKENS 展示模式下 Stripe 充值额度按 `Money` 换算而 Stripe 定价用 `StripeUnitPrice`，两者口径不一致会系统性算错）**，以及若干 P2/P3（退款重试放大、TOKENS 模式最小充值被放大 50 万倍、订阅预警档位被钱包数据污染、Creem 补单口径不一致）。

---

## 问题清单

### P1-1 Stripe 充值：扣款用 `StripeUnitPrice`，到账额度用 `Money`，两套单价不一致

**问题**：Stripe 充值的实付金额按 `StripeUnitPrice` 计算，但写入本地订单 `Money` 的金额不含该系数，而到账额度按 `Money` 换算。三处口径互不一致。

**证据**
- `controller/topup_stripe.go:450` — 实付：`payMoney := amount * setting.StripeUnitPrice * topupGroupRatio * discount`
- `controller/topup_stripe.go:414-421` — `GetChargedAmount(count, user)` 只有 `count * topUpGroupRatio`（**不含** `StripeUnitPrice`）
- `controller/topup_stripe.go:103` — `chargedMoney := GetChargedAmount(float64(req.Amount), *user)`
- `controller/topup_stripe.go:123` — `Money: chargedMoney` 写入 `TopUp`
- `model/topup.go:269-271` — 到账：`quota = WalletQuotaFromDecimalStrict(decimal.NewFromFloat(topUp.Money).Mul(decimal.NewFromFloat(common.QuotaPerUnit)))`
- `controller/topup_stripe.go:423-431` — `getStripeCreditedQuota` 也是 `amount * topupGroupRatio * QuotaPerUnit`（不含 `StripeUnitPrice`）
- `setting/payment_stripe.go:6` — `var StripeUnitPrice = 8.0`（默认值**不是 1**）
- `controller/misc.go:108` — `"stripe_unit_price": setting.StripeUnitPrice` 已下发前端

**影响（会不会算错钱）**：**会**。管理员把 `StripeUnitPrice` 设成 ≠1 之后，用户实付 `amount*StripeUnitPrice`，到账额度却按 `amount*QuotaPerUnit`：
- `StripeUnitPrice = 0.5` → 实付 $5，到账 $10 额度（**平台翻倍送钱**）
- `StripeUnitPrice = 10` → 实付 $100，到账 $10 额度（**用户损失，投诉/退款风险**）
- `amount` 是用户可控整数（`StripePayRequest.Amount`），无上界约束以外的其它保护。

**复现方式（未执行，需谨慎）**：后台把 Stripe 单价设为 0.5 → 充值页选 10 → 走 Stripe 支付 → 到账额度是实付金额的两倍。
本地安全复现法：不发起真实支付，直接单测 `RequestPay` 的 `chargedMoney` 与 `RechargeEpay`/`Recharge` 的到账换算，断言 `实付金额 ↔ 到账额度` 的换算关系；分别用 `StripeUnitPrice = 0.5 / 1 / 10` 三组取值跑表测试。

**建议修法**：把三处统一到同一单价，二选一：
1. 到账改为 `amount * QuotaPerUnit`（忽略 `Money` 里的倍率），或
2. 把 `StripeUnitPrice` 一并折进 `Money`（即 `chargedMoney = amount * StripeUnitPrice * topupGroupRatio`）。
   **只选其一**，并补一条回归测试锁死「实付金额 ↔ 到账额度」换算关系，覆盖 `StripeUnitPrice ≠ 1`。

**状态**：静态确认（读代码确认）。因未发起真实支付，未做端到端验证。

---

### P1-2 TOKENS 展示模式下 Stripe 最小充值被放大 50 万倍

**问题**：`TOKENS` 展示模式下，Stripe 的 `min_topup` 被额外乘以 `QuotaPerUnit`（500000），而 Waffo / Waffo Pancake 都没有这一步，导致各支付渠道的最小充值单位不一致。

**证据**
- `controller/topup_stripe.go:454-460` — `minTopup = minTopup * int(common.QuotaPerUnit)`（仅在 `QuotaDisplayTypeTokens` 时）
- `common/constants.go:23` — `var QuotaPerUnit = 500 * 1000.0`
- `controller/topup_waffo.go:126` — `waffoMinTopup := int64(setting.WaffoMinTopUp)`，**无**该乘法
- `controller/topup_waffo_pancake.go:362` — 同样**无**该乘法
- `web/src/features/wallet/lib/payment.ts:153-175` — 前端 `getMinTopupAmount` 按渠道取各自 `min_topup`，Stripe 分支直接取 `topupInfo.stripe_min_topup`

**影响（会不会算错钱）**：不直接算错已支付金额，但会让**页面最小充值数量显示成 500000（被误读为 tokens）**，而实际语义是「tokens 单位下的 1 美元量」。用户按提示输入 500000 会支付对应金额，属**单位错位导致的误解与超额充值**。同一站点不同支付渠道给出不同量级的最小值，是配置口径缺陷。

**复现方式**：后台把 `quota_display_type` 设为 `TOKENS` → 充值页切到 Stripe → 最小充值显示 500000；同页切到 Waffo 显示 1。对比 `/api/user/amount` 与 `RequestAmount` 返回的校验阈值。

**建议修法**：与 Waffo 对齐，去掉该乘法；或明确定义 Stripe 在 `TOKENS` 模式下 `min_topup` 按 tokens 计并在后端统一换算。同时给 `getMinTopupAmount` 补一条多渠道一致性测试。

**状态**：静态确认。

---

### P2-1 订阅退款「嵌套事务」——`RefundSubscriptionPreConsume` 的退款可能被外层事务吞掉 / 重复执行

**问题**：`RefundSubscriptionPreConsume` 自开事务，内部又调用一个自开事务且带 `lockForUpdate` 的函数，构成嵌套事务；SQLite 下 `lockForUpdate` 是 no-op，而外层已持有写事务。外层自带的 `refundWithRetry`（3 次重试）无法区分「已退款但提交失败」与「未执行」。

**证据**
- `model/subscription.go:1607-1630` — `RefundSubscriptionPreConsume` 内 `DB.Transaction(...)`
- `model/subscription.go:1624` — 事务内调用 `PostConsumeUserSubscriptionDelta(record.UserSubscriptionId, -record.PreConsumed)`
- `model/subscription.go:1755-1776` — `PostConsumeUserSubscriptionDelta` **自身又开** `DB.Transaction` 并 `lockForUpdate` 读同一行
- `model/locking.go:20-25` — `lockForUpdate` 在 SQLite 上直接 `return tx`（无锁）
- `service/billing_session.go:143` — `needsRefundLocked` 依赖 `sub.preConsumed > 0`
- `service/funding_source.go:121-128` — `SubscriptionFunding.Refund` → `refundWithRetry(...)`，`service/funding_source.go:130-149` 最多 3 次重试，每次失败都会重放整个 `RefundSubscriptionPreConsume`

**影响（会不会算错钱）**：**可能会**。若内层事务因锁/嵌套失败而外层提交成功，或重试期间前一次退款实际已生效但返回错误，则 `PostConsumeUserSubscriptionDelta(-PreConsumed)` 可能被应用多次 → **订阅额度被多退**（用户白得额度）。SQLite 单写者模型下嵌套事务失败概率更高。MySQL/PG 下嵌套 `Begin` 语义依驱动而异（SAVEPOINT 或独立连接），跨库行为不确定。

**复现方式**：在 SQLite（`SQL_MAX_OPEN_CONNS=1` 最易复现）与 MySQL/PG 上分别构造「订阅预扣 → 请求失败触发 Refund」，并发/串行各跑一次，断言 `amount_used` 只回退一次。AGENTS.md 要求的三库矩阵必须覆盖。

**建议修法**：
1. 让 `PostConsumeUserSubscriptionDelta` 接受外部 `*gorm.DB`（tx 感知），避免嵌套开事务；
2. 或让 `RefundSubscriptionPreConsume` 不依赖内层事务，直接用同一 tx 做条件 UPDATE；
3. `refundWithRetry` 的适用前提是「事务型幂等退款函数」——当前实现并不满足幂等前提（详见 `service/funding_source.go:71-73` 对 `WalletFunding` 不做重试的注释），需按该注释的标准重新评估订阅路径。

**状态**：未验证（未跑三库矩阵）。属**合理推断 + 静态确认**，需实测定性。

---

### P2-2 订阅额度预警档位台账与钱包共用一个 `QuotaWarnedLevels` 字段

**问题**：订阅剩余额度的「已提醒档位」记录，与钱包剩余额度写进**同一个** user setting 字段。两者量纲完全不同（订阅 `AmountTotal` vs 钱包 `Quota`），会互相剔除记录。

**证据**
- `service/quota.go:620-668` — `checkAndSendSubscriptionQuotaNotify` 调用 `refreshQuotaWarnLevels(relayInfo, remaining)`
- `service/quota.go:537-555` — `refreshQuotaWarnLevels` 读写 `currentSetting.QuotaWarnedLevels`
- `service/quota.go:612` — 钱包路径调用**同一个** `refreshQuotaWarnLevels`
- `service/quota.go:500-525` — `decideQuotaWarn` 的剔除规则：`remaining < int64(warned)` 才保留记录

**影响（会不会算错钱）**：不直接算错钱，但会让**订阅额度预警对持有订阅的用户失效**。用户钱包剩余额度回升时会剔除订阅侧写入的档位记录（或反之），导致同一档位被重复提醒或永不提醒。属用户可感知的功能缺陷。

**复现方式**：构造用户同时持有钱包余额与活跃订阅，让钱包 remaining 在档位上下反复穿越，观察 `QuotaWarnedLevels` 被订阅路径覆盖后钱包档位是否丢失（或反之）。

**建议修法**：订阅与钱包各用独立字段（如新增 `SubscriptionQuotaWarnedLevels`），或在同一字段内按「来源」分区。

**状态**：静态确认。

---

### P2-3 `ManualCompleteTopUp` 对 Creem 订单按 `Amount * QuotaPerUnit` 到账，与正常回调 `Amount` 直接到账不一致

**问题**：管理员补单统一走非 Stripe 分支（`Amount * QuotaPerUnit`），但 Creem 正常回调是 `Amount` **直接**作为 quota（不乘 `QuotaPerUnit`）。Creem 订单一旦走补单，到账额度被放大 `QuotaPerUnit` 倍。

**证据**
- `model/topup.go:488-492` — 补单非 Stripe 分支：`WalletQuotaFromDecimalStrict(decimal.NewFromInt(topUp.Amount).Mul(decimal.NewFromFloat(common.QuotaPerUnit)))`
- `model/topup.go:558-562` — 正常 Creem 回调：`WalletQuotaFromDecimalStrict(decimal.NewFromInt(topUp.Amount))`（**不乘** `QuotaPerUnit`）
- `model/topup.go:627-629` — Waffo 正常路径**乘** `QuotaPerUnit`
- `model/topup.go:687-689` — Waffo Pancake 正常路径**乘** `QuotaPerUnit`
- `web/src/features/system-settings/integrations/creem-product-dialog.tsx:249` — 占位符 `e.g., 500000`，配合 `:253-255` 文案 "Amount of quota to credit to user account."，印证 Creem 的 `quota` 就是额度单位（**不是**美元）

**影响（会不会算错钱）**：**会**。Creem 订单经管理员补单时到账额度会是正常回调的 `QuotaPerUnit`（50 万）倍。例：产品 `quota=100000`，正常到账 100000，补单到账 100000 × 500000。

**复现方式**：配一个 Creem 产品（`quota=100000`）→ 创建 pending 订单 → 不触发 webhook，改走 `POST /api/topup/complete`（`router/api-router.go:164`）→ 对比到账额度。

**建议修法**：抽一个 `creditedQuotaForTopUp(topUp *TopUp) (int, error)` 统一分派——按 `PaymentProvider` 复用各自正常回调的换算函数，消灭「补单走一套、回调走另一套」的双实现。补一条覆盖全部 provider 的表测试。

**状态**：静态确认。

---

### P3-1 `redeem` 把全部失败原因吞成单一错误码

**证据**：`model/redemption.go:273-276` — 事务内所有错误（含「该兑换码已过期」「该兑换码已被使用」「兑换码关联的订阅套餐不存在」）统一 `return RedeemResult{}, ErrRedeemFailed`，仅 `common.SysError` 记一行。

**影响**：不涉及算错钱，属可诊断性/体验问题。前端拿不到可区分的原因。

**建议修法**：定义哨兵错误（`ErrRedeemExpired` / `ErrRedeemUsed` / `ErrRedeemPlanUnavailable`），用 `errors.Is` 向上传递。**注意**：这是既有设计，改动会影响 `model/redemption_test.go:220` 的断言，需同步调整。

**状态**：静态确认。

---

### P3-2 前端预览价格未含充值分组倍率

**证据**
- `web/src/features/wallet/lib/format.ts:84` — `originalPrice = presetValue * priceRatio`
- `web/src/features/wallet/index.tsx:321` — `priceRatio={(status?.price as number) || 1}`（来自 `/api/status` 的 `operation_setting.Price`）
- 后端 `controller/topup.go:177` — `payMoney = dAmount * dPrice * dTopupGroupRatio * dDiscount`（**多一个** `TopupGroupRatio`）
- `controller/misc.go:107` — `/api/status` 只下发 `Price`，**未下发** `TopupGroupRatio`

**影响**：`TopupGroupRatio[group] ≠ 1` 的用户，卡片上的预设价会低于最终支付价。因为是**选中支付方式后被 `/api/user/amount` 覆盖**，属短暂显示不一致，最终金额由后端计算，**不会真正算错钱**。

**建议修法**：把 topup group ratio 纳入前端预览，或明确接受该短暂不一致并在 UI 上标注「以确认页为准」。

**状态**：静态确认（未起浏览器验证渲染）。

---

## 我核对过并确认正确的

1. **额度换算未违反 AGENTS.md 硬要求**：全仓库未见 `int(float64(quota) * ratio)` / `int(math.Round(...))` / `int(decimal.IntPart())` 之类把额度裸转型的写法。
   - `pkg/billingexpr/settle.go:38` 用 `common.QuotaRoundChecked`（并回传 clamp 供审计）
   - `service/task_billing.go:425` 用 `common.QuotaFromFloatChecked`
   - 钱包/充值统一 `common.WalletQuotaFromDecimalStrict`
   - `common/quota_math.go` 集中了饱和 + `SysError` 日志策略
2. **充值回调幂等**：`RechargeEpay`（`model/topup.go:176-233`）、`Recharge`（`:235-289`）、`RechargeCreem`（`:524-594`）、`RechargeWaffo`（`:596-654`）、`RechargeWaffoPancake`（`:656-714`）**全部**在事务内 `lockForUpdate(tradeNo)`（PostgreSQL 用 `"trade_no"` 引号分支）+ 状态机：`success` 直接返回（幂等），非 `pending` 报 `ErrTopUpStatusInvalid`，且校验 `PaymentProvider` 防跨网关回调。到账统一走 `creditTopUpQuota`（`model/topup.go:89-117`）——谓词与增量在同一条 UPDATE 中，原子守钱包上限。**多实例部署安全**。
3. **订阅订单幂等**：`CompleteSubscriptionOrder`（`model/subscription.go:658-738`）同样行锁 + `success` 幂等；`:697` 额外锁用户行，让 `MaxPurchasePerUser` 按用户串行。`:685-687` 注释说明了事务内必须用 `tx` 读 plan（SQLite 单连接死锁）。`ExpireSubscriptionOrder`（`:776-799`）仅 `pending` → `expired`。
4. **兑换码并发安全**：`Redeem`（`model/redemption.go:156-285`）两级 CAS：
   - 一次性码：`WHERE id=? AND status=enabled` 更新为 `used`，检查 `RowsAffected`；
   - 多用户码：`WHERE id=? AND status=enabled AND (max_uses - (SELECT COUNT(*) ...)) > 0`，并在事务内校验「同一用户对同一码仅一次」。
   - 事务外 `syncCreditUserQuotaCache` 处理缓存一致性（`:282`）。
5. **锁用法合规**：全仓库 grep 无 `Set("gorm:query_option", "FOR UPDATE")`；统一走 `model/locking.go:20-25` 的 `lockForUpdate(tx)`（SQLite 自动跳过，MySQL/PG 发 `FOR UPDATE`）。`model/db_conformance_test.go:456` 有专门的一致性测试。
6. **不存在支付退款/拒付入口**：除额度预扣回退（`service/billing_session.go:84-125`，`s.refunded` 幂等）与任务失败退款（`service/task_billing.go:216-273`，靠 `task.Quota=0` + CAS 防重）外，**无任何支付网关退款/拒付处理路径**。因此「退款会重复退还」这一类别在本项目中**不适用于真实支付订单**。
7. **删套餐不留悬空引用**：`AdminDeleteSubscriptionPlan`（`model/subscription.go:1117-1166`）先写 `SubscriptionPlanGrant` 配置快照再删套餐行；`GetPlanForSubscription`（`:1706-1728`）在套餐行缺失时回落快照。订阅按原 `end_time` 自然结束，权益不延长也不剥夺。快照被 `middleware/subscription-rate-limit.go:101`、`service/rate_limit_resolve.go:19`、`service/subscription_access.go:86` 正确消费。
8. **券码创建校验完整**：`Redemption.Insert`（`model/redemption.go:287-307`）、`Update`（`:315-335`）、`controller/redemption.go:94-103` 都做了 `Quota > 0` + `common.ValidateWalletQuota` + 关联套餐存在且启用；`GetMaxUses` 非负校验在 `controller/redemption.go:90-93`。
9. **订阅预扣记录幂等**：`SubscriptionPreConsumeRecord`（`model/subscription.go:1442-1452`）`request_id` 唯一索引，`PreConsumeUserSubscription`（`:1503-1604`）先查后建、唯一冲突时回读兜底（`:1572-1586`），`refunded` 状态不可重复预扣。
10. **额度管理员调整**：`AdjustUserQuota`（`model/user_quota_adjustment.go`）行锁 + `WalletQuotaFromDecimalStrict` + 差值同步缓存，权限校验 `operatorRole <= user.Role` 拦截。往返饱和被拒（`:33-35`、`:46-48`）。

---

## 未能验证的

1. **P1-1 的实际现网行为**：授权红线禁止发起真实 Stripe 请求，只能静态确认三处单价口径不一致。**需要 `StripeUnitPrice ≠ 1` 的本地 mock 实测**才能确定翻倍/缩水方向与幅度（默认值是 8.0 而非 1，尤其需要实测）。
2. **P2-1 的跨库行为**：未运行 SQLite / MySQL / PostgreSQL 三库矩阵，嵌套事务在各驱动下的实际提交/回滚语义未验证。按 AGENTS.md 的 DB 兼容要求，这是必须补齐的验证项。
3. **现网 `quota_display_type` / `TopupGroupRatio` 取值**：本地 `one-api.db` 中检索不到相关键（`quota_display_type`、`TopupGroupRatio`、`general_setting`、`AmountOptions` 均未命中），说明该库不是当前运行配置库，**无法判断 P1-2、P3-2 在现网是否已触发**。
4. **前端渲染**：未启动浏览器，P3-2 的显示不一致由代码路径推断，未做真实用户路径截图验证。
5. **`StripeUnitPrice` 的配置入口**：未在本次审计中定位到该变量的后台可编辑入口位置（`controller/misc.go:108` 只证明它会被下发），无法确认是管理员可在 UI 修改还是需改配置/环境变量。

---

## 审计纪律声明

- 全程**未修改任何文件**。
- **未调用任何真实支付/充值接口**，未发起任何外部付费请求。
- P1/P2 的定性均以 `file:line` 为证据；凡未经实际运行验证的结论已明确标注「静态确认」或「未验证」，未宣称「已修复」或「无 bug」。

---

## 复核（2026-10-20，HEAD `2bf6aa364`）

| 项 | 复核结论 | 证据 |
|---|---|---|
| P1-1 Stripe 单价口径 | **误报，维持现状**：`StripeUnitPrice` 的语义是「本币 / USD」汇率，不是应计入到账额度的系数。后台字段文案写明 `Unit price (local currency / USD)`、`e.g., 8 means 8 local currency per USD` | `web/src/features/system-settings/integrations/payment-settings-section.tsx:1382-1396`；`getStripePayMoney` 用它把「USD 额度」换算成实付本币，`Money` 记录请求额度、到账 = `Money × QuotaPerUnit`，三者自洽 |
| P1-2 TOKENS 模式最小充值 ×QuotaPerUnit | **误报，维持现状**：TOKENS 展示下 `amount` 的单位就是 token，`min_topup`（USD）换算成 token 才是同一单位；`getStripePayMoney` 同样先除 `QuotaPerUnit`。删掉乘法反而会让最小充值变成「1 token」 | `controller/topup_stripe.go:433-460`；`setting/operation_setting` 的 `quota_display_type` |

教训：审计条目必须回到**渲染该字段的组件**确认语义（见记忆 `ui-label-authority-is-the-component`），不能只看变量名推断单位。
