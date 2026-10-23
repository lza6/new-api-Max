## 0. 线上版本声明（唯一真相源）

> 更新于 2026-10-22。其他地方若与本表冲突，以本表为准。

| 项 | 值 | 校验命令 |
|---|---|---|
| 线上版本 | `v1.3.188` | `ssh ... 'docker ps --format "{{.Image}}"'` |
| 部署时间 | 2026-10-23 20:14（蓝绿，Caddy 上游 **3001**，耗时 26.6s 零停机） | `grep -oE "127.0.0.1:300[0-9]" /etc/caddy/Caddyfile \| head -1` |
| 容器资源 | new-api 1500m/swap2400m/2core/GOMEMLIMIT=1024MiB；postgres 1200m/shared_buffers 256MB；redis 192m/maxmemory 128mb | `docker inspect new-api --format "{{.HostConfig.Memory}}"` |
| 代码 SHA | `git rev-parse origin/main` | `git log origin/main -1` |
| 回滚镜像 | `new-api:v1.3.186`（保留最近 4 个版本：184/185/186/187） | `docker images new-api` |
| 回滚命令 | `cd /opt/new-api && ./deploy-zero-downtime.sh v1.3.186` | — |
| VERSION 文件 | `v1.3.187` | `cat VERSION` |

`VERSION` / 最新 tag / `CHANGELOG` 首条 / 本表四者一致；若不一致，必须先修本文档。

---

# workflow_status.md — 训练数据采集（多账号归属安全）终局闭环

> 最后更新：2026-10-20 18:00（Asia/Shanghai）
> 生产版本以 §0 为准（该批次完成时为 `v1.3.162`）

## 1. 任务契约（本轮）

| 编号 | 任务 | 交付物 | 验收标准 | 状态 |
|---|---|---|---|---|
| T1 | 多账号归属判定 fail-closed | `sink_multi.resolve` | **任一**账号列出失败即拒绝写入；B1/N4 用例通过 | done |
| T2 | 读侧兜底不翻转写归属 | `ReadFile` + `ErrFileNotFound` | 瞬时错误原样上抛；兜底成功也不改 `dirIdx`；N2 用例通过 | done |
| T3 | 删除真实落到所有账号 | `DeleteFile` 404→哨兵 + 逐账号删除 | 重复副本全部删除；瞬时错误必须上报；N3 用例通过 | done |
| T4 | 重复目录取最大 part 序号 | `resolve` + 严格 `maxPartSeq` | N4 用例 + 边界表通过；ERROR 日志 | done |
| T5 | Stat/列表暴露部分失败 | `Stat` + controller | 部分数据 + 错误 + `truncated=true`；N5 用例通过 | done |
| T6 | 失败信号有上限重试 | `worker.go` | 3 次上限；N1 用例通过 | done |
| T7 | 生产探针加固 | `scripts/prod-capture-e2e.sh` | `set -e`、分页、多账号/RootPath 继承、kind 断言、缓存失效、trap | done |
| T8 | 提交/发版/部署 | tag v1.3.154 | 干净构建 + 测试；生产 healthy | done |
| T9 | 生产真实 E2E（双账号） | 探针输出 | 用户 h1→账号#2、h4→账号#1；分片可解压、kind 合法 | done |
| T10 | 测试数据清理 | DB + 云盘 | users/tokens/consents/logs 0；两账号目录 0 | done |
| R1 | 独立审查（6 维） | 审查报告 | 发现 P1×2、P2×6、P3×7；已逐条处理 | done |
| R2 | 前后端衔接审计 | 缺口清单 | 无 P0/P1；P2×6、P3×10；已修 6 项 | done |
| R3 | 文档/运维一致性审计 | 缺口清单 | P0×1、P1×5、P2×11；已修主要项 | done |
| R4 | 修复审查发现并复验 | commit 901c66490 | 全量包测试 + 前端测试 + 构建通过 | done |
| R5 | HTML 变更报告 + 测验 | `计划书/变更报告-终局审计-20261018.html` | 覆盖 v1.3.151/v1.3.154/v1.3.155 + 25 题 | done |
| T11 | 重试顺序（原 P2-2） | `worker.go` `pendingRetries` | 重试优先于新信号；停机尽力回队 | done |
| T12 | 导出列表分页（原 P3-2） | `export-panel.tsx` | 20/页、可翻页、空页回退 | done |

## 2. 独立审查结果摘要

### 代码审查（6 维）
- **P1-1**：`remember()` 在读写兜底后改写写归属 → 已删除该机制；新增
  `TestMultiSinkReadFallbackDoesNotChangeWriteOwnership`。
- **P1-2**：重复目录 + 有账号不可读时仍选可读账号 → 改为**任一账号不可读即拒绝**；
  新增 `TestMultiSinkRefusesToGuessWhenAnyAccountUnreadable`。
- P2-1 删除只删一份 → 改为逐账号删除；P2-3 live test 断言过时 → 已更新；
  P2-4 探针 `grep -q` SIGPIPE / curl 中止 → 已修；P2-5 `truncated` → 已修；
  P2-6 OneDrive 404 映射缺单测 → 新增假 Graph 用例。
- **P2-2（已修复 v1.3.155）**：失败信号此前回到队尾，可能在新信号推进水位后重试并整段重传。
  现在暂存在 worker 内、下一轮优先处理；停机时尽力回队。`go test ./service/training_capture/` 通过。
- **未修（P3）**：P3-2 worker 重试注释已同步；仍有 P3：下载记录 100 条上限提示、
  后端原因串 i18n、部署健康检查用 `/readyz`。

### 前后端衔接
- 无 P0/P1 断链；15 条路由与前端调用逐条对齐，有契约测试。
- 已修：授权状态 30s 轮询（P2-1）、云盘 `last_error` 在配额已知时不可见（P2-2）、
  列表截断标志丢失（P2-3）、每日历史空态+错误态并现（P2-4）、
  `retried` 未展示（P2-5）、声明升版后重试死循环（P2-6）、
  非分片文件也显示分析/删除（P3-1）、`resumable` 字段未使用（P3-4）、
  下载字节数统一格式（P3-3）。
- 未修（P3）：导出列表分页、下载记录 100 条上限提示、`date`/`max_bytes`/`ttl_seconds`
  等后端能力的 UI 入口、错误双重提示、后端原因串 i18n。均不影响核心链路。

### 文档/运维
- **P0 已修**：README 记录格式补 `kind`（消费方契约）。
- P1 已修：归属/删除语义、多实例租约现状、探针 RootPath 继承、部署前构建步骤、
  `NODE_TYPE` 双 master 现状、SOP 版本。
- 其他已修：`.env.example` 补 `TRAINING_*`；目录示例；METRICS 已开启；
  回滚路径统一；audit-ledger 的租约矛盾；README 语言链接闭合。
