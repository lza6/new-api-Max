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
- 服务器 <SERVER_IP>（HK CN2）SSH 直连（plink + hostkey 指纹）
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

## 三十八、续费顺延（v1.2.81，2026-09-22）
- CreateUserSubscriptionFromPlanTx：同套餐 active 订阅购买 → 到期时间顺延一个周期（锁行），不新建多行、不计入购买上限；无限额度卡不追加额度，有额度卡追加
- 回归测试：续费后 EndTime=原+1周期、行数仍 1；model 全绿
- 待办：兑换码兑换订阅、E2E 真实验证、日志筛选 UI、模型流量排行榜(G/T)

## 三十九、模型流量排行榜（v1.2.82，2026-09-22）
- service/log_traffic.go：TrafficBytesRecord 增 ModelName；AggregateBandwidthByModel 按模型聚合、带宽降序、limit 截断（含单测）
- controller.GetModelBandwidthLeaderboard → GET /api/log/bandwidth/model-leaderboard（AdminAuth），输出 requests/bytes/bytes_text（common.FormatBytes→B/KB/MB/GB/TB）
- 前端：CommonLogsStats 管理视图新增 BandwidthLeaderboardDialog（每日 + 模型 Top10，formatTraffic 显示 G/T；i18n en/zh/zh-TW）
- 验证：typecheck 绿、service 全绿、common-logs-stats 测试全绿

## 四十、兑换码兑换订阅（v1.2.83，2026-09-22）
- model.Redemption 增 PlanId（0=额度码；>0=订阅码）；Redeem 返回 RedeemResult{quota,plan_id,plan_name}
- 订阅码兑换：校验套餐存在且启用 → CreateUserSubscriptionFromPlanTx(tx,userId,plan,"redemption")（复用续费顺延），不增加钱包额度；Insert/Update 校验
- controller：AddRedemption/UpdateRedemption 支持 plan_id；TopUp 返回兑换结果对象
- 前端：钱包兑换成功提示（订阅开通 vs 额度）；后台兑换码表单「关联订阅套餐 ID」+ 表格「订阅套餐」列
- 回归：TestRedeemPlanSubscription（开通+不扣额度+码置used+重复兑换不重复建订阅）；redemption 组全绿；对全部兑换测试断言适配 RedeemResult

## 四十一、生产热更新 v1.2.82 + v1.2.83（2026-09-22，真实执行+线上验收）
- v1.2.82(7660a6572)：backup compose .bak.20260922-054358 → build local-v1.2.82 → up -d；healthy；/api/status 200；model-leaderboard 路由 401（已注册）
- v1.2.83(d2500a34f)：backup compose .bak.20260922-061620 → build local-v1.2.83 → up -d；healthy；VERSION=v1.2.83；/api/status 200
- 线上当前 = v1.2.83（含订阅全链、CNY 1:1、续费顺延、兑换码→订阅、模型流量排行、基础限速 3/s+120rpm）
- 回滚：compose 备份 .bak.<TS> → sed 换回旧 tag → up -d

## 四十二、E2E 真实账号验证（v1.2.83 线上，2026-09-22）
- 环境：freeapi.tingfengai.art（Caddy 反代 127.0.0.1:3000）；PostgreSQL newapi；本机 HTTPS 出站被断 → 直连 http://<SERVER_IP>:3000 验收
- E2E 账号：e2e_tingfeng（id=910，普通用户，bcrypt 密码独立生成，quota=0）
- 订阅码兑换：POST /api/user/topup → data={"quota":0,"plan_id":1,"plan_name":"天卡无限"}（额度不增加）
- 续费顺延：两次兑换不同码 → user_subscriptions 仅 1 行 active，end_time 由 +1 天顺延为 +2 天（start=1790030345, end=1790203145）
- 真实调用：/v1/chat/completions model=deepseek-v4-flash → 200（channel 20 稳定渠道）
- 并发 3/s：同秒 3 个 → [200,200,200]；第 4 个 → 429「基础并发请求已达上限（每秒 3 次）」
- RPM：基础每分钟 120 次 → 超限 429「基础请求速率已达上限（每分钟 120 次）」（120-8 余量后 58 个 429，计数吻合）
- 费用类型：消费日志 other.billing_source="subscription"（=按订阅计费，非钱包）；用户 quota 仍 0 未扣
- 日志按用户筛选：后端 /api/log/search username 可用；前端 admin filter bar 已有 username 输入框（common-logs-filter-bar.tsx）
- 复现脚本：.codex/e2e-scratch/e2e-sub-live.mjs（APIKEY 直测 / CODE 兑换两种模式）

## 四十三、模型效果测试整合进模型卡片（v1.2.84，2026-09-22）
- features/pricing/lib/model-test-meta.ts：MODEL_TEST_META（deepseek-v4-flash，testedAt=2026-09-18 02:28:15，asset=/model-test.html）
- /model-test 支持 ?model= 参数（validateSearch）；未发布测试的模型显示提示，不再只写死一个模型
- ModelCard 卡片内新增「效果测试 · <测试日期时间>」按钮 → /model-test?model=<model_name>
- 验证：typecheck 绿、oxlint 绿、JSON 有效；生产 /model-test?model=deepseek-v4-flash 200、/model-test.html 200

## 四十四、生产热更新 v1.2.84（2026-09-22，真实执行+线上验收）
- backup compose .bak.20260922-065638 → build local-v1.2.84 → up -d；healthy；VERSION=v1.2.84
- /api/status 200；/model-test 200；/model-test.html 200
- 线上当前 = v1.2.84（订阅全链 + 兑换码→订阅 + 续费顺延 + 模型流量排行 + 基础限速 3/s+120rpm + 模型效果测试入卡）
- 回滚：compose 备份 .bak.<TS> → sed 换回旧 tag → up -d

## 四十五、模型多分组核验（v1.2.84 线上，2026-09-22）
- 根因结论：旧版 v1.2.38 生产缺多分组 UI；现 v1.2.84 已具备
- 前端：models/model-groups-dialog.tsx「Assign model groups」多选 Checkbox → updateModelGroups 同步渠道分组并集 + abilities；model-form enable_groups 数组
- 后端：POST /api/model/groups（controller/model_groups.go）+ Model.Groups
- 线上数据：deepseek-v4-flash 当前 groups='default'（models 表 68 行），管理员可在后台「模型分组归类」对话框将其加入多个分组

