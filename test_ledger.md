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

## 2026-10-21 v1.3.179/180 计费类改动 L3 输入确认（N3）

| 项 | 结果 |
|---|---|
| `ConfirmDialog.requireTypedText` | 新增可选 prop：渲染输入框，未逐字输入时确认按钮禁用 |
| 回归测试 | `npx vitest run src/components/__tests__/confirm-dialog.test.tsx` → **2/2 PASS**（含「无该 prop 时行为不变」） |
| 计费分区接入 | 保存时若改动 `QuotaPerUnit` / `USDExchangeRate` / 展示币种 / 自定义汇率 → 弹「修改价格换算？」并要求输入 `QUOTA` |
| 构建 / 类型 | PASS |
| 设置模块测试 | 57 用例 PASS |
| 部署 | v1.3.179 → v1.3.180 蓝绿上线，healthy，0 个 5xx |
| 生产交互验收 | **未取得**：该实例展示币种已是 CNY 且「每单位额度」未渲染，headless 下切换下拉的脚本挂起（已终止）。组件级契约测试覆盖了核心行为，生产点测留待下一轮 |
| 临时账号 | `zz-verify-temp9` 已删，剩余 **0** |

## 2026-10-21 v1.3.181 后端错误不外泄（N15 第一刀）

| 验证 | 命令 | 结果 |
|---|---|---|
| 后端构建 | `go build ./...` | PASS |
| 静态检查 | `go vet ./common ./controller` | 无输出 |
| common 全量 | `go test ./common/ -count=1` | PASS（含新增 3 个用例：大小写无关匹配、洗白、透传） |
| 控制器定向 | `go test ./controller/ -run 'TestRegister\|TestLogin\|TestOAuth' -count=1` | PASS |
| 前端类型/构建 | `bun run typecheck` / `bun run build` | PASS |
| 前端映射测试 | `vitest run src/lib/__tests__/server-error-code.test.ts` | 2/2 PASS |
| i18n | `check-missing-i18n.cjs` | 0 missing |
| 部署 | v1.3.181 蓝绿上线 | healthy，0 个 5xx，`x-new-api-version: v1.3.181` |
| 生产冒烟 | `prod-smoke-v13166.cjs` | 8/8 PASS |

**未取得**：生产环境的「内部错误被洗白」实测——该分支只在真实内部错误时触发；
本轮未人为构造生产故障，结论来自 Go 单测 + 代码审查 + 调用点未变（仅集中兜底）。

## 2026-10-22 v1.3.182 视觉批次 1（令牌层 + 概览试点）

| 验证 | 命令 | 结果 |
|---|---|---|
| 前端类型 | `cd web && bun run typecheck` | PASS（无输出） |
| Lint（改动文件） | `bunx oxlint -c .oxlintrc.json <10 files>` | 0 error（1 warning 已修：测试内多余花括号） |
| 构建 | `bun run build` | PASS；字体产物 `jetbrains-mono-latin` 40.4 kB，全子集 84 kB |
| 定向测试 | `vitest run src/features/dashboard src/components/ui` | 54/54 PASS |
| 读数组件测试 | `vitest run src/components/__tests__/metric.test.tsx` | 5/5 PASS |
| 全量测试 | `bun run test` | **1 failed**：`usage-logs/.../task-event-stream.test.tsx`（已知噪声，见本文件 :286）；单独跑 6/6 PASS |
| i18n | `node scripts/check-missing-i18n.cjs` | 0 missing |
| 术语 | `node scripts/check-terminology.mjs` | PASS（额度统一） |
| 格式 | `node scripts/format-with-protected-headers.mjs --check` | clean |
| 死代码门禁 | `node scripts/knip-gate.mjs` | 117 条报为"新增"，经核对**全部为既有基线漂移**，无一条指向本批文件（未改基线、未屏蔽） |
| 真实浏览器 E2E | `BASE=http://127.0.0.1:5173 node e2e-local/v13182-visual-local.cjs` | 登录成功 → /dashboard/overview；10 张截图；**axe 0 违规**；1 条登录前 401 console（良性） |
| 令牌实测（浏览器内求值） | Playwright evaluate | `--radius=.625rem`；`--surface-1`/`--rule` 正常；metric 值 `JetBrains Mono Variable` + `tabular-nums`；dashboard `rounded-2xl` 计数 0；body 字体仍 `Public Sans` |