- 未修（P2/P3）：翻译版 README 仍滞后；滚动更新旧脚本未标废弃；
  健康检查仍用 `/api/status` 而非 `/readyz`；`controller/training_data_bootstrap.go`
  与 `worker.go` 中仍有过时的多实例注释（属另一会话文件/需同步改动）。

## 3. 验证日志（真实命令与结果）

| 时间 | 命令 | 结果 |
|---|---|---|
| 2026-10-18 | `go build ./service/training_capture/` | exit 0 |
| 2026-10-18 | `go test ./service/training_capture/ -count=1` | ok（76.8s） |
| 2026-10-18 | `go test -race ./service/training_capture/ -count=1` | 4 次中 3 次 ok；1 次全 PASS 后二进制非零（无 DATA RACE/FAIL），未复现 |
| 2026-10-18 | 干净 worktree（HEAD+patch）`go build ./...` + 包测试 | exit 0 / ok（51.3s） |
| 2026-10-19 | `go build ./...`（含工作区） | exit 0 |
| 2026-10-19 | `go test ./service/training_capture/ -count=1` | ok（80.5s） |
| 2026-10-19 | `go test ./controller/ -run 'TrainingData\|TrainingCapture'` | ok（32.9s） |
| 2026-10-19 | `bun run vitest run`（training-data + profile settings） | 2 files / 10 tests passed |
| 2026-10-19 | `bun run i18n:check` | OK（0 missing） |
| 2026-10-19 | `bun run build`（web） | 成功 |
| 2026-10-19 | `bash -n scripts/prod-capture-e2e.sh` | 通过 |
| 2026-10-19 | `deploy-zero-downtime.sh v1.3.154` | healthy；外网 200；版本头 v1.3.154 |
| 2026-10-19 | 生产 E2E h1（hash→1） | 账号 #2；136B；request kind=message |
| 2026-10-19 | 生产 E2E h4（hash→0） | 账号 #1；176B；request + assistant |
| 2026-10-19 | 清理回读 | DB 剩余 0；账号#1/#2 剩余目录 0 |
| 2026-10-19 | 生产二进制 /metrics | `cannot safely resolve` 命中；metrics 200，24 条 capture 指标 |
| 2026-10-19 | `go test ./service/training_capture/`（v1.3.155 worker 改动） | ok（72.6s） |
| 2026-10-19 | `bunx oxlint` + `bun run build`（导出分页） | 干净 / 成功 |
| 2026-10-19 | `deploy-zero-downtime.sh v1.3.155` | healthy；外网 200；版本头 v1.3.155 |
| 2026-10-19 | 生产 E2E v1.3.155（id=1626） | 账号 #1；`kind=message`；清理 204/404 |
| 2026-10-19 | 热路径基准（本机） | 关闭 96.6ns/0 allocs；开启 3.4µs/224B/2；完整入队 2.1µs |

证据：`计划书/e2e-evidence/prod-capture-e2e-v13154-20261019.txt`

## 4. 已知噪声/边界

- 工作树若同时存在另一会话的未提交代码，整树构建可能被其未完成改动影响；
  判定本轮必须用 `git status` 区分文件归属。
- `-race` 有一次无法解释的「全 PASS 但二进制非零」；后续若复现，用 `go test -json` 抓
  `Action=fail`。
- 生产探针新用户 quota=0；补额度后必须 `DEL user:<id>`（已内置到脚本）。
- 探针无 assistant 行可能是模型只回 reasoning（正文为空），不一定是采集失败；
  h4 用例已证明 assistant 行可正常写入。

## 5. 阻塞项

- 无（本轮范围内）。
- 范围外/下一轮：Claude 计费两处决策待用户；累计充值金额（需三库验证）；p95 压测；
  下载记录 100 条上限提示等 P3 UI 项；`/readyz` 健康检查。

---

## 6. v1.3.156 上游对标加固（2026-10-19）

上游 `upstream/main` = `058c8231b`（2026-10-18），merge-base 2026-09-09，上游领先 242 个非 merge 提交。
逐条核对「已独立修复 / 仍存在 / 不适用」后，移植与本项目真实风险相关项。

### 已移植
| 类别 | 修复 | 验证 |
|---|---|---|
| 安全 | Waffo 回调校验 StoreID（未配置 fail-closed） | 表驱动单测 |
| 安全 | CreateUser 角色白名单 | `go build` + 静态确认（无现成 handler 测试） |
| 安全 | TLS 配置每处 `Clone()` | `TestOutboundTransportHonorsTLSEnv` |
| 安全 | Gemini `:countTokens` 拒绝转发 | `go build`（路由层静态确认） |
| 安全 | gorilla/websocket 1.5.3 | `go build` |
| 计费 | 图片按负载计数 | `TestOpenaiImageResponseCount` |
| 计费 | `output_tokens_details` → completion details | `TestNormalizeOpenAIUsageMapsOutputDetails` |
| 计费 | Responses 截断流估算 prompt | `TestOaiResponsesStreamHandlerBillsCutStream` |
| 计费 | Ollama done 帧 tool_calls | `TestOllamaStreamDoneFrameToolCalls` |
| 可靠性 | 任务提交接受任意 2xx | 代码核对（无现成 platform 测试） |
| 可靠性 | 通知固定窗口 + 原子计数 | `TestNotifyLimitFixedWindowMemory` |
| 可靠性 | 审计复合索引 | 三库 conformance |
| 可靠性 | task plugin source 1MiB | 三库 conformance（二次迁移幂等） |
| 可靠性 | 过载哨兵 cgroup + 3 次 + 新鲜度 | `TestContainer*` / `TestCheckSystemOverload` |
| 体验 | 静态资源不计页面限流 | `TestIsStaticAssetRequest` |
| 体验 | 模型限流按分组隔离 | `go test ./middleware/ -run TestModelRateLimit` |

### 明确不移植
- scoped access tokens（认证体系 6k+ 行重写）
- Responses WebSocket 中继（未实现的协议栈）
- 插件/jsplugin 引擎重构（实现路径不同）
- relaykit 跨协议转换批次（面广，需逐项测试；本轮记录）
- 管理端用户管理 step-up（后端单方面加校验会让现有 UI 全部 403，必须前后端配套）
- Claude tool_use 的 completion 估算（缺 `ToolUseCount` 字段；prompt 估算已由现有逻辑覆盖）

### 验证
- 三库 conformance（真实 SQLite/MySQL/PostgreSQL）：
  `TestDBConformanceAutoMigrateIdempotent` + `TestDBConformanceLogsIndexes` **PASS**
- 定向单测：common / service / middleware / openai / ollama 全部 PASS
- `cd relaykit && GOWORK=off go build ./...` PASS
- `go build ./...` PASS
- 生产部署与 E2E：见下方追加

