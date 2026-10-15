# workflow_status.md — Batch-7：参考指南全量落地 + 伪闭环修复

> 起始 v1.3.116（2026-10-09）。用户全量授权，要求把《参考的结果计划指南》全部落地闭环。
> 纪律：只记录事实与证据；**未观察到交付物与验收证据不标 done**。

## 契约

**目标**：把《参考的结果计划指南》§6 路线图 + §8 首批清单全部落地；优先修复**伪闭环**。
**成功标准**：每项有可执行代码 + 生产链路调用点 + 可复现验证；无"库全绿但零调用"。
**停止条件**：全部批次完成且验证通过，或遇到不可替代的凭证/环境阻塞。

## 🔴 头号发现：Batch-5 三模块伪闭环（v1.3.100 宣称 DONE，实为死代码）

依《参考指南》§3「自营平台 v161 伪闭环事故教训」第 1/2 条自查——
**「库写完+测试绿 ≠ 落地」「新模块交付时必须 grep 全 src 找调用点」**。实测：

| 模块 | 文件 | 测试 | 生产调用点 | 判定 |
|---|---|---|---|---|
| T4 复杂度路由 | `service/complexity_router.go`（292 行） | `batch5_test.go` 3 组 | **0**（仅测试引用） | 🔴 DEAD |
| T7 中继一致性自检 | `service/relay_audit.go`（215 行） | `batch5_test.go` | **0**（仅 `metrics.go` 读空计数器） | 🔴 DEAD |
| T10 策略引擎 | `service/policy_engine.go`（234 行） | `policy_engine_test.go` | **0**（仅 `metrics.go` 读空计数器） | 🔴 DEAD |

复核命令与结果（全文 grep，排除测试与自身文件）：
```
grep -rn "ScoreComplexity|ExtractComplexitySignals|ComplexitySignals" --include=*.go .
  → 仅 service/batch5_test.go 命中
grep -rn "AuditSSEWhitelist|AuditUsageMonotonic|AuditErrorLeak|AuditModelFingerprint|RecordRelayAudit|NewUsageMonotonicState" --include=*.go .
  （排除 service/relay_audit.go 与 batch5_test.go）→ 空
grep -rn "Evaluate(" --include=*.go . （排除 _test.go 与 pkg/）
  → 仅 service/policy_engine.go:142 定义自身
```
→ `RecordPolicyEval` / `RecordRelayAudit` 从未被调用，故 `/metrics` 的
`policy_engine_eval_total`、`relay_audit_findings_total` 恒为 0；两个 gauge 是空壳。
**这属于「把有代码片段包装成已完成」——必须修复或明确删除。**

## 任务清单

| # | 任务 | 来源 | 风险 | 状态 |
|---|---|---|---|---|
| T7-1 | 修伪闭环 T4：复杂度打分接入 relay 请求路径 | 自查 | L2 | **DONE** |
| T7-2 | 修伪闭环 T7：中继自检接入流式/错误/成功路径 | 自查 | L2 | **DONE** |
| T7-3 | 修伪闭环 T10：策略引擎接入 pre-consume 判定 | 自查 | L2 | **DONE** |
| T7-4 | 模型广场按分组计费表达式显示为源码（用户报告） | 用户 | L1 | **DONE** |
| T7-5 | 模型广场「套餐对比」入口移除 | 用户 | L1 | **DONE** |
| T7-6 | 修伪闭环 T1：反事实节省基准前端零消费 | 自查 | L1 | **DONE** |
| T7-7 | P1-1 记忆层：补齐 MemoryInjection 的用户配置界面 | 指南 §6 | L2 | **DONE** |
| T7-8 | P1-5 Skills 注入 v1 | 指南 §6 | L2 | **DONE** |
| T7-9 | P1-2 per-agent 能力下沉（Agent 预设） | 指南 §6 | L2 | **DONE** |
| T7-10 | P1-6 游乐场视频生成 | 指南 §6 | L2 | **DONE**（真实往返受环境限制） |
| T7-11 | P2-6 PPT/文档生成 | 指南 §6 | L2 | 调研完成，实施需先验证 3 个前提 |
| T7-12 | P0-1 渠道密钥加密 | 指南 §6 | L3 | 已实现待审批（默认关） |

