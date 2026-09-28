# Plan：UI 设计系统 / a11y 与移动端 / 性能预算回归（004）

## 阶段划分（依赖顺序）
- **Phase 1【Spec/计划】**: spec.md + plan.md + tasks.md（本批，已完成）→ 质量门：文档。
- **Phase 2【并行实现 · 3 Builder 子代理】**:
  - 2a `web/src/styles/theme.css`：语义阴影 tokens（:root + .dark）→ US-4.2.3
  - 2b pricing 组件（model-card/pricing-table/stats-card/search-bar/drawer）token 化 + 交互态 → US-4.2.3
  - 2c `webhook/__tests__/a11y-webhook-settings.test.tsx` + `keys/components/__tests__/a11y-keys-stepup.test.tsx`
    + `pricing/components/__tests__/a11y-stats-card.test.tsx` → US-4.2.4
  - 2d `web/scripts/knip-gate.mjs` + 基线 + `bundle-budget.test.ts` 扩展 + `ci.yml` 接入 → US-4.2.5
- **Phase 3【E2E/截图】**：375/768/1280 断点截图入 `计划书/e2e-evidence/browser-e2e-v1.3.56-a11y/`（主控，依赖 2b/2c 完成）。
- **Phase 4【审查循环】**：独立 Critic 六维审查（需求/逻辑/边界/质量/测试/运行证据）→ 主控修复 → 复验（默认最多 3 轮）。
- **Phase 5【质量门/交付】**：typecheck → lint → vitest（本批新用例）→ go 侧不受影响（纯前端）→ 主题 commit → push → tag v1.3.56 → Release（生产部署待用户授权）。

## 依赖关系
- Phase 2 内 2a/2b/2c/2d 互不重叠文件，可完全并行。
- Phase 3 依赖 2b（组件样式定稿）+ 2c（axe 修复后组件）。
- Phase 4 依赖 Phase 3；Phase 5 依赖 3/4。

## 并行机会
- 2a+2b（§4.2.3）与 2c（§4.2.4）与 2d（§4.2.5）三个 Builder 已并行启动。
- 截图脚本可与 2 并行编写，验证依赖实现完成。

## 风险与缓解
| 风险 | 缓解 |
|---|---|
| vitest 本机 worker 崩溃（旧环境噪声） | 已实测单文件 axe 测试可跑通（38s 绿），非噪声；本批用单文件/小集合跑 |
| knip 存量 600+ 问题导致全量门禁失败 | 基线对比门禁（只防新增，不清零存量）；探针实测正反例 |
| axe 扫描发现真实违规 | 这是任务核心价值——修复生产组件后复测至 0 违规 |
| 设计改动引入回归 | 只加 token/伪态，不改组件结构；typecheck+lint+build 兜底；截图对比 |
| 生产部署 | 本批不自动部署；交付后在用户授权下按 deploy.sh SOP |

## 质量门禁
- `cd web && bun run typecheck`（tsgo -b）exit 0
- `cd web && bunx oxlint -c .oxlintrc.json <改动文件>` 无 error
- `cd web && bunx vitest run`（3 个新 axe 用例 + bundle-budget）绿
- knip-gate 探针正反例实测通过
- 截图证据落盘 `计划书/e2e-evidence/browser-e2e-v1.3.56-a11y/`
- 独立审查 3 轮收敛或明确阻塞