### v1.3.157 追加
- Claude 切断流的 completion 计入 `tool_use` 块（约 40 token/块）。
- 任务协议预检：请求体超限返回 **413**，其它错误带真实原因。
- **踩坑**：首次打 tag 漏 stage `relay_responses.go`，服务器构建失败；
  已用干净 worktree 验证 tag 可编译后重打 tag 并重建。

---

## 7. v1.3.158/159 运营财务（2026-10-20）

### 已落地
- `model_costs` append-only 成本历史（渠道×模型，call/token，微元）。
- 结算时把成本快照写入 `logs.cost_quota`；报表只 SUM 快照 → 调价不改写历史。
- 运营面板：今日/本月/本年营收、成本、毛利、毛利率；预计月/年利润；
  建议每日上游充值（日均成本×1.5）；未配置成本时显式警告；成本起始时间提示。
- `GET /api/admin/ops/daily` 逐日财务序列；`GET/POST /api/admin/ops/model-costs`。
- 使用日志始终记录调用者 IP；成本字段 `json:"-"`，普通用户不可见。

### 生产配置
- 渠道 49：deepseek-v4.1-flash 0.0007、step-5-preview 0.00007、
  gpt-6-luna 0.0007、glm-5.3-flash 0.002（人民币元/次，共 4 行）。

### 验证
- `go test ./model/ -run 'TestModelCost|TestValidateModelCostInput|TestConsumeLogRecordsCostSnapshotAndIP'` PASS
- `go test ./controller/ -run 'TestOpsOverview|TestOpsRanking'` PASS
- 三库 conformance（含 ModelCost + logs.cost_quota）PASS
- 前端 typecheck/oxlint/i18n/build PASS；浏览器 E2E `ui-e2e-ops-profit.cjs` 11/11
- 生产真实请求：glm-5.3-flash → `quota=3000, cost_quota=1000`，IP 已记录；用户日志无 cost_quota
- 今日总览：10,470,400 / 2,801,460 / 7,668,940（margin 73.24%）

### v1.3.160 二轮独立复验修复（已部署）
- ClickHouse 建表/补列补 `cost_quota`（缺列会让所有消费日志写入失败）。
- Responses `response.error` 与 `error` 一样视为失败。
- 订阅 Waffo 下单要求 StoreID。
- token 成本饱和乘法（防 int64 回绕）。
- 不存在 `/static/*` 直接 404；生产实测 404，真实资源 200。
- 启动日志分别报告 `cpu=host|cgroup memory=host|cgroup`；
  本实例实测 `cpu=host memory=cgroup`（无 CPU 配额，内存有 900m 限制）。
- IP 开关文案改为「使用日志始终记录 IP，开关仅控制错误日志」。
- 413 状态码测试（超限 413 / 小 JSON 400）；通知测试清理 key。
- 生产 v1.3.160（上游 3001）：healthy、0 个 500、4 条成本规则保留、
  财务 totalview 冷 9.8s / 缓存 0.0s。

---

## 8. 技能检查记录（2026-10-20）

- 读取：`AGENTS.md`、`CLAUDE.md`、`web/AGENTS.md`（前端任务）、`policy.md`、
  `test_ledger.md`、`MEMORY.md` 及相关记忆（采集/上游/财务/部署/验证台账）。
- 本地技能：`.claude/skills/new-api-add-feature`（已新增 §17 财务/成本与日志 schema 纪律）；
  `计划书/ops/SOP-新功能交付.md`（已新增 §2.1 财务/成本数据纪律）。
- 未联网安装新技能：Go/GORM/Playwright/Vitest/Spec-Kit 本地均已具备，
  本轮问题不需要新的第三方技能；避免引入不可控依赖。
- 计划文档审计：由只读子代理扫描 `计划书/` 与根目录计划类 md，
  产出保留/归档/删除清单后再执行（见第 9 节，待其返回）。

---

## 9. 计划文档审计与清理（2026-10-20）

- 扫描：`计划书/` 下 22 篇 md + 根目录计划类文档；仅读，未删除任何文件。
- 结论：按「含未闭环/TODO/决策项即不可删除」的严格规则，**删除候选 0 个**。
- Manifest：`计划书/archive/2026-10-20-doc-audit/MANIFEST.md`（含永久保留/历史证据/开放项）。
- 本轮修正的文档事实：
  - 根 `workflow_status.md`、`test_ledger.md`、`policy.md`、`计划书/README.md` 的版本与互相矛盾引用；
  - `SOP-运维手册.md`：Caddy 主上游健康检查实际用 `/healthz`、METRICS 生产已开启；
  - `黄金范例.md` / `SOP-新功能交付.md` / `ADR-架构决策记录.md`：对已删除
    `计划书/workflow_status.md` 的引用改为「历史证据，git 找回」；
  - `CHANGELOG.md` 补录 v1.3.150/151。
- 仍开放的审计项（不可删）：`scan-billing` P1×2、`scan-auth` P2×4、`scan-contract` P2×2、
  `critic-v1.3.129-132` 若干、`geo-safe` 浏览器 E2E/条款裁决、`hotpath-stress` 3 项、
  `audit-ledger` 消耗排行/累计充值/Claude 计费。

## 10. Agent 管理 Agent：高价值 backlog（2026-10-20 联网检索）

| 优先级 | 项 | 状态 |
|---|---|---|
| P0 | Redis 限流脚本 NOSCRIPT → 500（`common/limiter` sync.Once + EvalSha） | **本轮已修**（Script.Run 自动回退） |
| P0 | 模型限流「先查后记」非原子（并发可绕过） | 未修（生产该限流默认关闭；记录） |
| P0 | AutoMigrate 在老 postgres 驱动下可能重写大表（上游 #4054） | 未修（需升级 gorm/驱动并全量回归，风险中） |
| P1 | 非流式上游 body 无读超时 | 未修（记录） |
| P1 | 成本/营收绑定当前汇率，历史人民币值会随汇率变化 | 未修（本实例 1:1；需 `cost_micros`/`fx_rate` 快照） |
| P1 | 未配置成本模型的毛利偏高 | **本轮已修**（`cost_rule_id` + 覆盖率告警） |
| P1 | SSE 注释心跳被丢弃 / fallover 下网关 ping 关闭 | 未修（记录） |
| P1 | 上游 FIN 无终止事件被当正常结束 | 未修（记录） |
| P1 | OneDrive 429 未读 Retry-After | 未修（记录） |
| P2 | OneDrive 用默认 Transport（每 host 2 空闲连接） | 未修（可改用 `common.NewOutboundClient`） |
| P2 | 大日志表聚合缺覆盖索引（9s 冷查询） | 未修（已用单聚合+缓存缓解；覆盖索引需 PG 分支） |
| P3 | 构建镜像 Go patch 版本落后 | 未修（下次镜像升级） |

说明：未修项不是「不知道」，而是本轮风险/收益排序后留给下一轮；均记录在案，禁止当作已完成。

## 11. 生产 UI 真实验证（2026-10-20，浏览器 + 截图）

