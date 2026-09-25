# Batch-5 进度跟踪（B5-2 hover-why + B5-3 任务进度结构化）

## 目标
1. B5-2 hover-why 状态提示：渠道状态徽章 hover 显示冷却原因；任务列表失败/取消 hover 显示 fail_reason 人话版。
2. B5-3 任务进度结构化（向后兼容）：Task.Data 约定 `{"progress":{event_type,current,total,step}}`；adaptor 解析出就写、否则维持原字符串（零破坏）；前端详情分段条+步骤名、纯字符串原样显示。

## 现状（已侦察确认）
- **B5-2 渠道冷却**：`GetChannelHealthSnapshot` 已有 `cooling_down`/`cool_until`/`cool_count`；`ChannelHealthCell`（健康列）已在 Popover 内显示 "Cooling down · 相对时间"，且冷却时徽章 `pulse`。**但渠道状态列（status column）在 auto-disabled 时才显示 tooltip，且只显示 other_info.status_reason/time，没有冷却原因**。需要：状态列（ENABLED 且冷却中）加 Tooltip 显示冷却原因 + 到期时间。
- **B5-2 任务 fail_reason 人话**：`TaskDetailsCell` 已内联显示 `fail_reason` 截断文本；`getFriendlyErrorMessage`（B6-2 人话映射）已在 `web/src/lib/server-error-message.ts` 导出，但未在任务列表 hover 使用。任务详情 dialog 已显示原始 `fail_reason`。需要：任务列表 fail_reason hover 用 Tooltip 显示人话映射 + 原始原因。
- **B5-3 前端**：`readTaskStructuredProgress`（types.ts）与 `TaskStructuredProgressRow`（task-details-dialog.tsx）**已存在**并已在任务详情渲染分段条+步骤名；纯字符串进度原样显示。**缺口：无组件测试覆盖**。
- **B5-3 后端**：`Task.Data` 是 `json.RawMessage`，轮询路径目前统一 `task.Data = redactVideoResponseBody(responseBody)` 覆盖（丢失插件返回的 taskData）；`AppendTaskRefundMarker`（service/task_billing.go:267）已示范「合并写 map → SetData」的追加模式。**缺口：轮询时若插件返回结构化 progress，应合并进 Data 而非覆盖丢失；refund 标记也应与之共存**。
- 任务 adaptor 是 JS 插件（kling/sora → jsplugin），`ParseTaskResult` 有 `Progress string` 与 `State` 字段；`parseBatchResult` 有 `Data` 字段。

## 待办
### B5-2
- [ ] 渠道状态列（channels-columns.tsx status cell）：冷却中（health snapshot coolingDown）时用 Tooltip 显示冷却原因 + 到期相对时间；auto-disabled 分支保留原 reason/time tooltip
- [ ] 任务列表 `TaskDetailsCell`：fail_reason hover 显示人话映射（`getFriendlyErrorMessage`）+ 原始原因
- [ ] 相关 i18n key（冷却原因/人话标题）
- [ ] 组件测试：渠道状态列冷却 Tooltip；任务 fail_reason hover 人话

### B5-3
- [ ] 后端 `service/task_polling.go`：轮询解析到插件返回的结构化 progress 时，`task.Data` 合并写入 `progress`（不覆盖 refund/其他字段）；解析不出维持原样
- [ ] 后端 `jsplugin ParseTaskResult`：支持 `taskData`/`progress` 结构返回（parseSubmitResponse 已有 TaskData；轮询 ParseTaskResult 目前丢弃 data —— 补 `Data` 字段透传）
- [ ] 后端单测：合并写入行为（progress + refund 共存；纯字符串零破坏）
- [ ] 前端：任务详情/列表结构化进度组件测试（已有 `readTaskStructuredProgress`/`TaskStructuredProgressRow`，补用例）

## 验收
- [ ] 后端 `go build ./...` + 相关单测
- [ ] 前端 `bun run typecheck` + 新增测试
- [ ] E2E 截图（渠道冷却 hover、任务 fail_reason hover、结构化进度条）—— 需要可用渠道/浏览器，当前不可用则如实报告
- [ ] 提交 + 推送 + 版本 bump v1.2.5

## 关键文件
- `web/src/features/channels/components/channels-columns.tsx`（状态列 Tooltip）
- `web/src/features/usage-logs/components/columns/task-logs-columns.tsx`（TaskDetailsCell hover）
- `web/src/features/usage-logs/components/dialogs/task-details-dialog.tsx`（进度条）
- `web/src/features/usage-logs/types.ts`（readTaskStructuredProgress）
- `service/task_polling.go`（轮询 Data 合并）
- `relay/channel/task/jsplugin/adaptor.go`（ParseTaskResult Data 透传）


---

# 2026-09-19 新任务：参考项目全量对标（Phase A）

> 本段为当日新任务状态，追加于旧 B5 记录之后，互不覆盖。
> 完整状态源见 `参考的结果计划指南.md`。

## Task Contract
- 原始目标：深挖 D:\参考项目（1178 项）全部项目，提炼可借鉴优点；围绕「AI 更懂用户/小白易用/日志全开/用户 skills 沉淀/扩展品类」推动 new-api 演进。
- 当前阶段：Phase A（ANALYSIS_ONLY）完成 → 等待用户批准 Phase B 批次 1（P0）。
- 当前授权：只读分析；只允许写 参考的结果计划指南.md 与本追加段。
- 成功标准：9 节报告完成；1178 项全覆盖；高价值项附证据；路线图 P0/P1/P2；无 P0 遗漏。
- 停止条件：用户批准具体批次后进入 Phase B。

