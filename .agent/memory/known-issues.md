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

## 2026-10-24 新增（改进指南 v1.3.195 重写批次）

| # | 问题 | 级别 | 证据 | 缺什么才能修 |
|---|---|---|---|---|
| 1 | **CI 门禁在本仓库的工作流下从不运行**：`ci.yml` 仅 `on: pull_request`，而 37 个提交/14 个 tag 全部直接 push main | P0 | 读 6 个 workflow 触发条件；`release.yml`/`docker-build.yml` 只构建不测 | 无（改 `ci.yml` 加 `push: branches: [main]`，T-A9） |
| 2 | `ci.yml` 的 `Test` 步骤跑不完：`bun run test` = `vitest run`，而 `vitest.config.ts` 强制 `maxWorkers:1` + `fileParallelism:false`，job timeout 10min，本地实测单进程 >25min、分片 707s | P0 | `web/package.json`、`web/vitest.config.ts:63-69`、`ci.yml:144-146` | 改成分片脚本 + 放宽 timeout（T-A9） |
| 3 | 版本口径门禁当前 **RED**（`计划书/README.md` 停 v1.3.192，实际 v1.3.195），已存活 ≥2 个版本 | P0 | 实跑 `check-version-consistency.mjs` | 改两处声明（T-A10） |
| 4 | **生产零备份**：仓库内 `rg 'pg_dump\|数据库备份\|backup' 计划书/ops scripts deploy` **零命中** | P0 | 本轮实跑 | SSH 凭据或业主本地执行（T-E1，已授权待落地） |
| 5 | **47KB 尼尔森指南此前未纳入版本控制**（只存在于工作区） | P1 | `git ls-files 计划书` 查不到 | **已于本批次提交修复** |
| 6 | 前端全量测试当前 1599/1601（2 条失败，`model-listing.test.tsx:771 square_state`），归因并行会话在途改动 | P2 | 本轮实跑 + 单跑复现 | 待 HEAD 隔离复现 |

## 2026-10-24 环境事实补充

- 本工作区**同时有 7 个会话 busy**（`ListAgents` 观察），跑测试会读到别人的半成品状态。
- 每次跑前端全量前先 `git status` 快照，否则会把别人的 bug 记成自己的回归。

## 2026-10-24 生产加固轮（含两条审计结论被证伪）

**方法教训（最重要的这条）**：审计结论必须标注**取证时间 + 取证对象**。
本文上一稿写"生产零备份"，实际只是"**仓库里**搜不到 pg_dump"——生产机 `crontab` 里早就有
`0 3 * * * /opt/new-api/backup.sh`。同类错误还有"3001 暴露公网"（实测早已 `127.0.0.1:3000`）
与"无 fail2ban"（实测已装且生效）。**今后凡涉及生产事实，必须连线取证，不得用仓库搜索代替。**

| # | 问题 | 级别 | 状态 | 证据 |
|---|---|---|---|---|
| 1 | 生产备份存在但**从未演练**、`.env`/`Caddyfile` 未纳入、dump 644、无异地 | P0 | **已修（本轮）** | `evidence/T-E1.json` |
| 2 | SSH `passwordauthentication yes` + `pubkeyauthentication no`（root 密码可登） | P0 | **已修（本轮）** | `evidence/T-E2.json` |
| 3 | ufw inactive（无主机防火墙） | P0 | **已修（本轮）** | 同上 |
| 4 | postfix 监听 `0.0.0.0:25` | P2 | **已修（本轮）** | `ss -lntp` 只剩 127.0.0.1:25 |
| 5 | **Docker 发布端口会绕过 ufw**（`DOCKER-USER` 链） | P1 | **未修** | 若将来 `docker run -p 0.0.0.0:xxxx` 仍会暴露 |
| 6 | root 密码曾明文出现在本机记忆与临时 askpass | P2 | 缓解（密码认证已关） | 建议轮换控制台密码 |
| 7 | 内核 `reboot-required` 自 10-06 未重启 | P2 | **未修** | 需停机窗口 |
| 8 | 异地副本目前是一次性手工拉取，非自动同步 | P1 | **未修** | 决策：rsync 第二台 / 对象存储 |
| 9 | 无 WAL 归档（非 PITR，RPO≈24h） | P2 | **未修** | 需 `archive_mode` |
| 10 | `db_structure.md` 写 55 表，生产实际 **62 表** | P1 | **未修** | 需重跑 `scripts/gen-db-structure.mjs` |
