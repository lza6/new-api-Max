# known-issues.md — 已知问题与风险

> 与 `project_specs.md` 区别：那份记**任务状态**，这份记**仍未解决的技术事实**。

## P0（阻塞级）

| # | 问题 | 证据 | 缺什么才能修 |
|---|---|---|---|
| 1 | ClickHouse 日志库路径从未验证 | 全仓无任何 CH 相关测试/脚本执行记录 | 本机无 `docker`（`docker: command not found`）；需 CH 运行环境 |
| 2 | 前端全量测试单进程跑不完 | 2026-10-23 实测 >25min 未完成 | 已提供 `web/scripts/test-shards.mjs`（T-A1），全量结果待跑完 |
| 3 | CI `frontend` job 的 `Test` 步骤仍是单进程全量 | `.github/workflows/ci.yml` step `Test: bun run test`，job timeout 10min | 应改为 `bun scripts/test-shards.mjs` 并放宽 timeout（待改） |

## P1

| # | 问题 | 证据 |
|---|---|---|
| 1 | 39 处 `ApiErrorMsg(...err.Error())` 直抛底层错误 | `grep -rn "ApiErrorMsg([^)]*err\.Error()" controller service middleware --include=*.go` = 39（8 文件） |
| 2 | 错误 toast 无 action 按钮 | `web/src/lib/handle-server-error.ts` 3 处 `toast.error` |
| 3 | 12 处数据区空态无下一步动作；`profile-insights/index.tsx:243,312` 写死英文未过 `t()` | `grep "No data\|No results"` = 17 |
| 4 | `toast.loading` 仅 2 处（长任务无进度/取消） | `grep -rn "toast.loading" web/src` |
| 5 | 15 处 `refetchInterval` 未接可见性门控 | `grep -rn "refetchInterval" web/src` = 15，`use-document-visible` 6 文件 |
| 6 | 日志/订阅表 `enableRowSelection: false` | `usage-logs-table.tsx:179` 等 3 处 |
| 7 | 2 处原生 `window.confirm` + 1 个 0 调用死 hook | `waffo-pancake-settings-section.tsx:279`、`use-form-dirty-guard.ts:57` |
| 8 | 表单校验时机：11 处 `onChange` / 1 处 `onBlur` | `grep -rn "mode: 'onChange'" web/src` |

## 环境/协作风险

| # | 风险 | 事实 |
|---|---|---|
| 1 | **多会话并发改同一工作区** | 2026-10-23 实测：本会话写作期间另有会话改 `web/src/styles/*`、`i18n/locales/*`、`ci.yml`、`package.json`，并把 `VERSION` 推到 v1.3.182 |
| 2 | `main` 的上游是 `upstream/main`（QuantumNous） | `git rev-parse --abbrev-ref @{u}` = `upstream/main`；**推送必须显式 `git push origin main`**，否则会打向不属于本仓库的上游 |
| 3 | 本机 HTTPS 直连不可用 | 记忆 `local-curl-schannel-broken.md`；生产探测需隧道 |
| 4 | 本机 PostgreSQL 服务损坏（0xC0000142） | 三库验证需 `initdb` 临时实例 |

## 2026-10-22 新增（视觉体系批次）

| # | 问题 | 级别 | 证据 | 缺什么才能修 |
|---|---|---|---|---|
| 1 | **前端测试基线是红的**：整批并行 18 个文件失败 | P1 | 对照实验：`git worktree` 检出 `6f284de93` 后跑同 18 文件 → 14 文件 / 24 用例失败 | 需要专门一批「测试基线归零」：逐文件判断是产品行为变了还是用例过时；**禁止为变绿削断言** |
| 2 | `knip-baseline.json` 陈旧，门禁报 117 条"新增" | P2 | `node web/scripts/knip-gate.mjs` 输出 | 逐条确认为既有积压后 `--update`（本轮只记录，未掩盖） |
| 3 | GitHub HTTPS 推送会短时全断（TLS eof） | P2 | 2026-10-22 13:33–13:36 连续 10 次 `TLS connect error`，部署后重试恢复 | 无法根治（跨境链路）；对策=重试 + 用 `git archive` 直传服务器构建，部署不依赖 push |
| 4 | 生产 CSS 经 lightningcss 降级，字面 `oklch(` 不出现在产物 | P3 | 服务器侧 `grep -c "oklch(" /tmp/s3.css` = 0，但 `transition-quiet`/`surface-grid`/`theme-preset=ink` 均在 | 生产视觉核对改为「令牌名 + axe」双证据，不再 grep 色值 |
| 5 | Windows 上 `rmdir /S` 会顺着 junction 删掉真实 `node_modules` | P1（工装事故） | 2026-10-22 清理临时 worktree 后 typecheck 报 `bun: command not found: tsgo` | 已恢复（`bun install --frozen-lockfile`）；纪律：含 junction 的目录只删目录本身，或先删 junction 再删目录 |

## 已修复（同一批次）

| 问题 | 修复 | 证据 |
|---|---|---|
| `anthropic` 预设白字 on 陶土 `#e37756` 仅 **2.9:1**（WCAG AA 违规） | `--primary` 由 `oklch(0.685 0.142 38)` 改 `oklch(0.57 0.128 38)` | axe 亮/暗各 0 违规；11 预设 × 2 页全 0 |
| `Table` 根 `[&_th]:text-sm` 压掉表头微标签字号（computed 14px） | 移除该后代覆盖 | 浏览器实测 `th` = 11px / tracking 0.275px |