**本地 E2E 环境要点（可复用）**：空库首次访问会跳 `/setup`，用
`POST /api/setup {username,password,confirmPassword}` 直接初始化管理员；后端
`SQLITE_PATH=<tmp>/v182.db PORT=3000 ./new-api-e2e.exe`，前端
`bunx rsbuild dev --port 5173`（proxy → 3000）。

### v1.3.182 生产部署与线上验证（2026-10-22）

| 验证 | 命令（服务器侧） | 结果 |
|---|---|---|
| 镜像构建 | `docker build -t new-api:v1.3.182 /opt/new-api-src-v182` | PASS（builder2 go build DONE） |
| 蓝绿切换 | `cd /opt/new-api && ./deploy-zero-downtime.sh v1.3.182` | 3000→3001 切换；新容器 200；自检 200；旧镜像清理后磁盘 13G/78G（17%） |
| 容器状态 | `docker ps` | `new-api:v1.3.182 Up (healthy)`，RestartCount=0 |
| 版本头 | `curl -sI --resolve freeapi…:443:127.0.0.1 https://freeapi.tingfengai.art` | `HTTP/2 200` + `x-new-api-version: v1.3.182` |
| 健康检查 | `/healthz`、`/readyz` | 200；`{"status":"ready","checks":{"database":"up","redis":"up"}}` |
| 业务接口 | `/api/status` | 200 |
| 首页 | `GET /` | 200，8666 字节，title 正常（`听风API — 一个接口调用多种 AI 模型`） |
| 新令牌生效（产物核对） | 拉取 `/static/css/index.fe22e679b3.css` | 含 `JetBrains Mono Variable`、`--surface-2/3`、`--rule/--rule-strong`、`--grid-line/step`、`--radius:.625rem` |
| 字体产物 | `GET /static/font/jetbrains-mono-latin-ext-*.woff2` | 200，15196 字节 |
| 运行期错误 | `docker logs \| grep -c ERROR / panic` | 0 / 0（最近 500 行） |

**回滚点**：`new-api:v1.3.181`（`./deploy-zero-downtime.sh v1.3.181` 一条命令回到上一版）；
本轮未触发回滚。
**未取得**：生产端**浏览器**截图（本机直连该域名的 TLS 被中间设备 RST，见
`tingfengai-domain-rst-20261010.md`），线上验证以服务器侧 curl + 产物内容核对为准。

---

## 2026-10-23 12:00–12:30 门禁可信度审计（主控会话，真实执行）

> 环境：Windows 本机，go1.26.1，node v25.8.1，bun 1.3.2，vitest 4.1.10，jsdom 29.1.1，**无 docker**。

| 项 | 命令 | 真实结果 |
|---|---|---|
| Go 根模块构建 | `go build ./...` | PASS 54.7s（会话开始）；中途因并行会话在改 `setting/response_cache_setting`（11:56）短暂失败，对方改完后复跑 PASS |
| 前端类型检查 | `cd web && bun run typecheck` | PASS 4.9s |
| 版本口径门禁 | `node scripts/check-version-consistency.mjs` | 自测 9/9 PASS；对真仓库先报 README 过期，修正后 `RESULT: consistent` |
| 术语门禁 | `node scripts/check-terminology.mjs` | **假绿已修**：原豁免正则 `/rate\|limit\|concurren/` 命中 `Unlimited` 里的 `limit`；改 `\b(...)\b` 后 RED 4 处 → 修文案后 GREEN 0 |
| 漏译门禁 | `node scripts/check-missing-i18n.cjs` | 0 missing |
| i18n 覆盖率门禁 | `node scripts/check-i18n-coverage.cjs` | OK（all locales translated） |
| 设计令牌门禁 | `node scripts/ds-tokens-check.mjs` | OK（659 tokens，hash 未变） |
| **knip 死代码门禁** | `node scripts/knip-gate.mjs` | **FAIL(1)**：5 个新死导出（FeatureSwitchKind / FeatureSwitchListData / FeatureSwitchRisk / RelayAuditFinding / IconComponent），来自并行会话在途改动；**未 --update 掩盖** |
| 前端分片全量 | `cd web && node scripts/test-shards.mjs` | 199 文件→5 片；**948 通过 / 13 失败**（第 1–4 片），第 5 片因下述死循环长期不结束 |
| 全量单进程 | `bunx vitest run` | **>25 分钟未完成**（P0-2 证据） |

### 本轮定位的关键缺陷：`plan-comparison.test.tsx` 死循环

