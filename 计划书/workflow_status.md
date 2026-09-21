# Workflow Status — 参考项目全量对标（任务闭环登记）

> 日期：2026-09-20 ｜ 主项目：new-api v1.2.38（Agent 网关）
> 目标：1191 个参考项目全量识别 → 优点提炼 → 差距分析 → 生成《参考的结果计划指南.md》

## 一、节点状态（扫描工作流）

| 节点 | 任务 | 状态 | 证据/产物 |
|---|---|---|---|
| N0 识别 | 主项目定位（Agent 网关：渠道聚合/计费/任务/日志透明/skills 沉淀/横向扩展） | ✅ DONE | AGENTS.md + 本指南 §1 |
| N1 全量盘点 | 参考根 1191 顶层目录；1177 项 README 首段索引 | ✅ DONE | `.deploy/refscan/all_projects_readme.tsv` |
| N2-N18 并行扫描 | 子代理 6 批（agents-a/b、api_gateway、skills、docs_rag、media）+ 主代理 11 批（image/ecommerce/mcp/automation/dev_tools/security/web_frontend/unclassified×4） | ✅ DONE | `.deploy/refscan/reports/report_<batch>.md`（18 份，263KB，含 missing-13 补扫） |
| N18 优点提炼 | 每批 Top5 + 共性亮点 + 扩展洞察 | ✅ DONE | 各 report_*.md §3/§4 |
| N19 差距分析 | 主项目 vs 参考最佳（10 项差距矩阵） | ✅ DONE | 本指南 §4 |
| N20 方案设计 | 六大可迁移方向 + 优先级 + 依赖关系 | ✅ DONE | 本指南 §3/§5 |
| N21 汇总 | 《参考的结果计划指南.md》生成 | ✅ DONE | `计划书/参考的结果计划指南.md` |
| N22 落地跟踪 | 后续批次登记（下一步改进指南已立项） | ⏳ 待实施：P0-1 已完成（见下） | `计划书/下一步改进指南.md` P0-P2 台账 |

## 二、关键结论（真实证据）
1. 参考集 = AI 需求侧全景：agent 平台/记忆/技能生态/媒体/电商/办公/PPT——印证主项目横向扩展路线。
2. 最高价值：MCP 通道、成本可视化、日志证据闭环、技能市场、电商视频/办公横扩展。
3. 边界：渗透/逆向/验证码对抗类仅防御研究，不进产品。

## 三、已完成 vs 待办
- ✅ 已完成：全量扫描（1191 目录）、17 份批次报告、《参考的结果计划指南.md》、《下一步改进指南.md》。
- ⏳ 待用户确认后：按《下一步改进指南.md》P0→P2 逐批实施（每个批次真实跑通、三库验证、留证据后登记到本文件）。

