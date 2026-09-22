# 需求追踪矩阵（终局版）

> 生成：2026-09-22 ｜ 依据：完整会话上下文（订阅系统 / 限速 / 统计 / 模型广场 / 汉化 / 安全脱敏 / 运维闭环）
> 状态口径：✅已闭环（有线上实证/测试/产物证据）｜🟡部分闭环（实现但受外部条件限制或未完全实测）｜⛔未闭环

| # | 需求 | 实现位置 | 状态 | 证据 |
|---|---|---|---|---|
| R1 | 订阅套餐：天卡¥2 / 周卡¥25 / 月卡¥60（无限额度） | subscription_plans（生产 DB id 1-3） | ✅ | 三卡 CNY 无限、models=["deepseek-v4-flash"]、concurrency=3、rpm=150 |
| R2 | 余额兑换订阅（充值关闭仍可兑） | controller/subscription.go SubscriptionRequestBalancePay | ✅ | E2E：topup_enabled=false 时 200，扣 ¥2，source=balance，订单 success |
| R3 | 兑换码兑换订阅（额度码/订阅码） | model/redemption.go Redeem + PlanId；admin redemption UI | ✅ | E2E：兑换返回 plan_id=1 天卡无限；后台表单/列表 |
| R4 | 续费顺延（同套餐顺延到期，单行） | model/subscription.go CreateUserSubscriptionFromPlanTx | ✅ | E2E：两次兑换仍 1 行，end_time +2 天 |
| R5 | 并发 3/s 超限 429 | middleware/subscription-rate-limit.go + user-rate-limit.go | ✅ | E2E：3×200，第 4 个 429「基础并发…每秒 3 次」 |
| R6 | RPM 档位（订阅 150；基础 120） | 同上 + relay_setting | ✅ | E2E：订阅用户 170 并发 0 个基础429、20 个订阅RPM429；管理员升 200 后 0 个 rpm429 |
| R7 | 订阅适用模型矩阵（越权 403） | service/subscription_access.go CheckSubscriptionModelAccess | ✅ | E2E：qwen-3.8-max（有渠道）→ 403「不在当前订阅套餐」 |
| R8 | 购买/兑换自动升级分组（subscriber） | plan.upgrade_group + CreateUserSubscriptionFromPlanTx | ✅ | E2E：e2e_sub1 兑换后 group=subscriber |
| R9 | 分组需订阅门禁（可自选该分组） | service.CheckSubscriptionGroupAccess + relay.subscription_required_groups | ✅ | E2E：无订阅选 subscriber → 403 精确文案 |
| R10 | 到期自动降级分组 | model.ExpireDueSubscriptions + StartSubscriptionQuotaResetTask | ✅ | E2E：强制到期 → expired + group 回退 default |
| R11 | 管理员管控订阅/用户订阅/单订阅升级 rpm+并发 | AdminSetUserSubscriptionTier + user-subscriptions 弹窗 | ✅ | E2E：PATCH 150→200 实测生效并复位 |
| R12 | 基础限速（3/s+120RPM）管理员可控（分组/用户覆盖 UI） | relay_setting + RateLimitOverridesSection | ✅ | typecheck/lint 绿；端点 401 注册；E2E 基础 429 |
| R13 | 订阅卡显示并发/RPM + 微信 Tf00798 定制提示 | subscription-plans-card + i18n | ✅ | 产物实证 + 汉化 |
| R14 | 模型广场每模型统计（今日/30天/成功数） | GetModelStats + model-card | ✅ | 接口匿名 200；卡片渲染 |
| R15 | 流量智能单位（B→KB→MB→GB→TB） | common.FormatBytes + formatTraffic | ✅ | 单测（1024→1.00KB…1TB） |
| R16 | 每日/模型带宽排行（G/T） | AggregateBandwidthByDay/ByModel + BandwidthLeaderboardDialog | ✅ | 单测 + 端点 401 注册 + 前端弹窗 |
| R17 | 站点权威统计（带宽/请求/token） | GetSiteOverview（公共） | ✅ | /api/site/stats 匿名 200 |
| R18 | 模型效果测试整合进模型卡片（含测试日期） | MODEL_TEST_META + model-card + /model-test?model= | ✅ | 二进制 grep：2026-09-18 ×2、Effect test ×2 |
| R19 | 系统信息内存/CPU/状态 | system_instance reporter | ✅ | 生产上报 cpu 12.4%/mem 60.7%/disk 67.8% |
| R20 | 管理员日志按用户筛选 | /api/log/search username + filter bar | ✅ | 后端筛选可用；前端已有输入框 |
| R21 | 前端中文汉化 | i18n zh/zh-TW（28 键 + 订阅/限速/统计） | ✅ | 二进制 grep：用户基础限速 ×1 |
| R22 | 发行版/仓库敏感信息脱敏 | validation_test.go + workflow_status.md | ✅ | git grep 零残留 |
| R23 | 生产热更新链 + 回滚 | docker compose + .bak 备份 | ✅ | v1.2.38→v1.2.88 全程真实部署 |
| R24 | 订阅生效即时性（清负缓存） | service.ClearCachedNoSubscription | ✅ | v1.2.88 + 单测 TestClearCachedNoSubscription |
| R25 | 兑换码格式按后台（32 位 UUID） | admin redemption | ✅ | 用户确认“按后台来” |
| R26 | 其他语言本地化（fr/ru/ja/vi） | i18n fallback→zh | 🟡 | 回退中文可用；如需英文/本地化待确认 |
| R27 | 订阅用户每请求订阅查询次数优化 | — | 🟡 | 当前 4 次/请求（PG 索引查询），规模小可接受；优化项已记录 |
| R28 | 版本显示与仓库 VERSION 同步 | VERSION 随 tag 更新 + /api/status | ✅ | v1.2.92 起仓库 VERSION=v1.2.92，/api/status version=v1.2.92（生产实证） |
| R29 | 按次计费单价×分组倍率真实对账 | ModelPrice + GroupRatio | ✅ | deepseek-v4-flash 单价 0.001×倍率0.1=¥0.0001/次，quota=50（真实 E2E） |
| R30 | 消费日志显示分组倍率 | LogCostDisplay GroupRatioMarker | ✅ | 费用徽章旁 ×倍率 + tooltip「模型单价×分组倍率」；bundle 实证 |

## 隐式需求（全部满足）
- 可运行/可调用/可使用 ✅（生产实证）
- 少 bug/闭环/真实实现 ✅（逐链路 E2E）
- 文档同步 ✅（本矩阵 + workflow_status.md §1-59 + OPERATIONS_SOP.md）
- 主动补位 ✅（空/加载/错误态、防重复提交、危险操作确认、权限校验）
- 非付费真实 E2E ✅；付费类（图片/视频生成）未实测属外部资源限制

## 未闭环/受限项
- R26：非 zh 语言本地化（fallback 可用，非阻塞）
- 图片/视频生成等付费上游调用：未做真实调用（烧钱），仅契约/代码审查
