# Feature Specification: 002 — 订阅套餐（天/周/月卡）+ 智能流量单位 + 站点权威统计

**Feature Branch**: `002-subscription-site-stats`
**Created**: 2026-09-22
**Status**: Draft → In Plan
**Input**: 用户需求（订阅系统 + 智能流量单位 + 排行榜 + 站点统计 + deepseek-v4-flash 入卡 + 上游地址泄漏排查）

> 依据项目宪法：生产优先、计费/资金安全不变量、三库兼容、前端体验/i18n、保守改动。

## User Scenarios & Testing

### User Story 1 — 订阅套餐（天/周/月无限卡 + 余额兑换）(Priority: P1)
用户可在订阅页购买「天卡无限（¥2）/ 周卡无限（¥25）/ 月卡无限（¥60）」，支持余额支付；
套餐说明展示微信 Tf00798（高并发定制）；购买成功自动升级到套餐绑定分组。

**Independent Test**：管理员建 3 个无限套餐 → 用户余额兑换天卡 → 断言 UserSubscription 生效、
分组升级、计划时间正确（24h / 7d / 30d）。

**Acceptance Scenarios**:
1. Given 平台开启订阅且 3 个无限卡套餐存在，When 用户用余额购买天卡，Then 生成 active 订阅 + 用户分组切到套餐组 + 前端订阅页显示到期时间。
2. Given 用户已有同套餐 active 订阅，When 再次购买，Then 按配置 MaxPurchasePerUser 限制或顺延，且不重复扣费/重复升级（幂等）。

### User Story 2 — 订阅档位限流（并发≈3 req/s + RPM 100~200，超限 429）(Priority: P1)
订阅用户按套餐档位获得并发/ RPM 上限；超限返回 429 与明确文案；管理员可对单个用户覆盖档位（升级 rpm/并发）。

**Independent Test**：为套餐设 concurrency=3、rpm=150 → 并发压测 >3 时第 4 个请求 429；
管理员对用户覆盖 rpm=300 后生效；未订阅用户走原有限流不变。

**Acceptance Scenarios**:
1. Given 用户订阅天卡（concurrency=3,rpm=150），When 同时发起 4 个请求，Then 第 4 个立即 429（rate_limit_exceeded / 订阅并发超限）。
2. Given 管理员把该用户档位覆盖为 rpm=300，When 该用户 1 分钟内第 151~300 个请求，Then 正常放行，>300 后 429。

### User Story 3 — 模型 × 套餐访问矩阵（deepseek-v4-flash 入卡）(Priority: P1)
管理员可在套餐上配置可用模型列表；订阅用户可访问套餐允许的模型，未订阅/模型不在套餐内被拒并给可读错误；
deepseek-v4-flash 默认挂入全部无限卡。

**Independent Test**：天卡套餐勾选 deepseek-v4-flash + gpt-4o-mini → 订阅用户请求 deepseek-v4-flash 成功、
请求未勾选模型被拒（403/insufficient_subscription）；未订阅用户请求 deepseek-v4-flash 按原有模型访问控制。

### User Story 4 — 管理员订阅管理（每个用户套餐 + 续费 + 档位覆盖）(Priority: P2)
管理端可查看每个用户的订阅套餐、起止时间、来源（order/admin）；可手动开通/续费/停用订阅；
可单独调整某用户的 rpm/并发覆盖；操作留审计日志。

**Independent Test**：管理端为用户 A 开通月卡（source=admin）→ 用户订阅页可见；
调整 A 的 rpm 覆盖 → 档位限流按覆盖值生效；停用 → active 订阅消失且分组回退。

### User Story 5 — 智能流量单位 + 日带宽排行 + 站点权威统计 (Priority: P1)
所有流量展示自动换算（B/KB/MB/GB/TB，1024 进制，2 位小数）；管理端有「每日网络带宽消耗排行」；
首页/关于页展示本站累计提供带宽、处理请求总数、token 总数等权威统计。

**Independent Test**：后端 FormatBytes(1_073_741_824) = "1.00 GB"；站点统计端点返回 total_bytes/
total_requests/total_tokens 与 consume log 聚合一致（抽样误差 <5%）；排行按日分组降序。

### User Story 6 — 发行版上游地址泄漏排查与整改 (Priority: P1)
审查已发布 Release 与仓库内是否存在上游渠道地址/密钥等敏感信息；泄漏则整改文档/产物并登记。

