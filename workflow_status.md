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
| T7-4 | 模型广场按分组计费表达式显示为源码（用户看不懂） | 用户报告 | L1 | **DONE** |
| T7-5 | 模型广场「套餐对比」入口移除 | 用户要求 | L1 | **DONE** |
| T7-6 | 修伪闭环 T1：反事实节省基准前端零消费 | 自查 | L1 | **DONE** |
| T7-7 | P1-1 记忆层：补齐 MemoryInjection 的**用户配置界面** | 指南 §6 | L2 | **DONE** |
| T7-8 | P1-2 per-agent 能力下沉（Agent 预设实体） | 指南 §6 | L2 | TODO |
| T7-9 | P1-5 Skills 注入 Hook v1 | 指南 §6 | L2 | TODO |
| T7-10 | P1-6 游乐场内嵌图片/视频生成（图片已有，视频缺） | 指南 §6 | L2 | TODO |
| T7-11 | P2-6 PPT/文档生成 | 指南 §6 | L2 | TODO |
| T7-12 | P0-1 渠道密钥加密：**默认关**（L3，需单独审批才可开） | 指南 §6 | L3 | 已实现待审批 |

## 本轮交付（commit 序列）

| 版本 | commit | 内容 |
|---|---|---|
| v1.3.117 | `a9c3bc2b2` | T4/T7/T10 三个死代码模块接线（后端） |
| v1.3.117 | `ec2abaa5c` | 模型广场计费显示修复 + 移除套餐对比 + T4/T7 结果前端可见 |
| v1.3.118 | `bde441e19` | 模型详情页/成本明细同样不再展示表达式源码 |
| v1.3.119 | `841a43300` | 记忆注入的用户配置界面（补齐 T8 另一半） |
| v1.3.120 | `9cd9ffbe4` | 反事实节省基准的前端消费（补齐 T1 另一半） |

**共性**：本轮主要在修「已宣称完成、实际用户/链路用不到」的伪闭环
（T4/T7/T10 死代码、T8 只有注入没有配置、T1 只有后端没有前端）。


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
