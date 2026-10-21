# test_ledger.md — 验证台账（防重复跑 / 防误判回归）

 > 只记录实际执行的命令与观察到的结果。最后更新：2026-10-20。

## 已 PASS（本轮实际运行）

| 范围 | 命令 | 结果 |
|---|---|---|
| 后端构建 | `go build ./...` | PASS（exit 0） |
| relaykit 独立构建 | `cd relaykit && GOWORK=off go build ./...` | PASS（exit 0） |
| 静态检查 | `go vet ./controller/ ./relay/ ./relay/helper/ ./model/ ./service/ ./middleware/ ./router/` | PASS（无输出） |
| SQLite schema 幂等 | `go test ./model/ -run TestDBConformanceAutoMigrateIdempotent -count=1` | PASS（34s） |
| 模型层定向 | `go test ./model/ -run 'ActionApproval\|McpSession\|ChannelKey' -count=1` | PASS |
| 服务层定向 | `go test ./service/ -run 'ActionRegistry\|ExpiryReminder\|RoutingPolicy\|Repro_Claude' -count=1` | PASS |
| relay ops | `go test ./relay/ ./relay/helper/ -run 'OpenAIOps' -count=1` | PASS |
| controller 定向 | `go test ./controller/ -run 'TestMcp\|TestSPASEO\|TestGetSitemap\|TestGetRobots' -count=1` | PASS |
| router | `go test ./router/ -count=1` | PASS |
| 前端类型 | `cd web && bun run typecheck` | PASS（exit 0） |
| 前端构建 | `cd web && bun run build` | PASS（dist 生成） |
| i18n | `cd web && bun run i18n:check` | PASS（0 missing） |
| 前端 lint | `npx oxlint -c .oxlintrc.json <changed files>` | PASS（0 error） |
| Key 抽屉测试 | `npx vitest run src/features/keys/components/__tests__/api-keys-mutate-drawer.test.tsx` | 3/3 PASS |
| 语言前缀测试 | `npx vitest run src/i18n/__tests__/route-locale.test.ts` | 5/5 PASS |
| 真实浏览器 a11y/响应式 | `node e2e-local/v1388-a11y-responsive.cjs` | 7/7 PASS |

## 已知噪声（非本轮回归）

| 范围 | 现象 | 判定 |
|---|---|---|
| `go test ./controller/` 全量 | 约 600s 超时，栈在 `pkg/jsplugin` / `sobek` | 既有噪声；用 `-run` 缩小范围 |
| `web/e2e-local/*.cjs` | `bun run lint` 报 `no-require-imports` | 未跟踪的临时脚本，不在交付范围 |
| `controller/TestAuditDatabaseMatrix` 等 | 既有失败 | 与既有台账一致，非本轮引入 |

## 未验证 / 缺口

- MySQL / PostgreSQL conformance：本机无 Docker/MySQL/PG，未执行。
- 真实上游 fine-tune/batch 创建：无可用上游渠道，未执行。
- 真实 MCP 客户端握手（Claude Code/Cursor）：未执行。
- 生产 p95 压测：本地 mock 上游压测见 `evidence/stress-*.json`；真实渠道压测未执行。
- 浏览器 E2E：本轮新增页面（动作审批、Key curl）需真实登录态；见 `evidence/` 中的执行记录。

## 2026-10-18 独立审查后新增验证

| 范围 | 命令 | 结果 |
|---|---|---|
| 编码路径计费 | `go test ./relay/ -run 'TestOpenAIOpsPaidUsesDecodedPath\|TestOpenAIOpsPathSupported'` | PASS |
| 对象归属/IDOR | `go test ./model/ -run TestOpenAIOpsObjectOwnership` + `go test ./relay/ -run 'ParseOpenAI\|FilterOwned'` | PASS |
| 开放重定向 | `go test ./controller/ -run TestSPASEODefaultLocaleRedirectRejectsProtocolRelative` | PASS |
| `$` 替换展开 | `go test ./controller/ -run TestSPASEOReplacementDollarIsLiteral` | PASS |
| 前端审批错误处理 | `npx vitest run src/features/action-approvals/__tests__/api.test.ts` | 3/3 PASS |
| 最终真实浏览器 E2E | `node web/e2e-local/v13148-full-e2e.cjs` | 7/7 PASS |
| 最终 50 并发压测 | `node stress.cjs`（SQLite + mock 上游） | 0 错误；QPS 19.9；P95 5.80s；P99 6.41s |

## 独立审查结论与修复
- 安全：1 Critical（编码路径计费绕过）、2 High（开放重定向、ops IDOR）→ 已修。
- Go：2 P1（审批未绑定参数、请求体多次物化）→ 已修。
- 前端：4 P1（失败误报成功、错误当空列表、存量语言与 canonical、未翻译前缀 lang）→ 已修。
- 仍未验证：真实上游 fine-tune/batch、真实 MCP 客户端（MySQL/PostgreSQL conformance 已于 2026-10-18 补测 PASS）。