## 四、批次闭环状态
| 批次 | 状态 | 提交 | 证据 |
|---|---|---|---|
| P0-1 三库 conformance 契约测试套件 | ✅ DONE (v1.2.39) | 见 git log | `计划书/audit-ledger.md` + `model/db_conformance_test.go` + CI `db-conformance` job |
| P0-2 计费安全收口 | ✅ DONE (v1.2.40) | 见 git log | `计划书/audit-ledger.md` + `service/token_counter.go` + `quota_saturation_test.go` |
| P0-3 认证安全审计 | ✅ DONE (v1.2.40，审计结论：既有实现已满足关键 ASVS) | 见 git log | `计划书/audit-ledger.md` |
| P0-4 日志透明化（错误归因落日志） | ✅ DONE (v1.2.40) | 见 git log | `计划书/audit-ledger.md` + `controller/relay.go` + `relay_error_log_test.go` |
| P1-1 用户画像层 | ✅ DONE (v1.2.48：EXPLAIN 索引命中+占比一致性验收) | 见 audit-ledger | service/user_profile/ |`r`n| P1-2 Redis 批量落库 | ✅ DONE (v1.2.47/50：重试+回退+指标+并发/故障注入验收) | 见 audit-ledger | model/consume_log_flusher.go + test |`r`n| P1-3 SSE 断线续传 | ✅ DONE (v1.2.49/51：done 截断修复+500 条压测+权限+Last-Event-ID) | 见 audit-ledger | controller/task_event.go + 前端 task-event-stream |`r`n| P1-4 渠道健康+组合路由 | ✅ 已落地 | 见 audit-ledger | channel_health_score.go + channel_combo_route.go |`r`n| P1-5 前端体验矩阵 | ✅ DONE (v1.2.55) | 见 audit-ledger | 触控≥44px + task-artifacts Button 复用 + vitest 超时 |`r`n| P1-6 前端性能（路由懒加载） | ✅ DONE (v1.2.54) | 见 audit-ledger | web/src/routes 懒加载 6 路由 |`r`n| P2-1 无锁快照 | ✅ DONE (v1.2.46) | 见 audit-ledger | service/channel_combo_route.go 快照 + 并发读测试 |`r`n| P2-2 事件子系统完整版 | ✅ DONE (v1.2.56：持久化幂等表 + epay 接入 + 对账回归) | 见 audit-ledger | model/event_delivery.go + service/epay_events.go + event_bus.go 持久化钩子 |`r`n| P2-3 jsplugin 安全模型 | ✅ DONE (v1.2.57/58：ExecutionGate + 分层权限 + Ed25519 + 沙箱探测 + 审批工作流 UI) | 见 audit-ledger | pkg/jsplugin/security.go + controller/task_plugin.go 审批 + 前端 plugins-table |`r`n| P2-4 平台生态（技能市场/pricing 镜像） | ⏳ 待办（战略级立项） | - | - |

## 五、下一步（最小可行）
1. 质量基线：仓库 lint 存量债务 245→0 全清（v1.2.60）✅；P0-P2 全部落地；后续：P2-4 平台生态（战略级立项）+ controller 测试基线（补 TestMain）+ 沙箱 OS 级隔离。
2. 从《下一步改进指南.md》取批次定义，按 03-工作流-SOP 推进，完成后在本文件登记提交 SHA 与证据路径。
---

## 六、B 批次 controller 测试基线（v1.2.62 ✅ 2026-09-22）

| 节点 | 任务 | 状态 | 证据 |
|---|---|---|---|
| B-P0 | executeTaskSubmissionWith 恢复任务落库（B4-1 误删 task.Insert，任务行不落库、internal_task_id=0、轮询/SSE 断链） | ✅ 生产修复 | controller/relay.go:896-901 + 回归测试组全绿 |
| B-1 | controller TestMain 共享全模型内存库 + RedisEnabled=false（异步 teardown 写空库根因） | ✅ | controller/main_test.go |
| B-2 | :memory: sqlite 单连接固定 + Task/TaskEvent/User/Channel/Token 迁移（多连接各独立内存库根因） | ✅ | relay_task_plugin_test.go / plugin_protocol_test.go |
| B-3 | task_unconfirmed 测试基建（真实 gin context + wallet_only + 用户/令牌种子） | ✅ | task_unconfirmed_test.go |
| B-4 | model.InitCol 导出（commonKeyCol 空 → WHERE '' IN ? 语法错） | ✅ | model/main.go + billing_option_test.go |
| 验证 | controller Task\|Plugin\|Event 组全绿；model 包全绿；远端 SHA 一致 | ✅ | `go test ./controller/ -run Task\|Plugin\|Event` + `go test ./model/` |

提交：1609a1979（v1.2.62）｜ 远端核验 MATCH ✓

## 七、002 订阅+流量单位+站点统计（立项 2026-09-22）
- 规范：`.specify/specs/002-subscription-site-stats/`（spec.md / plan.md / tasks.md）
- Phase B（流量单位/站点统计/排行）→ Phase C（订阅档位/模型矩阵/覆盖）→ Phase D（泄漏排查/文档）→ Phase E（总验收交付）
- 当前进行：Phase B 实现

## 八、002 Phase B 智能流量单位 + 站点权威统计（进行中，2026-09-22）