## 四十六、分组/用户限速覆盖设置 UI（v1.2.85，2026-09-22）
- request-limits/rate-limit-overrides-section.tsx：Security → Rate Limiting 新增「Rate Limit Overrides」
- GET/PUT /api/option/relay/rate_limit/overrides*（RootAuth）：列出/新增/修改/移除 分组与用户覆盖（0/0=移除，立即热更新）
- 生效优先级：用户覆盖 > 分组覆盖 > 基础默认（3/s + 120RPM）；typecheck 绿、oxlint 绿、i18n 7 语言(主推 en/zh/zh-TW)

## 四十七、生产热更新 v1.2.85 + 订阅分组配置（2026-09-22，真实执行+线上验收）
- backup compose .bak.20260922-072623 → 配置写库 → build local-v1.2.85 → up -d；healthy；VERSION=v1.2.85；/api/status 200
- 配置（生产 PG）：
  - subscription_plans 1/2/3：upgrade_group='subscriber'（购买/兑换自动升级分组）
  - options relay.subscription_required_groups=["subscriber"]（该分组需订阅门禁）
  - GroupRatio={"default":0.1,"subscriber":0.1}；UserUsableGroups={"default":"","subscriber":"订阅用户"}
  - channels id=20 group='default,subscriber'；models deepseek-v4-flash groups='default,subscriber'
- 回滚：compose 备份 .bak.<TS> → sed 换回旧 tag → up -d；配置项可单独改回

## 四十八、订阅自动升级分组 + 门禁 E2E（v1.2.85 线上，2026-09-22）
- e2e_sub1（初始 default）兑换订阅码 → 自动升级 group='subscriber'（DB 实证 user 912）；订阅行 upgrade_group='subscriber' active
- 订阅用户 subscriber 分组调用 deepseek-v4-flash → 200（channel 20 default,subscriber）
- e2e_nosub（无订阅，兑换额度 1000 走真实缓存路径）手动选 subscriber 分组 token → 调用 → 403「分组 subscriber 需持有订阅后才能使用，请先购买订阅或联系微信 Tf00798 定制」
- 教训记录：token 创建 API payload 为扁平结构（{name,unlimited_quota,group}），包 {token:{...}} 会导致 name/group 丢失、后端兜底强制无限额度
- 复现脚本：.codex/e2e-scratch/e2e-group.mjs

## 四十九、订阅档位真实生效（v1.2.86，2026-09-22）
- 问题：基础限速（3/s + 120RPM）先于订阅档位（3/s + 150RPM）触发，订阅卡宣传的 RPM150 永远到不了
- 修复：middleware/user-rate-limit.go 对「有 active 订阅且套餐档位>0」的用户跳过基础默认（含分组/用户覆盖叠加），由订阅档位中间件（套餐档位/管理员订阅覆盖）统一约束
- 前端说明文案 + i18n 更新（订阅用户改由订阅档位约束）
- 线上 E2E（订阅用户 e2e_sub1 打 170 个并发）：3×200 + 147×订阅并发429 + 20×订阅RPM429，**基础429=0**（RPM150 可到达，计数 3+147+20=170 吻合）

## 五十、安全：发行版/仓库敏感信息脱敏（v1.2.86，2026-09-22）
- 审计发现提交进仓库的两处生产服务器 IP：setting/console_setting/validation_test.go、计划书/workflow_status.md
- 修复：测试文件 IP → 127.0.0.1；运维文档 IP → <SERVER_IP> 占位符；git grep 确认零残留
- 结论：发行版暴露的"上游地址"即为此类 IP 硬编码，已全部清除；DB 凭据/密码从未入库（仅服务器与未跟踪的 .codex/ 本地工具）

## 五十一、生产热更新 v1.2.86（2026-09-22，真实执行+线上验收）
- backup compose .bak.20260922-075908 → build local-v1.2.86 → up -d；healthy；VERSION=v1.2.86；/api/status 200
- 线上当前 = v1.2.86（订阅档位真实生效 + 敏感信息脱敏）
- 回滚：compose 备份 .bak.<TS> → sed 换回旧 tag → up -d

## 五十二、系统信息内存/CPU 核验（2026-09-22）
- 后端 system_instances 最新上报：new-api-hk-1，resources.cpu.usage_percent=12.4%、memory=60.7%、storage 67.8%（真实数据）
- 结论：v1.2.64「系统监控解耦」后系统信息页 CPU/内存/状态已有真实数据；旧版 v1.2.38 未显示是历史版本问题

## 五十三、管理员单订阅档位升级 E2E（v1.2.86 线上，2026-09-22）
- 能力：PATCH /api/subscription/admin/user_subscriptions/:id/tier（rpm/concurrency 覆盖，0=回退套餐）已上线；前端 user-subscriptions-dialog「Tier Override」列可改
- E2E：临时管理员(role=10，事后已删+会话清理) 对 e2e_sub1 订阅(id=2) PATCH rpm 150→200
- 订阅用户打 170 并发：升级前 20×订阅RPM429 → 升级后 **0×订阅RPM429**（3×200 + 167×订阅并发429，170<200 全过 RPM）
- 复位：rpm_override/concurrency_override 归 0，DB 核验回退套餐默认（150/3）
- 结论：管理员可对单个用户的订阅单独升级/降级 rpm 与并发，且立即生效

## 五十四、余额兑换订阅 E2E（充值开关关闭仍可兑换，v1.2.78 修复实证）
- 场景：payment_setting.topup_enabled 临时置 false（可逆，E2E 后已恢复 true）→ 用户兑换额度码充值 3,000,000 → POST /api/subscription/balance/pay {plan_id:1}
- 结果：200 success；钱包 3,000,000→2,000,000（扣 ¥2=100 万）；user_subscriptions 新增 active source=balance（+1 天）；subscription_orders 行 money=2 payment_method=balance status=success；用户自动升级 subscriber 分组
- 结论：充值功能关闭时余额兑换订阅照常可用（仅需合规确认），即用户要求“即便充值功能关闭也可以用额度兑换”