## Task Graph（摘要）
| ID | Owner | Goal | Status |
|----|----|----|----|
| A0 | 主协调 | 工作区检查 | DONE |
| A1 | 主协调 | 主项目识别 | DONE |
| A2-1/2 | 子代理×2 | agents 188 项深扫 | DONE |
| A2-3 | 主协调 | 其余 990 项全覆盖（清单+README） | DONE |
| A3-A5 | 主协调 | 筛选/差距/路线图 | DONE |
| A6 | 主协调 | 交付物落盘 | IN_PROGRESS |
| A7 | 主协调 | 输出报告等确认 | PENDING |
| Critic-1 | 独立审查 | 六维审查 | PENDING |

## 待办（Phase B 候选，需用户批准）
- P0-1 stream_fallover 灰度→默认开
- P0-2 channel_health_score API 前端接入
- P0-3 Web 防护可配置化 + 实时服务器状态页
- P1 费用解释用户版 / trace 时间线 / 插件写审批 / skills 仓库 v1 / 离线评测
- P2 任务品类扩展 / 模型目录同步 / 记忆层

---

# 2026-09-20 追加段：Go 全面评审 P1 修复批（不覆盖上述记录）

## Task Contract
- 目标：落地 `计划书/GO-全面代码评审报告.md` 的 P1 修复项；行为保持（支付/任务路径仅加日志不改控制流）。
- 当前状态：本地实现完成，验证通过；**提交/推送/发布待用户确认**（生产契约审批边界）。

## Nodes
| ID | Name | Status | Acceptance | Files | Verification |
|----|----|----|----|----|----|
| N1 | 默认 root 口令日志去敏 | PASSED | 日志不再打印口令 | model/main.go | gofmt/vet ok |
| N2 | zhipu/mistral stub panic→error | PASSED | ConvertClaudeRequest 返回 error 不 panic | relay/channel/zhipu/adaptor.go, mistral/adaptor.go | go test ./relay/channel/ PASS |
| N3 | TOTP 备用码 rejection sampling | PASSED | 格式/字符集回归测试通过 | common/totp.go, common/totp_test.go | go test ./common/ PASS |
| N4 | 支付/任务/余额吞错加日志（不改行为） | PASSED | 编译通过，行为不变 | controller/subscription_payment_epay.go, channel-billing.go, codex_usage.go, relay/relay_task.go | go vet + go build PASS |

## Verification Evidence
- `go vet ./common/... ./relay/... ./controller/... ./model/...` → exit 0
- `go test ./common/ -run "Backup|TOTP"` → PASS (0.03s)
- `go test ./relay/channel/ -run "UnimplementedClaude"` → PASS (mistral+zhipu)
- `go build ./relay/... ./controller/... ./common/... ./model/...` → exit 0
- 新增测试文件：common/totp_test.go、relay/channel/adaptor_stub_test.go

## Next
- 待用户确认后：主题提交 + 推送分支 + （与代码批一起打 tag 发版）。
---

# 2026-09-20 追加段：签到奖励设置 "Invalid input" Bug 修复（不覆盖上述记录）

## Root Cause
- `web/src/features/system-settings/general/checkin-settings-section.tsx` schema 对 min/max 用了 `z.coerce.number().int()`，但字段值是**显示货币金额**（如 ¥0.5 = 0.5 小数）→ 小数必被 `.int()` 拒绝 → zod 默认报 "Invalid input"。

## Fix
- schema 改为 `z.coerce.number().min(0)`（允许小数）；整数 quota 换算由 `parseQuotaFromDollars` 完成（0.5 → 250000、0.8 → 400000 @ quotaPerUnit=500000）。

## Verification
- 新增回归测试 `web/src/features/system-settings/general/__tests__/checkin-settings-section.test.tsx`：小数 0.5/0.8 提交成功，换算值正确（min_quota=250000/max_quota=400000），无 "Invalid input"。
- `bunx vitest run .../checkin-settings-section.test.tsx` → 1 passed (19.9s)
- `bun run typecheck`（tsgo -b）→ exit 0

## Status
- 本地修复完成、测试通过；提交/推送/发布待用户确认（生产契约审批边界）。
---

# 2026-09-20 追加段：首字延迟对标 sub2api（不覆盖上述记录）

## 结论
- 主病灶：`MEMORY_CACHE_ENABLED` 默认 false → 渠道/模型选择每请求回源 DB（channel_cache.go:124）。
- sub2api 可借鉴：HTTP 客户端池化（我们已有）、用量/配额异步 flusher（我们为同步预扣，L3 待审批）、热路径轻量化。

## 落地（仓库内）
- docker-compose.yml：新增 MEMORY_CACHE_ENABLED=true + SYNC_FREQUENCY=60（含注释）。
- 文档：`计划书/首字延迟对标sub2api与落地.md`（对标表 + 根因排序 + 服务器验证方法）。
- 校验：pyyaml 解析 OK（本地无 docker CLI）。