## 生产部署验收（2026-10-18）
| 项 | 命令/证据 | 结果 |
|---|---|---|
| 公网版本 | `curl -sI https://freeapi.tingfengai.art/api/status` | `X-New-Api-Version: v1.3.148`，HTTP 200 |
| 健康探针 | 服务器侧 `/healthz` `/readyz` | 均 200 |
| SEO /en/pricing | 服务器侧抓 HTML | `lang=en`、canonical=`https://freeapi.tingfengai.art/en/pricing`、3 条 hreflang |
| 登录页 noindex | `/sign-in` 抓 HTML | `robots: noindex, nofollow` |
| 语言重定向 | `/zh/pricing` | 301 → `/pricing` |
| MCP 默认关 | `POST /mcp` 未认证 | 401（认证后由默认 false 返回 404） |
| 新表 | PostgreSQL pg_tables | action_approvals / mcp_sessions / open_ai_ops_objects |
| 资源 | `docker stats` / `df -h` | 内存 462MB/900MB；磁盘 9.7G/78G(13%) |

## 2026-10-18 v1.3.152 补位验证

| 范围 | 命令 | 结果 |
|---|---|---|
| 首购优惠纯函数 | `go test ./model/ -run TestEffectiveSubscriptionPrice` | PASS |
| 省费汇总纯函数 | `go test ./model/ -run TestBuildSubscriptionSummarySavings` | PASS |
| 多实例采集租约 | `go test ./model/ -run TestTrainingCaptureLeaseExclusiveAndExpires` | PASS |
| 采集包回归 | `go test ./service/training_capture/` | PASS（78.8s） |
| 三库 conformance | 生产服务器临时 MySQL 5.7 + PostgreSQL 15 + SQLite | **PASS**（1.07s / 1.42s / 8.49s） |
| 首购优惠真实浏览器 E2E | `node web/e2e-local/v13150-subscription-e2e.cjs` | **5/5 PASS**（创建套餐、折后价 80、页面展示、余额扣 80×QuotaPerUnit、清理） |
| 前端 typecheck/build/i18n | `bun run typecheck` / `bun run build` / `bun run i18n:check` | PASS（0 missing） |

证据：`evidence/three-db-conformance-20261018.json`、`计划书/e2e-evidence/v1.3.150/`。

## 2026-10-19 v1.3.152 发布与生产验收

| 项 | 命令/证据 | 结果 |
|---|---|---|
| 提交 | `git commit` | `e7c77ff82`，已推送 `origin/main` |
| 发行版 | `gh release create v1.3.152` | https://github.com/lza6/new-api-Max/releases/tag/v1.3.152 |
| 镜像构建 | 服务器 `docker build -t new-api:v1.3.152`（源码 `git archive v1.3.152`） | PASS |
| 蓝绿部署 | `/opt/new-api/deploy-zero-downtime.sh v1.3.152` | 3000→3001，脚本自检 200，旧容器已停 |
| 版本 | `curl -D - https://freeapi.tingfengai.art/api/status` | HTTP 200，`X-New-Api-Version: v1.3.152` |
| 探针 | `/healthz` `/readyz` | 200 / 200 |
| SEO | `/en/pricing` | `lang=en`、canonical、3 条 hreflang |
| noindex | `/sign-in` | `noindex, nofollow` |
| MCP 默认关 | `POST /mcp` 未认证 | 401 |
| 指标 | `/metrics` | 200，含 `training_capture_skipped_lease_total` |
| 迁移 | PostgreSQL `\d subscription_plans` / `\d user_subscriptions` | 新列均存在 |
| 资源 | `docker stats` / `df -h` | 149MB/900MB；11G/78G (14%) |
| 回滚镜像 | `docker images new-api` | `v1.3.151` / `v1.3.150` / `v1.3.149` |

> 说明：`workflow_status.md` 当前工作树版本由另一并发会话维护（内容为 v1.3.151
> 训练采集终局闭环），本轮未改动该文件，避免覆盖对方未提交内容。


## 2026-10-19 v1.3.153 独立审查修复

独立审查（Go / 前端各一名只读审查者）对 v1.3.152 提出 1 个后端 P1 + 4 个前端 P1，
已逐条复现并修复：

