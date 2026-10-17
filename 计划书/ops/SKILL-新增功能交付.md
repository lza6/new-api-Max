> **镜像说明**：本文件是  的
> **受版本管理副本**。 被 .gitignore 忽略（第 27 行），
> 因此技能本体不会被提交、换机/换人 clone 后会丢失。
> **两份内容保持一致**：改了一边请同步另一边（这是已知的重复，见下方替代方案）。
>
> 替代方案（若你想消除重复）：把技能本体移到这里、让  只留一个指向本文件的短指针。
> 未采用是因为 Claude Code 的技能加载偏好自包含文件。

---

---
name: add-gateway-feature
description: new-api 网关「端到端新增一个功能/接口」的交付工作流——先读文档/记忆/规则判过时，再按 数据层→路由→鉴权→前端→性能→测试 的顺序逐节点落地并留下可执行证据，最后给出回滚路径。当你要加一个新端点、一张新表、一个新管理端页面，或把已有能力接入新场景时使用。
---

# 加一个新功能：端到端交付工作流

> 目标：从「需求」到「可回滚的证据」一次走通，不踩本项目的硬红线、不制造静默失败。
> 适用：新 API 端点 / 新管理端页面 / 新数据表 / 新后台任务 / 新采集或运营能力。
> 与 `new-api-add-feature` 的关系：那个 skill 偏「渠道 / relay 格式 / 计费表达式」的领域细节；
> 本 skill 是**交付流程骨架 + 本仓真实工程教训**。两者互补，都要看。

---

## 0. 何时使用（触发条件）

命中任意一条即适用：

- 要新增/修改一个 HTTP 端点（管理端、用户端或 relay 之外的功能端点）。
- 要新增一张表、一个 `model` 方法、一次 `AutoMigrate` 登记。
- 要把一个「模块」接入请求热路径或后台 worker。
- 要新增一个管理端页面并把前端接到后端。
- 要把本会话学到的范式复用（如路由契约测试、开关接线、压缩切分顺序）。

不适用：纯上游渠道适配（走 `new-api-add-feature`）、纯计费表达式改动（先读
`pkg/billingexpr/expr.md`）、纯前端样式微调（直接读 `web/AGENTS.md` 即可）。

---

## 1. 前置阅读清单（每步都先做，不可跳过）

| # | 读什么 | 为什么 | 怎么判断过时 |
|---|---|---|---|
| 1 | 根 `AGENTS.md`（用 Read 工具读，不要靠 `@import`） | 硬纪律全集：JSON 封装、三库、计费安全、relaykit、ASVS、i18n、受保护标识 | 对照 `AGENTS.md` 提到的 file:line 是否还在（如 `common/json.go`、`model/main.go`） |
| 2 | `web/AGENTS.md`（若碰前端） | 组件复用、i18n、测试、构建门禁 | `web/package.json` 里的脚本名是否仍匹配文中命令 |
| 3 | 本目录 `new-api-add-feature` SKILL.md | 领域细节与历史批次教训（v1.3.59–133） | 文中 `file:line` 与开关默认值是否仍成立 |
| 4 | `计划书/audit-ledger.md`（仓库根） | 本次相关主题是否已闭环、有没有「别重复跑/改过」的记录 | 末尾章节的日期与 `VERSION` 对照 |
| 5 | 跨会话记忆 `memory/*.md`（尤其 `new-api-verification-ledger-*`） | 哪些验证已 PASS（勿重跑）、哪些失败是**既有噪声**（勿误判回归） | 记忆里的 VERSION 与当前 `VERSION` 对照 |
| 6 | 本目录 `计划书/ops/SOP-*.md`、`计划书/adr/*.md` | 交付/排障/回滚 SOP 与既有架构决策 | 各文件头部「最后核对」版本号 |

**判过时**：文档里每一句「路径/命令/默认值/开关名」都必须能在当前代码里 grep 到；
对不上就是过时，**先更新文档再动代码**（不要用一份过时文档指导实现）。

---

## 2. 步骤化流程（每步：做什么 → 产出 → 怎么验证）

### 2.1 需求与规范

- 做什么：用 `brainstorming` skill 澄清意图；有歧义先问（≤5 问），不沉默选择。
- 产出：`specs/NNN-<feature>/spec.md`（用户故事 + 验收场景 + FR + 成功标准）+ `plan.md`。
- 验证：把每条验收场景写成「可执行断言」；涉及三库/计费/认证的，列出所引用的 AGENTS 规则。

### 2.2 数据层（三库 + JSON + 迁移）

