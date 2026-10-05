# 需求追踪矩阵（Requirements Traceability Matrix）

> **用途**：把本项目全部显式/隐式需求映射到真实实现与证据，防止"说过了但没落实"。
> **状态口径**：✅ 已闭环（有可运行证据） ｜ 🟡 部分闭环（有实现但缺验证/缺边界） ｜ ⛔ 未闭环 ｜ ➖ 不适用
> **证据等级**：`实测`（真跑过）/ `静态`（读码确认）/ `推断`（间接证据）/ `待验`（缺环境）
> **维护纪律**：每批改动回填本表；状态只在有证据时升级。建立：2026-10-04。
> **关联**：`计划书/workflow_status.md`（批次台账）、`计划书/audit/perf-verification-ledger.md`（验证台账）、`.specify/memory/constitution.md`（宪法）。

---

## 0. 需求来源清单（本矩阵覆盖的上下文需求）

| 来源 | 内容 |
|---|---|
| S1 | 首轮任务提示词：响应式/性能/设计系统/UX/技术架构 7 段计划 |
| S2 | 用户确认："开始推进，完整落地闭环" |
| S3 | 终局审计提示词：需求闭环/功能闭环/调用闭环/工程闭环/质量闭环 |
| S4 | 主动补位清单 A–I（交互/API/后端/数据/衔接/配置/工程/测试/安全） |
| S5 | Spec Kit 工作流要求（Constitution→Specify→Clarify→Plan→Tasks→Analyze→Implement） |
| S6 | `project-delivery` 技能交付约束 |
| S7 | 用户指定：HTML 变更报告 + 测验、独立审查线程循环、工作流/skills 沉淀、Agent 管理 Agent 增长探索 |
| S8 | 项目宪法 `.specify/memory/constitution.md`（7 条核心原则） |

---

## 1. 显式需求追踪

### 1.1 首轮 7 段计划（S1）— 落地状态

| ID | 需求 | 实现位置 | 状态 | 证据 | 缺口/后续 |
|---|---|---|---|---|---|
| R1.1 | 断点策略（320/768/1024/1440+）| `web/src/hooks/use-mobile.ts`（768 分界）、Tailwind 默认断点、全站 1048 处响应式前缀 | ✅ | 静态：`rg '\b(sm\|md\|lg\|xl\|2xl):'` 1048 处/294 文件 | 无 |
| R1.2 | 移动优先（保留）| Tailwind 默认 mobile-first | ✅ | 静态 | 无 |
| R1.3 | `viewport-fit=cover`（刘海机 safe-area 前置）| `web/index.html:6` | ⛔ | 实测：`grep viewport index.html` 仅 `width=device-width, initial-scale=1.0` | **待修**：加 `viewport-fit=cover` |
| R1.4 | safe-area 系统化 | 仅 3 处零散（`common-log-mobile-card.tsx:333` 等）| 🟡 | 静态 | 待系统化到底部导航/抽屉 |
| R1.5 | 触控目标 ≥44×44 | `components/ui/button-touch-target.ts` 存在 | 🟡 | 静态 | 待系统审计 |
| R1.6 | 动态视口 svh/dvh/lvh | 已大量正确使用 | ✅ | 静态：`authenticated-layout.tsx:49`、`sidebar.tsx:253`、各 dialog `dvh` | 无 |
| R1.7 | 流体缩放 clamp | 仅 hero 标题 | 🟡 | 静态：`hero.tsx:178` | P3 增强 |
| R2.1 | LCP<2.5s / INP<100ms / CLS<0.1 | 无 Lighthouse 基线 | 🟡 | 待验 | **待建性能基线** |
| R2.2 | 60fps 动画（compositor 属性）| 已用 transform/opacity | ✅ | 静态：`index.css` 微交互 | 无 |
| R2.3 | 懒加载/代码分割 | WebGL lazy + 路由分割 + vendor 分包 | ✅ | 实测：`rsbuild.config.ts:29-82,139`、`webgl-hero.tsx` | 无 |
| R2.4 | 首屏预算门禁 | `bundle-budget.test.ts` | 🟡 | 静态：阈值宽松（index<5MB vs 实测 906KB）| **待收紧** |
| R2.5 | console.log 清理 | 4 文件残留 | ⛔ | 实测：`overview-dashboard`/`channels-primary-buttons`/`model-details-api`/`copy-to-clipboard` | **待清** |
| R3.1 | 设计 token（色彩/间距/排版/阴影/动效）| `theme.css` OKLCH 25+ 语义色 | ✅ | 实测 | 无 |
| R3.2 | 亮/暗双模式 | `.dark` 双份 token | ✅ | 实测 | 无 |
| R3.3 | 54 主题预设 | `theme-presets.css` | ✅ | 实测：54 个 `data-theme` | 无 |
| R3.4 | 语义阴影 5 级 | `--elevation-*` / `--shadow-*` | ✅ | 实测 | 无 |
| R4.1 | 层级与可扫描性 | 已用 | 🟡 | 静态 | 无 |
| R4.2 | 反馈/骨架/微交互 | 骨架 shimmer + 三态组件 | ✅ | 静态 | 见 R6.2 错误态缺口 |
| R4.3 | 响应式导航 | 顶栏 + 移动抽屉 | ✅ | 实测：`mobile-drawer.tsx` | 见 R1.10 底部导航 |
| R4.4 | WCAG 2.1 AA | 部分 | 🟡 | 见 R6 章节 | 对比度/断链待修 |
| R4.5 | 表单 UX | RHF+Zod | ✅ | 静态 | 无 |
| R4.6 | 动效 reduced-motion | 已覆盖 | ✅ | 实测：16 文件 | 无 |
| R4.7 | 空/错/权限态 | 见 R6.2 | 🟡 | — | DataTable 错误态缺口 |
| R5.1 | 技术栈升级 | 已最新，保持 | ✅ | 实测：package.json | 无 |
| R5.2 | 组件架构/目录 | feature-based | ✅ | 实测：32 feature | 无 |
| R5.3 | CSS 策略 | utility-first + token | ✅ | 静态 | 无 |
| R5.4 | 响应式测试策略 | `web/e2e-local/*.cjs` Playwright | 🟡 | 实测：6 脚本 | 缺 6 断点系统截图 |