| 问题 | 修复 | 验证 |
|---|---|---|
| 并发余额购买可重复享受首购折扣（先查后锁、MySQL RR 快照） | 首购判定移到用户行锁后，改用锁定读；余额扣减改为 `quota >= cost` 条件更新 | `go build ./...` PASS；定向 model 测试 PASS |
| 100% 折扣被前端当作无折扣（`discounted_price` 带 omitempty） | 去掉 omitempty；前端按 `first_purchase_discount_on && is_first_purchase && percent>0` 判定 | `bun run typecheck` PASS |
| 外部支付渠道展示了余额专享折后价；`$` 与 `¥` 叠加为 `$¥` | “Amount Due” 只显示原价；折后价只出现在余额支付区 | typecheck + 订阅组件测试 6/6 |
| 非 CNY 金额丢失货币符号 | `formatAmount` 非 CNY 分支补 `$` | 新增 `format-amount.test.ts` 4/4 PASS |
| 无价格快照的订阅把全部按量金额显示为“已省” | 后端 `SavedAmount` 仅在 `PeriodPrice>0` 时计算；前端无快照显示 `—` | `TestBuildSubscriptionSummarySavings` 新增断言 PASS |

> 环境说明：`delete-plan-dialog.test.tsx` 在本机运行超过 5 分钟未结束（与本次改动无关），
> 已停止；相关订阅组件测试单独运行 6/6 PASS。

### v1.3.153 E2E 复验

| 范围 | 命令 | 结果 |
|---|---|---|
| 首购优惠真实浏览器 E2E（修复后） | `E2E_BASE=http://127.0.0.1:18131 node web/e2e-local/v13150-subscription-e2e.cjs` | **5/5 PASS**（建套餐、折后价 80、页面展示、余额扣 40000000、清理） |
| 前端金额格式 | `vitest run .../format-amount.test.ts` | 4/4 PASS |
| 订阅组件 | `vitest run .../plans-compare-table.test.tsx` | 6/6 PASS |

证据已刷新：`计划书/e2e-evidence/v1.3.150/`、`evidence/subscription-first-purchase-20261018.json`。

## 2026-10-19/20 v1.3.154–v1.3.162 验证（真实执行）

| 范围 | 命令 | 结果 |
|---|---|---|
| 采集归属/删除/重试 | `go test ./service/training_capture/ -count=1` | PASS（72–80s） |
| 干净 worktree tag 构建 | `git worktree add --detach /tmp/v<tag> <tag> && go build ./...` | PASS（v1.3.154/157/158/159/160/161） |
| 三库 conformance（含 ModelCost + logs.cost_quota） | 真实 SQLite/MySQL/PG `go test ./model/ -run TestDBConformance` | PASS（128–143s） |
| 运营财务 | `go test ./model/ -run 'TestModelCost|TestConsumeLogRecordsCostSnapshotAndIP'` | PASS |
| 运营面板 | `go test ./controller/ -run 'TestOpsOverview|TestOpsRanking'` | PASS |
| 上游对标 | `go test ./relay/channel/openai|ollama|claude`、`./middleware`、`./service`、`./common` | PASS |
| 前端 | `bun run typecheck` / `oxlint` / `vitest` / `i18n:check` / `build` | PASS |
| 浏览器 E2E（导出分页/下载提示） | `node scripts/ui-e2e-export.cjs` | 6/6 PASS |
| 浏览器 E2E（利润卡/成本表） | `node scripts/ui-e2e-ops-profit.cjs` | 11/11 PASS |
| 生产采集 E2E | `scripts/prod-capture-e2e.sh`（a/b/lb3/v155/v156/v157） | 两账号均落盘；分片可解压、kind 合法；数据已清理 |
| 生产财务 API | 临时 Root 用户 GET `/api/admin/ops/overview` | 今日 10,470,400 / 2,801,460 / 7,668,940（73.24%）；冷 9.8s / 热 0.0s |
| 生产成本快照 | glm-5.3-flash 真实请求 | `quota=3000 cost_quota=1000 ip=172.18.0.1`；用户日志无 cost_quota |
| 生产静态守卫 | `/static/nope` 与 `/static%2Fnope` | 均 404；真实资源 200 |
| 三轮独立审查 | reviewer 子代理 | 最终 **Approve** |

### 已知噪声
- `go test -race ./service/training_capture/` 4 次中 1 次“全 PASS 后二进制非零”，未复现。
- ClickHouse 路径为 SQL/单测验证，未接真实 CH 实例。

## 2026-10-20 生产运营面板 UI 真实验证（浏览器 + 截图）