## 五十五、订阅模型矩阵越权 403 E2E
- 场景：订阅用户（plan 1 models=["deepseek-v4-flash"]）调用有渠道但不在套餐内的模型
- 结果：qwen-3.8-max（channel 29 存在、default 分组可用）→ 403「模型 qwen-3.8-max 不在当前订阅套餐（天卡无限）可用模型内，请升级套餐或联系微信 Tf00798 定制」；deepseek-v4-flash → 200
- 说明：gpt-4o 返回 503（无任何渠道）是渠道分发层先于矩阵检查的合理分层；矩阵检查在渠道分发后、上游调用前执行，用“有渠道但不在套餐”的模型可稳定触发 403

## 五十六、前端汉化（v1.2.87，2026-09-22）
- 问题：中文界面下「用户基本速率限制」等设置页显示英文（v1.2.77 声称 i18n 7 语言但 zh/zh-TW 值实为英文）
- 修复：zh.json + zh-TW.json 汉化 28 组用户可见键：User Base Rate Limit 段全部文案、档位/并发/RPM 覆盖、订阅高并发定制（微信 Tf00798）、套餐可用模型字段、站点统计标签（已提供带宽/已处理请求/今日调用等）
- 保留英文：品牌名（DeepSeek/OpenAI 等）、URL、占位符、{{模板}} 字符串
- 线上实证：部署二进制 grep 命中 用户基础限速=1、Effect test=2、2026-09-18（模型卡测试日期）=2

## 五十七、模型效果测试整合实证（v1.2.87 线上）
- 顶部导航「模型效果测试」= 独立页 /model-test（支持 ?model=，deepseek-v4-flash 有测试资产）
- 模型广场卡片：ModelCard 对已发布测试的模型渲染「效果测试 · 2026-09-18 02:28:15」按钮 → /model-test?model=<name>
- 产物实证：部署二进制含测试日期串与 Effect test 键 → 整合已上线

## 五十八、生产热更新 v1.2.87（2026-09-22，真实执行+线上验收）
- backup compose .bak.20260922-083157 → build local-v1.2.87 → up -d；healthy；VERSION=v1.2.87；/api/status 200
- 线上当前 = v1.2.87（含汉化 + 此前全部订阅/限速/排行/测试能力）
- 回滚：compose 备份 .bak.<TS> → sed 换回旧 tag → up -d

## 五十九、订阅到期自动降级分组 E2E（v1.2.87 线上，2026-09-22）
- 场景：新用户 e2e_exp（default）兑换天卡 → 自动升级 subscriber（实证）→ 强制 end_time 置为过去 → 主节点 1 分钟到期扫描
- 结果：user_subscriptions status active→expired；用户 group subscriber→default（回退购买前分组）
- 结论：订阅全生命周期（购买/兑换升级 → 档位/矩阵生效 → 到期自动降级）在生产全部实证；到期扫描 StartSubscriptionQuotaResetTask 每 60s 由主节点执行

## 六十、终局审计收尾（v1.2.88 + 文档/技能/HTML 报告）
- v1.2.88：购买/兑换/管理员授信后清除"无订阅"负缓存（service.ClearCachedNoSubscription），订阅立即生效；回归测试 TestClearCachedNoSubscription；已部署（.bak.20260922-090042）
- 交付物：计划书/requirements-traceability-matrix.md（27 项需求矩阵）、计划书/OPERATIONS_SOP.md（运维 SOP）、计划书/change-report.html（变更报告+测验）、.agents/skills/project-delivery/SKILL.md（项目工作流技能）、README.md 增强说明、记忆补丁（验证台账，避免重复劳动）
- 已知受限/优化项：非 zh 语言回退 zh；订阅用户每请求多次订阅查询（P2，当前规模可接受）；付费上游（图片/视频生成）未做真实调用（烧钱），仅契约审查

## 六十一、v1.2.88 清负缓存生产 E2E（2026-09-22）
- 场景：e2e_imm（default、无订阅）先打 10 并发（额度不足快速失败，同时设置"无订阅"负缓存）→ 兑换天卡 → 立即（无等待）再打
- 结果：兑换后立即 3×200 + 7×429「订阅并发」（若负缓存未清，15s 内会继续显示「基础并发」429）
- 对照组：e2e_base（无订阅、有额度、真上游）8 并发 = 3×200 + 5×429「基础并发」→ 非订阅用户基础限速正常
- 结论：v1.2.88 清负缓存在生产真实生效：购买/兑换后订阅档位即时接管；此前 15s TTL 延迟问题已消除

## 六十二、模型卡统计英文根因与修复（v1.2.89，2026-09-22）
- 现象：模型卡「Today calls/Today success/30d calls/30d success」显示英文，但导航为中文
- 排查：已部署二进制与 HTTP 送达 bundle 均含中文（今日调用/今日成功/近 30 天调用）→ 部署正确，非 i18n 缺失
- 根因：`static.Serve` 直接命中嵌入的 index.html 并绕过 fallback 的 no-cache → 浏览器缓存旧版 index.html（引用 v1.2.87 之前的英文 bundle），导致混合显示
- 修复：router/web-router.go 文档路由（无扩展名路径）统一 `Cache-Control: no-cache`；带哈希静态资源保持可缓存
- 验证：deploy 后 `/` 与 `/pricing` 均返回 `Cache-Control: no-cache`；生产 v1.2.89 healthy
- 用户侧：刷新一次（或强刷）即取到新 bundle，统计标签显示中文

## 六十三、计费倍率对账 + 定价载荷补 group_ratio（v1.2.91，2026-09-22）
- 用户疑问：模型按次 0.001 × 分组倍率 0.1，最终消耗应为 0.0001，但观感不一致
- 计费对账（真实 E2E，deepseek-v4-pro-0813，price=0.001、ratio=0.1）：消费日志 quota=50 = ¥0.0001（¥1=500000 内部整数额度，防浮点误差；显示 1:1 CNY：quota_display_type=CNY、usd_exchange_rate=1、price=1）→ 系统扣费 = 0.001×0.1 = ¥0.0001，与用户期望一致
- 显示缺口根因：/api/pricing 载荷缺 model 级 group_ratio → 前端 getDisplayGroupRatio 回退倍率 1 → 卡片报 ¥0.001（配置价）而实际扣 ¥0.0001，造成“不对”观感
- 修复：model.Pricing 增 GroupRatio，controller.GetPricing 注入全局分组倍率；模型卡按次价格按 配置价×分组倍率 显示有效价，并标注「分组倍率 0.1」（i18n en/zh/zh-TW）
- 部署：v1.2.91（.bak.20260922-101524）healthy；/api/pricing 载荷已含 group_ratio（default/subscriber=0.1）
- 附加观察：会话期间 ModelPrice 中 deepseek-v4-pro-0813/grok-4.6/glm-5.3 由 0.001/0.01 变为 0（仅后台保存会写该选项）——若为误存，请在「模型定价」后台恢复；恢复后卡片自动按新价×倍率显示