- 现象：单跑 `src/features/subscriptions/__tests__/plan-comparison.test.tsx` **>6 分钟不结束**，`--testTimeout=30000` 也压不住（不是慢，是循环）。
- 定位手法：分片 → 二分（40→20→10→5→1）→ 单文件隔离 → **只 import 不 render**（25s 通过，排除 import 死锁）→ 读组件 effect 依赖。
- 根因：测试 mock `react-i18next` 时用内联箭头 `t: (key) => key`，每次渲染都是新引用；组件 `useCallback([isSignedIn, t])` → `useEffect([loadData])` → `setLoading` 触发重渲染 → **无限循环**。
- 修复：mock 里的 `t` 提为模块级稳定引用。**>6 分钟 → 2.9 秒**。
- 修好后暴露 2 个此前被永久遮蔽的真实失败（见下），未削弱断言。

### 当前前端真实失败清单（13 条，均不在本会话改动范围）

| 分片 | 文件 | 失败用例（摘要） |
|---|---|---|
| 1 | `features/models/__tests__/metadata-editing.test.tsx` | 价格草稿跨 tab 保留 |
| 1 | `features/models/__tests__/model-listing.test.tsx` | 3 例：`text-warning` 类名 / 后端方块状态 / 未识别表达式 |
| 1 | `features/system-settings/__tests__/section-metadata.test.ts` | registry 9 vs 10（新增分区未登记） |
| 1 | `features/models/components/dialogs/__tests__/bulk-pricing-dialog.test.tsx` | 3 例：找不到 `Model ratio` / `Cache ratio` 标签、`Apply to all selected` 按钮 |
| 2 | `features/model-pricing/__tests__/editor-layout.test.tsx` | 2 例：固定操作栏保存 |
| 2 | `features/task-plugins/__tests__/marketplace-install-dialog.test.tsx` | 页脚在受限滚动体之外 |
| 3 | `features/task-plugins/__tests__/plugin-icon-file.test.ts` | 图标 digest 一致/不一致行为 |
| 3 | `features/usage-logs/audit/__tests__/viewer.test.tsx` | 每页行数切换后分页重置 |
| 4 | （4 例，明细待补） | — |

归因：这些文件均属并行会话正在改的模块（models 定价编辑、dashboard 状态卡、task-plugins、usage-logs a11y、system-settings 新区块），本会话只改了 `controller/*.go`、`common/gin.go`、waffo 设置分区、术语脚本、locale 2 个键、脚本与文档。

### 已 PASS 且可引用

- `go test ./common/ -run 'TestApiError|TestLooksInternalError' -v` → 5/5 PASS（含 2 条新增）
- `go vet ./controller/` → clean；`go test ./controller/ -run 'XXX_NoSuchTest'` → 测试包可编译
- `bunx vitest run src/i18n/locale-consistency.test.ts` → 2/2 PASS
- `bunx vitest run src/i18n/__tests__/config-default-language.test.ts` → **4/4 PASS**（原 4 例全红：`vi.stubGlobal('window')` 在 jsdom 29 抛 "Cannot redefine property: window"，改为 spyOn(Storage.prototype) 后修复）

## 2026-10-22 v1.3.182 之后：V7 空态工程网格（待发版）

| 验证 | 命令 | 结果 |
|---|---|---|
| 类型 | `cd web && bun run typecheck` | PASS |
| Lint | `bunx oxlint`（empty-state / ui/empty / table-empty / card-grid / 测试） | 0 error（测试内 `<button>` 缺 type 已修） |
| 组件测试 | `vitest run src/components/__tests__/empty-state.test.tsx src/components/__tests__/metric.test.tsx` | 11/11 PASS |
| 浏览器实测（`/keys` 空表） | Playwright evaluate | `[data-slot=empty]` 含 `surface-grid`；`::before` background = `repeating-linear-gradient(...)`（取自 `--grid-line`），mask = `radial-gradient(72% 72% …)`；标题/描述/动作均可见 |

### 🔴 新发现（P1，既有问题，非本批引入）：前端测试基线本身是红的

- 全量并行跑 `bun run test` 时有 **18 个测试文件**失败（chat / model-pricing / models / onboarding /
  pricing / task-plugins / usage-logs / system-settings / subscriptions）。