- 做什么：
  - 新表用显式 `TableName()`（避免 GORM 复数化跨版本差异，见 `model/training_data.go:54-57`）。
  - 迁移登记到 `model/main.go`（新表加到 `AutoMigrate` 列表，参考 `model/main.go:384-385,436`）。
  - 所有 JSON 编解码走 `common.Marshal/Unmarshal/DecodeJson`，**不直接 import `encoding/json`**
    （`AGENTS.md:80-90`；实现见 `common/json.go:40-66`）。
- 产出：model 代码 + conformance 用例。
- 验证：
  - `go build ./...` + `go vet ./...`
  - 三库矩阵：`TEST_MYSQL_DSN=... TEST_POSTGRES_DSN=... go test ./model/ -run TestDBConformance`
    （用例见 `model/db_conformance_test.go:139+`，环境变量见 `:78,:95`）
  - 或本地一键：`make db-check`（`Makefile:53-54` → `scripts/db-conformance.ps1:49-50,76-77`）
  - 迁移**至少跑两次**证明幂等（fresh DB + 上一版升级）。

### 2.3 后端接口与路由

- 做什么：
  - **管理端路由放到 `/api/admin/...`，不要放进 `api-router.go` 的 `adminRoute`**——
    后者父组是 `/api/user`，实际前缀变成 `/api/user/...`（`router/api-router.go:89,167-168`）。
    正确范式：独立 router 文件 + `/api`→`/admin` 分组 + `AdminAuth()`，照
    `router/web-protection-router.go:10-26` 和 `router/training-data-router.go:29-51`。
  - 在 `router/main.go` 的 `Set*Router` 序列里登记新 router（`router/main.go:32-33` 是范本）。
  - 读接口加 `middleware.DisableCache()`（`router/training-data-router.go:35-50` 是范本）。
- 产出：`router/<name>-router.go` + `controller/<name>.go` + 路由契约测试。
- 验证：**路由契约测试**（见 §5 第一条坑）——扫描前端源码里的 `/api/...` 路径，
  断言每个都在后端注册表里存在（`router/training_data_router_test.go:50,62,107,115-118`）。

### 2.4 鉴权与安全（涉及认证则强制 ASVS）

- 做什么：
  - 认证/授权/绑定/会话相关改动，必须先读 OWASP ASVS 与对应 Cheat Sheet（`AGENTS.md:49-55`）。
  - 多租户归属校验用 `model.Get*ByIds(id, userId)` 之类的**归属过滤**（见 `new-api-add-feature` §6）。
  - 迁移/放行类改动一律 **fail-closed**（拿不到证据就拒绝）。
- 产出：安全回归用例，覆盖失败/过期/重放/绕过。
- 验证：`go test` 定向安全用例；越权路径必须仍被覆盖（不能为了通过而削弱断言）。

### 2.5 计费（涉及则必读）

- 做什么：必读 `pkg/billingexpr/expr.md`；配额转换只走 `common.QuotaFromFloat*/QuotaRound*/
  QuotaFromDecimal*`（`AGENTS.md:118,122-132`）；乘数走既有上界常量。
- 产出：回归用例与饱和标记。
- 验证：改动前先读记忆 `never-change-production-billing-directly.md`；**绝不在生产直接改计费**。

### 2.6 前端

- 做什么：先搜 `web/src/components/` 与既有 feature 复用（`web/AGENTS.md:3.3` 强制）；
  文案全走 i18n；新 key 用**纯文本行插入**（见 §3 红线 5）。
- 产出：`web/src/features/<feature>/`（`api.ts` + `components/` + `hooks/`）。
- 验证（`web/package.json` 实有脚本）：
  - `cd web && bun run typecheck`（`tsgo -b`）
  - `bun run lint`（`oxlint`）+ `bun run format:check`
  - `bun run i18n:check`（缺失键）、`bun run i18n:sync`（回填）
  - `bun run copyright:check`
  - `bunx vitest run <受影响目录>`
  - `bun run build`

### 2.7 性能（碰热路径/新增公开端点则强制）

- 做什么：
  - 热路径**不做 DB 操作**、不做秒级聚合；缓存要 **SWR + 单飞**（见 `new-api-add-feature` §16.1）。
  - 复制截断文本用 `strings.Clone` 切断对大原文的引用（见 ADR-006）。
  - 常驻 loop 用 `service/background_loop.go`，可停可重启。
- 产出：压测脚本入 `计划书/e2e-evidence/<batch>/`。
- 验证：公开端点**真压测**（单元测试测不到）；热路径报出微秒级耗时。

### 2.8 测试

- 做什么：遵守 `AGENTS.md:134-146` 与 `web/AGENTS.md:3.14`——TDD 先 RED；
  修复必须**先复现**；测试按行为命名、禁止 fake fuzz/smoke/覆盖率凑数。