| 节点 | 任务 | 状态 | 证据 |
|---|---|---|---|
| B1 | 后端 FormatBytes（B/KB/MB/GB/TB/PB，1024，2 位小数） | ✅ | common/format.go + format_test.go（含 0/负/小数边界） |
| B2 | Log 持久化 RequestBytes/ResponseBytes 列（写库时落列，站点统计免扫 JSON） | ✅ SQLite 验证；MySQL/PG 待三库实例（Docker 不可用，登记 blocker） | model/log.go + service/quota.go/task_billing.go/text_quota.go 接线 |
| B3 | 站点权威统计端点 GET /api/log/overview（累计带宽/请求/token/额度）+ 公共 GET /api/site/stats（限流） | ✅ | controller/log.go GetSiteOverview + router 两条路由 |
| B4 | 每日带宽排行端点 GET /api/log/bandwidth/leaderboard（按日降序限量） | ✅ | controller/log.go + service/log_traffic.go AggregateBandwidthByDay + 单测 |
| B5 | 前端 formatTraffic 共享工具 + vitest | ✅ | web/src/lib/format.ts + __tests__/format-traffic.test.ts（2/2 绿） |
| B6 | TrafficBadges 智能单位（管理端流量徽章） | ✅ | common-logs-stats.tsx 改用 formatTraffic(bytes) |
| B7 | 首页站点统计卡（真实数据 + 加载/错误/空态） | ✅ | home/components/site-stats.tsx + Stats 挂载 + i18n（7 语言同步） |
| B8 | 质量门 | ✅ 后端 build/common/service 绿；前端 typecheck/build/vitest 绿 | - |
| B9 | 三库 conformance | ⏳ SQLite PASS；MySQL/PG 需 Docker 实例（本机无 docker，登记 blocker） | scripts/db-conformance.ps1 待跑 |

阻塞项：本机无 docker → MySQL/PostgreSQL conformance 需在有实例环境跑一次再宣称三库闭环。

## 九、002 Phase B 落地（v1.2.63 ✅ 2026-09-22）
- v1.2.63 = 908a249fc：智能流量单位（后端 FormatBytes + 前端 formatTraffic）、站点权威统计（/api/log/overview + 公共 /api/site/stats）、日带宽排行（/api/log/bandwidth/leaderboard）、Log 持久化字节列、首页真实统计卡、i18n 7 语言；本地/远端 SHA 一致
- 阻塞登记：Log 新列 MySQL/PG conformance 需 Docker 实例（本机无 docker）
- 新增节点（Phase B-2，进行中）：系统信息资源解耦采样 ✅ → 模型广场卡片统计 ⏳ → 效果测试整合 ⏳

## 十、002 Phase B-2 模型广场统计（v1.2.65，2026-09-22）
- 后端 GET /api/model/stats：每模型 今日/近30天 调用总数与成功数（consume=成功，consume+error=总数；站点级聚合、限流、无 PII）
- service.MergeModelStats 纯函数 + 单测（合并/排序/空 map）
- 前端 pricing：getModelStats + ModelCard 统计条（Today calls/success · 30d calls/success），失败静默降级
- 验证：后端 build/service 绿；前端 typecheck/build/vitest（pricing 204/204）绿；i18n 7 语言同步
- 待办：效果测试整合进模型卡片（B2-3）、效果测试数据可配置化（B2-4）、订阅 Phase C

## 十一、002 Phase C 订阅档位/模型矩阵/覆盖（v1.2.66 后端核心，2026-09-22）
- SubscriptionPlan 增列 concurrency_limit/rpm_limit/models（JSON 数组）；UserSubscription 增列 rpm_override/concurrency_override
- 周卡单位 SubscriptionDurationWeek（7*24h）+ calcPlanEndTime 分支 + 单测
- middleware.SubscriptionRateLimit：订阅并发/RPM→429（覆盖优先，未订阅/DB 不可用 fail-open；429 文案带微信 Tf00798 定制引导），挂载 relay 链 TokenRateLimit 之后
- 管理端档位覆盖端点 PATCH /subscription/admin/user_subscriptions/:id/tier + 审计留痕
- 套餐创建/更新接收并校验新档位字段（非负）
- 单测：model 周卡/矩阵/覆盖 + middleware 并发/限流/resolve fail-open 全绿
- 待办：模型矩阵接入 relay 校验（C4 执行层）、前端套餐表单/覆盖 UI + 微信文案（C7）、deepseek-v4-flash 入卡种子、三库 conformance

