# 专项分析 · 前端包体积与工程化（T9 纵深）

> 定位：主指南 §T9 的**深挖文档**：基线、拆包路径、复用审计与性能预算；只读整理。
> 生成：2026-09-25 · 锚点：`web/package.json`、`计划书/audit/bundle-size.md`、`web/AGENTS.md`。

## 1. 现状（证据）
- 栈：React 19 + Rsbuild 2 + TanStack Router/Table/Query + Zustand + Base UI + Tailwind 4；图表 recharts/vchart 双依赖。
- 基线：`计划书/audit/bundle-size.md` 记录 0002/0003（历史 JS 59,352 kB；异步 cacheGroup 已落地）；入口历史 4.3MB。
- features 已从 40 → 31（主指南基线）。

## 2. 缺口（主指南 §T9）
- E2：入口懒加载瘦身（高权重编辑器/图表/模型卡片按路由 `React.lazy`）。
- E3：组件复用审计（收敛重复实现，不重构正常代码）。
- 性能预算回归：轻量测试断言关键页面资源请求数上限（CI 可跑）。

## 3. 落地路径（建议）
1. **基线刷新**：`cd web && rm -rf dist && bun run build`；统计各 chunk（Top 10）；回填 bundle-size.md 新记录。
2. **懒加载**：对 Codemirror、recharts/vchart、可视化/模型卡片按需引入；验证 index.js 体积下降 + 首屏路由不回归（Playwright 冒烟）。
3. **复用审计**：盘点 `web/src/components` 与 `features` 重复；只收敛明确重复项（不重构正常代码）；`bunx knip` 无新增 dead code。
4. **性能预算**：轻量测试（Playwright 或 vitest）断言关键页面资源请求数上限；CI 接入。

## 4. 纪律
- 复用优先：新 UI 先查 `web/src/components/` 与既有 feature（web/AGENTS.md 强制）再动手，不重复造组件。
- 包改动（依赖/构建配置）跑 `bun run typecheck && bunx vitest run && bun run build` 全绿。
- 体积数字必须真实（构建输出），不推断；回填 bundle-size.md。

## 5. 验证
- `cd web && bun install && bun run i18n:sync && bun run typecheck && bun run build && bunx vitest run && bunx knip`
- `ls -la dist` / rsbuild 输出的 chunk 清单记录。

## 闭环状态（2026-09-27，v1.3.44 回填）
- ✅ 基线刷新：bundle-size.md 记录 0004（v1.3.41 实测 Total 59,408.8 kB / index 4,419.7 kB）。
- ✅ 性能预算回归测试：web/src/lib/__tests__/bundle-budget.test.ts，index<5MB / 总<70MB / 最大 chunk<8MB，3/3 绿。
- ⚪ 入口进一步瘦身：记录 0002/0003 结论「重依赖全异步 + 路由级 autoCodeSplitting 已最优，拆共享层收益低风险高，不推荐」。