- 方式：SSH 隧道 9600 → 生产 443，Playwright 以 `freeapi.tingfengai.art` 域名访问
  （`--host-resolver-rules` 映射 127.0.0.1 + `ignoreHTTPSErrors`），用真实登录页登录。
- 结果：`scripts/ui-e2e-prod-finance.cjs` **16/16 PASS**。
  - 运营面板：利润与成本（今日/本月/本年收入、上游成本、毛利、毛利率）、
    月/年利润预测、建议每日上游充值、成本覆盖率告警；
  - 模型成本表：渠道 #49 四条按次规则（glm-5.3-flash 0.002、gpt-6-luna 0.0007、
    step-5-preview 0.00007、deepseek-v4.1-flash 0.0007，人民币），含生效时间与“历史永不改写”说明；
  - 320px 移动端无横向溢出；安全与访问页隐私文案与行为一致；浏览器控制台 0 error。
- 截图证据：`计划书/e2e-evidence/v1.3.162-ui/`（5 张，已逐张查看核对）。
- 临时验证账号 `ui_e2e_1792500016` 与 22 条会话已删除，未留下 tokens/logs；隧道已关闭。
- 结论：**UI 层已验证**，不再只是接口/单测结论。验证手法与坑写入 `test_ledger.md` 与记忆。

## 12. v1.3.163 收口批次（2026-10-21）

| 事项 | 结论 | 证据 |
|---|---|---|
| 模型成功限流非原子（P0） | **已修**：Lua ZSET 预占 + 失败退还；键升级 `MRRLS:v2` | `middleware/model-rate-limit.go`；60 并发恰好 10 个通过 |
| 失败请求烧成功额度（P1） | **已修**：内存路径去掉 `_check` 计数器，改预占/退还 | handler 级测试 |
| OneDrive 429 忽略 Retry-After（P1） | **已修**：解析秒数/HTTP-date 并与指数退避取大，封顶 2 分钟 | `parseRetryAfter` / `nextWorkerBackoff` 测试 |
| OneDrive 用默认 Transport（P2） | **已修**：专用克隆 Transport（32 空闲连接），且不允许继承跳过 TLS 校验 | 单测断言 `InsecureSkipVerify=false` |
| AutoMigrate 旧库漂移重写（P0 疑点） | **已验证不成立**：真实 MySQL/PG 漂移后一次修复、二次零变更 | 新 conformance 用例，三库 PASS（141s） |
| SSE 注释心跳 / FIN 无终止事件 | **不改**：注释行按设计丢弃（网关自建 ping）；「终止事件」语义按 provider 不同，通用强制会误杀 Ollama/Claude | 代码复核 |
| 非流式读超时 | **已有界**：整请求受 `RELAY_TIMEOUT` 与 ctx 约束 | 复核 |
| 大日志表覆盖索引（P2） | **暂不做**：1.46M 行写入频繁，五列索引放大写放大；已有 5 分钟单聚合+进程缓存 | 实测冷 9.8s / 热 0.0s |
| 审计 P1 复核 | 认证两条 P1 早已闭环；Stripe 两条为误报 | `计划书/audit/` 复核节 |

### v1.3.163 发布与线上验收（2026-10-21）
- commit `e27aa6972`，tag `v1.3.163` 已推 origin main；GitHub Release 已建（HTTP 201）。
- 服务器本地构建 `new-api:v1.3.163`（9 分钟），`deploy-zero-downtime.sh` 蓝绿 **3001→3000**。
- 线上：`x-new-api-version: v1.3.163`、`/healthz` `/readyz` 200、15 分钟 0 个 500、
  采集 worker `sink=onedrive` 正常启动。
- 生产浏览器 E2E **16/16**（证据 `计划书/e2e-evidence/v1.3.163-ui/`）；临时管理员与 2 条会话已删。
- 轻量压测（服务器本机）：300 请求并发 20 → **300/300 = 200**，1.1s；外网 `/api/status` 200 / 108ms。
- 回滚：`/opt/new-api/deploy-zero-downtime.sh v1.3.162`（镜像仍在本地）。

### v1.3.164 复审后修复与再次上线（2026-10-21）
- 独立复审抓到 1 个真实 P1：`common.Init` 在 `TLS_INSECURE_SKIP_VERIFY=true` 时就地改写
  `http.DefaultTransport`，懒构造的 OneDrive sink 克隆后仍会跳过证书校验。已显式清标志并保留 HTTP/2，
  补「先污染再构造」回归测试。
- 同时修正：成功槽先于总请求桶预占、被拒退还（恢复旧语义）；panic 路径 `defer` 退还；
  handler 级断言 v2 键为 ZSET（原测试直接调 helper，无法发现路由层回退）。
- commit `4ccbb3a8d` / tag `v1.3.164` / Release 201；蓝绿 **3000→3001**，版本头 v1.3.164，
  healthz/readyz 200，0 个 500；生产浏览器 E2E **16/16**（`计划书/e2e-evidence/v1.3.164-ui/`）。
- 回滚：`deploy-zero-downtime.sh v1.3.163`。

## 13. 剩余风险收口（2026-10-21，v1.3.165）

| 原风险 | 处理 | 证据/结论 |
|---|---|---|
| 成本覆盖率仅 7.9% | **实测今日 99.14%**（12,173,450 中 12,068,850 命中）；本月 3.21% 是 10-20 前无成本规则的历史，面板已标注起始日 | 生产 SQL 聚合；不可追溯补历史成本（无历史成本数据，强行补=伪造财务） |
| 大日志表冷查询 9.8s | 新增覆盖索引 `idx_log_type_created_finance`，生产 `CREATE INDEX CONCURRENTLY` 预建；PG Index Only Scan **12.2s→1.67s**；接口冷 2.99s、热 0.002s | `EXPLAIN (ANALYZE)`；三库 conformance PASS（103s） |
| 登录时序侧信道 | 账号不存在/无密码分支执行一次 dummy bcrypt；可注入校验器单测证明执行 | `model/user.go`；两个单测 |
| 注册用户名/邮箱枚举 | 预检只查用户名；邮箱验证开启时用户名冲突也返回统一成功响应并发通知 | `controller/user.go` |
| 历史 e2e/测试账号 | 13 个账号已**禁用**（status=2、role=1、清空口令），撤销 102 条会话与 39 个 token；备份表 `_bak_testusers_20261021` | logs 128 条保留（财务记录不删），账号不可再登录 |
| FX 快照 | **不适用**：`cost_quota`/`quota` 均为结算时写入的整数快照；生产 `USDExchangeRate=1`，历史金额不随汇率改写。展示层换算不修改数据 | 无 schema 变更 |
| 本机 PostgreSQL 服务损坏 | **受权限阻塞**：服务以 NetworkService 运行，修复需管理员改服务/ACL；已用 `initdb` 临时实例完成三库验证 | 复现命令写入 test_ledger |
| 其它会话未提交文件 | **不动**：`controller/webhook.go`、`model/shadow_price.go` 等属其他会话，未经授权不提交 | git status 仍显示为未暂存 |