- 产出：定向单测 + 契约测试 +（跨模块）集成用例。
- 验证：临时破坏修复点 → 测试必须变红 → 还原变绿（**RED 反向验证**）。

---

## 3. 本项目的硬红线（违反会被测试或审查拦下）

1. **三库兼容**：任何影响 DB 行为的改动，必须用**真实** SQLite + MySQL ≥5.7.8 + PG ≥9.6 验证，
   单元测试/mock/只测一个方言都不算数（`AGENTS.md:92-98`）。
2. **JSON 封装**：业务代码只用 `common.*`，不直接 `encoding/json`（`AGENTS.md:80-90`，`common/json.go:40-66`）。
3. **relaykit 独立**：动 `relaykit/` 必须 `cd relaykit && GOWORK=off go build ./...`（`AGENTS.md:75-78`）。
4. **管理端前缀 = `/api/admin`**：放错到 `adminRoute`（前缀 `/api/user`）会**整套 404**
   （`router/api-router.go:167-168`；事故记录见 `router/training-data-router.go:12-21`）。
5. **i18n 纯文本行插入**：新增 locale 键要**手写文本行**，并只在跑 `bun run i18n:sync` 后用其
   输出；**不要**用会整体 re-serialize 且不认混淆键的工具改 `web/src/i18n/locales/*.json`——
   文件里有混淆键 `footer.newapi.projectAttributionSuffix`（`web/src/i18n/locales/en.json:2590`），
   一旦被普通 JSON 工具解码成明文 `newapi` 就破坏了受保护标识（`AGENTS.md:166-173`）。
   sync 脚本自身会保形（`web/scripts/sync-i18n.mjs:25-30,122-128`）。
6. **受保护标识**：`new-api` / `QuantumNous` 相关引用一律不得修改删除（`AGENTS.md:166-173`）。
7. **指针 + omitempty**：客户端 JSON 解析后回传上游的请求结构体，可选标量必须用指针
   （`AGENTS.md:114-116`）。
8. **计费安全不变量**：永不产生负扣费；配额转换只用 `common/quota_math.go` 的 helper；
   乘数必须有上界（`AGENTS.md:122-132`）。
9. **不外发付费 API**：真实付费调用默认预算为 0，用 mock/fixture 验证。
10. **不新增 `docs/` 文件**（`AGENTS.md:148`）；不给单次使用建抽象/加未要求灵活性。

---

## 4. 验收清单（可直接勾选，每条是可执行命令）

后端：

- [ ] `go build ./...`
- [ ] `go vet ./...`
- [ ] `cd relaykit && GOWORK=off go build ./...`（改了 relaykit 才必跑）
- [ ] `TELEMETRY=... go test ./<受影响包>/ -run <用例名>`
- [ ] 涉 DB：`make db-check` 或 `TEST_MYSQL_DSN=... TEST_POSTGRES_DSN=... go test ./model/ -run TestDBConformance`
- [ ] 路由契约：`go test ./router/ -run TestTrainingDataAdminRoutes|TestFrontend.*PathsExistOnBackend`
- [ ] 迁移幂等：fresh 库 + 上一版库各跑一次 startup/AutoMigrate

前端：

- [ ] `cd web && bun run typecheck`
- [ ] `bun run lint`
- [ ] `bun run format:check`
- [ ] `bun run i18n:check`
- [ ] `bun run copyright:check`
- [ ] `bunx vitest run <受影响目录>`
- [ ] `bun run build`

证据/文档：

- [ ] 每个新端点有一条真实 E2E（起本地网关 `SQL_DSN=local SQLITE_PATH=... PORT=18100` 打新端点）
- [ ] 新增 key 在 7 个语言文件里齐全（`i18n:check` 缺失=0）
- [ ] 回填 `计划书/audit-ledger.md` + 记忆台账 +（必须时）`计划书/ops/SOP-回滚.md` 覆盖到新东西
- [ ] 每条断言都用**实际运行输出**支撑；未跑的命令标「未验证」，不写「通过」

---

## 5. 常见坑表（来自本次会话的真实事故）