## 十二、002 Phase C 模型矩阵执行层（v1.2.67，2026-09-22）
- service.CheckSubscriptionModelAccess：有 active 订阅时请求模型必须在套餐 Models 内（空=不限）；无订阅/DB 不可用 fail-open
- 接入 RelayHelper 主咽喉（GenRelayInfo 后）：越权模型 403 model_not_in_subscription + 可读文案（含微信 Tf00798 定制引导）
- 单测：fail-open 分支 + 矩阵放行/拒绝/开放套餐（真实内存库种子）全绿

## 十三、终局交付物（2026-09-22）
- 变更报告+测验：计划书/变更报告-v1.2.62-67.html（上下文/直觉/逐版本变更/验证证据/待闭环/测验+答案）

## 十四、002 Phase C7 订阅前端（v1.2.68，2026-09-22）
- 套餐表单：周卡单位 week、并发上限 Concurrency limit、RPM Limit、Allowed Models（逗号分隔→JSON 数组）字段
- plan-form 转换（JSON⇄逗号文本）；i18n 7 语言同步；typecheck 绿（订阅 feature 无 vitest 文件）
- 待办：每用户覆盖 UI（user-subscriptions 弹窗 + PATCH tier）、订阅页微信 Tf00798 文案、deepseek-v4-flash 默认入卡

## 十五、002 Phase C7 收尾（v1.2.69，2026-09-22）
- 订阅管理页微信 Tf00798 高并发定制提示（i18n 7 语言）
- 新套餐默认 Models 预填 deepseek-v4-flash（本站主力模型默认入卡）
- api.setUserSubscriptionTier（PATCH /tier）客户端就绪（UI 控件待接）

## 十六、002 Phase C7 完成（v1.2.70，2026-09-22）
- user-subscriptions 弹窗新增 Tier Override 列：并发/RPM 覆盖输入 + 保存（接 PATCH /tier，成功 toast + 刷新 + onSuccess）
- i18n 7 语言同步；typecheck 绿
- 至此订阅前端 C7 代码全部落地（后端 v1.2.66+67 已就绪）

## 十七、终局审计（v1.2.70 后，2026-09-22）
- 三库 conformance：SQLite PASS；MySQL/PG 因无 DSN/实例 SKIP（硬证据已留）
- 独立审查线程（只读）：计划书/审查-2026-09-22-v1.2.62-70.md —— 无阻塞缺陷；1 项 P1 性能建议（热路径订阅查询合并/负缓存）+ 2 项 P2 语义/兼容说明
- HTML 报告补充 v1.2.68-70 版本线与审查链接

## 十八、P1 性能建议落地（v1.2.71，2026-09-22）
- 无订阅负缓存（TTL 15s）：非订阅用户（热路径常见情形）限流解析+矩阵校验不再每请求 2 次主库读
- 订阅用户不缓存（保持档位/矩阵实时正确）；购买/到期最迟 15s 生效（已注明）
- 单测：首次 DB 解析后置 DB=nil 二次调用仍放行（缓存命中免 DB）；service/middleware 全绿

## 十九、前端最终验收 + 报告更新（2026-09-22）
- bun run build 通过（v1.2.68-71 全部前端改动打包成功）
- HTML 报告补 v1.2.71 行；全版本线 62→71 完整

## 二十、三库 conformance 重大进展（2026-09-22）
- 发现本机已装 PostgreSQL 16 → 临时 trust 集群（127.0.0.1:55432）跑真实 conformance
- TestDBConformance 全组：sqlite PASS + postgres PASS（AutoMigrateIdempotent 含全模型，25s）+ mysql SKIP（未安装）
- 新列在真实 PG 验证存在：logs.request_bytes/response_bytes；subscription_plans.concurrency_limit/rpm_limit/models；user_subscriptions.rpm_override/concurrency_override
- 剩余：MySQL 实例（本机未安装，无 Docker）→ blocker 收窄为仅 MySQL