---

# v1.3.182 — 视觉批次 1（令牌层 + 概览试点）

> 更新：2026-10-22。施工契约见 `计划书/视觉识别体系-反AI通用美学界面改造指南.md`（V0–V3）。

## 交付内容

| 卡片 | 内容 | 状态 |
|---|---|---|
| V0 | 删除死配置 `config/fonts.ts` + `context/font-provider.tsx`，`theme-customization.ts` 成为字体轴唯一真相 | ✅ |
| V1 | 令牌层：`--surface-1/2/3`、`--rule*`、`--grid-*`、`--accent-quiet*`、`--motion-*`、`--ease-quiet`、`--font-numeric`；`--radius` 1rem→0.625rem；引入 JetBrains Mono（自托管） | ✅ |
| V2 | `Card.tone`（panel/plain/inset）+ dashboard 试点：PanelWrapper、概览容器、性能健康面板改表面+规则线；彩虹渐变改中性面；`rounded-2xl` 归零 | ✅ |
| V3 | `Metric`/`MetricLabel`/`MetricValue` 读数排版件 + dashboard 三处接入（读数条、模型分析条、概览卡） | ◐ 其余页面待接 |

## 证据

- 构建/门禁：`bun run typecheck`、`oxlint`（改动文件）、`bun run build`、`check-missing-i18n.cjs`、
  `check-terminology.mjs`、`format --check` 全 PASS；全量 `vitest` 1 失败为**已知噪声**
  （`task-event-stream.test.tsx`，单跑 6/6 PASS，见 `test_ledger.md:286`）。
- knip 门禁 117 条"新增"经核对**全部为既有基线漂移**，无一条指向本批文件；**未改基线、未屏蔽**。
- 真实浏览器 E2E：10 张截图 + axe 0 违规 → `计划书/e2e-evidence/v1.3.182/RESULTS.md`。
- 浏览器实测：`--radius=.625rem`、metric 值 `JetBrains Mono Variable`+`tabular-nums`、
  dashboard `rounded-2xl`=0、body 仍 `Public Sans`。

## 已知边界

1. 本地空库，读数为 0；真实数据形态下的读数条留到 V4/V6 之后复测。
2. 暗色只覆盖概览一页。
3. `knip-baseline.json` 陈旧（117 条漂移）是**既有问题**，本轮只如实记录，未擅自 `--update` 掩盖。

---

## 14. 2026-10-23 主控会话：门禁可信度与错误面收口（真实执行）

> 本条只记本会话（独立于并行批次）做了什么、验证到什么程度、哪些未闭环。
> 详细证据：`evidence/T-A5.json`、`T-A6.json`、`T-A8.json`、`T-B1.json`、`T-B2.json`、`T-B9a.json`。

| 任务 | 状态 | 关键证据 |
|---|---|---|
| T-A5 版本四口径门禁 | **done** | `scripts/check-version-consistency.mjs`；自测 9/9；真实仓库 `consistent`；已进 CI |
| T-A6 CI 补门禁 | **done**（术语 + 版本两 step） | `Bun.YAML.parse(ci.yml)` = 3 jobs / frontend 12 steps |
| T-A8 三份规格文件 | **done** | `db_structure.md`（55 表/637 列，脚本生成）、`project_specs.md`、`.agent/memory/×6` |
| T-B1 39 处错误直抛 | **done** | 改写为 `ApiError` / 新增 `ApiErrorMsgLogged`；`go build`+`go vet`+`go test ./common/` 5/5 |
| T-B2 清原生 confirm + 删死 hook | **done** | `window.confirm` 实调 0 处；`use-form-dirty-guard.ts` 已删；typecheck/lint/locale 全绿 |
| T-B9a N1 术语残余 + 门禁假绿 | **done** | 门禁 RED 4 → 文案修复 → GREEN 0 |
| T-A1 前端分片脚本 | **done（工具）/ 全量未绿** | `web/scripts/test-shards.mjs`；199 文件 5 片；948 通过 / 13 失败（失败均属并行批次模块） |
| 死循环修复 | **done（额外发现）** | `plan-comparison.test.tsx` mock 的 `t` 不稳定导致无限 effect：>6 分钟 → 2.9 秒 |
| T-A4 ClickHouse 真跑 | **blocked** | 本机 `docker: command not found`，需运行环境 |
| T-A3 三库 conformance | **blocked** | 本机 PostgreSQL 服务损坏 + 需临时实例；本轮未跑 |
| T-A2 controller `-shuffle` | **todo** | 全量约 600s 超时（已知噪声），需单独排 |
| 生产部署 / 发版 | **未由本会话执行** | 并行会话已发布 v1.3.182 并留回滚点 v1.3.181（见 ledger 生产段）；本会话不重复部署，避免与其撞车 |

**未闭环项（不掩饰）**：knip 死代码门禁当前 FAIL(1)；前端 13 条真实失败；ClickHouse 未验证；生产最终版本以并行会话的部署记录为准。

---

# v1.3.183 — 参考对标批次 B-1…B-6（2026-10-23）

**结论：complete（B-1…B-6 全部落地，已提交 ef80dd8ae，已部署生产 v1.3.183，发行 tag v1.3.183 已推送）**

## 交付内容

| 批次 | 内容 | 关键文件 |
|---|---|---|
| B-1 | i18n 门禁从「假绿」转正：新增语言覆盖 + 占位符一致性检查，补 446 条翻译，修 ja/vi 占位符事故，CI 接入三步门禁 | `web/scripts/check-i18n-coverage.cjs`、`web/src/i18n/locales/*`、`.github/workflows/ci.yml` |
| B-2 | 设计令牌快照 + 漂移门禁（34 blocks / 659 tokens） | `web/scripts/ds-tokens-*.mjs`、`web/.ai/ds/ds-tokens.json` |
| B-3 | 内置计费表达式可达性守卫（防 legacy 默认表静默遮挡） | `setting/billing_setting/builtin_billing_test.go` |
| B-4 | 护栏观测统一：敏感词拦截计入 `policy_decision_total{guardrail:block}`（不重复实现已有拦截） | `service/policy_engine.go`、`controller/relay.go` |
| B-5 | 模型级单请求预算（`relay.policy_model_request_limits`，403 `model_request_limit`） | `setting/relay_setting/relay_setting.go`、`service/policy_engine.go` |
| B-6 | 语义响应缓存（默认关 + 白名单 + 阈值 0.97 + 有界内存 + `/metrics` 三指标）与管理端配置分区 | `service/semantic_cache.go`、`setting/response_cache_setting/*`、`web/src/features/system-settings/request-limits/policy-center-section.tsx` |

