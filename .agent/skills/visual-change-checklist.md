# Skill — 视觉/令牌改动清单（new-api web）

> 适用：改 `web/src/styles/*`、`components/ui/*`、新增视觉令牌、调主题预设。
> 契约来源：`计划书/视觉识别体系-反AI通用美学界面改造指南.md`（§2.4 判定口径、§3.3 令牌、§3.4 禁令）。

## 前置读取
1. `计划书/视觉识别体系-…md` 的 §3.3（令牌）与 §3.4（禁令 B1–B7）；
2. `web/AGENTS.md §3.10/§3.17`（样式 + 可用性契约）；
3. `.agent/memory/known-issues.md`（a11y/对比度历史坑）。

## 步骤
1. **先判断归属**：改的是**取值来源**（令牌）还是**结构**（布局/组件）。落地页只允许改取值来源。
2. **令牌派生原则**：新令牌必须由 `--foreground/--background/--primary` 经 `color-mix(in oklch)` 派生，
   这样 11 套预设 + 亮暗自动成立；**不要**在 `.dark` 里重复定义派生令牌。
3. **改完必须做三件事**：
   - `cd web && bun run typecheck && bunx oxlint -c .oxlintrc.json <改动文件>`；
   - 目标目录定向 `vitest`（不要为此跑全量，全量有已知红基线）；
   - **11 套预设 × 2 页 axe `color-contrast` 全扫**（脚本见下），任何预设 1 个节点都不许放过。
4. 视觉证据：`BASE=… PRESETS=default,anthropic,ink WIDTHS=1440,320 node web/e2e-local/visual-baseline.cjs`。

## 验收标准
- typecheck / lint / 定向测试 PASS；
- 11 预设 axe `color-contrast` = 0；
- 浏览器实测令牌生效（`getComputedStyle` 读 `--radius`/`--surface-1`/`--font-numeric`），不靠肉眼猜；
- 320px 无横向溢出，触控目标 ≥44px 不破。

## 回滚
令牌层改动集中在 `web/src/styles/theme.css`（+ `theme-presets.css` 单预设块），
`git revert <sha>` 或单独回退这两个文件即可；组件层新增 prop 一律默认值兼容旧观感。

## 常见坑（都踩过）
1. **后代选择器压过工具类**：`Table` 根的 `[&_th]:text-sm` 会盖掉 `<th>` 自己的 `text-[11px]`（specificity 更高）。
   改表头字号必须同时移除根上的后代覆盖。
2. **canvas 不吃 `color-mix()`**：图表序列色要用 `lib/chart-palette.ts`（canvas 2d 归一化 + sRGB 混合）。
3. **`--chart-1..5` 兼作 `StatusBadge` 文字色**：直接改这些令牌会打穿徽章对比度，须先做 11 预设扫描。
4. **品牌色达 AA**：陶土 `#d97757` 上白字只有 2.9:1；预设主色必须先算对比度再定亮度。
5. **主题预设是"整套调色板"**：它同时覆盖亮暗（CSS 级联在后），不要假设预设只作用于亮色。