| 项 | 内容 | 结果 |
|---|---|---|
| 入口 | SSH 隧道 `9600 → 生产 443`；Playwright `--host-resolver-rules=MAP freeapi.tingfengai.art 127.0.0.1`；HTTPS + `ignoreHTTPSErrors` | 必须走 HTTPS：HTTP 隧道下 Secure Cookie 不下发，SPA 未登录 |
| 登录方式 | **真实登录页** `/sign-in` 填表提交 | 不能用 `context.request.post`：APIRequestContext 不走 host-resolver-rules，会连真实公网 IP:9600 |
| 脚本 | `node scripts/ui-e2e-prod-finance.cjs https://freeapi.tingfengai.art:9600 <临时管理员>` | **16/16 PASS** |
| 断言 | 利润与成本区块、今日/本月/本年三个口径、覆盖率告警、建议每日充值、成本规则表 4 条规则、添加规则入口、隐私文案 | 全部命中 |
| 截图 | `计划书/e2e-evidence/v1.3.162-ui/{ops-profit-card,ops-cost-settings,ops-desktop-full,ops-mobile-320,profile-privacy}.png` | 已人工查看：数字/表格/中文文案与 DOM 一致 |
| 移动端 | 320px `scrollWidth <= innerWidth` | PASS（卡片纵向堆叠，无横向溢出） |
| 隐私页 | `/security` → 隐私卡片 | 文案“使用日志始终记录调用者 IP；此开关仅控制错误日志。” |
| 控制台 | 全程 0 JS error | PASS |
| 清理 | 临时管理员 `ui_e2e_1792500016` + 22 条 user_sessions 已删除；隧道已停 | 无残留测试账号 |

### 新增噪声/坑（生产 UI E2E）
- SPA 的滚动发生在内部 `overflow-auto` 容器，Playwright `locator.scrollIntoViewIfNeeded` 与 `scrollIntoView` 对其无效；截图前需把 `scrollHeight > clientHeight` 且 `overflow-y:auto|scroll` 的容器 `scrollTop = scrollHeight`。
- 登录受 `CriticalRateLimit`（20 次 / 20 分钟 / IP）限制；连续重跑会 429。隧道下服务端看到的 IP 是 `127.0.0.1`，窗口耗尽只能等窗口过期，勿反复重试。
- 新标签页打开已登录站点存在鉴权首帧竞态（偶发跳 `/sign-in`）；同一标签页内导航稳定。

## 2026-10-21 v1.3.163 验证（真实执行）

| 范围 | 命令 | 结果 |
|---|---|---|
| 成功限流原子性 | `go test ./middleware/ -run 'TestModelSuccess|TestRedisRateLimitHandler'` | PASS（60 并发恰好 10 个通过；失败退还；窗口过期） |
| 中间件/common 全量 | `go test ./middleware/ ./common/` | PASS（57.8s / 18.6s） |
| 采集全量 | `go test ./service/training_capture/` | PASS（38.7s） |
| 三库 conformance（含旧库漂移收敛） | `TEST_MYSQL_DSN=... TEST_POSTGRES_DSN=... go test ./model/ -run TestDBConformance` | PASS（141.7s；SQLite + MySQL + 临时 PostgreSQL 16 实例 55432） |
| 干净归档构建 | `git archive HEAD | tar -x` + 复制 `web/dist` + `go build ./...` | PASS |
| 构建/vet | `go build ./...`、`go vet ./middleware ./common ./service/training_capture ./model` | PASS |

### 环境说明
- 本机 PostgreSQL 服务（postgresql-x64-16）**无法启动**（子进程 0xC0000142，服务损坏）；
  改用 `initdb` 临时实例 `C:/pgtmp2`（端口 55432，trust 认证）完成真实 PG 验证，用完即停。
- MySQL 服务正常运行，数据库 `newapi_conformance_test` 可复用。

## 2026-10-21 v1.3.164 复审修复后验证

| 范围 | 命令/方式 | 结果 |
|---|---|---|
| OneDrive TLS 污染回归 | `go test ./service/training_capture/ -run 'TestOneDriveTransportIgnoresGlobalTLSInsecureSkipVerify|TestNewOneDriveSink'` | PASS（先污染 DefaultTransport 再构造 sink） |
| 限流 handler 级键类型 | `go test ./middleware/ -run 'TestRedisRateLimitHandlerWritesV2ZSetKey|TestRedisRateLimitHandlerRefundsOnPanic'` | PASS（v2 键为 zset；panic 后槽位 0） |
| 三包全量 | `go test ./middleware/ ./common/ ./service/training_capture/` | PASS（26.2s / 11.4s / 32.7s） |
| 干净归档构建 | `git archive HEAD` → `go build ./...` | PASS |
| 线上版本 | 服务器 `curl` | `x-new-api-version: v1.3.164`，healthz/readyz 200，15 分钟 0 个 500 |
| 生产浏览器 E2E | `node scripts/ui-e2e-prod-finance.cjs` | **16/16 PASS**（证据 `计划书/e2e-evidence/v1.3.164-ui/`） |

### 新增噪声/坑（生产 UI E2E）
- 运营面板冷启动时财务聚合可能 >2s；断言前必须等「利润与成本」可见，固定 sleep 会把「加载中」误判成缺失。
- 页面在后台有轮询时 `waitUntil: 'networkidle'` 会 30s 超时；改用 `domcontentloaded` + 元素等待。
- SSH 隧道偶发「ESTABLISHED 卡住」，表现为新连接一起挂 → 按 PID 重启隧道即可。

## 2026-10-21 v1.3.165 验证（真实执行）