### Edge Cases
- 订阅到期/余额不足购买：明确错误 + 不扣费；已升级分组回退到 PrevUserGroup。
- 并发限流与既有全局并发/分组限流叠加：取最严生效，不绕过既有防线。
- 覆盖档位与套餐档位冲突：覆盖优先；取消覆盖回套餐值。
- 流量统计单位：0 字节 → "0 B"；负数不入库；跨 1024 边界取整规则一致（四舍五入 2 位）。
- 排行榜/统计在无日志时返回空数组/0，不报错；日志库为 ClickHouse 时 SQL 兼容（用 GORM 方法）。
- deepseek-v4-flash 未在任何套餐时：订阅页模型列表为空态 + 引导文案。

## Requirements

### Functional Requirements
- FR-001 订阅套餐 CRUD（标题/副标题/价格/币种/时长单位 day|week|month/额度 0=无限/允许余额支付/升级分组/模型列表/并发/RPM 档位）
- FR-002 周卡时长单位（新增 SubscriptionDurationWeek = "week"；时长计算 7*24h）
- FR-003 套餐级并发上限（concurrency）与 RPM 上限；订阅用户请求超限 429
- FR-004 管理员单用户档位覆盖（rpm/concurrency），覆盖优先级最高
- FR-005 模型×套餐访问矩阵（套餐 models 字段 + 订阅访问校验 + 可读错误）
- FR-006 余额兑换订阅（AllowBalancePay=true 时从钱包扣款生成订阅；幂等防重复）
- FR-007 购买/到期自动升级/回退分组（复用 UpgradeGroup/DowngradeGroup/PrevUserGroup）
- FR-008 管理端订阅管理（列表/开通/续费/停用/档位覆盖 + audit 留痕）
- FR-009 订阅说明文案位（微信 Tf00798 + 定制引导），前端订阅页与套餐卡片展示
- FR-010 智能流量单位：后端 `FormatBytes(int64) string`（B/KB/MB/GB/TB，1024，2 位小数）+ 前端共享 util 复用
- FR-011 站点权威统计端点：累计总带宽 / 总请求数 / token 总数（prompt+completion），首页展示
- FR-012 每日带宽消耗排行端点（按日分组、按带宽降序、LIMIT 分页）
- FR-013 deepseek-v4-flash 默认挂入天/周/月无限卡（种子数据/后台可改）
- FR-014 发行版泄漏排查登记 + 整改（README/Release 描述/示例配置去敏感化）

### Key Entities
- **SubscriptionPlan**（扩展）：+ Models json、ConcurrencyLimit int、RpmLimit int（新增列，三库迁移）
- **UserSubscription**（扩展）：+ RpmOverride int、ConcurrencyOverride int（0=未覆盖）
- **DailyTraffic / TrafficHuman**（服务层）：按日聚合 + 可读单位
- **SiteStats**（聚合查询结果）：total_bytes/total_requests/total_tokens

## Success Criteria
- SC-001 天/周/月三卡可从后台创建、用户余额购买、分组自动升级/回退，全链路单测+契约测试绿
- SC-002 并发>3 / RPM 超限请求 429 且文案可读；未订阅用户不受影响；既有限流不绕过
- SC-003 模型矩阵：订阅用户只可访问套餐内模型；deepseek-v4-flash 入卡后按预期放行/拒绝
- SC-004 FormatBytes 与前端 util 行为一致（B→TB）；站点统计与 consume log 抽样一致误差<5%
- SC-005 三库（SQLite/MySQL/PG）迁移与聚合查询通过 conformance；无回归
- SC-006 发行版泄漏排查有结论 + 整改登记

## Assumptions
- 复用既有订阅支付/账本/分组升级基建，不改动既有计费数值口径
- 并发/RPM 档位默认值：天/周/月卡 concurrency=3、rpm=150（用户口径：约 3 req/s、100~200 rpm），后台可改
- 站点权威统计口径：consume log 的 request_bytes+response_bytes 求和=总带宽；行数=请求数；prompt_tokens+completion_tokens 求和=token 总数
- 管理端为 root 管理员可用；单用户覆盖仅 root 可操作
- 发行版泄漏排查仅做仓库/Release 描述层面整改与登记，不回改已发布产物（历史版本只登记）
