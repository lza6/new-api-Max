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

---

## 四、未闭环 / 待验证（如实）

- 多实例（跨副本）采集协调 —— 已文档化约束，**未实现**
- 浏览器端真实 E2E（需浏览器工具，本会话不可用）：训练数据页、运营面板、web 防护分页修复后的实际渲染
- 生产环境部署与线上验收
- HTTP 层 p95 压测对照（需真实上游渠道）
- Claude 计费两处疑似缺陷（需业主拍板）
- 用户可见的消耗排行榜 + 累计充值金额（**未实现**）