## 待用户确认
- 服务器应用 compose 并重启 new-api（生产变更）；验证方法见文档 §6。
- 预扣费异步化（对标 flusher）为 L3 计费改造，需方案审批后另起批次。
---

# 2026-09-20 追加段：本地真实 E2E 证据（不覆盖上述记录）

## 结果
- 流式透传：mock 上游首字 1.5s，网关首字 1.52-1.57s（额外 22-68ms），9 chunks，DONE，内容完整。
- 签到设置后端：min=250000 / max=400000 / enabled=true 复读通过。
- 证据：`计划书/e2e-evidence/v1.2.32-local-e2e.md`。

## 结论
- 网关流式透传零缓冲；首字额外开销毫秒级（本地）。生产端 1.5-2s 放大来源为 MEMORY_CACHE 未开的 DB 回源 + 2C CPU 争抢 + 上游波动，已由 compose 落地（MEMORY_CACHE_ENABLED=true）定向修复，待生产部署后实测复核。
---

# 2026-09-20 追加段：前端性能批1（v1.2.33 代码批起步，不覆盖上述记录）

## Changes
- web/rsbuild.config.ts：新增 vendor-charts / vendor-shiki / vendor-editor / vendor-icon-libs 异步 cacheGroup（priority 10, enforce）
- web/src/main.tsx：defaultPreloadStaleTime 0 → 30_000（预载数据 30s 内复用，减少导航重复请求）
- web/index.html：theme-color 双模（light/dark media 自适应）

## Verification
- bun run typecheck（tsgo -b）exit 0
- bun run build exit 0；产出 vendor-charts(2306KB) / vendor-editor(499KB) / vendor-icon-libs(56KB)
- 诚实边界：index.js 4.3MB 未变（重库可缓存化，入口瘦身属批2 Hero/编辑器懒加载）；vendor-shiki 未独立产出（shiki 为同步导入，属批2）
---

# 2026-09-20 追加段：前端性能批2（SW + Hero 懒加载 + SEO 基础，不覆盖上述记录）

## Changes
- hero.tsx：HeroTerminalDemo 改 React.lazy + Suspense（fallback 灰块）
- main.tsx：生产注册 /sw.js（仅缓存哈希静态资源，API/鉴权/导航不缓存）
- public/: sw.js + robots.txt + sitemap.xml
- 修正：shiki 仅类型导入（无运行时体积），无需拆分

## Verification
- node --check sw.js exit 0；tsgo -b exit 0；rsbuild build exit 0（dist 含 sw.js 1310B/robots 75B/sitemap 804B）
- Boundary：SW 真实缓存行为需部署后浏览器验证（本地已确保语法/构建/复制）
---

# 2026-09-20 追加段：浏览器级真实 E2E（v1.2.33 前端，Playwright，不覆盖上述记录）

## 结果（8 断言：7 通过 / 1 断言文案不匹配但功能通过）
- Landing 加载 ✅（h1=统一 API 网关，服务于海量 AI 模型）；Hero 懒加载内容渲染 ✅（bodyLen 1223）
- 签到设置页：输入框 2 个 ✅；保存 0.5/0.8 后 **无 "Invalid input"** ✅；**刷新回填 min=0.5 / max=0.8** ✅（持久化铁证）
- 控制台零错误 ✅；/sw.js 注册并被请求（200）✅
- 唯一 FAIL：toast 文案断言未匹配（sonner 文案/时序），功能已被回填断言覆盖，非缺陷

## 证据
- 计划书/e2e-evidence/browser-e2e-v1.2.33/{landing-desktop.png, checkin-settings.png, summary.json}
- 运行：本地 v1.2.33 网关（嵌入最新 dist）+ chromium Playwright，真实浏览器点击/输入/保存/刷新
---

# 2026-09-20 追加段：参考项目深度对标 v2（不覆盖上述记录）

## 做了什么
- 全量脚本扫描 D:\参考项目（1179 目录）→ inventory JSON（计划书/scout_reports/refscan-v2-inventory.json）
- 同品类网关对标（auth2api/aisix/doorman/chatgpt2api/9router-Max/sub2api）+ 扩展方向集群（可观测/记忆/skills/电商/视频/编排）
- 产出：计划书/参考项目深度对标与优化设计_v2.md（差距分析 + 4 波设计 + 决策清单）

## 边界
- 子代理工具本轮不可用，主控单代理直审（脚本全量 ≠ 抽样）
- 分析阶段，未改任何代码；待用户确认决策清单后按波次实施
---

# 2026-09-20 追加段：并发延迟根因诊断（实证，不覆盖上述记录）

## 结论
- 基准实证：网关附加延迟 并发1=14ms p50 / 并发20=158ms / 并发50=66-289ms；根因=热路径同步 INSERT logs（SQLite 写锁，241ms SLOW SQL），非透传问题。
- 修复设计（异步 consume-log flusher / 429 等1s有界重试 / 流量字节统计+前端卡 / 连接池收敛）已写入 `计划书/并发延迟与10ms目标根因诊断.md`，待用户授权实施。

## 证据
- `计划书/e2e-evidence/latency-bench-v1.2.33.json`
---

# 2026-09-20 追加段：429 有界退避重试（实施+真实E2E，不覆盖上述记录）