## 真实 E2E（本地二进制 + mock 上游，15/15）

基线 200 → 预算 403 → 取消后恢复 200 → 敏感词拦截 + 计数 2→3 → 语义关 2 次上游 →
精确缓存基线 0 次 → 语义命中 0 次（hits 0→200）→ 无关问题 1 次（不误命中）→ 关开关立即回滚。

## 部署

- 生产 `154.37.208.171`：镜像 `new-api:v1.3.183` 蓝绿部署到 3000 端口，旧容器已停；`/healthz` 200、`X-New-Api-Version: v1.3.183`、`/readyz` = database up / redis up；公网 `https://freeapi.tingfengai.art/healthz` 200（服务器侧探测）；镜像内确认含 `semantic_cache_hits_total` / `policy_model_request_limits` / `Unified Policy Center` 符号。
- 回滚：`bash /opt/new-api/deploy-zero-downtime.sh v1.3.182`（镜像仍在，前端/后端均无 schema 迁移）。

## 未做（明确披露）

- **累计型预算**（当日/会话）未做：需要跨实例计数存储，属独立架构工作，不在本批。
- **真实语义 embedding** 未接：内置 embedder 是词袋哈希；真语义需外部付费 embedding 服务（付费红线，未批准预算）。
- 前端全量 `bun run test` 基线仍为红（既有 18 文件失败，见 `test_ledger.md`），本批不触碰。

---

## 15. 2026-10-23 v1.3.187 发版与生产部署（已验收）

| 项 | 值 |
|---|---|
| 版本 | `v1.3.187` |
| 提交 | `3d316a530`（fix(web): 前端套件转绿 1582/1582 + 插件图标 fail-closed + 策略中心进导航） |
| 构建 | 服务器侧 `docker build`（Dockerfile 注入 `common.Version`），镜像 `new-api:v1.3.187` 275MB |
| 部署 | 蓝绿：旧容器在 3001 → 新容器 3000 → Caddy 切上游 → 外网自检 200 |
| 回滚点 | `new-api:v1.3.186`（`./deploy-zero-downtime.sh v1.3.186` 一条命令） |
| 磁盘 | 13G/78G (17%)，已清理 v1.3.183 |

**线上验收（服务器侧真实命令）**

| 检查 | 结果 |
|---|---|
| 版本头 | `x-new-api-version: v1.3.187` |
| 健康 | `/healthz` 200、`/readyz` 200、`/api/status` 200 |
| 首页 | 200，8666 字节 |
| 运行期错误 | 最近 400 行 `ERROR/panic` = 0 |
| 二进制自报版本 | `v1.3.187`（此前构建未注入 ldflags 时会报 `v0.0.0`，Dockerfile 已注入） |

**未做的验证**：生产端**浏览器**点测（策略中心导航项、插件图标）未执行——本机对该域名的 TLS 被中间设备 RST（记忆 `tingfengai-domain-rst-20261010.md`），需隧道+管理员登录，本轮未取得。

---

# 2026-10-23 全量审计轮（进行中）

## A. 线上加固（**已闭环，有实测**）

触发：2026-10-23 09:03/09:05 UTC 生产 PG 两次 `oom_kill` → 崩溃恢复 → 全站 147 个 503（集中在 17:05–17:06 本地时间）。
根因：`controller/log.go:220` GetLogsTraffic 的 30 天全窗口聚合对 2642MB heap 做 Parallel Seq Scan，撑爆 256MB 容器。

| # | 变更 | 验证 |
|---|---|---|
| A1 | postgres 256m→**768m→1200m**、shared_buffers 32→**256MB**、work_mem 8MB、jit off、effective_cache_size 2GB、random_page_cost 1.1、effective_io_concurrency 100、**shm_size 256m** | `pg_settings` 实测 |
| A2 | 新增覆盖索引 `idx_logs_type_created_stats (type,created_at) INCLUDE (model_name,request_bytes,response_bytes)` 134MB；删冗余 `idx_logs_type_created_bytes` | `indisvalid=t`；日流量 18134ms→**1824ms**，模型排行 9610ms→**1086ms** |
| A3 | `VACUUM (ANALYZE, PARALLEL 0) logs` 刷 visibility map | `relallvisible` 78%→**99.98%**；Heap Fetches 286173→**81~372** |
| A4 | new-api 900m→**1500m**/swap 2400m/2core/GOMEMLIMIT **1024MiB** | 蓝绿部署 14.7s 零停机，`docker inspect` 实测 |
| A5 | Caddy 健康检查 2s/2s → **5s/5s**（容忍窗口 6s→15s，修 503 放大链） | `systemctl reload caddy`，外网 10/10=200 |
| A6 | compose `image: new-api:v1.3.88` → **v1.3.187**（差一步降级生产）+ 顶部 7 行警告头 | `docker compose config --quiet` 通过 |
| A7 | `deploy-zero-downtime.sh` 资源参数同步 1500m/2400m/1024MiB | `bash -n` 通过 |

**验收**：外网 20/20=200、近 5 分钟 0 个 5xx、load 0.33、available 2166MB、Caddy 上游 3001、`x-new-api-version: v1.3.187`。

## B. 计划文档闭环审计（`evidence/audit-20261023-docs.json`）

规模：**57 份文档 / 121 个任务单元**；判定 保留 53 / 可删除 2 / 需用户决策 2。
任务级：已闭环 61、部分 14、**伪闭环 9**、未闭环 75、待决策 9、过时 2。

### B1. 已修复的伪闭环（5/6，均有实测）

| # | 问题 | 修复 | 验证 |
|---|---|---|---|
| B1-1 | 指南 7 处写 `bun run test:shards`，但 `package.json` 未注册该 script（`Script not found`） | `web/package.json` 补 `"test:shards": "bun scripts/test-shards.mjs"` | `bun run test:shards --dry-run` → `201 test files -> 5 shards` |
| B1-2 | `project_specs.md` 版本基线停在 v1.3.182/181（实际 187）；T-B1/T-B2 标 Todo、T-A8 标 Doing 但均已闭环 | 版本基线→v1.3.187；T-B1 复测直抛=0 → **Done**；T-B2 复测真实调用点=0（3 处为注释）→ **Done**；T-A8 三产物齐 → **Done** | `grep -c` 实跑 + `.agent/memory/` 6 文件 ls |
| B1-3 | `计划书/README.md` 把两份 html 标为「仓库根」（实际在 `计划书/`）；ops 表漏登 2 份 | 路径改为「本目录」；补登 `DECISION-claude-billing.md`、`测试分片矩阵.md`（附待归档标注） | 文件 ls 实测 |
| B1-4 | `audit/plan-docs-audit-20261023.md` 仍称指南「951 行/v1.3.166/T-xx」，且建议「重写」——该动作已完成 | 顶部加**勘误块**（保留原始观察，不改正文） | `wc -l` = 950、指南首行 = v1.3.181 |
| B1-5 | 视觉指南 **V7 标 ✅ 但证据不成立**（验收要求三态各 2 页截图 + 单测） | 据实降级 **✅ → ◐**，写明：组件 3 文件在、单测仅 `empty-state` 一份、截图 0 张、CHANGELOG v1.3.184 未提及 | `ls` 组件三文件 + `find e2e-evidence -iname "*v7*"` 空 |