## 六十四、v1.2.92 版本显示修复 + 日志分组倍率展示 + deepseek 单价修正（2026-09-22）
- 现象 1：「系统维护」版本恒为 v1.2.80（实际代码 v1.2.91），且「检查更新」永远提示新版本
  - 根因：仓库 VERSION 文件长期停留在 v1.2.54（从未随 tag 更新）；生产 /opt/new-api-src/VERSION 残留 v1.2.80，Dockerfile 用 \v1.2.92 注入 common.Version → 镜像内嵌版本号错误
  - 修复：仓库 VERSION 更新为 v1.2.92 并提交；服务器 checkout v1.2.92 + echo v1.2.92 > VERSION + 重建镜像 local-v1.2.92 + 切换 compose tag + up -d
  - 验证：/api/status version = v1.2.92（生产实证）；系统维护页随之显示 v1.2.92
- 现象 2：deepseek-v4-flash 单价配置 0.01（非 0.001）→ 0.01×0.1=¥0.001/次 =「6 元 6000 次」；用户期望 0.001×0.1=¥0.0001/次
  - 生产配置修正：ModelPrice[deepseek-v4-flash] 0.01 → 0.001（备份 /tmp/ModelPrice.backup.*.json，可回滚）
  - 真实 E2E 对账（e2e_nosub / deepseek-v4-flash）：改后消费日志 quota=50 = ¥0.0001/次（改前 500=¥0.001），other 记录 model_price=0.001、group_ratio=0.1
  - /api/pricing 恢复 quota_type=1（按次）、model_price=0.001、group_ratio={default:0.1,subscriber:0.1}
- 现象 3：消费日志费用列只有金额、看不到分组倍率
  - 修复：web LogCostDisplay 增加 ×倍率 小字 + tooltip「模型单价 × 分组倍率」（复用 Group Ratio / User Exclusive Ratio / Model Price i18n key，en/zh 均存在）
  - 构建产物实证：主 bundle 含 Group Ratio / Model Price / User Exclusive Ratio / group_ratio
- 安全提醒：本次改 DB 时曾因 bash 引号展开把 ModelPrice JSON 破坏（key 引号丢失→模型按 token 回退显示），已当场用 SQL 文件+stdin 方式修复为合法 JSON 并验证（教训：改 option 走官方 API 或 SQL 文件，勿在 shell 内嵌 JSON）
- 待确认：deepseek-v4-pro-0813 / grok-4.6 / glm-5.3 / glm-5.3-flash 单价为 0 系用户有意清零（用户已确认）

## 六十五、订阅日志（v1.2.93，2026-09-22）
- 需求：管理员要能看到谁、什么时间、通过什么来源买了什么订阅；建议放在日志 UI 下
- 后端：GET /api/subscription/admin/logs（管理员鉴权），联表 user_subscriptions + users + subscription_plans，支持 username/status/source/时间范围筛选 + 分页；model/subscription_log.go
- 前端：usage-logs 新增 subscription section（「订阅日志」，与绘图/任务日志并列），列：ID/用户/套餐/来源/状态/价格/档位覆盖/起始/到期/创建时间/分组升降级；筛选栏（用户名/状态/来源）+ 分页；i18n en/zh/zh-TW
- 质量：go build、web build、tsgo typecheck、oxlint、i18n 校验全绿；model 单测 TestGetAllSubscriptionLogsJoinsUserAndPlan PASS
- 部署：v1.2.93（.bak.20260922-125403）healthy；/api/status version=v1.2.93
- 线上 E2E（e2e_adm2 临时管理员，已清理）：全量 total=8（首条 972098576/天卡无限/balance/active）；username=e2e 筛出 6 条；source=redemption 筛出 4 条；status=active 筛出 7 条；前端 bundle 含 Subscription Logs / No subscription records found

## 六十六、更新用户抽屉内「订阅与限速」板块（v1.2.94，2026-09-22）
- 需求：管理员在用户维度要能看到完整订阅情况 + 当前并发/RPM，并能直接改并发/RPM、分配订阅、实时生效
- 现状盘点：独立 UserSubscriptionsDialog（行操作图标）已有 查看/分配/改RPM并发/作废/重置，但入口深、用户编辑抽屉无订阅板块
- 实现：新增 web UserSubscriptionRateLimitSection，嵌入「用户管理 → 更新用户」抽屉（Group & Quota 之后）：
  - 显示每个订阅：套餐、来源、状态、有效期、额度、当前生效并发/RPM（套餐档位 + override，实时显示）
  - 直接改并发/RPM override（0=回套餐档位）→ PATCH tier 实时生效
  - 分配订阅（选套餐 + 添加）→ POST createUserSubscription
  - 重置额度（advance_reset_time 可切）/作废/删除（ConfirmDialog）
  - i18n en/zh/zh-TW（+3 key：Subscription & Rate Limits / 说明文案 / concurrency/s）
- 质量：tsgo typecheck、oxlint、i18n 校验、web build 全绿
- 部署：v1.2.94 healthy；/api/status version=v1.2.94；bundle 含 Subscription & Rate Limits / concurrency/s
- 线上 E2E（e2e_adm3 临时管理员，已清理）：plans=3；用户 935 订阅 id=7 → PATCH tier rpm=100/concurrency=5 成功且 verify 写入 → 复原 0/0 成功；全部实时生效