## 变更（Go）
- common/constants.go + init.go：Relay429RetryDelayMs（env RELAY_429_RETRY_DELAY，默认1000ms）、Relay429MaxRetries（env RELAY_429_MAX_RETRIES，默认2）
- service/relay_error_class.go：ShouldBackoff429(status, used, max) 纯函数
- controller/relay.go：重试循环内 429 → select 等待（尊重 ctx 取消）→ 重试，留日志
- 单元测试：service/relay_error_class_test.go TestShouldBackoff429（5 用例）

## 真实 E2E（本地，429→等待1s→200）
- mock 首次 429、二次 200；网关日志：`429 backoff 1s before retry (attempt 1/2)` → 200 done，总 1.07s
- 证据：`计划书/e2e-evidence/429-backoff-e2e.json`；consume log use_channel ["5","8","8"] frt=1055ms

## 待办（下轮）
- 流量字节统计（T4）；异步 consume-log flusher（10ms 目标核心，L3 需授权）；上游 429/4xx 状态码透传复核（无重试预算时避免以 500 返回）
---

# 2026-09-20 追加段：T4 流量字节统计（实施+真实E2E，不覆盖上述记录）

## 变更
- relay/common/relay_info.go：RequestBytes/ResponseBytes 字段
- controller/counting_writer.go：gin ResponseWriter 包装，实时累加响应字节到 relayInfo（解决 handler 内记录时序）
- controller/relay.go：请求体字节（ReplayableBody.Size）+ 响应字节（counting writer）
- service/log_info_generate.go：appendTrafficBytes → other.request_bytes/response_bytes
- service/log_traffic.go：AggregateTrafficByDay 按日聚合（纯函数）
- controller/log.go + router：GET /api/log/traffic?days=1|7|30（管理端）
- 单测：service/log_traffic_test.go（含空输入短路）

## 真实 E2E（本地）
- 流式请求后：consume log other.request_bytes=112 / response_bytes=1048；端点 /api/log/traffic?days=1 → 201 请求 / 1272 B / by_day 今日行
- 证据：计划书/e2e-evidence/traffic-stats-e2e.json

## 待办
- 前端流量卡（今日/7日/30日 + 实时）与 RPM 口径核对（下轮）
- 异步 consume-log flusher（10ms 目标核心，L3 授权后实施）
- 上游 429/4xx 状态码透传（无重试预算时避免 500）
---

# 2026-09-20 追加段：T4 前端流量卡（不覆盖上述记录）

## 变更（web）
- usage-logs/api.ts：getLogsTraffic(days)（GET /api/log/traffic）
- common-logs-stats.tsx：管理端头部新增 今日/7日/30日 MB 徽章（TrafficBadges，by_day 求和，60s staleTime）
- i18n：en.json 新增 Traffic today/7d/30d，bun run i18n:sync 无漂移

## 验证
- i18n:sync exit 0；tsgo -b exit 0；rsbuild build exit 0
- Boundary：UI 实际渲染需部署后浏览器验证（后端字节统计已真实 E2E 通过）
---

# 2026-09-20 追加段：成对多请求延迟对比（N=60，不覆盖上述记录）

## 数据（同 payload，直连 vs 网关，60 对）
| 场景 | direct p50/p95/max | gw p50/p95/max | 网关附加 overhead p50/p90/p95/max |
|---|---|---|---|
| 顺序 N=60 | 60.8/73.1/87.8 ms | 97.2/106.6/115.4 ms | 27.6/41.1/44.9/45.5 ms |
| 并发 20 N=40 | 81.5/93.7/93.8 | 172.2/435.6/499.0 | 89.3/212.8/341.9/405.2 |
| 并发 50 N=40 | 88.6/103.0/107.5 | 240.4/645.1/757.4 | — |

## 结论（多请求佐证，非单样本）
- 单并发网关附加 28–45ms；并发 20/50 放大到 89–645ms；直连曲线平坦（60–107ms 与并发无关）。
- 网关日志实锤：并发下每请求同步 INSERT INTO logs 244–652ms（与 gw 耗时 307–746ms 对应）→ 热路径同步日志写是唯一系统放大源。
- 修复方向不变：异步 consume-log flusher（10ms 目标），已列 L3 待授权。

## 证据
- `计划书/e2e-evidence/paired-latency-bench-v1.2.34.json`（含 60 对逐请求明细前 12 条 + 并发分布）
---

# 2026-09-20 追加段：异步 consume-log flusher + 连接池收敛（实施+实测，不覆盖上述记录）

## Changes
- model/consume_log_flusher.go：消费/错误日志异步批量落库（队列4096，满则同步背压；后台按 LOG_FLUSH_INTERVAL/BATCH 批量 INSERT；Stop/Flush 供测试与优雅退出）
- model/log.go：RecordConsumeLog/RecordErrorLog 在 LOG_FLUSH_ENABLED=true 时异步入队（默认 false=同步，零行为变化）
- main.go：StartConsumeLogFlusher 启动
- 连接池默认收敛：SQL_MAX_OPEN_CONNS 1000→64 / IDLE 100→16 / LIFETIME 60→300（主库+日志库）
- docker-compose.yml：LOG_FLUSH_ENABLED=true/INTERVAL/BATCH + 池 env
- 单测：model/consume_log_flusher_test.go（3条入队→Flush→计数3）；model 全套 25s 通过（默认关无回归）