### 1.2 用户追加需求（S2/S7）

| ID | 需求 | 状态 | 说明 |
|---|---|---|---|
| R2.6 | Phase 1 转化断点 4 项 + OnboardingGuide bug | ✅ | 本会话已落地，94/94 测试 + 11/11 E2E PASS（未提交）|
| R7.1 | HTML 变更报告 + 测验 | ⛔ | **待产出** |
| R7.2 | 独立审查线程循环 | 🟡 | 本会话已建 3 审计线程 |
| R7.3 | 工作流/skills 沉淀 | 🟡 | 已有 `project-delivery`/`speckit-*`；待补"新增 API/功能"工作流 |
| R7.4 | Agent 管理 Agent（增长/性能/安全探索）| ⛔ | **待启动** |
| R7.5 | 记录验证台账避免重复 | ✅ | `perf-verification-ledger.md` + 记忆台账已建 |
| R7.6 | SOP/交接文档 | 🟡 | `ops/*.md` 存在；缺 `OPERATIONS_SOP.md`（被技能引用但不存在）|

---

## 2. 隐式需求追踪（可运行/可调用/闭环/非伪实现）

| ID | 隐式需求 | 状态 | 证据 |
|---|---|---|---|
| I1 | 可构建 | ✅ | 后端 `go build ./...`、前端 `bun run build` |
| I2 | 可运行/可部署 | ✅ | 生产 v1.3.87 部署中 |
| I3 | 前后端真实打通 | 🟡 | 见 §3 contract-audit 待回填 |
| I4 | 无伪实现/占位冒充 | 🟡 | `playground/lib/input/input-tool-utils.ts` 曾全占位（另一会话改中）|
| I5 | 文档与行为一致 | 🟡 | `project-delivery` 引用的 traceability 矩阵缺失 → 本文件补 |
| I6 | 既有验证不重复跑 | ✅ | 台账已建 |

---

## 3. 主动补位清单追踪（S4：A–I）

| 维度 | 关键检查项 | 状态 | 证据 |
|---|---|---|---|
| A 用户交互 | 空/加载/错误态、防重复提交、危险确认、深链恢复 | 🟡 | DataTable 无错误态；`#content` 断链；其余见 fe-audit 待回填 |
| B API 调用 | 参数校验、错误码一致、分页协议、时间格式 | 🟡 | 见 contract-audit 待回填 |
| C 后端逻辑 | 主链路、边界、事务、并发、审计 | 🟡 | 见 be-audit 待回填 |
| D 数据层 | 迁移、三库、索引、缓存一致 | ✅ | `db_structure.md` + 三库 conformance 台账 |
| E 前后端衔接 | 字段/枚举/状态/错误对齐 | 🟡 | 见 contract-audit 待回填 |
| F 配置环境 | env 模板、默认值、硬编码 | 🟡 | CORS 已修（env 白名单）；见 be-audit |
| G 工程维护 | README、变更说明、排障 | 🟡 | 本轮补 traceability + 报告 |
| H 测试验证 | 冒烟/主路径/回归/E2E | 🟡 | 见各 audit |
| I 安全合规 | 权限、注入、脱敏、CORS、上传限制 | 🟡 | CORS 已修；见 be-audit |

---

## 4. 变更台账（本会话）

| 批次 | 内容 | 状态 | 证据 |
|---|---|---|---|
| P1 | Phase 1 新手转化 4 断点 + OnboardingGuide 冻结 bug | ✅ | 8 源文件 + 3 测试 + 1 E2E；`vitest 94/94`、`E2E 11/11`、`计划书/e2e-evidence/v1.3.85/`；**未提交** |
| P2 | a11y 修复：`viewport-fit=cover`（G1）+ `SkipToMain` 断链（G2，含公开页+登录页）| ✅ | `web/index.html`、`main.tsx`、`public-layout.tsx`、`auth-layout.tsx`；`bun run build` EXIT 0；`E2E v1.3.88 7/7`（首 Tab 焦点=Skip to Main、Enter 落 #content、6 断点无溢出）；**未提交** |

