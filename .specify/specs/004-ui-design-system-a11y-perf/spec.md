# Feature Specification: UI 设计系统 / a11y 与移动端 / 性能预算回归（004）

> Spec-Kit Phase 2 · 对应指南 §4.2.3 / §4.2.4 / §4.2.5
> 状态：IMPLEMENT（三个 Builder 子代理并行执行中）

## Problem Statement

前端在 v1.3.40 已做过 a11y 冒烟与三断点截图、v1.3.46 新增了定价页/订阅统计/webhook 等页面，
但存在三类未收口的问题：

1. **设计系统 token 化不完整**：颜色/圆角 tokens 已完整，但语义阴影层缺失，组件混用 Tailwind 默认
   shadow，卡片/抽屉/表格层级不统一；hover/focus/active 交互态覆盖不系统（尤其 focus-visible）。
2. **新页面未接 a11y 门禁**：webhook 设置页、keys step-up 弹窗、订阅统计卡没有 vitest-axe 用例，
   v1.3.46+ 新页面未做断点截图。
3. **性能预算无防回归机制**：bundle-budget 已有但无 code-splitting 复核；knip 死码门禁未入 CI。

## User Stories

### US-4.2.3：视觉层级统一与交互态完备

As a 用户 / 前端维护者
I want 卡片、抽屉、表格的间距/圆角/阴影来自统一 token，且交互元素有完整 hover/focus/active 反馈
So that 界面高级、一致、深浅主题都清晰，且键盘用户有焦点可见性。

**Acceptance Criteria:**
- [ ] `theme.css` 新增语义阴影 tokens（card/raised/drawer/popover/overlay），:root 与 .dark 双套
- [ ] pricing 组件（model-card / pricing-table / stats-card / search-bar / drawer）改用 token 化阴影与统一层级
- [ ] 交互元素补齐 `focus-visible` 焦点环 + hover 反馈 + active 按压
- [ ] 深浅主题下阴影/层级均清晰（不默认全暗）
- [ ] 对照 `rules/web/design-quality.md` 反模板清单不踩禁用项

### US-4.2.4：新页面 a11y 用例与断点截图

As a 可访问性工程师
I want webhook 设置页、keys step-up、统计卡有 axe 用例，新页面有 375/768/1280 截图
So that a11y 回归可控、移动端无横向溢出。

**Acceptance Criteria:**
- [ ] `webhook/__tests__/a11y-webhook-settings.test.tsx`：axe 0 违规
- [ ] `keys/components/__tests__/a11y-keys-stepup.test.tsx`：step-up 弹窗 axe 0 违规
- [ ] `pricing/components/__tests__/a11y-stats-card.test.tsx`：统计卡 axe 0 违规
- [ ] axe 若发现真实违规（checkbox 无 label / 对比度等）→ 修复生产组件后复测
- [ ] 375/768/1280 三断点截图入 `计划书/e2e-evidence/browser-e2e-v1.3.56-a11y/`

### US-4.2.5：性能预算与死码门禁

As a CI/维护者
I want 新增页面纳入 code-splitting 复核，knip 死码门禁入 CI
So that 预算不膨胀、死码不新增。

**Acceptance Criteria:**
- [ ] `web/scripts/knip-gate.mjs` 基线对比门禁（--update 建基线；默认跑对比，新增 dead code → exit 1）
- [ ] 探针正反例实测：加未使用导出 → exit 1；删除 → exit 0
- [ ] `bundle-budget.test.ts` 扩展：每个路由/页面级异步 chunk 有独立文件（code-split 生效）
- [ ] `.github/workflows/ci.yml` frontend job 接入 knip-gate
- [ ] 新页面路由懒加载复核（已懒则记录证据；未懒则补）

## Non-Functional Requirements

- **Performance**: 新增 token/样式不改变运行时热路径；JS 预算保持 index<5MB / total<70MB / max chunk<8MB
- **Accessibility**: axe 0 违规；键盘可达；对比度 WCAG AA
- **Compatibility**: 跨 375/768/1280 无横向溢出；light/dark 双主题
- **Maintainability**: token 化集中在 theme.css；测试文件进各模块 `__tests__/`；脚本跨平台（Windows+Linux CI）

## Success Metrics

- vitest-axe 3 个新用例全绿（本机实测）
- knip 门禁探针正反例实测通过
- bundle-budget 仍绿
- typecheck / lint 通过
- 三断点截图落盘

## Out of Scope

- 不重构 theme-presets.css 的 9 个预设结构（仅可在 theme.css 加 shadow tokens）
- 不清零 knip 存量 600+ 问题（只防新增）
- 不做视觉回归像素比对框架（仅断点截图）
- 本批不部署生产（按用户纪律按需部署）

## Clarifications

（本批无待澄清项——验收标准即指南原文，已由主控按真实环境事实裁定。）
