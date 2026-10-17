# 验证台账（Audit Ledger）

> 目的：**避免重复跑同一验证**。改到某区域前先查本文件，确认哪些已跑过、结论是什么、
> 什么条件下才需要重跑。只记事实与证据，不记推测。
> 维护规则：每次做完一轮审查/压测/契约验证，追加一行；改动相关区域时先读这里。

**最后更新**：2026-10-17

---

## 一、已完成的审查（勿重复跑）

| 日期 | 审查对象 | 方法 | 结论 | 重跑条件 |
|---|---|---|---|---|
| 2026-10-17 | **运营面板 `/ops-panel` 全链路** | 只读静态审计（代码 + 路由契约测试 + 前端组件） | **链路完整贯通**：页面→路由→导航入口齐备；6 项指标全部有后端支撑（含「排除管理员后剩余额度」「预估可运营天数」）；一键加额度真写库（`AdjustUserQuota` + 行锁 + 审计日志 + `LogTypeTopup`）；无 mock/假数据/空按钮 | 改动 `controller/ops_panel.go`、`web/src/features/ops-panel/`、或 `model/user_quota_adjustment.go` |
| 2026-10-17 | **Web 防护页 `/web-protection`** | 只读静态审计 | **发现真实缺陷**：表格分页控件渲染为 null（根因见下）——已修 | 改动 `web-protection-page.tsx` 或 `data-table` 的 page-footer 机制 |
| 2026-10-17 | **用户消耗排名诉求** | 只读静态审计 | **未闭环**：用户维度消耗排名仅存在于管理员界面；「累计充值金额」前后端**均无实现**（零命中） | 实现后更新本行 |
| 2026-10-17 | **占位/死链路扫描（ops-panel + web-protection）** | grep `mock\|fake\|dummy\|hardcod\|TODO\|FIXME\|sample` | 零命中 | — |
| 2026-10-17 | **分页控件不可见排查（全 feature 首页）** | 逐页比对是否套 `SectionPageLayout` | 只有 web-protection 与 training-data 两个**页级**表格受影响（均已修）；`pricing` 用客户端分页且显式渲染 `DataTablePagination`、`system-settings` 的表格在对话框内 —— 两者**不需要**该布局 | 新增页级分页表格时按此判据自查 |
| 2026-10-17 | **回复采集的下游集成（分析链路）** | 代码审计 + RED 测试 | **发现真实缺陷**：`AssembleConversation` 丢 `role` → 分析提示词要求「引用用户原话」却分不清用户/模型 → 模型会把自己的回复当用户原话。已修（新增 `AnnotatedText`） | 改动 `service/training_capture/conversation.go` 或分析提示词 |
| 2026-10-17 | **训练数据管理端全链路**（前端 ↔ 路由 ↔ controller） | 只读静态审计 + 契约测试比对 | **发现 2 处真实缺陷**：① 深度分析成功路径必抛错（判 `!res.success`，而后端返回原始 relay 体无 `success`）→ 分析/报告/追问全不可用；② `CountItems(ctx,3)` 深度不足致文件数恒为 0。均已修并 RED 验证 | 改动 `training_data_analyze.go`、`training-data/api.ts`、`CountItems` 调用处 |
| 2026-10-17 | **用户侧采集开关链路** | 只读静态审计 | 主流程已闭环（授权/撤回/免责声明/不可用隐藏均正确）；**发现 2 处 P2 静默失败**（读取失败无提示、无加载态）已修 | 改动 `training-data-preferences-card.tsx` |

### 关键的「根因 vs 表象」（避免误判）
- **「web 防护没有翻页」**：不是后端缺分页（后端三张表全部支持 `page/size` 且返回 `total`），
  也不是前端没用分页组件。根因是 `WebProtectionPage` 未套 `SectionPageLayout`
  → `PageFooterPortal` 无 Provider → 直接 `return null`。
  见 `web/src/components/layout/components/page-footer.tsx` 与 `section-page-layout.tsx`。
  **教训：分页不可见先查布局容器，不要先去改后端。**

---

## 二、既有测试失败噪声表（勿误判为回归）

| 范围 | 现象 | 判定 |
|---|---|---|
| `go test ./controller/` 全量 | 约 600s 后超时失败，栈在 `pkg/jsplugin` / `grafana/sobek`（`task_plugin_test.go`） | **非回归**：jsplugin 用例耗时长致整包超时；单跑该用例通过。改到 controller 时用 `-run` 缩小范围 |
| `web/` 的 `e2e-local/*.cjs` | `bun run lint` 报 `no-require-imports` 等 | **非回归**：未跟踪的临时调试脚本，不在交付范围 |