---

## 5. 缺口汇总（本矩阵当前识别，待审计线程回填）

### 已闭环
- ✅ **G1** `viewport-fit=cover`（`web/index.html`）—— E2E v1388 验证
- ✅ **G2** `SkipToMain` `#content` 断链（公开页 + 登录页）—— E2E v1388 首 Tab/Enter 落点验证
- ✅ **G3** DataTable 错误态（`DataTablePage` + 三渲染路径 + 10 张表 + 测试 + i18n）—— vitest 63/63 + 3/3
- ✅ **G4** 真实运行时 console.log 0（删 channels-primary-buttons 2 处；其余为示例字符串）
- ✅ **G6** `requirements-traceability-matrix.md` 已建

### 仍待办
- **G5** 首屏预算门禁收紧（`bundle-budget.test.ts` 阈值宽）—— P2
- **G7** `OPERATIONS_SOP.md` 缺失（技能引用）—— P2（已有 `ops/deployment-sop.md` 等分散文档）
- **R7.1** HTML 变更报告 + 测验 —— ✅ 已产出 `计划书/change-report-2026-10-04-frontend-closure.html`
- **R7.2** 独立审查线程 —— ✅ 已派 `final-review`（循环中）
- **R7.3** 工作流/skills 沉淀 —— ✅ `new-api-add-feature` 技能已补 §11 审计编排纪律
- **R7.4** Agent 管理 Agent 增长探索 —— ⛔ 未启动（需用户确认方向）
- **R7.6** `OPERATIONS_SOP.md` —— 同 G7

### P3（一致性，不阻塞）
- 前端分页参数拼写不统一（keys `size` vs redemptions `page_size`；后端 `GetPageQuery` 两者都认）

---

*最后更新：2026-10-04 · 维护者：审计线程 + 主线程*

---

## 6. 审计线程发现（2026-10-04，4 线程）

> 详细逐条在 `计划书/workflow_status.md §11.1`。此处只列**未修复**项（待用户授权/下一轮）。

### 后端（be-audit，最高优先）
- ✅ **P0** 配置热更新 map 并发 fatal —— billing_setting 快照替换 + 反向验证；同类 11 包批量加固（snapshot-guard 线程）
- ✅ **P1** 缓存渠道共享指针 `Status` 锁外读 race —— 新增 `CacheGetChannelStatus` 锁保护访问器 + 反向验证
- ✅ **P2** SSRF 黑名单缺 CGNAT —— 对齐 common 清单（含 `::ffff:0:0/96` 折叠陷阱规避）
- ✅ **P2** 5 个 cleanup loop 未纳入优雅关闭 —— 改用 backgroundLoop + 接入 main.go 停机序列

### 契约（contract-audit）
- ✅ **P1** 渠道 key 揭示契约相反 —— 前端改「先试后补 proof」+ 共享 helper + 测试重写（4/4）
- ⏸️ **P1** CORS 收紧 —— 部署前确认生产 `CORS_ALLOWED_ORIGINS`（非代码改动，运维项）
- ⏸️ **P2** channel key 404 语义 —— 评估后保留既有契约（管理端业务错误）
- ✅ **P2** `GetTokenStatus` 死代码 —— 已核实零引用并删除（`go build`/`vet` 0 错）

### 前端（fe-audit）
- ✅ P0-1 首屏预算门禁、P1-1 15 缺失 i18n 键
- ✅ P1-2 axe 对比度假绿 → 真实浏览器对比度 E2E 抓出真实违规（`--primary` 2.71→5.45 含动画稀释根因）+ footer/营销页透明度 → **contrast 门禁 3/3 稳定全绿**
- ✅ P2-1 ai-elements 死码（22 文件已删，在用 9 + response-renderer 保留）
- ✅ P2-2 3 面板门控轮询（`useDocumentVisible`）
- ✅ P2-4 ConfirmDialog spinner、P2-5 Playground 删除确认、P2-7 日历 aria
- ✅ P2-6 footer 低透明度文本（1.81/1.98/2.61 → 5.97）
- ➖ P2-3 订阅表 URL 持久化 —— **核实后不适用**（70 行客户端小表，无可持久化状态，审计误报）
- ➖ P2-8 虚拟滚动 —— **暂缓**（pageSize 有界 100，重构风险 > P3 收益）
- ⏸️ 新发现：11 个零引用死文件（layout/home 改版遗留）—— 超本次清单，待用户裁决

---

*最终更新：2026-10-04（P3 全清单处理完毕：3 项已闭环、2 项经核实不适用/暂缓）*

*审计轮次更新：2026-10-04（第二轮：全部后端 P0/P1 + 契约 P1 已闭环）*