**未处理（B1-6）**：`参考的结果计划指南.md` 同文件内 `PHASE_B_IN_PROGRESS` 与「B-1…B-6 全 DONE」自相矛盾 —— 该文件被 `.gitignore:65` 忽略（删除不可恢复）且属另一会话写保护，**留待用户裁决**。

### B2. 删除候选（**已执行，2026-10-23 用户授权**）

| 路径 | 理由 | 结果 |
|---|---|---|
| `前端改进计划.md` | §14–§18 全是 v1.3.166–170 已执行事项，零未闭环 | **已归档 + `git rm`**：`docs/archive/plans/20261023/前端改进计划.md`（字节一致，见该目录 `archive-manifest.md`） |
| `计划书/ops/测试分片矩阵.md` | 核心结论「本机无法单进程跑完全量」已被 T-A1 推翻 | **已归档 + `git rm`**：`docs/archive/plans/20261023/测试分片矩阵.md`；引用已改（`CHANGELOG.md`、指南 §3.1/§3.3/§5.3/§6 P2-3/§13.2、`计划书/README.md`、本文件） |

回滚：`git checkout 664a8489b866607e896771bbccf3575c746740c0 -- "前端改进计划.md" "计划书/ops/测试分片矩阵.md"`（详见归档目录 `archive-manifest.md` §5）。

### B3. 必须保留（含未闭环任务）

`下一步改进指南`（16 卡未闭环）/ `尼尔森指南`（N2/N6/N7/N10/N13/N14）/ `视觉指南`（V9/V10/V11 + V4b/V5/V6b + 本轮降级的 V7）/ `参考的结果计划指南`（.gitignore 保护）/ `ops/DECISION-claude-billing.md`（与指南 D-E 冲突待裁） / `audit/*.md` 中 22 项未核销项（保守判未闭环）。

## B4. 计费与中继链路审计（完成，`evidence/audit-20261023-billing.json`）

结果：**P0 0 / P1 2 / P2 3 / P3 4**。已修复 2 条 P1：

| # | 缺陷 | 修复 | 验证 |
|---|---|---|---|
| B4-1 (P1) | `middleware/model-rate-limit.go` 两处（success 预占、total 检查）Redis 出错直接 `abortWithOpenAiMessage(500, "rate_limit_check_failed")`。IP/用户限流早已按 2026-09-29 事故改为降级，唯独模型限流没改 → 管理员一开 `ModelRequestRateLimitEnabled`，Redis 抖动即让全部 `/v1/*` 500，而 `/api/status` 仍 200 使探针继续导流 | 两处均改为 `memoryRateLimitHandler(...)(c)` 降级（内存实现已存在且 `Init` 幂等不重置状态） | 新增 2 用例：`TestRedisRateLimitHandlerFallsBackToMemoryWhenRedisDown`（不是 500、请求走到下游）、`TestRedisRateLimitHandlerFallbackStillEnforcesLimit`（降级后第 2 个请求仍 429，证明降级≠放行）。`go test ./middleware/` **全量 ok 9.3s** |
| B4-2 (P1) | `pkg/billingexpr/settle.go` 的 `ComputeTieredQuotaWithRequest` 无 `cost<0` 守卫；保存期 `smokeTestExpr` 只跑 P/C/Len 向量（CR/CC/Img/AI 恒 0），覆盖不到「缓存折扣超过本金」的组合 → 表达式转负 → `SettleBilling` 走 `IncreaseUserQuota`，**每命中一次缓存反而给用户返额度** | 转换前钳 `cost` 到 0 并 `common.SysError` 留审计记录 | TDD：先写 `TestComputeTieredQuota_ClampsNegativeCost` 拿到 **RED（实际 -3000）**，修后 **GREEN**；`go test ./pkg/billingexpr/` 全量 ok；`./service ./setting/... ./relay/...` build exit 0 |

**未修（留待排期）**：B-3 批量落库失败静默丢整窗额度增量（`model/utils.go:128-157`）；B-4 `BillingSession.Refund` 异步 goroutine 撞停机排空顺序致退款丢失（`billing_session.go:100-125` + `main.go:361-371`）；B-5 任务完成 `usageFacts` 覆盖冻结快照且无上界（`service/task_polling.go:738-752`）。这三条都需要 DB 故障注入环境才能真验证，已标 `待验证`。

**已核对无问题（下轮免重复排查）**：`*Checked/*Strict` 覆盖全部计费换算、预扣饱和一律 400 不回绕、`max_tokens` 四格式均限、auto 分组每次选渠道重算 ratio、GPT 格式 `cr+cc>prompt` 已钳 0、客户端断开除在空流 failover 外、SSRF 有 dial 级 IP 校验。

## C. 并行审计进行中

| 代理 | 范围 | 状态 |
|---|---|---|
| audit-data | 慢查询/索引/N+1/事务/数据保留/三库兼容 | 进行中 |
| audit-billing | 配额安全/并发幂等/限流降级/错误语义/密钥边界 | 进行中 |
| audit-frontend | 首屏包体/不可见内容/i18n/尼尔森十项/前后端契约 | 进行中 |
| audit-ops | 单点故障/磁盘/安全面/可观测性/回滚/文档与现实差距 | 进行中 |

产出将写入 `evidence/audit-20261023-{data,billing,frontend,ops}.json`，汇总后按 P0→P1 修复并走独立审查循环（≤3 轮）。

## D. 已在生产执行但**不在仓库**的变更（服务器侧，务必记档）

> 这些改动直接落在生产机，换个实例会丢失。已逐条实测。