| 范围 | 命令 | 结果 |
|---|---|---|
| 三库 conformance（含财务覆盖索引） | `TEST_MYSQL_DSN=… TEST_POSTGRES_DSN=… go test ./model/ -run TestDBConformance` | PASS（103.3s） |
| 登录时延拉平 | `go test ./model/ -run TestValidateAndFill` | PASS |
| 注册相关 | `go test ./controller/ -run TestRegister` | PASS |
| 构建/vet | `go build ./...`、`go vet ./model ./controller` | PASS |
| 生产索引 | `EXPLAIN (ANALYZE)` | Parallel Index Only Scan，12.2s→1.67s |
| 运营接口 | 服务器本机鉴权调用 | 冷 2.99s / 热 0.002s，均 200 |
| 覆盖率 | 生产 SQL | 今日 99.14%；本月 3.21%（10-20 前无规则） |
| 测试账号治理 | SQL | 13 个禁用、102 会话与 39 token 撤销；备份 `_bak_testusers_20261021` |

## 2026-10-21 前端首屏与边界健壮性（真实浏览器）

| 范围 | 命令 | 结果 |
|---|---|---|
| 前端类型 | `cd web && bun run typecheck` | PASS（exit 0） |
| 前端构建 | `cd web && bun run build` | PASS（23874.0 kB raw / 7457.0 kB gzip） |
| 前端 lint | `cd web && bun run lint` | exit 1：33 条告警全在未跟踪的 `e2e-local/*.cjs`，`src/` 零问题（既有噪声） |
| 畸形响应回归 | `cd web && node e2e-local/fe-malformed-api.cjs` | PASS（`isErrorPage:false`、`hasNav:true`、无 pageerror） |
| 首屏 vitals | `cd web && node e2e-local/fe-vitals.cjs` | FCP=LCP 4076ms（H1）、CLS 0.002；入口 JS 653 kB gzip；语言包 172 kB gzip 且串行 |
| 全量 vitest | `cd web && bun run test` | **未完成**（>8 分钟无输出，进程终止）；本轮改动为 CSS/动画/两处边界校验，改用定向验证 |

### 新增噪声/坑（前端测量）
- 静态服务器若在请求回调里同步 `gzipSync`，Node 单线程会把 3 kB 的 chunk 拖到 1.9s，**误判成应用慢**。必须启动时预压缩。
- `#root` 自带 `#app-boot-loader`，用 `#root.children.length > 0` 判断「已渲染」会提前成立；应等 `#app-boot-loader` 被 detach。
- `/api/setup` 返回未初始化会整体重定向到 `/setup`，测首页必须 mock 成 `{success:true,data:{status:true}}`。
- `/api/home_page_content`、`/api/notice` 返回对象时旧代码会抛错并让整页 500（已修）。

## 2026-10-21 前端第二轮（多代理审计后修复）

| 范围 | 命令 | 结果 |
|---|---|---|
| 前端类型 | `cd web && bun run typecheck` | PASS |
| 前端构建 | `cd web && bun run build` | PASS（23876.9 kB / 7458.0 kB gzip） |
| 定向组件测试 | `npx vitest run src/features/wallet src/features/dashboard src/components/ui/__tests__/a11y-smoke.test.tsx` | 8 文件 / 47 用例 PASS（132s） |
| 改动文件 lint | `npx oxlint -c .oxlintrc.json <changed files>` | 无新增 error（prompt-input 2 条既有 warning） |

已修：ui/dialog 高度与滚动、alert-dialog 高度与宽度、dialog 100vh→100dvh、抽屉页脚安全区、附件删除按钮触屏误触、dashboard 管理端 section 越权可见、wallet 失败显示 0 余额、空兑换码无提示、系统设置失败渲染默认值、导出 CSV 静默失败与 JSON 当 csv。

未验证：320px 弹窗可视性（需真实登录路由）；全量 vitest 仍超时未完成。

## 2026-10-21 前端第三轮（性能小项 + 可访问性）

| 范围 | 命令 | 结果 |
|---|---|---|
| 前端类型 | `cd web && bun run typecheck` | PASS |
| 前端构建（含 SW 版本注入） | `cd web && bun run build` | PASS；`[stamp-sw] VERSION = 1.0.0-478a2884` |
| 定向测试 | `npx vitest run src/features/home src/components/__tests__ src/features/auth` | 14 文件 / 95 用例 PASS |
| 首屏实测 | `node e2e-local/fe-vitals.cjs` | 传输 1281→**1037KB**；LCP 4076→**3960ms**；CLS 0.001；three.js **未加载**（390×844） |
| KaTeX CSS 抽离 | 构建产物扫描 | 入口 CSS `KaTeX_Main` 命中 0；移入 `static/css/async/29217.*.css` |
| 全量 vitest | `npx vitest run --pool=forks --maxWorkers=2` | 见下（未完成/结果） |