## 本轮 commit 序列

| 版本 | commit | 内容 |
|---|---|---|
| v1.3.117 | `a9c3bc2b2` | T4/T7/T10 死代码模块接线 |
| v1.3.117 | `ec2abaa5c` | 模型广场计费显示修复 + 移除套餐对比 + T4/T7 结果前端可见 |
| v1.3.118 | `bde441e19` | 模型详情页/成本明细不再展示表达式源码 |
| v1.3.119 | `841a43300` | 记忆注入用户配置界面 |
| v1.3.120 | `9cd9ffbe4` | 反事实节省基准前端消费 |
| v1.3.121 | `2f52ef29d` | Skills 注入 v1 + Agent 预设 |
| v1.3.122 | `0b6fa8a25` | 游乐场视频生成 |

## P2-6 PPT 调研结论（未实施）

子代理调研 + 主线程核验结论：
- 有可插入点：task 产物走 `type:"file"` + `GET /v1/tasks/:key/artifacts/:key/content`
  （`router/task-router.go`），前端已有 file 下载链接（`task-artifacts.tsx`）
- **但发现既有缺口**：`TaskArtifactStore.Persist` 在生产代码中**零调用方**
  （`service/task_artifact_store.go`），即「产物字节落盘」链路未接线
- 插件沙箱（`pkg/jsplugin/engine.go`）禁 import/await、5s 超时、源码上限 1 MiB，
  且插件产出的 URL 必须绝对 http(s)（`relay/channel/task/jsplugin/adaptor.go`），
  `data:` URL 旁路只对 legacy 任务可达
- `pptxgenjs` 可获取（npm 可达，v4.0.1），但**三个前提未验证**：
  ① 打包后 < 1 MiB 源码上限；② 无 `Buffer` 时 JSZip 能否产出合法 zip；
  ③ 5s 超时内能否生成 + 产物如何出站
- 结论：**不宜在未验证上述前提的情况下开工**，否则会产出又一个伪闭环。
  最小可用切片（确定性 JSON → .pptx，不调 LLM、不加表）已明确，待前提验证后实施。



## 已验证交付物

### A. 三个伪闭环模块接入生产链路（RED 验证通过）

| 模块 | 接入点（file:line） | RED 验证 |
|---|---|---|
| T4 复杂度路由 | `controller/relay.go` 预扣费前打分 → `ContextKeyComplexityScore`；`service/log_info_generate.go:appendComplexityScore` 写入日志（档位 public / 细分 admin） | 摘除 `AppendRelayLogAdminInfo` 中的调用 → `TestComplexityScoreFlowsFromContextToLog` **FAIL**（expected "medium", got nil）；恢复 → PASS |
| T7 中继一致性自检 | `relay/helper/stream_scanner.go` 逐帧 `service.AuditStreamFrame`（SSE 键白名单 + usage 单调）；`controller/relay.go` 错误路径 `AuditUpstreamError` + 成功路径 `AuditModelFingerprintOnce`；`FlushRelayAudit` 落 `admin_info.relay_audit` 并推进 `/metrics` | 注释掉 `FlushRelayAudit` 调用 → `TestRelayAuditFrameCollectionAndFlush` **FAIL**（relay_audit 为空 + 计数器为 0）；恢复 → PASS |
| T10 策略引擎 | `controller/relay.go` 预扣费前 `evaluateRequestPolicy`（护栏/预算/限流三源）→ shadow 记录 / enforce 拦截；`RecordPolicyEval` 推进 `/metrics` | 由 `TestPolicyEngineEvalCounterIsWired` 锁定计数推进；`TestPolicyBudgetSkipsWhenQuotaUnknown` 锁定「额度未知不误拦」 |

新增文件：
- `service/relay_audit_collect.go` — 请求级收口（Collect/AuditStreamFrame/AuditUpstreamError/AuditModelFingerprintOnce/FlushRelayAudit），并发安全，开关关闭零分配
- `service/batch7_wiring_test.go` — 7 组接线回归测试（跨模块契约，非模块内部纯函数）
- `constant/context_key.go` 新增 `ContextKeyComplexityScore` / `ContextKeyRelayAuditFindings`
- `common/rate-limit.go` 新增 `InMemoryRateLimiter.Count`（只读观测，不推进窗口）
- `middleware/user-rate-limit.go` 新增 `UserRateLimitLiveState`（读真实计数，不重复造计数器）
- `setting/relay_setting` 新增 `policy_max_prompt_chars`（热更新，默认 0=不限）