| # | 变更 | 位置 | 验证 |
|---|---|---|---|
| D-1 | postgres 扩容与调参：`mem_limit 1200m`、`shared_buffers 256MB`、`work_mem 8MB`、`maintenance_work_mem 128MB`、`jit=off`、`effective_cache_size 2GB`、`random_page_cost 1.1`、`effective_io_concurrency 100`、`shm_size 256m` | `/opt/new-api/docker-compose.yml`（备份 `*.bak-pre-sqltune-*`） | `pg_settings` 实测 |
| D-2 | new-api 资源：1500m / swap 2400m / 2 core / `GOMEMLIMIT=1024MiB` | `deploy-zero-downtime.sh` + `docker update` | `docker inspect` 实测 |
| D-3 | 覆盖索引 `idx_logs_type_created_stats (type,created_at) INCLUDE (model_name,request_bytes,response_bytes,prompt_tokens,completion_tokens,quota)` 182MB | 生产 DB | `indisvalid=t`；日流量 18.1s→1.8s、模型排行 9.6s→1.1s、`/api/site/stats` 10.7s→**0.41s** |
| D-4 | 端口绑定收紧为 `127.0.0.1:${NEW_PORT}:3000`（原 `0.0.0.0:3001` 明文暴露公网，绕过 Caddy 的 TLS 与安全头） | `deploy-zero-downtime.sh` | `ss -tlnp` 显示 `LISTEN 127.0.0.1:3000` |
| D-5 | Caddy `/metrics` 限本机（公网曾 200，暴露 114 条运营指标） | `/etc/caddy/Caddyfile`（备份 `*.bak-metrics-*`） | 外网 **404** / 本机 200 |
| D-6 | Caddy 健康检查 `2s/2s` → `5s/5s`（容忍窗口 6s→15s，修 503 放大链） | 同上 | reload 后外网 10/10=200 |
| D-7 | 每日数据库备份：`/opt/new-api/backup.sh`（pg_dump -Fc + `pg_restore -l` 校验 + 保留 7 天），cron `0 3 * * *` | 生产主机 | 实测产出 182MB dump、耗时 44s、校验通过 |
| D-8 | fail2ban：`/etc/fail2ban/jail.local`（sshd, systemd backend, maxretry 4, bantime 1h） | 生产主机 | `fail2ban-client status sshd` = active |

## G. 日志日汇总预聚合（v1.3.189/190 —— 根治方案）

### 为什么必须做
覆盖索引只是**对症**：30 天窗口仍要扫 130 万索引项（实测 1.8s），且随天数线性恶化 ——
一年后回到 4-5s。真正的根治是预聚合，让查询从「扫窗口内全部原始行」变成「读 ≤N 行」。

### 实现
| 组件 | 说明 |
|---|---|
| 表 `log_daily_stats` | `day_key + dim_type(total\|model) + dim_value` 唯一，含 requests/bytes/prompt_tokens/completion_tokens/quota |
| `RefreshLogDailyStats(days)` | 事务内「删窗口 + 整批重插」，幂等；不用 UPSERT（GORM OnConflict 在 MySQL 曾渲染空子句） |
| `StartLogDailyStatsRefresh()` | master 每 10 分钟重算最近 2 天；表空则回填历史（上限 400 天）；停机可 Stop |
| 查询侧 | `DailyStatsCovers(fromDay)` 判断是否覆盖整个窗口；不覆盖则回退原扫表路径 |
| 维度选择 | 只做 `total` 与 `by-model`（字符串维度），数字维度需 CAST 会引入方言分支；`/api/log/report` 六维保持原路径 |

### 生产实测
| 项 | 值 |
|---|---|
| 回填 42 天 | **1.24 秒**（`log daily stats refreshed (window days=400, took=1.244722589s)`） |
| 汇总表规模 | total **43 行** / model **365 行**（43 天） |
| **历史口径一致性** | 与原始聚合**逐位相等**：req `1,629,848` / bytes `1,708,786,385,745` / quota `1,141,459,216` |
| `/api/site/stats` | 10.7s → **84-99ms** |

### 🔴 踩到的坑（重要）
`migrateLOGDB()` 在生产**没有真正创建** `log_daily_stats`：启动日志打印了
`database migration started`，但表不存在，刷新任务首次 tick 报 `42P01 relation does not exist`。
**同批的 `migrateLogTrafficIndex` 也没生效**（死索引 `idx_logs_traffic` 还在，是我手动 DROP 的）——
两件事指向同一处：`migrateLOGDB()` 内的 schema 变更在这个部署形态下不落地。
处置：把 `AutoMigrate(&LogDailyStat{})` 兜底到 `RefreshLogDailyStats` 开头（v1.3.190），
建不出来直接报错而不是静默退化成全表扫描。**根因仍未查清**，已列为待办 E-9。

## E. 剩余待办（本轮未做，按优先级）

| ID | 优先级 | 事项 | 阻塞原因 |
|---|---|---|---|
| E-1 | ~~**P0** 部署 v1.3.188~~ | **已完成** 2026-10-23 20:14 蓝绿部署，端口 3001，26.6s 零停机 | 外网 20/20=200、版本头 v1.3.188、近 10min 0 个 5xx |
| E-2 | ~~P0 删除死索引 `idx_logs_traffic`~~ | **已完成** `DROP INDEX CONCURRENTLY` 成功 | 索引列表已无该项 |
| E-3 | P0 | SSH 改密钥登录 + 关密码登录（24h 16867 次撞库） | **需用户提供公钥**，否则锁死 |
| E-4 | P1 | logs 保留策略（现 184 万行 / 42 天 / 3.5GB，`StartLogCleanupTask` 存在但无定时触发） | 删除数据属不可逆，**需用户定保留天数** |
| E-5 | P1 | `/api/log/report` 六个维度的全窗口聚合（`model/usage_report.go`） | 分组列（channel/user/group/token）不在覆盖索引里，需评估是否再加索引 |
| E-6 | P1 | 宿主 0 swap 而容器 `memswap 2400m` 是幻觉；`docker rm -f` 强杀旧容器导致在途流式请求 502（24h 53 个） | 需改容器停止策略 + 决定是否加 swap（历史教训：6G swap 打满反而更糟） |
| E-7 | P1 | 内核 5.15.0-30（2022 年）已 reboot-required 17 天 | 重启需维护窗口，**需用户决定** |
| E-8 | P2 | 前端审计代理未回（`evidence/audit-20261023-frontend.json` 缺失） | 代理未完成 |

## F. 本轮阻塞：SSH 被自己装的 fail2ban 封禁

**事实**：装 fail2ban 后，用于免交互 SSH 的 askpass 临时文件被 `/tmp` 反复清理，每次重试都计一次认证失败，累计超过 `maxretry=4`，触发 1 小时封禁（仅 22 端口、仅本机出口 IP）。**80/443 与线上服务不受影响**，封禁前后均无破坏性操作。

**恢复方式（三选一）**：
1. 等待 `bantime=1h` 自动解封（起始约 10:58 UTC，预计 **11:58 UTC** 恢复）；
2. 用户通过云服务商控制台（VNC/救援模式）执行 `fail2ban-client unban --all`；
3. 用户提供另一台可 SSH 的跳板。

**解封后应立即执行**（防再次自锁）：
```bash
sed -i 's/^maxretry = 4/maxretry = 10/' /etc/fail2ban/jail.local
sed -i '/^\[DEFAULT\]/a ignoreip = 127.0.0.1/::1' /etc/fail2ban/jail.local
systemctl restart fail2ban
cd /opt/new-api && ./deploy-zero-downtime.sh v1.3.188
docker exec postgres psql -U newapi -d "new-api" -X -c "DROP INDEX CONCURRENTLY idx_logs_traffic;"
```