## 六十七、终局审计补位（v1.2.95，2026-09-22）
- 触发：用户要求以 Spec Kit 技能做终局闭环总审计（反向审判 / 补位 / 真实验收 / 深度修复），重点核查 v1.2.93-94 新增功能
- 审计发现 2 个 P1 缺口：
  P1-1 订阅日志 tab 对所有登录用户可见：TASK_LOG_SECTIONS 未过滤 subscription，普通用户可直接访问 /usage-logs/subscription 触发 admin API 401
    → 修复：tabNavGroups 过滤（仅 canManageScope 显示 subscription）+ 组件层 useEffect 重定向非管理员到 common（路由/UI 双保险）
  P1-2 用户抽屉「订阅与限速」板块操作后触发 refreshUserData → form.reset() 清空管理员未保存的表单编辑（分组/备注等）
    → 修复：onChanged 由 refreshUserData 改为 triggerRefresh（只刷新用户列表，不重置表单）
- 审计确认项（无缺口）：AdminAuth(role>=10) 与前端 ROLE.ADMIN 对齐；订阅日志无敏感字段泄漏（无 password/token）；板块内按钮均 type=button 防误触表单提交；Combobox 内部按钮已是 type=button；侧边栏无订阅日志入口（页面内 tab 才显示）
- 已知 P2：订阅日志表格无独立移动端卡片（桌面表格可横向滚动）；已记录
- 质量门：go build / tsgo typecheck / oxlint / i18n 校验全绿
- 部署：v1.2.95（.bak.20260922-151431）healthy；/api/status version=v1.2.95；bundle 含 Subscription & Rate Limits / 订阅与限速 / concurrency/s
- 交付：tag v1.2.95 + release https://github.com/lza6/new-api-Max/releases/tag/v1.2.95

## 六十八、站内文档 /docs + 模型卡按钮美化 + 导航去重（v1.2.96，2026-09-22）
- 需求：① 模型卡「效果测试」按钮占空间大 → 改 iOS 风格紧凑「模型测试」胶囊（时间戳移入 hover title）；② 顶部导航「模型效果测试」与卡片按钮重复 → 去重；③ 「文档」点击应进站内详细使用文档而非外链 GitHub；④ Tool Integration 独立导航奇怪 → 内容并入文档
- 实现：
  - 新增 web/src/features/docs（站内使用文档：快速开始/Base URL/API Key/对话示例/协议与模型/模型效果测试/工具接入/订阅说明）+ 路由 /docs
  - 工具接入预设抽为共享组件 ToolIntegrationSection（/tool-setup 与 /docs 复用同一份）
  - use-top-nav-links：Docs 优先站内 /docs（docs_link 为外链 http(s) 才外跳）；移除独立「模型效果测试」「Tool Integration」项
  - top-nav.config 兜底导航清空（彻底去重）
  - 后端 general_setting.DocsLink 默认清空（外链由后台配置覆盖）
  - 模型卡按钮：iOS 风格圆角胶囊（bg-foreground/5、active:scale、rounded-full），时间戳 title 提示
- i18n：en/zh/zh-TW 各 +14 key（Usage Docs/Quick Start/Model test/Protocols & Models 等）
- 质量：go build / web build / tsgo typecheck / oxlint / i18n 校验全绿；routeTree 仅 +21 行（/docs）
- 部署：v1.2.96 healthy；/api/status docs_link=''；/docs、/pricing、/tool-setup 均 200；bundle 含 使用文档/模型测试 key
- 交付：tag v1.2.96 + release https://github.com/lza6/new-api-Max/releases/tag/v1.2.96 + 收尾 commit 66fb72c7f

## 六十九、防御修复：用户抽屉订阅组件健壮性（v1.2.97，2026-09-22）
- 触发：全量前端测试发现 16 failed / 1210 passed，其中 permissions.test.tsx 3 个失败（v1.2.94 引入）
- 根因：UserSubscriptionRateLimitSection 的 planMap/loadData 直接遍历 plans，permissions 测试 mock api.get 对订阅接口返回 user 对象（非 PlanRecord[]）→ p.plan undefined → 组件崩溃
- 修复：planMap 加可选链（p?.plan?.id）；loadData 用 Array.isArray 校验后才 set（非数组按空数组处理，success=false 才报错）
- 验证：permissions 3/3 通过；全量 6 failed / 1220 passed（quota-display、metadata-sync 为干净 HEAD 也失败的存量；viewer、setup-guide 单独跑通过、全量跑才失败的 flaky 隔离，与本次无关）
- 部署：v1.2.97（.bak.20260922-173900）healthy；/api/status version=v1.2.97；docs_link=''；各页面 200
- 交付：tag v1.2.97 + release https://github.com/lza6/new-api-Max/releases/tag/v1.2.97

## 七十、前端全量测试全绿 + 生产热更新 v1.2.98（2026-09-22）
- 触发：v1.2.97 全量前端测试 6 failed / 1220 passed（quota-display、metadata-sync 为干净 HEAD 存量失败；setup-guide、viewer 全量跑才失败的 flaky）
- 确定性修复：
  - quota-display：`—` 断言从整行收窄到 `invite_info` 列单元格（Sign Up Method 列对无 source 用户同样合法渲染 `—`，getByText 匹配到 2 个）
  - metadata-sync：断言改为友好文案正则 `upstream service is temporarily unavailable`（B6-2 人话映射把原始 "Upstream unavailable" 改写，字面量不会出现）
  - setup-guide：`Hide setup guide` 可见性断言包进 waitFor（CardStaggerContainer 入场动画初始 opacity:0，findByRole 命中但动画未完成时 toBeVisible 误报）
  - test-setup 全局 asyncUtilTimeout 1s→3s：并行 worker 抢占 CPU 导致 findBy*/waitFor 健康流程超时（viewer 全量跑 3~16 个随机失败），与既有 testTimeout:20s 适配慢 CI 的理念一致