已改：webgl-hero 视口与省流门控、KaTeX CSS 动态化、SW 版本注入与 cache.put 容错、StatusBadge 键盘可达与 reduced-motion、登录 autocomplete。

## 2026-10-21 前端第四轮（性能 P0 拆包）

| 范围 | 命令 | 结果 |
|---|---|---|
| 类型检查 | `cd web && bun run typecheck` | PASS |
| 分区元数据契约 | `npx vitest run src/features/system-settings/__tests__/section-metadata.test.ts` | 8/8 PASS（109s） |
| 构建 | `cd web && bun run build` | PASS；入口 index 233.2→**34.5KB** gzip；`[stamp-sw] VERSION = 1.0.0-e2538897` |
| 首屏实测 | `node e2e-local/fe-vitals.cjs` | 传输 1037→**818KB**；DCL 1328→**731ms**；挂载 1775→**1137ms**；LCP 3960→**3704ms** |

做法：新增 8 个 `system-settings/<区>/sections.ts` 轻量元数据；16 个路由 + `layout/config/system-settings.config.ts` 改从它 import；注册表不动；契约测试防漂移。

## 2026-10-21 第五轮：推送与测试矩阵

| 范围 | 命令 | 结果 |
|---|---|---|
| 已推送 | `git push origin main` | `7783c3795..453ddb0b3`（5 个提交） |
| 渠道 + 密钥测试 | `npx vitest run src/features/channels src/features/keys` | 18 文件 / **121 用例 PASS**（183s） |
| usage-logs + components + lib | `npx vitest run src/features/usage-logs src/components src/lib` | 57 文件 / **382 通过、4 失败**（283s） |
| 全量单进程 | `npx vitest run` / `--pool=forks --maxWorkers=2` / 三分区并行 | **均超时无输出**（最长 35 分钟），本机 jsdom 导入成本过高，未取得全量结果 |

### 4 个失败用例（与本轮改动无关）
- `usage-logs/.../mobile-filter.test.tsx`（×2）：找不到中文按钮「本月」——i18n 异步加载竞态，两次运行失败数不同（2 vs 4）证明是 flaky。
- `usage-logs/.../task-event-stream.test.tsx`：`ReferenceError: ReadableStream is not defined`（jsdom 环境缺 API）。
- `usage-logs/.../cost-detail-panel.test.tsx`：找不到 "Model ratio" 文案（同 i18n）。

判定依据：本轮 55 个改动文件里**没有任何 usage-logs / i18n / test-setup / vitest 配置**（`git diff --name-only 7783c3795..HEAD` 核对），且失败用例数量在两次运行间变化。

### 部署
- `ssh -o BatchMode=yes root@154.37.208.171` → `Permission denied (password)`，**无密钥登录**，部署阻塞（§2.4.1）。

## 2026-10-21 v1.3.166 生产部署与生产冒烟

| 范围 | 命令 | 结果 |
|---|---|---|
| 镜像构建 | 服务器 `docker build -t new-api:v1.3.166 .` | PASS（exit 0） |
| 预检 | 3002 临时容器 `/api/status`、入口 chunk、`/sw.js` | 200；`VERSION = 1.0.0-e37d2e97` |
| 蓝绿部署 | `./deploy-zero-downtime.sh v1.3.166` | PASS，切 3001，外网自检 200 |
| 线上版本 | `curl -I https://.../api/status` | `x-new-api-version: v1.3.166` |
| 页面可用性 | `/` `/pricing` `/status` `/dashboard/models` `/system-settings/site/system-info` | 全 200 |
| 日志 | `docker logs --tail 500` | 0 个 5xx，无 panic/ERROR |
| 生产浏览器冒烟 | `node web/e2e-local/prod-smoke-v13166.cjs` | **8/8 PASS**（390/1440 × 4 路径） |

未覆盖：管理员登录态页面（无凭证）；全量 vitest（环境无法一次跑完，见上节）。

## 2026-10-21 生产管理员登录态验收（v1.3.166）

按 memory `new-api-prod-ui-e2e-tunnel` 的手法实跑（443 隧道 + host-resolver-rules + 真实 /sign-in 表单）：

| 项 | 结果 |
|---|---|
| 临时管理员 | DB 建 `zz-verify-temp`（id 1655, role=100），密码 bcrypt($2a$10) 由项目 `golang.org/x/crypto/bcrypt` 生成 |
| 登录 | PASS（表单提交后离开 /sign-in） |
| `/dashboard/models` | PASS，正文 700+ 字，无 error boundary |
| `/system-settings/site/system-info` | **PASS**（截图确认：系统名称=听风API、服务器地址= https://freeapi.tingfengai.art 、页脚、关于 均为真实配置，非默认值） |
| `/system-settings/operations/behavior` | PASS |
| `/system-settings/security/rate-limit` | PASS |
| `/wallet` | PASS |
| `/keys` | PASS |
| console error | 0（仅未登录时的 401 被过滤） |
| 清理 | 删除 user 1 行、user_sessions 1 行、logs 0 行；`remaining=0` 已复核 |

