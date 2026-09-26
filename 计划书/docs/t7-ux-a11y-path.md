# 专项分析 · 前端 UI/UX 与可访问性增强路径（T7 纵深）

> 定位：主指南 §T7 的**深挖文档**：现状、缺口、落地路径与验收，只读整理（未改业务代码）。
> 生成：2026-09-25 · 锚点：`web/AGENTS.md`、`计划书/audit/ux-interaction-ledger.md`（旧版）、web/package.json。

## 1. 现状（证据）
- 前端栈：React 19 + TypeScript + Rsbuild 2 + TanStack Router/Query/Table + Zustand + Base UI + Tailwind 4 + i18next（en/zh/zh-TW/fr/ru/ja/vi 7 语言）。
- 既有规范：`web/AGENTS.md` 强制错误处理 `handleServerError`/`getFriendlyErrorMessage`、加载/空态要求、touch target 测试 `button-touch-target.test.tsx`。
- 组件库：`web/src/components/` 共享组件 + `web/src/features/` 31 功能域；vitest + @testing-library + vitest-axe + axe-core 已依赖。

## 2. 缺口与优先级（对照主指南 §T7）
| 缺口 | 现状 | 落地 | 优先级 |
|---|---|---|---|
| 交互反馈盘点表刷新 | 旧版 `ux-interaction-ledger.md` 为 40+ 功能域；现行 31 features | 重新盘点：操作→加载/成功/失败/空态反馈→证据行号→优先级 | P1 |
| a11y 自动化 | vitest-axe/axe-core 已依赖，未全面启用 | 关键页面（登录/渠道/日志/钱包/任务插件）axe 用例 + 修复高危项 | P1 |
| 移动端三断点 | 无截图证据 | Playwright 375/768/1280 截图入 e2e-evidence/browser-e2e-ux/ | P1 |
| 人话错误映射 | `web/src/lib/server-error-message.ts` 已存在 | 补 429/502/504/额度用尽/渠道无可用/订阅过期 + i18n 7 语言 + 测试 | P1 |
| 交互反馈细节 | 部分组件有 loading/空态，未全覆盖 | 逐 feature 核对（按钮禁用态、表单错误内联、空态 CTA） | P2 |

## 3. 落地路径（最小改动）
1. **盘点表**：脚本/手写生成 `计划书/audit/ux-interaction-ledger.md` 新版，覆盖当前 31 features；每条含文件:行号证据；缺口标 P 级。
2. **a11y**：`web/src/features/{auth,channels,usage-logs,wallet,task-plugins}` 各加 1 个 vitest-axe 用例（render → axe() → 断言无 violations）；修复颜色对比/焦点/aria 高危项；touch target 测试沿用。
3. **移动端**：Playwright 配置三 viewport，对登录/首页/渠道/日志/钱包截图；检查弹窗/抽屉/表格横向滚动可操作；输出到 `计划书/e2e-evidence/browser-e2e-ux/`。
4. **错误映射**：`server-error-message.ts` 增映射 + `friendly-error-mapping.test.ts`（现有风格）；i18n:sync 后 7 语言补齐。

## 4. 验收与纪律
- `cd web && bun run typecheck && bunx vitest run && bun run build` 全绿；新增测试覆盖缺口。
- 前端改动全部走 i18n（t('English key')），不硬编码中文；共享组件复用优先（web/AGENTS.md 强制）。
- 浏览器证据与组件测试分开标注；未跑真实浏览器处标「待浏览器实测」。

## 5. 建议立即做（低成本高价值）
- `bunx vitest run src/features/auth` 现有测试是否含 axe；补登录页 axe 用例（改动最小）。
- `bun run build` 观察 dist chunk（对照 `计划书/audit/bundle-size.md` 基线 59,352 kB JS）。

## 闭环状态（2026-09-27，v1.3.44 回填）
- ✅ v1.3.40 已闭环：a11y-smoke（axe-core 真实组件）、browser-e2e-ux 375/768/1280 三断点 9 截图、friendly-error-mapping 7/7（429/502/504/额度用尽/渠道无可用/订阅过期）。
- 交互反馈盘点表 ux-interaction-ledger.md 已覆盖现行 31 features。