- **对照实验**：在改动前的提交 `6f284de93` 上（`git worktree` + `node_modules` junction）跑同样 18 个文件：
  **14 个文件失败 / 24 条用例失败**。
  → 结论：这些失败**不是 v1.3.182 引入的**，是仓库既有状态；CI 的 `bun run test` 门禁目前不可能为绿。
- 处置：本批**不动**这些测试（不属视觉范围，且禁止为变绿削弱断言）；列为独立待办，
  需专门一批「测试基线归零」处理。

---

## v1.3.183（门禁转正 / 策略中心 / 语义缓存）— 2026-10-23

### 已 PASS 且不必重跑（除非对应文件变化）

| 项目 | 命令 | 结果 |
|---|---|---|
| i18n 语言覆盖（新门禁） | `cd web && node scripts/check-i18n-coverage.cjs` | `OK (all locales translated)`；12 个 locale 文件全 complete（fr/ja/ru/vi 各 95 键、zh-TW 66 键、本轮新 UI 14 键已补，8 语种均 7439 键） |
| i18n key 用法（旧门禁回归） | `node scripts/check-missing-i18n.cjs` | `OK (0 missing)` |
| 术语门禁 | `node scripts/check-terminology.mjs` | `PASS: quota 相关文案已统一为「额度」` |
| 设计令牌漂移 | `node scripts/ds-tokens-check.mjs` | `OK (659 tokens)`；负向：改 `--radius` → exit 1 并精确报 `:root :: --radius`，还原 → exit 0 |
| 前置：令牌快照幂等 | `node scripts/ds-tokens-export.mjs` ×2 | 第二次 `no change`（34 blocks / 659 tokens） |
| 内置计费可达性 | `go test ./setting/billing_setting/ -run TestBuiltinBillingExpressionsStayReachable` | PASS；负向：往 `defaultModelRatio` 插同名模型 → FAIL 并报「legacy default price tables 遮挡」 |
| 策略引擎 / 语义缓存单测 | `go test ./service/ -run "Policy|Semantic|ResponseCache"` | ok |
| 语义缓存配置回退 | `go test ./setting/response_cache_setting/` | ok（阈值越界回退 0.97、空白名单不生效） |
| 前端策略中心解析 | `bunx vitest run src/features/system-settings/request-limits/__tests__/policy-center-section.test.ts` | 11/11 PASS |
| 静态检查 | `go vet ./service ./setting/... ./controller ./common` / `go build ./...` / `bun run typecheck` | 全部 0 错 |
| 真实 E2E（进程级） | `node .refscan/e2e-policy.mjs`（真实二进制 + mock 上游） | **15/15 PASS**（详见 workflow_status.md v1.3.183 节） |

### 压测（本地，非生产）

| 场景 | 并发 | qps | p50 | p95 | 上游调用 |
|---|---|---|---|---|---|
| 语义命中（200 次） | 20 | 35.2 | 142.9ms | 2312ms | +0（`semantic_cache_hits_total` 0→200） |
| 精确命中（200 次，对照） | 20 | 33.6 | 151.3ms | 1913ms | +0 |
| 精确命中（200 次） | 2 | 32.1 | 42.9ms | 159ms | +0 |
| 直连上游（100 次，限流前） | 10 | 268 | 9.2ms | 82.3ms | — |

**归因结论**：语义命中与精确命中延迟同量级（p50 143 vs 151ms），延迟随并发线性上升（2 并发时 42.9ms）→ 瓶颈是本机 CPU/内存（压测时本机内存 93%）与限流中间件，**不是语义层引入的开销**。真实吞吐需在有独立配额的机器上重测（`待验证`）。

### 新增环境要点（下次本地 E2E 直接用）

- 本机 `curl` HTTPS 全局不可用，探测用 `node -e fetch(...)`；推送 GitHub 需 `git -c http.proxy=http://127.0.0.1:10808 push`（否则 `TLS connect error: unexpected eof`）。
- 本地起服务必须：`SSRF_GUARD_DISABLED=true`（否则内网 mock 上游被 `ValidateChannelURL` 拒为 `upstream address rejected`）、`METRICS_ENABLED=true`（否则 `/metrics` 不输出）、并把 `performance_setting.monitor_memory_threshold` 提到 99（本机内存常驻 93%，否则 503 `system_memory_overloaded`）。
- 管理端 API 登录返回 JWT（`data.access_token`）+ refresh cookie；只带其一可能 401。Token 列表接口返回**掩码 key**，真 key 只能从创建响应 `data.key` 取。
- 敏感词 option 是**换行分隔**文本（不是 JSON 数组）；`/api/option/` 用 **PUT**（POST 会落到 relay 报 `Invalid URL`）。
- 生产 SSH：`py -3 .refscan/ssh.py "<cmd>"`；源码走服务器 `git clone`（SFTP 未开放，`cat >` 流式上传会 socket 关闭）。