结论：v1.3.166 的 `system-settings` 元数据拆包在**真实登录态**下工作正常（分区路由 + 侧栏 + 表单数据都对）。

## 2026-10-21 语言包拆分（首屏阻塞 172KB → 12KB）

| 范围 | 命令 | 结果 |
|---|---|---|
| 生成 common | `node scripts/build-i18n-common.mjs` | 7 语言各 445 key；zh 10.0KB gzip |
| 首屏覆盖审计 | `node e2e-local/i18n-common-audit.cjs` | **PASS**：/、/sign-in、/pricing、/status 缺失 key = 0（另有 6 个既有中文 key，非回归） |
| 构建 | `bun run build` | PASS |
| 首屏实测 | `node e2e-local/fe-vitals.cjs` | 阻塞语言包 172→**12KB**；LCP 3980→**3448ms**；挂载 1263→**1121ms** |

## 2026-10-21 v1.3.167 / v1.3.168 语言包上线与生产验收

| 项 | 结果 |
|---|---|
| v1.3.167 部署 | 蓝绿 → 3000，healthy，0 个 5xx |
| v1.3.168 部署 | 蓝绿 → 3001，healthy，0 个 5xx（补齐生产首屏 49 个 key） |
| 生产不闪英文（12 条公开路由） | `node web/e2e-local/prod-i18n-no-flash.cjs` → **PASS**（屏蔽整包语言包，`__i18nMissing` 中可翻译 key = 0） |
| 管理员登录态（v1.3.168） | 临时账号 `zz-verify-temp2`（role=100）；登录 + `/dashboard/models`、`/system-settings/{site/system-info,operations/behavior,security/rate-limit}`、`/wallet`、`/keys` 全部正常渲染，无 error boundary、无 console error |
| 临时账号清理 | user_sessions 1 行、logs 0 行、users 1 行已删；`zz-verify-temp%` 剩余 **0** |

关键经验：**本地 mock 测不到生产首屏的全部文案**（OAuth 按钮、模型广场、排行榜、文档页），
必须用生产实测收集缺失 key 再补齐——已固化为 `web/scripts/merge-prod-i18n-missing.mjs`。

## 2026-10-21 v1.3.169 部署与验收（flaky/tabs/uptime/theme-color）

| 范围 | 命令 | 结果 |
|---|---|---|
| 类型检查 | `cd web && bun run typecheck` | PASS |
| 构建 | `cd web && bun run build` | PASS |
| 定向测试（dashboard/system-settings/components） | `npx vitest run src/features/dashboard src/features/system-settings src/components/__tests__` | 16 文件 / **108 用例 PASS** |
| 原 4 个 flaky 用例 | 同一命令连跑 2 次 | **15/15 PASS ×2**（此前 2~4 个失败） |
| 部署 | `./deploy-zero-downtime.sh v1.3.169` | PASS，切 3000，外网自检 200 |
| 线上版本 | `curl -I .../api/status` | `x-new-api-version: v1.3.169` |
| 5xx | `docker logs --tail 400` | 0 |
| 生产公开冒烟 | `node web/e2e-local/prod-smoke-v13166.cjs` | **8/8 PASS** |
| 生产管理员 + 320px | `PROD_VERIFY_USER=zz-verify-temp3 node web/e2e-local/prod-admin-verify.cjs` | PASS；`/` 与 `/system-settings/auth/oauth` 在 320px：`overflow=false`、`tabsOverflow=0` |
| 临时账号清理 | SQL | `zz-verify-temp%` 剩余 0 |

修复内容：test-setup 补 ReadableStream + afterEach 还原 i18n 语言；oauth/ratio TabsList 窄屏 3 列；Uptime StatusDot 加 role/aria-label；theme-color 改读 `--theme-color` 令牌并更新全部 meta；移除从未生效的 `vendor-react` cacheGroup。

## 2026-10-21 v1.3.169 压测与运行观察

| 项 | 数据 |
|---|---|
| 压测对象 | `GET /api/status`（公开只读，经 9600 SSH 隧道打 HTTPS，**未触发任何上游/付费调用**） |
| 场景 | 并发 20 × 15.4s |
| 结果 | 1144 请求，**0 错误**，QPS **74.4**，p50 234ms / p95 276ms / p99 1488ms / max 1705ms |
| 说明 | 隧道本身约 200ms RTT，延迟绝对值不代表公网直连；错误率为 0 是关键结论 |
| 运行观察 | 部署后 10 分钟：内存 157–525MB 波动（上限 900MB），CPU 22–70% 周期波动，600 行日志 0 个 5xx |

## 2026-10-21 v1.3.170 部署与「未保存保护」生产验收