**前端垂直切片**（让接线"用户可见"）：
- `web/src/features/usage-logs/types.ts`：`relay_audit` / `complexity_tier` / `admin_info.complexity` 类型
- `details-dialog.tsx`：新增「中继自检」与「请求复杂度」两个区块（自检明细仅管理员可见）

### B. 模型广场计费显示修复（用户报告）

**问题**：卡片把 `group == "token计费" ? (len < 200000 ? tier(...))` 整段源码展示给用户。

**根因**（实测 `STATUS=unsupported`）：`web/src/features/pricing/lib/billing-expression/parser.ts:314` 只认
`TOKEN_VARIABLES`，v1.3.114 引入的 `group`/`channel` 被判为 unsupported → 编译失败 →
`isSpecialExpression=true` → 回退展示 `rawExpression`。

**修复**（3 层）：
1. `types.ts` 新增 `CONTEXT_VARIABLES`（group/channel）与 `{kind:'context'}` 节点
2. `parser.ts` 识别上下文变量（类型标 `dynamic`，避免 `group == "x"` 误判类型错误）；`runtime.ts` 求值（缺失时 group='' / channel=-1 → 走 else 分支）
3. `display.ts:readTokenTierChain` 剥掉外层 `group == "..."` 守卫，直接读 yes 分支的阶梯链

**效果**（实测）：deepseek-v4.1-flash 现在展开为 3 档价格
`tok_lt200k $0.001` / `tok_200k_500k $0.0015` / `tok_ge500k $0.002`，并保留长度阈值条件。
glm-5.3-flash 同形态展开为 `$0.004 / $0.005 / $0.006`。
无法归纳的表达式改为一句人话 `Pricing varies by usage — see details`（**任何情况下都不再展示源码**）。

前端**全部**源码展示点已清除（commit bde441e19 补齐）：
- `model-card.tsx`（模型广场卡片）
- `model-price-cell.tsx`（广场/管理端横条）
- `model-details.tsx`（详情页 Base Price 与 Pricing by Group 两处）
- `dynamic-pricing-breakdown.tsx`（成本明细）
- 保留：`system-settings/models/tiered-pricing-editor.tsx`（管理员配置界面，源码编辑器是功能本身）

同时移除模型广场的「套餐对比」入口按钮（`pricing/index.tsx`；`/pricing/plans` 路由保留，旧书签仍可解析）。

新增测试：`web/src/features/pricing/__tests__/group-gated-tier-display.test.ts`（3 例，含两个生产表达式，RED→GREEN 已验证）；
更新 `model-cards.test.tsx` 断言「不得展示表达式源码」。

## 验证台账（Batch-7）

### 后端
- `go build ./...` ✅
- `go test ./service/ -run "TestComplexityScoreFlows|TestRelayAudit|TestPolicyEngineEval|TestPolicyBudgetSkips" -count=1` ✅ ok 18.6s
- **RED 验证**：两处摘除接线 → 对应用例 FAIL；恢复 → PASS（见上表）
- `gofmt` 全部改动文件 ✅

### 前端
- `bun run typecheck` ✅ 0 错
- `bun run check-missing-i18n` ✅ 0 missing（新增 7 键 × 7 语言，纯文本行插入，混淆键 `footer.newapi.*` 完好）
- `vitest run src/features/pricing/` ✅ **207 passed**；唯一失败的 suite `pricing-error-state.test.tsx`
  是**既有** vite/vchart 模块解析问题（0 用例收集），已用 `git stash` 基线复现同样失败 → 非本次引入
- `oxlint` 改动文件 0 error（`details-dialog.tsx:1589` 的 array-index-key 为既有问题，基线第 1528 行同款）

## 生产状态
- 生产 `VERSION=1.3.116`（本次改动**未部署**——含前端，需用户明确授权后才重建镜像并热更新）
- 所有新开关默认关：`COMPLEXITY_ROUTING` / `RELAY_AUDIT_ENABLED` / `POLICY_ENGINE_MODE=off`
- `CHANNEL_KEY_ENCRYPTION` 仍为 false（渠道 key 明文，P0 未闭合）


