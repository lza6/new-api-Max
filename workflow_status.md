# workflow_status.md — 训练数据采集（多账号归属安全）终局闭环

> 最后更新：2026-10-20 18:00（Asia/Shanghai）
> 当前生产：`new-api:v1.3.162`（`freeapi.tingfengai.art` / 154.37.208.171，容器 healthy，上游随蓝绿交替）

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
