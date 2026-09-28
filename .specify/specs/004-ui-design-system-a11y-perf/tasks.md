# Tasks：UI 设计系统 / a11y 与移动端 / 性能预算回归（004）

**Prerequisites**: spec.md（004）、plan.md
**组织方式**：按验收 US 拆分，3 个 Builder 并行（已启动），主控负责截图/审查/交付。

## US-4.2.3 — 设计系统 token 化与交互态（Builder A）
- [ ] **[A-1]** `web/src/styles/theme.css`：新增语义阴影 tokens —— `--shadow-card` / `--shadow-raised` / `--shadow-drawer` / `--shadow-popover` / `--shadow-overlay`（:root 与 .dark 双套，oklch + color-mix），并在 `@theme inline` 映射为 Tailwind token。文件：theme.css。
- [ ] **[A-2]** `model-card.tsx`：Card `shadow-card`，hover 提级 `hover:shadow-raised`（保留现有 hover:ring），补 `focus-visible:outline-2 focus-visible:outline-ring focus-visible:outline-offset-2`。文件：pricing/components/model-card.tsx。
- [ ] **[A-3]** `pricing-table.tsx`：行 hover 保留 + 焦点行 `focus-visible` ring。文件：pricing-table.tsx。
- [ ] **[A-4]** `site-subscription-stats-card.tsx`：`shadow-sm` → `shadow-card`；统计值无焦点环必要（非交互），仅确认对比度。文件：site-subscription-stats-card.tsx。
- [ ] **[A-5]** `search-bar.tsx`：确认 focus 态完整（`focus-visible` ring + border-primary）。文件：search-bar.tsx。
- [ ] **[A-6]** `model-details.tsx`（抽屉）：SheetContent 视觉层级加 `shadow-drawer`（不改 sheet 组件结构）。文件：model-details.tsx。
- [ ] **[A-7]** 对照 `rules/web/design-quality.md` 反模板清单逐项自查结论。交付物：改动清单 + 验证输出。

## US-4.2.4 — a11y 用例与断点截图（Builder B + 主控）
- [x] **[B-1]** `webhook/__tests__/a11y-webhook-settings.test.tsx`：mock api.get，render 页，axe 0 违规；断言表单控件 label 可达。文件：webhook/__tests__/a11y-webhook-settings.test.tsx。
- [x] **[B-2]** `keys/components/__tests__/a11y-keys-stepup.test.tsx`：mock 依赖，触发 step-up 弹窗，弹窗内 axe 0 违规；`getByRole('dialog')` 可定位。文件：keys/components/__tests__/a11y-keys-stepup.test.tsx。
- [x] **[B-3]** `pricing/components/__tests__/a11y-stats-card.test.tsx`：mock `/v1/stats/subscriptions`，render 统计卡，axe 0 违规；断言统计值渲染。文件：pricing/components/__tests__/a11y-stats-card.test.tsx。
- [x] **[B-4]** axe 发现真实违规 → 修复生产组件后复测（实测 0 违规，无需改生产组件；仅修测试自身 2 处相对导入 + 1 处未用导入）。
- [x] **[B-5]** 3 个 axe 用例本地实跑（`bunx vitest run <files>`）全绿 + typecheck（3 passed / TYPECHECK_EXIT=0 / LINT_EXIT=0）。
- [x] **[M-1 主控]** 375/768/1280 断点截图入 `计划书/e2e-evidence/browser-e2e-v1.3.56-a11y/`（9 张 PNG + summary.json；pricing/webhook/keys-stepup 三页 × 三断点，DOM 断言 18/18，无横向溢出 9/9，色块丰富非空白）。

## US-4.2.5 — 性能预算与死码门禁（Builder C）
- [ ] **[C-1]** `web/scripts/knip-gate.mjs`：跑 `bunx knip --reporter json`（先确认结构），解析为问题 key 集合，与基线 `knip-baseline.json` 对比，新增 → exit 1；`--update` 写基线。文件：scripts/knip-gate.mjs、scripts/knip-baseline.json。
- [ ] **[C-2]** 探针正反例实测：加未使用导出 → exit 1；删除 → exit 0。
- [ ] **[C-3]** `bundle-budget.test.ts`：补"路由/页面级异步 chunk 有独立文件"断言（dist 缺失仍跳过）。文件：lib/__tests__/bundle-budget.test.ts。
- [ ] **[C-4]** `.github/workflows/ci.yml` frontend job 加 knip-gate 步骤。
- [ ] **[C-5]** 新页面路由懒加载复核（routes/ + routeTree.gen.ts）：已懒记录证据；未懒补 `React.lazy`/动态 import。

## 质量门清单（全部通过才标记 done）
- [x] `bun run typecheck` exit 0
- [x] oxlint 改动文件无 error（含审查修复后复验）
- [x] 3 个 axe 用例 + bundle-budget vitest 绿（axe 3/3、budget 5/5）
- [x] knip-gate 探针正反例通过（+探针→exit1 / 删探针→exit0）
- [x] 截图落盘 3 断点（18/18，9 PNG + summary.json）
- [x] 独立审查 3 轮收敛（Critic a16b51df60c0360bf：REQUEST CHANGES → 主控修复 P1-1/P2-1/P2-2/P2-3 → 复验全绿）
- [ ] 主题 commit + push + tag v1.3.56
- [ ] HTML 变更报告 + 测验（用户要求）

## 审查修复记录（2026-09-28，Critic 第 1 轮后）
- **P1-1**（bundle-budget `index!` lint error 击穿 CI）：改为 `index?.sizeBytes ?? 0` + 前置 toBeTruthy；三处 regex 换 `startsWith`。复验 lint exit 0。
- **P1-2**（断点截图证据为 FAIL 产物）：已在主线程重跑 18/18 PASS（服务 3000 + setup root + token），summary.json 已为通过版。已闭环。
- **P2-1**（theme.css 阴影同名自引用自环）：运行时变量改 `--elevation-*`（与 `--color-*` 异名间接一致）。复验构建产物 utility 生成、light/dark 双值、无自环残留。
- **P2-2**（pricing-table 可聚焦行语义缺口）：补 `aria-label`（`${model} · Details`）+ Space 键 + `e.target !== currentTarget` 守卫。
- **P2-3**（code-split 断言过弱）：加 `async/` 子目录路由 chunk 下限断言（`path.dirname === 'async'`，跨平台）。复验 5/5。
- P3-1/P3-2 记录为后续增强，不阻塞。