| 症状 | 根因 | 怎么避免 | 怎么验证 |
|---|---|---|---|
| 管理端接口**全部 404**（"库全绿但生产点不开"） | 直接调 handler 的测试绕过了路由注册；管理路由放进了前缀为 `/api/user` 的 `adminRoute` | 管理端一律独立 router 文件 + `/api/admin` 分组；测试必须**走真实路由表** | 路由契约测试扫前端源码里的 `/api/...` 路径，逐个断言后端存在（`router/training_data_router_test.go:107-133`）；RED：改错一个前端路径 → 测试失败并指出「会 404」 |
| 云盘分片解压 `unexpected EOF`／预览显示二进制垃圾 | **先整体 gzip 再按字节切** → 分片是坏 gzip 碎片 | **先按原始 JSONL 的行切，再对每块各自 gzip**（`service/training_capture/worker.go:465-488`） | 不变量「分片原始上限 ≥ 文本上限 × 8」；RED：改回旧序 → 测试打印泄漏的 gzip 魔数 |
| OAuth 老账号被**接管**（攻击者用旧 provider id 绑到别人账号） | 历史绑定迁移没有账号证据就放行 | 迁移要求「provider 侧**已验证**邮箱与目标账号邮箱归一化后一致」，拿不到就**拒绝（fail-closed）**（`controller/oauth.go:443-477,654-685`） | 5 条用例含攻击者场景（`controller/oauth_legacy_migration_test.go:30-85`）；证据接口 `oauth/provider.go:56-59`、`oauth/github.go:204-234` |
| 超限会话**尾部内容永远采不到**（静默丢数据） | 只保留头部，增量以头部为前缀比对 | 截断保留头+尾；`TailOnly` 走尾部增量（`service/training_capture/hotpath.go:155-179,181-224`） | 断言超限文本的总长严格 ≤ 上限且含尾部片段 |
| 单个坏数据**杀进程** | worker 无 `recover()`，panic 发生在 goroutine 上 | worker 顶层 `defer recover()`；与「绝不拖垮主业务」承诺对齐 | 注入会 panic 的输入，断言进程存活 |
| `Close` 后仍有 `send` → `panic: send on closed channel` | `Close` 关了数据通道，`TryEnqueue` 并发 send | **根本不关数据通道**，另开 `stop` 通道表达停止消费（`service/training_capture/queue.go:61-88,177-187`） | 200 轮并发回归；热路径零锁且更快 |
| 撤回授权后仍在上传 | 缓存 TTL 窗口内旧值被写回（TOCTOU） | 撤回时**主动失效缓存**，消费前**复查授权** | 撤回 + 并发入队回归用例 |
| 目录/文件数恒为 0 | 队列起点把 RootPath 拼了两次 | 起点用空串，前缀由 `itemPath` 负责 | 断言 `CountItems` 真实计数 |
| 「自适应队列容量」不生效，启动日志报假容量 | 队列是包级变量，构造时预算还是零值（死代码） | 启动时按预算**重建**队列（`service/training_capture/resources.go:270-293`） | 断言预算初始化后再取容量；日志报**实际**值 |
| 测试**假过**（前序用例污染预算） | 包级状态未复位 | 加 `ResetBudgetForTest()` 隔离 | 单跑该用例也必须过 |

> 共同教训：**注释里写的「绝不会/立即生效」必须有并发的或反向的用例去证伪它**——
> 本次三处 BLOCKING 全是「串行用例绕过了并发窗口」。

---

## 6. 反模式（明确禁止）

- **直接调 handler 做集成测试**，不经过路由注册 → 路径写错也全绿（本次 404 事故的根因）。
- **用 `encoding/json` 直用** → 违反 `AGENTS.md`，且绕过统一 codec。
- **先压缩再切分** → 产出坏 gzip 碎片，静默乱码（见 §5）。
- **只测一个库就说「三库兼容」**；用 mock 冒充真实集成。
- **注释宣称未验证的保证**（“绝不会 panic”“立即生效”）却没有用例证伪它。
- **用 Write 覆盖已存在的文件**（本仓曾因此摧毁已提交文件）——写前先 `git ls-files <path>` 或 Read。
- **为了覆盖率加 smoke/sleep/随机输入测试**；为让测试变绿而削弱断言。
- **把静态检查描述成真实用户路径验证**；未实际运行就宣称「通过」。

---

## 7. 收尾与交付

1. 走 §4 验收清单，逐条留下**真实输出**。
2. 起一个**独立审查**子代理（不继承实现者结论），对需求完整性/边界/安全/测试缺口逐条复验。
3. 回填文档：`计划书/audit-ledger.md` + 记忆台账；新东西若影响回滚，更新
   `计划书/ops/SOP-回滚.md`。必要时把新的架构取舍写成 `计划书/adr/ADR-*.md`。
4. `graft build` 刷新知识图（若项目用 graft）。
5. 交付说明用四级标签：`已验证`（实际运行）/ `静态确认`（读码）/ `合理推断` / `待验证`。
6. 发布/回滚见 `计划书/ops/SOP-新功能交付.md` 与 `计划书/ops/SOP-回滚.md`。