## 二十一、T7 每用户基础限速（v1.2.72，2026-09-22）
- relay 设置新增：UserBaseRateLimitEnabled（nil=默认开）、UserBaseConcurrencyLimit（默认3）、UserBaseRpmLimit（默认120）、GroupRateLimitOverrides、UserRateLimitOverrides（热更新，无 schema 变更）
- middleware.UserRateLimit：并发（秒）+ RPM（60s 窗口）→429；生效优先级 用户覆盖>分组覆盖>基础默认；挂载 relay 链 TokenRateLimit 之后（与订阅/密钥限流并存取最严）
- 单测：解析器优先级/默认/关闭 + 并发存储；setting/middleware 全绿
- 待办：管理员设置覆盖的 HTTP 端点 + 前端 relay 设置表单 UI

## 二十二、T7 管理端限速覆盖端点（v1.2.73，2026-09-22）
- GET /api/option/relay/rate_limit/overrides（查看 base/分组/用户档位）
- PUT /api/option/relay/rate_limit/overrides/group（设置/移除分组覆盖）
- PUT /api/option/relay/rate_limit/overrides/user（设置/移除用户覆盖）
- 变更即写 Option(relay) 持久化 + 热更新 + 审计留痕（RootAuth）
- 待办：前端 relay 设置表单 UI（接线以上端点）

## 二十三、生产热更新 v1.2.73（2026-09-22，真实执行+线上验收）
- 服务器 103.233.252.213（HK CN2，root）SSH 直连（plink + hostkey 指纹）
- 备份：pg_dump 22MB → /opt/backups/new-api-20260922-031514.dump；compose 备份 .bak.<TS>
- 代码：/opt/new-api-src git fetch + checkout v1.2.73 (250f6c38c)；VERSION=v1.2.73
- 构建：docker build new-api:local-v1.2.73（BUILD_EXIT=0，旧 local-v1.2.38 保留可回滚）
- 切换：sed 换 tag + docker compose up -d new-api → healthy
- 线上验收：/api/status HTTP 200；容器 Up healthy；真实计费日志 ¥ 正常；7 新列（request_bytes/response_bytes/concurrency_limit/rpm_limit/models/rpm_override/concurrency_override）在生产 PG AutoMigrate 完成
- 回滚：cp 备份 compose → sed 换回 local-v1.2.38 → docker compose up -d new-api

## 二十四、三库 conformance 全闭环（2026-09-22）
- MySQL 8 真实实例（服务器 docker + SSH 隧道）→ TestDBConformance 全组 PASS
- 至此 SQLite + PostgreSQL16 + MySQL8 三库 conformance 全部 PASS（唯一硬质量门关闭）

## 二十五、订阅 CNY 1:1 定价（v1.2.74，2026-09-22）
- controller/subscription.go：计划币种默认/强制 USD→CNY（4 处）
- calcSubscriptionBalanceQuota 已是 价格×QuotaPerUnit → 人民币 1:1（2元=100万额度、25元=1250万、60元=3000万），加计费回归测试
- 待办：前端订阅页 ¥ 符号显示（当前硬编码 $）、三张无限卡创建、部署 v1.2.74 到生产

## 二十六、生产热更新 v1.2.74（2026-09-22，真实执行+线上验收）
- checkout v1.2.74(4e3a12cce) + VERSION 写入 + docker build local-v1.2.74 BUILD_OK
- compose tag 换 local-v1.2.74 + up -d；容器 healthy；/api/status HTTP 200；外部真实流量正常
- 至此线上 = v1.2.74（含 CNY 1:1 定价、订阅全链、基础限速、管理端点、统计）
- 回滚：compose 备份 .bak.<TS> → sed 换回旧 tag → up -d

## 二十七、生产三张无限卡创建（2026-09-22，真实执行）
- 幂等插入 subscription_plans：天卡¥2/day、周卡¥25/week、月卡¥60/month；CNY；无限额度(total_amount=0)；并发3/RPM150；models=["deepseek-v4-flash"]；allow_balance_pay=true；subtitle 含微信Tf00798
- 验证：3 rows 查询确认（id 1-3，sort_order 1-3）
- 待确认：自动升级分组 upgrade_group 留空（需用户指定分组名后配置）