---

## 三、已 PASS 的验证（可引用，不必重跑）

| 范围 | 命令 | 结论 |
|---|---|---|
| 训练数据采集（后端） | `go test ./service/training_capture/` | 全 PASS |
| 同上 + 竞态 | `go test -race ./service/training_capture/ -run 'TestProcessSignal\|TestResplit\|…'` | PASS |
| 训练数据（controller） | `go test ./controller/ -run 'TrainingData\|ResponseCapture\|E2ECapture\|Extract'` | PASS |
| 运营面板口径 | `go test ./controller/ -run 'TestOpsRanking'` | 2/2 PASS（含 500+ 管理员回退路径） |
| 前端类型 | `cd web && bun run typecheck`（tsgo -b） | PASS |
| i18n 一致性 | `cd web && npm run i18n:check` | 0 missing |
| 路由契约 | `go test ./router/ -run 'TrainingDataRouter'` | PASS（会扫描前端源码里的 `/api/` 字面量与 gin 路由表比对） |
| Web 防护页 | `npx vitest run src/features/web-protection/` | 2/2 PASS |
| 分析接口契约 | `npx vitest run src/features/training-data/api.test.ts` | 3/3 PASS（锁定原始 relay 体，不做信封解包） |
| 每日采集聚合 | `go test ./service/training_capture/ -run TestAggregateDailyStats` | 5/5 PASS |
| 目录计数深度语义 | `go test ./service/training_capture/ -run TestCountTree` | 6/6 PASS（含 maxDepth=1..9 穷举） |
| 日志分片轮转 | `go test ./logger/` | 全 PASS（6.3 实际早已落地） |
| 管理端云盘查询缓存 | `go test ./controller/ -run TestTimedCache` | 5/5 PASS（含 16 并发合并为 1 次） |
| HTML 报告测验 | Node DOM stub 模拟作答 | 全对→pass、全错→fail、漏答→fail（判分逻辑已实证） |

---

## 四、未闭环 / 待验证（如实）

- 多实例（跨副本）采集协调 —— 已文档化约束，**未实现**
- 浏览器端真实 E2E（需浏览器工具，本会话不可用）：训练数据页、运营面板、web 防护分页修复后的实际渲染
- 生产环境部署与线上验收
- HTTP 层 p95 压测对照（需真实上游渠道）
- Claude 计费两处疑似缺陷（需业主拍板）
- 用户可见的消耗排行榜 + 累计充值金额（**未实现**）

---

## 五、2026-10-17 第六轮：改进指南全量审计与本轮落地

### 审计范围
`计划书/下一步改进指南.md`（v1.3.88 蓝图，Batch-0..5）、`TODO_seo-optimization.md`、
`计划书/seo-优化行动计划.md`、`batch2/5-status`、`playground-rework-status`、
`MCP-Gateway-设计落档`、两份 `workflow_status.md`、`参考的结果计划指南.md`、
`.specify/specs/001-008`、`specs/001-003`。

### 本轮真实落地（均有代码 + 测试）
| 项 | 证据 |
|---|---|
| 登录/注册页 noindex | `controller/spa_seo.go` `spaNoIndexPath`；`TestSPASEOAuthEntryPagesAreNoIndex` |
| Organization + BreadcrumbList | `spaJSONLDScript`；`TestSPASEOHomeDeclaresOrganization` / `TestSPASEOPublicPagesDeclareBreadcrumb` |
| hreflang + 语言路径前缀 | `/en/...`、`/zh-TW/...`；canonical 带前缀；`TestSPASEOLocalePrefixEnglish` / `TestGetSitemapXmlIncludesLocaleAlternates`；前端 `routeLocaleBasepath` + `pathWithLanguage`（5 用例） |
| 创建 Key 后 curl 示例 | `api-keys-mutate-drawer.tsx` `created-key-curl`；vitest 3/3 |
| 渠道密钥加密接线 | `main.go` 启动调用 `AssertChannelKeyEncryptionState` + `MigrateChannelKeyEncryption`；README 启用步骤 |
| 续费提醒 3 天/1 天双阈值 | `subscription_reset_task.go` `parseSubscriptionExpiryReminderDays` + `GetDueExpiryReminderSubscriptionsBetween`；新增 3 用例 |
| OpenAI 兼容端点 | `/v1/files`、`/v1/fine-tunes`、`/v1/batches` 改为真实 relay；`OPENAI_OPS_DEFAULT_MODEL` / `OPENAI_OPS_FLAT_QUOTA`；`OpenAIOpsIsPaid` / `OpenAIOpsPriceData` + 请求解析测试 |
| G4 路由 enforce | `controller/relay.go` 在预扣费前应用 `ApplyEnforceGuard`；候选校验含令牌白名单 + 订阅矩阵；价格不可用时回退原模型 |
| G5-3 动作级审批 | `model/action_approval*`、`service/action_registry.go`、管理端 API + `/action-approvals` 页面；一次批准只授权一次调用（CAS） |
| MCP Gateway | `POST /mcp`（JSON-RPC 2.0 / Streamable HTTP）；会话落库跨实例；工具/资源；SSE progress；付费工具逐次审批；开关默认关 |
| a11y/响应式 | 真实 Playwright 7/7 PASS（SkipToMain、6 断点无横向溢出、暗色无控制台错误） |