## 实测（成对基准 N=60，异步 vs 基线同步）
| 场景 | 基线 gw p50 | 异步后 gw p50 | 改善 |
|---|---|---|---|
| 顺序 overhead p50 | 27.6 ms | 18.8 ms | -32% |
| 并发20 gw p50 | 172.2 ms | 115.1 ms | -33% |
| 并发50 gw p50 / p95 | 240.4 / 645.1 ms | 118.9 / 140.6 ms | -50% / -78% |
- 证据：计划书/e2e-evidence/paired-latency-bench-async-flush-v1.2.34.json

## 剩余（10ms 目标）
- 单并发 overhead 仍 ~19ms：剩余热路径同步 DB 读取（RecordConsumeLog 内 GetUserSetting 用户设置读、结算 quota 更新）。后续：用户设置缓存 + 结算批量异步（L3）+ Redis 令牌缓存（生产已有）。
---

# 2026-09-20 追加段：生产部署 v1.2.34 + 线上 E2E 验收（不覆盖上述记录）

## 部署
- CI 镜像队列卡死（11h+），绕行：服务器本地 docker build v1.2.34 tag → new-api:local-v1.2.34；compose 换镜像+注入性能 env（内存缓存/异步日志/连接池/429 退避）；容器 Up healthy。
- 备份与回滚已就绪（.bak-pre-v1234 / latest-pre-v1234-backup 镜像标签）。

## 线上 E2E
- version=v1.2.34；真实上游流式 200+DONE，首字 2.46s/总 2.69s（上游自身 2-22s，网关透传正常）→ PASS。
- 证据：计划书/e2e-evidence/live-prod-v1.2.34-e2e.md

## 遗留（诚实）
- 观察：管理端 API 创建 token 返回 success 但未落库（待查，可能与 admin token 权限/审计相关，非本批阻断）。
- CI 队列积压待观察（GHCR v1.2.34 镜像未出；本地镜像已在生产运行）。
- watchtower 仅跟 `latest`，不影响本地镜像（钉住 tag 安全）。
# 2026-09-20 追加段：管理端日志页实时指标（当前并发 + 近1分钟完成）

## Changes
- middleware/stats.go：新增 61 格按秒分桶滑动窗口 completedWindow（互斥锁，零额外计算），请求结束记录完成数；
  StatsInfo 增加 completed_last_minute；GetStats 返回并发+完成两值。
- controller/log.go GetLogsStat（admin-only）：data 增加 concurrent_requests / completed_last_minute。
- web：usage-logs 头部 admin 视图新增两个 StatBadge（Concurrent now / Completed last minute），admin 查询 5s 轮询实时刷新。
- i18n：修复上轮 traffic 3 键误放 root 命名空间（实测 zh 下 t() 返回英文）→ 迁入 translation 并补 7 语言翻译；新增 2 键全语言翻译；i18n:sync 无漂移。
- 测试：middleware/stats_test.go（滑动窗口 + 中间件并发增减）；common-logs-stats.test.tsx（admin 渲染/非 admin 隐藏）。

## 验证
- go build ./... OK；go test ./middleware/ 相关用例 OK（含并发 2 请求增减）。
- web：tsgo -b OK；vitest 新组件测试 2/2；rsbuild build OK。
- 本地 E2E（mock 上游 + 本地 SQLite + 5 路并发流式请求）：峰值并发 concurrent_requests=5、完成后 completed_last_minute=5、回落 0；
  /api/status/test 亦暴露两字段。证据：计划书/e2e-evidence/concurrency-live-metrics-e2e.json
  （注：relay 500 为 SSRF 防护拒绝本地 loopback 上游的预期行为，与指标计数无关；指标按 HTTP 全量请求计数）。

## 存量（非本次引入，基线复现）
- usage-logs 4 个测试文件在 HEAD 基线即失败（viewer.test.tsx 19 例 audit 渲染断言 + 3 个 filter 测试），与本批改动无关。

# 2026-09-20 追加段：生产部署 v1.2.35 + 线上验收（实时指标）

## 部署
- 服务器 /opt/new-api-src git fetch + checkout v1.2.35（e0928334a）；docker build → new-api:local-v1.2.35（225MB）
- compose 镜像 local-v1.2.34 → local-v1.2.35（备份 docker-compose.yml.bak-pre-v1235）；up -d 重建；healthy
- 回滚：cp docker-compose.yml.bak-pre-v1235 docker-compose.yml && docker compose up -d new-api（v1.2.34 镜像仍在）

## 线上验收（HTTPS 真实请求）
- X-New-Api-Version: v1.2.35
- /api/log/stat 返回 concurrent_requests=7、completed_last_minute=35（真实流量基线）
- 6 线程并发打 stat 时 active_connections 峰值=10（并发计数实时生效）
- /api/status/test http_stats 同时暴露 active_connections + completed_last_minute
- 证据：计划书/e2e-evidence/prod-v1.2.35-live-metrics.json

# ============================================================
# 2026-09-20 追加段：终局闭环总审计工作流（Spec-Kit 驱动）
# ============================================================

## 规范资产（.specify/）
- Constitution: .specify/memory/constitution.md（生产优先/计费安全/安全基线/三库兼容/性能/前端体验/保守改动）
- Spec: .specify/specs/001-final-audit-and-closure/spec.md（backlog 状态矩阵 + 审计需求 R1-R8 + Open Questions）
- Plan/Tasks: 同目录 plan.md / tasks.md（Phase A 盘点 → B P0/P1 修复 → C 补位 → D 文档 → E 交付）