## 二十八、前端订阅 ¥ 符号（v1.2.75，2026-09-22）
- lib/format.formatPlanPrice：currency=CNY→¥，否则 $
- 替换 3 处硬编码 $：订阅表格 Price 列、用户订阅弹窗计划下拉、购买弹窗价格
- typecheck 绿

## 二十九、生产热更新 v1.2.75（2026-09-22，真实执行+线上验收）
- checkout v1.2.75(0fb195106) + build local-v1.2.75 + tag 切换 + up -d
- 容器 healthy；/api/status 200；订阅页 ¥ 显示已上线（三张 CNY 卡可见）
- 回滚：compose 备份 + 换回旧 tag + up -d

## 三十、分组订阅门禁（v1.2.76，2026-09-22）
- relay_setting.SubscriptionRequiredGroups（需订阅分组清单）+ IsSubscriptionRequiredGroup
- service.CheckSubscriptionGroupAccess：未订阅用户使用需订阅分组 → 403 可读文案；未标记/DB 不可用 fail-open
- 接入 RelayHelper（模型矩阵校验后）
- 单测：标记判定 + 放行/拒绝/fail-open；setting/service 全绿
- 配置：管理员在 relay 设置填分组名（如 subscriber）即启用；三张卡 upgrade_group 待分组名确认后配置

## 三十一、生产热更新 v1.2.76（2026-09-22，真实执行+线上验收）
- checkout v1.2.76(707d5a183) + build local-v1.2.76 + tag 切换 + up -d
- 容器 healthy；/api/status 200；分组订阅门禁已上线
- 回滚：compose 备份 + 换回旧 tag + up -d

## 三十二、前端 relay 每用户基础限速设置表单（v1.2.77，2026-09-22）
- request-limits/user-rate-limit-section.tsx：启用开关 + 并发(秒) + RPM 表单（写 relay.user_base_rate_limit_* option，热更新）
- registry 挂载（Security→Rate Limiting 组）+ defaultSettings + SecuritySettings 类型补 3 键 + i18n 7 语言
- typecheck 绿
- 待办：分组/用户覆盖 UI（后端端点已就绪 v1.2.73）；部署 v1.2.77

## 三十三、余额兑换不依赖充值开关（v1.2.78，2026-09-22）
- SubscriptionRequestBalancePay 移除 IsTopUpEnabled 门控：充值关闭仍可用钱包额度兑换订阅（保留合规确认）
- 前端订阅卡显示并发/RPM/微信提示、兑换码兑换、续费顺延、日志按用户筛选 → 待办

## 三十四、订阅卡显示并发/RPM + 微信定制提示（v1.2.79，2026-09-22）
- wallet/subscription-plans-card：benefits 增加 并发/s + RPM + 微信Tf00798高并发定制 两行
- i18n 7 语言；typecheck 绿

## 三十五、生产热更新 v1.2.79（2026-09-22，真实执行+线上验收）
- checkout v1.2.79(ccbba4c5e，含 77/78/79) + build local-v1.2.79 + tag 切换 + up -d
- 容器 healthy；/api/status 200
- 已上线：余额兑换不受充值开关限制(78)、限速设置表单(77)、订阅卡并发/RPM+微信提示(79)
- 回滚：compose 备份 + 换回旧 tag + up -d

## 三十六、修复：模型广场"会话过期"误报（v1.2.80，2026-09-22）
- 根因：/api/site/stats、/api/model/stats 挂了登录型限流 SearchRateLimit → 匿名请求 401 → 前端误判"会话过期"
- 修复：两个公共只读聚合接口移除 SearchRateLimit（保持无认证）
- 线上证据：model/stats 连续 401（82.41.50.93）；pricing 200

## 三十七、生产热更新 v1.2.80（2026-09-22，真实执行+线上验收）
- checkout v1.2.80(8cd311842) + build local-v1.2.80 + tag 切换 + up -d
- 容器 healthy；STATUS_HTTP=200；MODEL_STATS_HTTP=200（匿名可访问，会话过期误报已修复）
- 说明：首次部署会话被中断，应急重新 up -d 后确认 healthy（服务器脚本本身已完成构建与切换）
- 回滚：compose 备份 + 换回旧 tag + up -d