### 审计判定（保留结论）
- 主指南 Batch-0/1/2/3/4 的绝大多数项已由 v1.3.89–v1.3.147 落地；本轮补齐的是
  SEO 收尾、curl 示例、密钥加密接线/文档、双阈值提醒、OpenAI 运维端点、
  G4 enforce、G5-3 审批、MCP。
- **仍未实施（非伪闭环，明确记录）**：订阅首购优惠、订阅省费可视化、
  真实渠道冗余（需上游）、Claude 计费两处（需业主拍板）、多实例采集协调、
  生产 p95/浏览器验收。
- **hreflang 只声明 zh-CN/en**：服务端页面文案目前只有中英两份；fr/ru/ja/vi
  可通过路径前缀访问（UI 切换），但补齐服务端翻译前不声明 hreflang，避免错误标注。

### 本轮实际运行验证
```bash
go build ./...                                   # PASS
cd relaykit && GOWORK=off go build ./...         # PASS
go vet ./controller/ ./relay/ ./relay/helper/ ./model/ ./service/ ./middleware/ ./router/  # PASS
go test ./model/ -run 'TestDBConformanceAutoMigrateIdempotent' -count=1   # SQLite PASS（MySQL/PG 未跑）
go test ./model/ -run 'ActionApproval|McpSession|ChannelKey' -count=1     # PASS
go test ./service/ -run 'ActionRegistry|ExpiryReminder' -count=1          # PASS
go test ./relay/ ./relay/helper/ -run 'OpenAIOps' -count=1                # PASS
go test ./controller/ -run 'TestMcp|TestSPASEO|TestGetSitemap|TestGetRobots' -count=1  # PASS
go test ./router/ -count=1                                                # PASS
cd web && bun run typecheck                                               # PASS
cd web && npx vitest run src/i18n/__tests__/route-locale.test.ts src/features/keys/components/__tests__/api-keys-mutate-drawer.test.tsx  # 8/8 PASS
cd web && bun run i18n:check                                              # 0 missing
node e2e-local/v1388-a11y-responsive.cjs                                  # 7/7 PASS（真实 Playwright）
```

### 验证缺口（如实）
- **MySQL / PostgreSQL conformance 未跑**：本机无 Docker/MySQL/PG（与既有台账一致）；
  新表 `ActionApproval` / `McpSession` 只用标准列类型，已加入 `conformanceModels()`，
  下次在有三种数据库的环境执行 `TestDBConformanceAutoMigrateIdempotent` 即可覆盖。
- OpenAI 运维端点 / MCP 的**真实上游转发**只做了 fake-upstream 单测，未在真实渠道上跑通
  fine-tune/batch 创建；需有真实 OpenAI 兼容渠道时补 E2E。
- MCP 的 SSE progress 已实现，但未用真实 MCP 客户端（Claude Code/Cursor）做握手验收。
- 生产部署与线上验收未执行（按部署纪律需用户明确授权）。

### 用户本轮决策（已记录，后续不要重复问）
- 渠道密钥加密：**保持关闭**，只保留代码 + README 启用步骤；生产 `CHANNEL_KEY_ENCRYPTION=false`。
- hreflang：采用路径前缀（`/en/...`、`/zh-TW/...`）；默认 zh-CN 无前缀；
  服务端暂无 fr/ru/ja/vi 文案，因此只为 zh-CN/en 声明 hreflang。
- 运营者信息：只写站点名与站点 URL，不编造个人/公司资质。
- 法务页：开放 `/user-agreement`、`/privacy-policy`（正文来自后台配置，已假定已填写）。
- 大项：OpenAI 运维端点、MCP Gateway（完整版）、G4 enforce + G5-3 审批已落地；
  订阅首购优惠、省费可视化、渠道冗余未纳入本轮（不伪造实现）。