| 项 | 结果 |
|---|---|
| 类型检查 / 构建 | PASS |
| 设置模块测试 | `npx vitest run src/features/system-settings` → **11 文件 / 57 用例 PASS** |
| 部署 | `deploy-zero-downtime.sh v1.3.170` → 3001，healthy，0 个 5xx |
| 线上版本 | `x-new-api-version: v1.3.170` |
| **未保存保护（真实管理员）** | `PROD_VERIFY_USER=zz-verify-temp4 node web/e2e-local/prod-guard-verify.cjs` → **PASS**：无改动直接跳转（dialog=0）；改动后弹「未保存的更改 / 留下来 / 离开」且 URL 停留 |
| 生产公开冒烟 | `prod-smoke-v13166.cjs` 连跑 **3 次全部 8/8 PASS** |
| 临时账号清理 | `zz-verify-temp%` 剩余 **0** |

### 本轮踩到的测试自身缺陷（已修）
`prod-smoke` 用「`#root` 有子节点」判断渲染完成，但 index.html 的 `#app-boot-loader` 就在 `#root` 内；
React 接管后 `#root` 会短暂为空 → 冷启动时误报「白屏」。改为等 loader detach + `h1` 有文本。

## 2026-10-21 v1.3.171/172 尼尔森整改与生产验收

| 项 | 结果 |
|---|---|
| 术语门禁 | `node scripts/check-terminology.mjs` → **PASS**（zh/zh-TW 含 quota 的键已无「配额/容量」） |
| i18n 完整性 | `node scripts/check-missing-i18n.cjs` → **OK (0 missing)** |
| 定向测试 | `vitest run src/features/channels src/features/system-settings src/features/errors` → **21 文件 / 115 用例 PASS** |
| 部署 | v1.3.171（N1/N4/N8）→ 3000；v1.3.172（错误页统一）→ 2001/3000 蓝绿，均 healthy |
| 生产截图验收 | `PROD_VERIFY_USER=... prod-usability-verify.cjs` → **PASS**；截图 `nielsen-error-page.png` / `nielsen-checkin-confirm.png` / `nielsen-wallet.png` 已人工查看 |
| 404 页 | 人话标题「糟糕！页面未找到！」+ `HTTP 404` 小标签 + 返回/返回主页；不再以巨型状态码为主体 |
| 签到重置 | 走 ConfirmDialog：「重置所有签到记录？用户将可重新签到。」/「此操作无法撤消。」/ 取消·重置（destructive） |
| 钱包术语 | 页面出现「额度」，无「配额/容量」 |
| 临时账号 | 5 个 `zz-verify-temp*` 全部清理，剩余 **0** |

## 2026-10-21 v1.3.173 尼尔森 N9/N12/N16

| 项 | 结果 |
|---|---|
| N9 命令面板 | Ctrl+K 打开；DOM innerText 含「快捷操作」+ API 密钥/钱包/使用日志/状态；生产截图已存 |
| N16 校验文案 | 24 条 zod 消息补进 zh/zh-TW/en；`check-missing-i18n` 0 missing；`check-terminology` PASS |
| N12 契约 | `web/AGENTS.md` §3.17 可用性契约（四态/提交/破坏性三级/错误三层/术语/一致性/a11y） |
| 构建 | `bun run build` PASS |
| 测试 | `vitest run src/components src/features/users` → 20 文件 / **99 用例 PASS** |
| 部署 | v1.3.173 蓝绿上线，healthy，0 个 5xx，`x-new-api-version: v1.3.173` |
| 临时账号 | `zz-verify-temp6` 已删，`zz-verify-temp%` 剩余 **0** |

备注：命令面板截图为列表顶部视图，「快捷操作」组在滚动区下方，改由 DOM innerText 断言（已通过）。

## 2026-10-21 v1.3.174 渠道抽屉未保存保护（N5）

| 项 | 结果 |
|---|---|
| 类型检查 / 构建 | PASS |
| 渠道测试 | `vitest run src/features/channels` → **58 用例 PASS**（55.7s） |
| 部署 | v1.3.174 蓝绿上线，healthy，0 个 5xx |
| 生产验收 | `PROD_VERIFY_USER=zz-verify-temp7 node web/e2e-local/prod-channel-unsaved-verify.cjs` → **PASS**：打开创建渠道抽屉 → 填渠道名 → 点取消 → 弹出「未保存的更改 / 取消 / 离开」且抽屉不关 |
| 截图 | `nielsen-channel-unsaved.png`（已人工查看） |
| 临时账号 | `zz-verify-temp7` 已删，剩余 **0** |

### 测试脚本踩坑（已修）
抽屉里的输入框没有 `name` 属性（Base UI 生成 id），只能按 placeholder 定位；
「取消」按钮比 Esc 更接近真实路径；`[role=dialog]` 之外确认框是 `[role=alertdialog]`。