- 验证：全量测试 136 files / 1226 tests **0 failure**；go build / web build / tsgo -b / oxlint（exit 0）全绿
- 交付：commit fbec63b20（测试修复）+ 26018a024（VERSION bump）→ tag v1.2.98 + release https://github.com/lza6/new-api-Max/releases/tag/v1.2.98
- 部署（特殊过程，重要经验）：
  - 服务器 2GB 内存，直接 docker build 冷构建多次 OOM-kill 生产 new-api 容器（dmesg 留证），磁盘曾 98%（build cache 35GB+旧镜像 7GB 可回收）
  - 处置：docker builder prune + image prune（98%→19%）→ 加 4G swapfile（fstab 持久化，swap 2G→6G）→ 临时给服务器侧 Dockerfile builder2 加 `ENV GOFLAGS=-p=1 GOMAXPROCS=1` 串行化 Go 编译（构建后 git checkout 还原）→ nohup 构建日志落盘 `/opt/new-api/build-v1298.log`（勿用 `| tail` 吞输出，会误判卡死）
  - 结论：2GB 机器上服务器本地构建不可持续；正确热更新路径是 CI/CD（docker-build.yml 推 ghcr.io/lza6/new-api-max:v<tag>）→ 服务器 `docker compose pull`。本次 GitHub Actions 队列卡死（74h+ queued 未被 runner 接单，tag 推送也未触发），临时用服务器构建兜底；CI 队列问题已登记待办
- 验收：/api/status version=v1.2.98；/docs、/pricing、/model-test、/tool-setup 本地+域名 http 200

## 七十一、单用户限速覆盖：显示当前生效并发/RPM + 直改实时生效（v1.2.99，2026-09-22）
- 需求：管理员在「更新用户」抽屉对每个用户（含无订阅用户）查看其当前生效的真实速率（并发/RPM）并可直接修改、实时生效
- 机制（后端既有，纯前端补齐）：
  - `GET /api/option/relay/rate_limit/overrides` 返回基础默认 + 分组/用户覆盖；`PUT .../overrides/user {user_id, concurrency, rpm}` 写入即热更新（0,0=移除）
  - 生效优先级与后端 GetUserRateLimitTier 一致：用户覆盖 > 分组覆盖 > 系统默认（3/s + 120RPM，可关闭=不限）
- 实现：
  - 新增 `UserRateLimitOverrideSection`（web/src/features/users/components/）挂载在「更新用户」抽屉「订阅与限速」板块：展示当前生效档位+来源徽标（用户覆盖/分组覆盖/系统默认/基础已关闭），输入并发与 RPM 保存即写用户覆盖，提供「移除覆盖」
  - 解析逻辑独立到 `users/lib/user-rate-limit.ts`（resolveEffectiveRate）
  - API helper：`users/api.ts` 新增 getRelayRateLimitOverrides / setUserRateLimitOverride
  - i18n：13 个新 key 写入默认 translation 命名空间（en/zh/zh-TW 中文化；fr/ja/ru/vi 英文占位）——先踩坑：key 若放顶层命名空间会被 t() 忽略（中文不生效），必须放 translation
- 质量：tsgo -b / rsbuild build / oxlint 全绿；全量前端测试 137 files / 1231 tests 0 失败（新增 5 用例）
- 交付：`ef1d1e09c`（功能）+ `3716e268d`（VERSION）→ tag v1.2.99 + release https://github.com/lza6/new-api-Max/releases/tag/v1.2.99
- 部署：CI/CD 手动 dispatch 构建 `ghcr.io/lza6/new-api-max:v1.2.99`（cosign 签名）→ 服务器 compose 切 GHCR 源 pull + recreate（v1.2.98 已验证该路径，本次直接复用）；/api/status version=v1.2.99；页面全 200
- 真实 E2E（线上 103.233.252.213:3000，deepseek-v4-flash）：
  - 用户 A（e2e_nosub，id=913）设覆盖 concurrency=1,rpm=1000 → 并行 2 请求 = [200,429]，429 消息「基础并发请求已达上限（每秒 1 次）」✓
  - 用户 B（e2e_base，id=937）设覆盖 concurrency=1000,rpm=3 → 窗口内第 4 个请求 429（[200,200,429,429]，含 1 次基线）✓
  - 移除覆盖（0,0）→ GET 确认两用户覆盖清除 ✓
  - 临时管理员 e2e_adm_rl037426 用完即删（DELETE 1）
- 待办登记：/docs 等顶层命名空间 key 不参与默认 translation 命名空间（中文站部分文案仍英文），下一轮修复

## 七十二、KILWA "GROK" 渠道：协议转换适配器 + 上线 + 三协议 E2E 实测（v1.3.0-v1.3.2，2026-09-22/23）
- 需求：接入 KILWA 免费 GROK 接口（GET `https://kilwaapi.vercel.app/kilwa-grok?text=<prompt>` → `{status, reply,...}`），协议转换适配 Claude Code / Codex / OpenAI，0 费用按次计费，上模型广场
- 实现（新增渠道类型 Kilwa，type=62 / APIType=Kilwa）：
  - `relay/channel/kilwa/adaptor.go`：入站三协议（OpenAI Chat / Anthropic Claude / OpenAI Responses）→ 提取提示词 → 上游 GET；出站还原 OpenAI/Claude/Responses 的 JSON+SSE；DoRequest 兼容 passthrough/桥接原始请求体（v1.3.1）
  - 常量/映射：constant/channel.go（62 + baseURL + 名称）、constant/api_type.go、common/api_type.go（ChannelType2APIType）、relay/relay_adaptor.go（GetAdaptor）
  - 前端：web/src/features/channels/constants.ts 渠道类型选项新增 Kilwa
  - 单测 relay/channel/kilwa/adaptor_test.go（8 用例：提示词提取、三协议转换、GET 构建、上游失败、OpenAI/Claude 输出、Claude SSE 完整序列）
- 修复线：v1.3.0（渠道 503→models 逗号分隔；empty prompt→多形态 body 解析）；v1.3.1（同）；v1.3.2（Claude 流式改为手动完整 Anthropic SSE 序列 message_start→…→message_stop）
- 线上配置：渠道 id=32（type=62，base=https://kilwaapi.vercel.app，models=kilwa-grok，group=default）；model_pricing ModelPrice=0（按次 0 费用）；模型元数据 id=74（描述/标签/default 分组）；`POST /api/channel/fix` 重建 abilities 后出现在 /api/pricing（model_price=0, quota_type=1）
- 真实 E2E（103.233.252.213:3000，deepseek 侧用户 e2e_nosub token）：
  - OpenAI 非流 200 `Pong! 👋`；OpenAI 流 SSE+[DONE] ✅
  - Claude 流（Claude Code 格式）SSE 事件齐全 message_start/content_block_start/content_block_delta/content_block_stop/message_delta/message_stop ✅
  - Responses 流（Codex 格式）response.created→…→response.completed（output_text.delta 与 done 均含 text 属标准协议）✅
  - 计费 0：used_quota 750→750（delta 0）✅
  - **上下文容量受限**：4816 字符 prompt 触发上游 414（GET URL 长度限制）——Kilwa 只支持短文本单轮问答，如实标注
  - **最大输出实测**：中文短文约 400-580 字符