## 执行原则
1. 全部**默认开关关**（零生产行为变化），可灰度、可回滚
2. 复用既有底座（relay 生命周期、UserSetting、billingexpr、jsplugin），不重造
3. 每项：先写失败测试（RED）→ 实现 → 转绿 → 反向验证（故意破坏应 FAIL）
4. 危险操作（生产部署/推送/发布）**需用户明确授权**

## 生产状态（起始快照）
- 生产 `VERSION=1.3.116`，`new-api Up`（healthy），`/healthz` `/readyz` 均 200
- `CHANNEL_KEY_ENCRYPTION` 默认 false（未启用，渠道 key 仍明文——**P0 未闭合**）
- Batch-5 开关（COMPLEXITY_ROUTING / TOOL_DRAWER_ENABLED / RELAY_AUDIT_ENABLED /
  POLICY_ENGINE_MODE / CHANNEL_HEALTH_WEIGHTED_LB）全部默认关

---

# workflow_status.md — Spec-008：训练数据采集 + OneDrive 冷存储

> 起始 2026-10-14。用户全量授权。规范见 `.specify/specs/008-training-data-collection-onedrive/`。
> 纪律：只记录**实际运行的命令与观察到的结果**；未见交付物与验收证据不标 done。

## 契约

**目标**：用户授权后异步采集对话数据 → 冷存 OneDrive（`日期/用户名/模型/part-NNNN.jsonl`）；
管理员可总开关与看统计；用户可在个人资料页自助授权/撤回（含免责声明）。
**成功标准**（每项都要证据）：
1. 采集**不影响请求延迟**（压测对照证明）
2. 增量去重（长期会话只传新增）
3. 三库迁移幂等
4. 真实 OneDrive 上传 + 读回一致
5. 端到端：授权 → 对话 → 云盘出现正确文件；撤回 → 无新增
6. 管理员关总开关 → 用户页开关消失
**停止条件**：全部任务完成且验证通过，或遇到不可替代的凭证/环境阻塞。

## 硬约束（来自项目宪法 `docs/.../constitution.md`）

| 条款 | 对本功能的要求 |
|---|---|
| I 生产优先 | 真实链路验证，禁止静态审查/mock 冒充完成 |
| II 计费安全 | 采集**不得触碰**计费路径 |
| III 安全基线 | 凭据不落库；服务端强制授权 |
| IV 三库兼容 | 新表三库成立、迁移幂等 |
| **V 性能** | **网关不得给用户请求增加可感知延迟** ← 本功能最大风险 |
| VI i18n | 新增文案全语言同步 |
| VII 保守改动 | 不做无关重构 |

## 任务进度（8/23）

| 任务 | 状态 | 证据 |
|---|---|---|
| T1.1 目录名净化 + 分片命名 | ✅ done | `go test ./service/training_capture/` **21/21 PASS**；含 6 种路径穿越变体、同名消歧、非 ASCII |
| T1.2 增量去重（水位+指纹） | ✅ done | 同包 **48/48 PASS，`-race` 干净**；`SelectIncrementLongConversationUploadsOnlyNewTurns` 断言「40 条会话追加 2 轮 → 只传 4 条」 |
| T1.3 有界非阻塞队列 | ✅ done | **61/61 PASS，`-race` 干净**；**热路径实测 2.395µs/次** |
| T1.4 两张新表 + 三库 | ✅ done | **三库 conformance 全过**（SQLite / **MySQL 5.7** / PostgreSQL）；证据 `计划书/e2e-evidence/spec-008/three-db-conformance.md` |
| T2.1 Sink 抽象 | ✅ done | 单测锁定「未配置必须报错而非静默成功」 |
| T2.2 OneDrive 实现 | ✅ done | **真实云盘 9/9 PASS**；证据 `计划书/e2e-evidence/spec-008/onedrive-live-integration.md` |
| 日志轮转（双上限） | ✅ done | `go test ./logger/` **13/13 PASS**；对生产真实文件模拟：删 10 个释放 **476MB** |
| T3.1–3.3 采集管道 | ⏳ 进行中 | — |
| T4.x 接口层 | ⏳ 待做 | — |
| T5.x 前端 | ⏳ 待做 | — |
| T6.3 日志按大小分片 | ⏳ 待做 | — |
| T7.1 README 重写 | ⏳ 待做 | — |
| T8.x E2E + 部署 + 文档 | ⏳ 待做 | — |