## 2026-10-22 v1.3.184 视觉批次 2（V4 表格 / V5 状态色 / V6 图表序列）

| 验证 | 命令 | 结果 |
|---|---|---|
| 类型 | `cd web && bun run typecheck` | PASS |
| Lint（改动文件） | `bunx oxlint -c .oxlintrc.json <8 files>` | 0 error（顺带修掉 charts.ts 既有 `parseFloat` warning） |
| 构建 | `bun run build` | PASS（总产物 24571.4 kB，字体无新增） |
| 定向测试 | `vitest run src/features/dashboard src/components/ui src/components/__tests__` | 81/81 PASS |
| 图表色阶测试 | `vitest run src/lib/__tests__/chart-palette.test.ts` | 4/4 PASS（canvas 缺 2d context 时仍返回合法 hex） |
| 浏览器实测：canvas 管线 | Playwright evaluate | `--primary` → `#146fa8`，`ctx.fillStyle` 规范化后仍是 `#146fa8` ⇒ **oklch→canvas 通路成立**，序列色不需要 color-mix |
| 浏览器实测：表头 | `/keys` `th` computed | `font-size: 11px`、`letter-spacing: 0.275px`、颜色 muted ⇒ 微标签生效（并发现 `Table` 根的 `[&_th]:text-sm` 会压掉表头字号，已移除该覆盖） |
| 浏览器实测：空态 | `/keys` | 网格 + 表框 surface-1 + 规则线，截图 `web/e2e-local/v13182/v183-keys-table.png` |

**V4/V5/V6 的实际范围（如实记录）**
- V4：`TableHead` 改微标签、`Table` 根移除 `[&_th]:text-sm` 覆盖、data-table 外框与移动端卡片列改 `border-rule + surface-1 + rounded-xl`。**数字列右对齐未做**（需要列级 metadata 契约，留待 V4b）。
- V5：`Alert` 新增 `success/warning/info` 三个令牌变体；`system-tasks-panel` 状态色与进度条、`setup/database-step` 三条 Alert 全部令牌化（该文件调色板直写归零）。**全站存量 `bg-*-50` 未清完**，仍按批推进。
- V6：新增 `lib/chart-palette.ts`（canvas 安全单色阶，读 `--primary`/`--background`）；dashboard 的 `getDashboardChartColors` 与 `USER_COLORS` 改用它。**`codex-usage-dialog` 等仍直接用 `--chart-N` 令牌**，令牌本身未改（避免 StatusBadge 的 `text-chart-N` 对比度回退），留待 V6b 统一。

### v1.3.185 生产部署与线上验证（2026-10-22）

| 验证 | 命令（服务器侧） | 结果 |
|---|---|---|
| 镜像构建 | `docker build -t new-api:v1.3.185 /opt/new-api-src-v185` | PASS |
| 蓝绿切换 | `./deploy-zero-downtime.sh v1.3.185` | 3001→3000；新容器 200；自检 200；清理后磁盘 13G/78G |
| 版本头 | `curl -sI --resolve … https://freeapi.tingfengai.art` | `HTTP/2 200` + `x-new-api-version: v1.3.185` |
| 健康 | `/healthz`、`/readyz` | 200；`{"status":"ready","checks":{"database":"up","redis":"up"}}` |
| 业务接口 | `/api/status` | 200 |
| 首页 | `GET /` | 200，8666 字节，title 正常 |
| 新样式产物 | 拉取 `index.987b66e7a5.css` | 含 `surface-grid`、`border-rule`（V4/V7 产物已上线） |
| 运行期 | `docker logs \| grep -c ERROR/panic`；`RestartCount` | 0 / 0；0 restarts |

**回滚点**：`new-api:v1.3.184`（`./deploy-zero-downtime.sh v1.3.184`）。本轮未触发回滚。
**注**：本次部署的 HEAD 同时包含并行工作流的 v1.3.183/184（策略中心、语义缓存、i18n 门禁、
策略中心文本域上限）；那部分后端行为不在本会话的验证范围内，线上仅确认健康检查/接口/日志无错。