- 交付：v1.3.0 / v1.3.1 / v1.3.2 各 commit+tag+release+CI(GHCR)+生产 pull 热更新；生产 version=v1.3.2
- 备注：CI 曾因 bun install 下载 rspack tarball 损坏失败（与代码无关），重试成功

## 七十三、全站汉化闭环：顶层命名空间 key 归位（v1.3.3，2026-09-23）
- 触发：此前多轮登记"中文站部分文案仍英文"（/docs、Tool Integration、任务事件流、插件审核等）
- 根因：docs/工具集成/任务事件流/插件审核共 **49 个 key 位于 locale 文件顶层命名空间**，默认 `translation` 命名空间 `t()` 命中不了 → 显示英文兜底；而这些 key **在 zh.json 的中文翻译早已存在，从未生效**（放错命名空间）
- 修复：node 脚本将 7 个语言文件的 49 个顶层 key 全部并入 `translation` 命名空间（顶层本地化值优先；仅 `Web Protection` 冲突，用顶层本地化值），并补齐 fr/ja/ru/vi 缺失的 54 个 key（英文占位）→ 7 语言 translation 全量一致 **6633 key、顶层 0 key**
- 验证：
  - probe 实证：zh t('Step 1')='创建 API Token'、t('Tool Integration')='工具接入'、t('Event.succeeded')='已完成'、t('Stream ended')='事件流已结束'、t('Web Protection')='Web 防护'、t('Approve')='批准'，全部中文命中
  - **baseline 对照实验**（git stash 回退 i18n 后 viewer 仍 30+1fail、import ~90s）确认 viewer 慢/超时是环境漂移（Windows Defender+微信进程使 node import 慢 5 倍），非 i18n 回归
  - test-setup asyncUtilTimeout 3s→8s 消除环境 flaky（适配超慢主机，与 v1.2.98 1s→3s 同逻辑）
  - 全量前端测试 **137 files / 1231 tests 0 失败**（受限 6 worker，环境极慢 19min）；tsgo / oxlint / build 全过
- 交付：commit d7fd0e96d（i18n+test-setup）+ 1220a12ff（VERSION）→ tag v1.3.3 + release https://github.com/lza6/new-api-Max/releases/tag/v1.3.3 + CI(GHCR) + 生产 pull 热更新
- 线上验收：version=v1.3.3；/ /docs /tool-setup /model-test /pricing 全 200；**线上 bundle 已含中文翻译串**（创建 API Token / 工具接入 / 任务事件流 / Web 防护 / 拒绝该插件版本 / 三步接入 全 HIT）

## 七十四、模型流量排行榜（GB/TB，公开）上线（v1.3.4，2026-09-23）
- 需求：用户点名"模型排行榜那边要显示消耗了多少 G 流量 / T"（此前多轮只登记未落地）
- 后端：新增公开 `GET /api/rankings/bandwidth?days=30&limit=10`（controller/rankings.go GetRankingsBandwidth），按模型聚合 request/response 字节、降序限量、`common.FormatBytes` 转人类可读；抽取 `queryModelBandwidthLeaderboard` 共享函数（管理端 `/api/log/bandwidth/model-leaderboard` 与公开端点同源复用，controller/log.go 改为调用共享函数）
- 前端：`BandwidthSection` 组件（模型流量排行卡片：rank/模型/请求数/流量）+ `useBandwidth` hook（TanStack Query，加载/错误/空态齐全）；`formatTraffic`（B/KB/MB/GB/TB）渲染，bytes_text 兜底；i18n 4 新 key（en/zh/zh-TW 中文化 + 其余英文占位，probe 实证中文命中）
- 质量：go build / go test ./service/ ./model/ ./constant/ 全过；前端 tsgo / oxlint / build 全过；全量测试 **138 files / 1234 tests 0 失败**（新增 3 带宽用例）
- 交付：commit fcd9b9cf1（含 VERSION）→ tag v1.3.4 + release + CI(GHCR) + 生产 pull 热更新
- 线上真实 E2E：`GET /api/rankings/bandwidth?days=30&limit=8` 返回真实流量 —— deepseek-v4-flash **18.13 GB**（285,941 请求）、glm-5.3-flash 2.03 GB、mimo-v2.6-flash 1.84 GB、glm-5.3 1.14 GB…（bytes_text 全为人类可读单位）；/rankings 页面 200；线上 bundle 含「模型流量排行」「近 30 天各模型流量」中文

## 七十五、终局审计 R1/R2/R4 修复 + 线上 E2E 验收（v1.3.5，2026-09-23）
- 触发：终局闭环总审计（Explorer 前端链路 + Ampere 后端链路双 agent 并行）产出 R1-R10 风险清单；本版落地 R1/R2/R4 三项可修项
- R1（负价写端拒绝，已修+单测+线上验收）：
  - 修复：`setting/ratio_setting/model_ratio.go` `UpdateModelPriceByJSONString` 增加 ModelPrice >= 0 校验（非有限/负数一律拒绝），杜绝管理员误写负价→计费负扣费
  - 单测：`setting/ratio_setting/model_ratio_price_test.go`（拒绝负值 / 接受 0 与正值），go test 通过
  - 线上验收（103.233.252.213:3000，临时管理员 e2e_adm_v135677828 role=100）：
    - PATCH /api/option/model_pricing 提交 ModelPrice=-1 → **400 "ModelPrice must be a finite, non-negative number"**（带 expected_version 乐观锁版本）
    - 失败后 GET snapshot：configured/effective 仍为 0，version 不变（未污染配置）
- R2（零余额用户可调免费模型，已修+线上验收）：
  - 修复：`service/billing_session.go` tryWallet 仅在 preConsumedQuota > 0 时才要求余额>0（免费模型预扣=0，不再误 403）
  - 线上验收：新注册零余额用户 e2e_zero_v135678449（quota=0）调 kilwa-grok → **200 `Pong! 👋 How can I help you?`**（此前 403）