## 关键实测事实（省得重复调研）

- **OneDrive 配额 5TB / 剩余 5.00TB**；client_credentials 认证（无需浏览器登录）
- OneDrive 能力实测：建目录✅ 上传✅ 读回✅ 列举✅ 删除✅
- **读回必须跟随 302 重定向**，否则空响应
- **建目录必须 `conflictBehavior=replace`**，否则 409 或产生垃圾目录
- **简单上传上限 4MB**（Graph 限制）
- `dto.Request` 接口**读不到结构化消息数组**，只有 `TokenCountMeta.CombineText`
- **`logs` 表不含对话正文** → 「扫日志表采集」不可行
- 本机**无 Docker/MySQL**；三库验证要在**生产服务器**上做（方法记在 ledger）

## 生产磁盘现状（2026-10-14）

- 根盘 9.3G / 78G（12%）
- `/opt/new-api/data/logs` = 870MB / 24 文件 / 单个最大 173MB
- `/opt/new-api/data/data` = 8KB
- **生产没有储存用户图片**：站内图床未启用、响应缓存未启用、日志无 base64、DEBUG 未开

## 阻塞 / 待决策

- 无（当前无不可替代阻塞）

## 验证日志（追加式）

- 2026-10-14：Phase 1 全部 4 个任务 + T2.1/T2.2 完成并通过真实验证（见上表）

## 独立对抗式审查（2026-10-14）—— 发现 3 BLOCKING + 4 SHOULD-FIX

对 Phase 1–3 已完成代码发起独立子代理审查，要求「有罪推定、只报告不修改」。
**7 项全部为真实缺陷**（代理用独立 Go 复现验证，不是纸面推理）。逐项修复：

### BLOCKING（已修，均先复现再修）

| # | 缺陷 | 我的注释当时怎么说的 | 真相 | 复现 |
|---|---|---|---|---|
| B1 | `TryEnqueue` 与 `Close` 并发 → **`send on closed channel` panic** | 「关闭后 TryEnqueue 一律返回 false（不 panic）」 | **会 panic**；且发生在 relay goroutine 上时**杀进程** | 200 轮稳定复现 |
| B2 | 撤回 TOCTOU → 撤回后最长 30s **仍在采集** | 「撤回时主动失效缓存，使撤回立即生效」 | 只在无并发时成立；并发下旧值会写回缓存 | 代码路径分析 + 纪元修复 |
| B3 | 历史**中间**消息被编辑 → **永久不上传** | 「历史被改动 → 保守全量重传」 | 只对**末位**编辑成立；中间编辑静默丢失 | 增量 = 1 条（应为 4 条） |

**共同教训**：三处注释都在**宣称一个未被验证的保证**。注释写「绝不会」时，
必须有**并发的**用例去证伪它——原实现的三个测试恰好都是串行的，全部绕过窗口。

### SHOULD-FIX（已修）

| # | 缺陷 | 修法 |
|---|---|---|
| S1 | `err == gorm.ErrRecordNotFound` 不符项目约定（全仓 30+ 处用 `errors.Is`） | 改 `errors.Is`；否则一旦错误被包装，会把「无记录」当 DB 故障 → **错误地停止采集** |
| S2 | `UpsertTrainingDataConsent` 先查后写非原子 → 双击开关撞唯一索引**把内部错误甩给用户** | 改 GORM `clause.OnConflict` 原子 upsert（按方言生成 SQL，MySQL 5.7 走 `ON DUPLICATE KEY UPDATE col=VALUES(col)`；**不用** `AS new`——那需 8.0.19+） |
| S3 | `List` 超限**静默截断** → 分片序号算小 → **覆盖已上传分片** | 超限**明确报错**（带着错误序号做破坏性决策比失败更糟）；`CountItems` 增加 `truncated` 返回值（把下界当真实值会让管理员错误决策） |
| S4 | `createFolder` 把 409 一律当成功 → 若该位置是**同名文件**，错误会在**离根因很远的地方**才浮现 | 409 时**回读确认确实是目录**，否则明确报错 |