### 文档清理
按用户指示删除已完成的施工蓝图/状态/检查表（清单见 git 变更）；保留
`计划书/ops/`（SOP/ADR/黄金范例）、`计划书/audit-ledger.md`、
`计划书/e2e-evidence/`（验收证据）与 `计划书/audit/`（历史审查报告）。
同时清理了本地残留：`bin/new-api-*.exe`（6 个）、`one-api.db`、`e2e-v1365.db-journal`。

---

## 六、2026-10-17 独立审查（Go / 前端 / 安全）与修复

### 审查结论（Verdict）
- Go：Request Changes（2 个 P1）
- 前端：Request Changes（4 个 P1）
- 安全：BLOCK（1 个 Critical + 2 个 High）

### 已修复（均有代码 + 回归测试）
| # | 问题 | 修法 | 验证 |
|---|---|---|---|
| C1 | `POST /v1/fine%2Dtunes` 百分号编码绕过计费 | 付费判定改用解码后的 `c.Request.URL.Path`；新增 ops 路径白名单 | `TestOpenAIOpsPaidUsesDecodedPath` / `TestOpenAIOpsPathSupported`；本地编码路径实测走到付费/退款路径 |
| R1 | `/zh//evil.com` 开放重定向 | `DefaultLocaleRedirectTarget` 拒绝 `//`、`/\`，回落到 `/` | `TestSPASEODefaultLocaleRedirectRejectsProtocolRelative` |
| R2 | `/v1/files|fine-tunes|batches/:id` 跨用户 IDOR | 新增 `openai_ops_objects` 归属表；创建登记、列表过滤、单对象/删除/取消校验归属 | `TestOpenAIOpsObjectOwnership` / `TestFilterOwnedOpsList...` / 本地 E2E |
| P1 | 审批未绑定请求参数 | `create_image` 审批键 = `model+n+size+sha256(args)` | 代码 + MCP E2E（换参数会得到新审批） |
| P1 | ops 请求体被 `Bytes()` 多次物化，可致 OOM | 只读 64KB 前缀提取 model；handler 用 body storage 流式转发 | 代码审查 + 构建；multipart 压缩已跳过 |
| P1 | 前端审批失败仍提示成功 / 列表失败显示为空 | 全部改用 `requireServerSuccess`，错误走 `handleServerError` | `action-approvals/__tests__/api.test.ts` 3/3 |
| P1 | 无前缀公开页与 localStorage 语言冲突 | 首次进入公开页时跳转到对应语言前缀 | 前端代码 + E2E `/en/pricing` |
| P1 | fr/ru/ja/vi 前缀内容英文却声明对应 lang | 服务端 `lang=en` + 对这些前缀 `noindex` | `TestSPASEODoesNotAdvertiseUntranslatedHreflang` |
| S5 | 无持久 `CRYPTO_SECRET` 仍迁移 / 可关闭加密 | 迁移前强制显式 `CRYPTO_SECRET`；存在密文时拒绝关闭开关 | 代码 |
| S2/S3/S4 | MCP 会话膨胀 / DELETE 越权 / 内部地址可配远程 | 会话上限 20 + `TokenRateLimit`；DELETE 校验归属；内部地址强制回环（可显式放行） | 代码 |
| S6 | `ReplaceAllString` 的 `$` 展开破坏 canonical | 动态替换串 `$` → `$$` | `TestSPASEOReplacementDollarIsLiteral` |

### 未关闭（P2/P3，如实）
- `ALPHA_SEARCH_ENABLED` 只控制 MCP 是否列出 `web_search`，不是 `/v1/alpha/search` 的全站门控。
- `Limit(n).Update/Delete` 在 PostgreSQL 的行为未跑三库验证。
- 无前缀 URL 在 `Accept-Language: en` 下仍输出英文内容 + 无前缀 canonical（SEO 口径取舍，已记录）。
- 审批管理接口仍只展示 action/resource；resource 已含 model/n/size/参数哈希，但无独立 payload 预览 UI。

### 复验后追加硬化（v1.3.149）
- 审批 resource 哈希前置，避免 191 字节截断丢失参数绑定。
- ops 列表响应 JSON 解析失败时 fail-closed（返回空 data），不再原样透传。
- list/create 响应超过 8MB 时明确 502，不返回被截断的 JSON。
- 三位独立审查员复验结论：Go **Approve**、前端 **Approve**、安全 **Approve**。