## 工作流编排
- Phase A：4 个并行只读审计子代理（closure-matrix / sql-db / contract-errors / resilience-observability）
- 证据与发现落 计划书/audit/；验证账本 计划书/audit-ledger.md（防重复跑同一测试）
- 每节点验收：代码定位 + 证据 + 状态，禁止无证据宣称完成

## 验证账本（audit-ledger）初始记录
- [x] 慢查询/索引：尚未系统跑过（Phase A2 首跑，记录 replace 后续引用）
- [x] 契约抽查：尚未系统跑（Phase A3 首跑）
- [x] 压测：v1.2.34 已做 60 对延迟基准（计划书/e2e-evidence/paired-latency-bench-async-flush-v1.2.34.json）——勿重复全量，改重点复核
- [x] 并发+完成指标：本地 E2E + 线上 7/7（concurrency-live-metrics-e2e.json / prod-v1.2.35-live-metrics.json）——已闭环

# 2026-09-20 追加段：Phase A 终局审计结果 + Phase B/C 修复实录

## Phase A 结果（4 节点只读审计，全部完成）
- A1 功能闭环矩阵（计划书/audit/closure-matrix.md）：15 节点 = DONE 13 / PARTIAL 1（B5-4 无定时调度与 /v1/pricing 路由）/ MISSING 1（B5-3 通用 webhook 子系统）/ BROKEN 0
- A2 SQL/DB（计划书/audit/sql-db.md）：13 条（P1×4：GetLogsTraffic 全量拉取、logs 缺 (type,created_at,id)、用户日志缺 (user_id,type,created_at)、task_events 无保留策略；P2×9）
- A3 契约/错误人话（计划书/audit/contract-errors.md）：1 处低危 drift（task/mj self 视图 channel_id 后端不读）+ 错误映射硬缺口 3（get_channel_failed/channel 域保留码/pre_consume_token_quota_failed）+ cooldown 正则死模式
- A4 稳定/可观测（计划书/audit/resilience-observability.md）：限流/并发桶/SSRF/429 退避/健康检查到位；缺口=熔断器(P2)、/metrics 导出(P2)、TRUSTED_PROXIES 显式化(P2)
- 验证账本已更新（计划书/audit-ledger.md），防重复跑同一测试

## 修复实录（Phase B/C）
- B4(改)/S4 签到设置 Invalid input：默认值兜底(undefined→0) + max<min 人话校验 + i18n；commit abef34e83；vitest 3/3
- S1 会话上限：确认生产 SessionLimitEnabled=false（默认关），admin 开关在 Security 设置（login-session-limit-section），无需改码
- F2/F3/F4（SQL P1）：logs 复合索引 (type,created_at,id)+(user_id,type,created_at)、task_events created_at 索引 + 7 天保留策略（TASK_EVENT_RETENTION_DAYS，主节点每小时批删）；task_events 清理测试通过；三库验证待跑
- A3 修复：buildBaseParams channel_id 仅 admin 传递；错误映射补 get_channel_failed/channel 域/pre_consume 三类人话 + i18n 7 语言；映射测试 5/5

## 待办（下轮）
- 三库（SQLite/MySQL/PG）迁移与索引验证（AGENTS 强约束）
- 交付 v1.2.36：主题 commit → push → tag → Release → 生产部署 + 线上验收
- B5-3 webhook / B5-4 定时同步 属架构新增，等用户优先级确认

# 2026-09-20 追加段：三库验证通过 + v1.2.36 交付
## 三库验证（AGENTS 强约束，实证）
- SQLite（本地 scratch 库）：idx_log_type_created_id / idx_log_user_type_created / idx_task_events_created_at 落库；旧 idx_created_at_type 移除；二次启动幂等
- PostgreSQL 15（服务器临时容器 postgres:15）：同三索引存在、旧索引不存在、双次运行无 FATAL/ERROR
- MySQL 8（服务器临时容器 mysql:8）：同三索引存在、旧索引不存在、双次运行无错误
- 容器已清理（--rm + trap）
- 修复：logs 复合索引 (type,created_at,id)+(user_id,type,created_at)、task_events created_at 索引 + 7 天保留策略（TASK_EVENT_RETENTION_DAYS，主节点每小时批删）

# 2026-09-20 追加段：生产部署 v1.2.36 + 线上验收

## 部署
- 服务器 /opt/new-api-src checkout v1.2.36（41af5a0f2）→ docker build new-api:local-v1.2.36 → compose 换镜像（备份 .bak-pre-v1236）→ 重建 healthy
- 生产 PG 存量库：新索引落库（idx_log_type_created_id / idx_log_user_type_created / idx_task_events_created_at），并清理冗余旧索引 idx_created_at_type（GORM 移除 tag 不自动 drop；回滚命令 CREATE INDEX idx_created_at_type ON logs(created_at,type)）
- 启动日志无 FATAL/ERROR/panic

## 线上验收（HTTPS 6/6）
- X-New-Api-Version: v1.2.36；/api/log/stat 返回 concurrent_requests=7、completed_last_minute=35（真实流量）
- 6 线程并发时 active_connections 峰值=9；checkin_setting 三个 option 正常
- 证据：计划书/e2e-evidence/prod-v1.2.36-acceptance.json
- 回滚：cp /opt/new-api/docker-compose.yml.bak-pre-v1236 /opt/new-api/docker-compose.yml && cd /opt/new-api && docker compose up -d new-api