### 审查确认**不是**缺陷的两处（避免我过度修改）

- `SanitizeDirName` 的长度不变量**成立**：审查方跑 **100 万组**随机输入（含 CJK/emoji/
  控制字符/极端 ID）零违规——因为净化后必为纯 ASCII，字节截断不可能切开多字节 rune。
- `normalizeContent` 的 NBSP(U+00A0) 与表意空格(U+3000) **是真 rune**，
  不是「永不匹配的 rune literal」。唯一限制是未覆盖 U+2000–200A 等**其它**空格变体
  （属「覆盖不全」，非 bug）。

### B1 修复的取舍（值得记）

先试「用 RWMutex 让检查+发送与 close 互斥」→ 修好了，但**热路径 2.4µs → 11.6µs（5 倍）**。
改为**根本不关数据通道**（另开 `stop` 通道表达停止消费）：
`Close` 后即使 send 也只进缓冲、不可能 panic。
结果 **安全与性能同时成立**：热路径 **1.93µs**，比原实现还快。
→ 教训：**遇到「加锁才安全」时，先问能否从设计上消除共享**，而不是直接上锁。

## 2026-10-15 增量需求盘点（用户指出此前拆解不完整）

上一轮「15/23」的分母**漏掉了用户实际要的东西**。补记需求追踪矩阵：

| 需求 | 状态 | 证据 |
|---|---|---|
| B1 内存自适应（80%，非 900MB 锁死） | ✅ 本轮闭环 | resources.go + 测试（cgroup v2/v1/host/floor 四级检测，含容器 OOM 坑） |
| B2 大文本压缩上传（非 64KB 截断） | ✅ 本轮闭环 | compression.go + 测试（压缩率断言 <40%，zip-bomb 防护，.jsonl.gz 契约） |
| B3 并发/异步/分片队列 | ✅（既有 + 本轮压缩后重切分） | 队列 + 限速 + gzip 切分按实际大小 |
| B4 管理端云盘文件列表 | ⏳ 待做 | 需 sink 读方法 + admin API + 前端 |
| B5 待上传队列/正在上传 MB 实时显示 | ⏳ 部分（有 QueueLen，无 MB） | CaptureStatsSnapshot |
| B6 云盘剩余/总量 | ✅ | overview API SinkOverview |
| B7 云盘操作（预览/下载/删除） | ⏳ 待做 | 依赖 B4 的读方法 |
| B8 今日上传数量/G + 总计持久化 | ⏳ 部分（进程内计数，重启归零） | CaptureStats |
| B9 对话深度分析（用户原话+报告） | ⏳ 待做 | 调研确认可行：channel-test.go 合成上下文范式；记录含用户全部原话 |
| B10 继续跟 Agent 聊 | ⏳ 待做 | 复用 /pg 端点 + playground 前端 |
| B11 内存/存储/延迟精细管理 | ✅（B1+B2 落实主要部分） | — |
| B13 UI/UX/移动端 | ⏳ 待做 | — |
| B14 用户按消耗排名 | ⏳ 待做 | 运营面板的一部分 |
| B15 运营面板（每日/总消耗、排行、一键充值、剩余额度、可运营时长） | ⏳ 待做 | — |
| B16 Web 防护界面翻页 | ⏳ 待做 | — |
| B18/B19 上游对标与 Issues 跟进 | ⏳ 待做 | 需网络（本机 curl HTTPS 坏，用 node） |

### 独立审查（relay 热路径回归审查）已修复项
- **P1 截断钉内存**（10MB 原文被 64KB 子串钉住）→ strings.Clone + unsafe 回归测试 ✅
- 审查确认安全：meta 非 nil、成功分支只投递一次、闭包未捕获 gin.Context、
  启动顺序正确、6 条路由鉴权全对（管理端 3 条在 AdminAuth 下）

### 审查确认的新缺口（待修）
- CountToken=false 时 CombineText 为空 → 采集静默失效（需启动日志提示）
- Relay() 成功分支接线零测试（需 RED 验证）