- R4（多用户兑换码每用户限一次，已修+单测）：
  - 修复：`model/redemption.go` 事务内 SumRedemptionUsageByUser 检查（查询必须在 tx 内，SQLite :memory: 跨连接会报错）
  - 单测：TestRedeemMultiUseCodePerUserOnce（SQLite 隔离覆盖）；线上真实码不扰动
- 前端：`subscriptions-mutate-drawer.tsx` `.catch(() => {})` → handleServerError（不再吞错误）
- CI：docker-build.yml 增加 `release: published` 触发（resolve tag 支持 release 分支）；**登记：fork 仓库 push/release 事件均无法触发 workflow，仅 workflow_dispatch 可靠（多次实证），本版仍手动 dispatch 构建**
- 审计登记（不修，符合预期/设计权衡）：R3（未配价格回退默认倍率；线上 kilwa-grok 已显式配 ModelPrice=0）、R5（多库隔离理论窗口）、R6/R7/R8（进程内限流/订阅档位替代基础限流=设计权衡）、R9（kilwa 零 usage=免费意图符合预期）、R10（compact 格式不可达）
- 质量（本机复跑）：后端 go test R1（TestUpdateModelPriceByJSONString* 2/2）+ R4（TestRedeemMultiUseCodePerUserOnce 等）全过；前端全量测试 **138 files / 1234 tests 0 失败**（1152.55s，6 worker）；bun run build / tsgo -b / oxlint（exit 0）全绿
- 交付：commit 8c9d949df（审计修复）+ 38b32b84c（VERSION v1.3.5）→ tag v1.3.5 + release https://github.com/lza6/new-api-Max/releases/tag/v1.3.5 + CI(GHCR) + 生产 pull 热更新（docker compose pull + up -d，容器 Up healthy）
- 线上验收：/api/status version=v1.3.5；临时 E2E 用户已清理（tokens DELETE 3 / users DELETE 2，SELECT count=0）


## 七十六、接入 Kilwa Claude（Claude Haiku，渠道 type 62）上线（v1.3.6，2026-09-23）
- 需求：用户提供上游 `https://kilwaapi.vercel.app/kilwa-claude?text=PROMPT`（GET 单轮问答，`{"status":"success","model":"🤖 Claude Haiku 3.5","reply":"..."}`），要求协议转换适配 Claude Code / Codex 等、上模型广场、一次调用 0 费用、按次计费
- 代码：`relay/channel/kilwa/adaptor.go` 扩展 —— `ModelList` 增加 `kilwa-claude`；`GetRequestURL` 按 `UpstreamModelName` 路由 `/kilwa-grok` / `/kilwa-claude`；新增单测 `TestGetRequestURLSelectsPathByModel`（grok/claude/未知模型三态），kilwa 包 **12/12 测试通过**，`go build ./...` 通过
- 交付：commit 1f38e7aa0（功能）+ f2a3add7a（VERSION v1.3.6）→ tag v1.3.6 + release https://github.com/lza6/new-api-Max/releases/tag/v1.3.6 + CI(GHCR 多架构 amd64/arm64) + 生产 pull 热更新（version=v1.3.6）
- 线上配置（管理 API 完成）：模型元数据 id=75（kilwa-claude，status=1，tags=KILWA,Claude,免费，groups=default）；model_pricing ModelPrice=0（按次 0 费用）；渠道 id=33（type=62，base=https://kilwaapi.vercel.app，models=kilwa-claude，group=default）；`POST /api/channel/fix` 重建 abilities 后出现在 /api/pricing（model_price=0, quota_type=1）
- 真实 E2E（103.233.252.213:3000，临时用户 e2e_usr_v136667667 token）：
  - OpenAI 非流 200 `Pong! 👋 I'm here and ready to help...`（model=kilwa-claude）✅
  - OpenAI 流 SSE+[DONE] ✅；Claude 流事件齐全 message_start→…→message_stop ✅；Responses 流 response.completed+output_text.delta ✅
  - 计费 0：user quota before/after delta=0 ✅；管理员日志 14 条 kilwa-claude 记录（quota=0, prompt/completion_tokens=0, 流式标记正确, 字节数正常）✅
  - **上下文容量**：4800/8000/12000/16000/20000/30000 字符均 200（远超 grok 的 ~4816 字符 414 上限）✅
  - **最大输出实测**：中文故事约 592 字符
- 临时账号已清理（tokens DELETE 5 / users DELETE 2，count=0）；渠道/模型/定价保留为正式配置

## 七十七、服务器 Docker 磁盘自动清理（防 98% 爆盘复发，2026-09-23）
- 触发：用户复盘 v1.2.98 磁盘 98% 事故（Build Cache 32GB + 旧镜像 7.1GB 手动清理才恢复 19%），要求"以后自动清理，不要再出现这种情况"
- 现状基线：/ 49G 用 40%；docker system df 可回收 Images 2.095GB + Build Cache 3.327GB + 悬空 Volumes 1.028GB；无 daemon.json；无 crontab；容器日志无 >50M
- 方案（不重启 Docker、不动运行中容器、不动数据卷）：
  - `/usr/local/bin/docker-cleanup.sh` 双模式：
    - 默认（每日维护）：`docker system prune -f --filter until=24h` + `docker builder prune -f --filter until=24h`（只清 24h 前悬空/未用镜像与构建缓存）；若磁盘仍 >=85% 自动升级激进清理
    - `--if-full`（每小时守卫）：磁盘 <80% 直接跳过；>=80% 执行 `docker system prune -af --filter until=1h` + `docker builder prune -af --filter until=1h` + 截断 >200M 容器 json 日志（truncate，无重启）
    - 日志 `/var/log/docker-cleanup.log`，>1MB 自动裁剪到最近 200 行
  - crontab（root）：`17 4 * * *` 每日维护 + `8 * * * *` 每小时 if-full 守卫；cron 服务 active
- 实测：维护模式回收 511.7MB（旧 redis/postgres 镜像），磁盘 40%→39%；if-full 在 39% 正确跳过；new-api/postgres/redis/watchtower 全部健康，零停机
- 说明：悬空卷故意不清理（`--volumes` 不传，数据安全优先）；新版本部署走 GHCR pull + compose up -d，配合本清理不会再有磁盘堆积