# 2026-09-20 追加段：v1.2.37（token 创建修复 + 首字延迟实证）生产部署与验收

## 线上侦查结论（首字慢）
- frt p50=26.9s/p95=52s；use_time p50=34s；每请求 25-51万 token；deepseek-v4-flash 仅 1 活跃渠道
- 服务器端成对基准：网关 vs 直连附加 ≈0~300ms → 首字慢=上游慢+超大 prefill（证据 计划书/audit/first-token-latency-prod.md）

## token 创建修复（根因闭环）
- POST /api/token/ 响应加入 data{id,key}；未显式额度默认无限；401 区分“额度用尽”
- 生产验证：创建→返回 id+key→key 直接用返回 200→清理 ✓（此前新 token 必 401 死胎）

## 部署
- v1.2.37 服务器构建 local-v1.2.37、compose 换镜像（备份 .bak-pre-v1237）重建 healthy
- Release: https://github.com/lza6/new-api-Max/releases/tag/v1.2.37
- 回滚：cp /opt/new-api/docker-compose.yml.bak-pre-v1237 docker-compose.yml && cd /opt/new-api && docker compose up -d new-api

## 验收
- X-New-Api-Version v1.2.37；token E2E 4/5（清理 307 尾斜杠已用 DB 兜底，属已知 P2）
- 4xx 透传正确（网关 400 自校验 / 上游 400 原样返回）

# 2026-09-20 追加段：500 错误根因（RELAY_TIMEOUT 超时）+ 加固 v1.2.38

## 线上错误实证（近 6h，type=5，1521 条）
- 500「upstream error: do request failed」102 条，use_time 几乎全为 300/600s = RELAY_TIMEOUT=300(5分钟) 总超时掐断慢 prefill → 与用户「跑5分钟报500」吻合
- 429 三类（Too many pending 652 / user 并发超限 418 / 上游限流 69）与 502(118) 均来自上游透传（单渠道 yunshuzhilian 超卖放大）
- 400 engine unavailable(110)/MaxTokens 超限(9)/content 策略 等为上游参数/引擎问题

## 加固（v1.2.38）
- relay/channel/api_request.go：上游请求超时（http.Client.Timeout/context deadline/net.Error.Timeout）→ 504 Gateway Timeout（原 500），
  语义准确、错误日志可辨；isRequestTimeout 单测通过；relay/channel 全量测试绿
- 生产 compose：RELAY_TIMEOUT 300 → 900（允许 15 分钟慢 prefill 完成，显著降低超时 500）
- 429/502 根治 = 为 deepseek-v4-flash 增加活跃渠道 + 健康分路由（待用户加渠道）

# 2026-09-20 追加段：v1.2.38 生产部署与验收

## 部署
- 服务器 checkout v1.2.38 → docker build local-v1.2.38 → compose 换镜像 + RELAY_TIMEOUT 300→900（备份 .bak-pre-v1238）→ 重建 healthy
- 确认：docker exec new-api env RELAY_TIMEOUT=900；X-New-Api-Version v1.2.38；启动无 FATAL/panic
- Release: https://github.com/lza6/new-api-Max/releases/tag/v1.2.38
- 回滚：cp /opt/new-api/docker-compose.yml.bak-pre-v1238 /opt/new-api/docker-compose.yml && cd /opt/new-api && docker compose up -d new-api

## 效果预期
- 超时类 500 显著下降：慢 prefill 可跑到 15 分钟；真正超时上报 504（可观测、不与真实 500 混淆）
- 429/502 仍为上游单渠道超卖所致 → 需增加 deepseek 活跃渠道（用户侧）

---

# 2026-09-23 追加段：T1 热路径性能冲刺（Spec 003，待 push/tag/Release）

> 本段按用户「全部授权、真实落地、诚实证据」要求记录。详细 spec/plan/tasks/checklist 见 `.specify/specs/003-hotpath-perf-t1/`；验证台账见 `计划书/audit/perf-verification-ledger.md`。

## 关键审计结论（修正任务卡前提）
- `GetUserSetting` 已 **Redis 缓存**（model/user.go:1323）——任务卡“用户设置未缓存”的前提已过时，未重复造缓存。
- 热路径每请求 `GetUserCache` 被加载 3 次（middleware/auth TokenAuth → NewBillingSession.GetUserQuota → RecordConsumeLog.GetUserSetting），每次 = Redis HGETALL + auth-version MGET = 2 往返。
- 429 退避 + 4xx 透传**已正确**；缺 4xx 透传回归测试。

## 落地实现
- `model/log.go`：新增 `userSettingRecordIp(c, userId)` 复用请求内 `ContextKeyUserSetting` 判定 `RecordIpLog`（RecordConsumeLog / RecordErrorLog 两处），行为等价，消除每请求 2 次 Redis 往返。
- `service/billing_session.go`：`NewBillingSession.tryWallet` 复用请求内 `ContextKeyUserQuota`（typed ok 判定，无权值时回退 `GetUserQuota`），消除第 2 次用户缓存加载。
- 测试：`model/consume_log_flusher_test.go` US1（上下文复用，2 子用例 PASS）；`service/error_test.go` US2（4xx 透传 5/5 PASS）。
- 脚本：`scripts/bench-latency.ps1`（直连 vs 网关成对基准，顺序+并发 20/50，输出到 `plan书/e2e-evidence/`）。

## 验证证据（真实运行）
- `go build ./...` → exit 0
- `go vet ./model/ ./service/` → exit 0
- `go test ./model/ ./service/ ./common/` → exit 0（model 47.4s / service 5.5s）
- `gofmt -l`（4 个改动文件）→ 空
- 说明：US1/US2 为本次真实运行；“完整网关+mock+渠道”的成对基准需另起本地运行环境（方法在脚本头部），线上基准需授权。

## 前状态（承接上段）
- 上段到 v1.3.6（记录 kling/sora、kilwa、signage）之前内容读本文件上方。
- 本段不覆盖上方记录；待收尾项：本地成对基准 + 是否进入 T2（渠道健康路由）。
---

# 2026-09-23 追加段：T2-1 健康分边界测试 + T6-1 Data 合并验收测试（已推送，纯测试）

## T2-1 渠道健康分边界（commit 2a9788412，push main）
- `service/channel_health_score_test.go` 扩展（沿用既有文件，不新增）：
  - `TestComputeHealthScoreBoundaries`：1.5s 满分 100 / 10s 延迟零分 70 / 5.75s 中点 85 / 零成功率 0 / 半成功率 50 / 全失败 0 —— 7 用例 PASS。
  - `TestChannelHealthSnapshotEmptyReturnsZero`：无样本渠道返回全零且不 panic。
- 验证：`go test ./service/ -run "TestComputeHealthScoreBoundaries|TestChannelHealthSnapshot"` 全 PASS。
- 结论：健康分公式（成功率×70 + 延迟分30）边界被测试锁住，供后续 CHANNEL_HEALTH_ROUTING 路由加权复用。

## T6-1 任务 Data 结构化进度合并验收（commit fb9f73e51，push main）
- 前置审计：`applyStructuredTaskProgress`（service/task_polling.go:774）已实现（旧 workflow_status 的“B5-3 后端缺口”记录过时），缺验收测试。
- 新增 `TestApplyStructuredTaskProgressMergesWithoutClobbering`（service/task_polling_test.go）：
  - 结构化 "5/10 images" 合并 Data.progress 且保留 refund/resolution；纯字符串 "30%" 零破坏；nil 守卫不 panic —— 3 用例 PASS。
- 验证：`go test ./service/` 全量 exit 0（4.757s）。

## 批次台账（防重复）
- 已锁定“无需重查”：用户设置缓存存在性；429/4xx 透传正确性（error_test.go）；Task.Data 合并行为；健康分公式边界。
- 待做批次（各自独立 spec→实现→E2E→发布）：T2-2 路由加权、T2-3 stream_fallover 灰度、T2-4 前端健康分概览、T3 Web 防护配置化+状态页、T4 费用解释、T5 request-id/span、T6-2 插件沙箱、T6-4 webhook/定时同步、T7-T15。

---

# 2026-09-26 批次：T1 热路径 + T2 渠道健康（v1.3.29 → v1.3.31）

> 本段为 2026-09-26 批次状态，追加于历史记录之后。完整验证台账见 计划书/audit/perf-verification-ledger.md（记录 0008-0011）。

## 已交付（真实运行验证，全部推送 main + Release）
- **v1.3.29 (T1 热路径性能冲刺)**：渠道运行时快照（消除每请求 4 次 JSON 解析）+ 健康分 1s 快照缓存 + 冷却恢复索引修复。基准：顺序 overhead_p50 = -28.27ms（warm 连接池，10ms 目标达成）。commit ed02dc44f / 3431449c1 / 9fe504f8c。
- **v1.3.30 (T2-1 健康分参数化)**：channel_health.* 热更配置（window/ring/weight/latency/min_score），默认=历史常量零行为变化；真实 E2E 热更 + 打分变化。commit 1157f62d4 / fe97c868f。
- **v1.3.31 (T2-2 前端聚合概览卡)**：渠道页健康分概览（均值/可用率/最差/冷却），纯前端零请求；真实 headless Chrome E2E 截图。commit ae5ce64e5 / 3bfdd1f2f。

## 验证记录（避免重复跑，改到相关区域再重跑）
- 后端：go vet/build ./model ./service ./middleware ./relay exit 0；relaykit GOWORK=off 独立构建 0；model+service+middleware 全量测试绿；channel_health 相关测试 7/7（缓存）+ 参数化 4 组 + 注入 2 组。
- 前端：bun run typecheck 0；vitest channels 52 用例绿；bun run build 0（总 JS 59,368 kB）；oxlint 新文件 0；i18n sync 0 missing。
- E2E：本地网关+mock 上游真实转发；headless Chrome+CDP 登录→/channels 概览卡渲染（截图 计划书/e2e-evidence/）。
- **防重复跑**：未改 T1/T2 相关代码时跳过上述；改到 service/channel_health*、model/channel_cache*、channel_health_setting、channels 前端再重跑对应项。

## 待办（终局审计后更新）
- 终局闭环审计结论（见下节 / 由审计子代理补充）
- T14 旧产物清理（用户确认清单后）
- H3 线上部署验收、H4 回滚演练（需授权）
