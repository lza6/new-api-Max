# verified-scope.md — 已验证范围（可引用，勿重复跑）

> 每条都带**命令**与**有效期条件**。条件一旦不成立（相关代码变了），结论自动失效。

| 验证项 | 命令 | 结果 | 有效期条件 |
|---|---|---|---|
| Go 根模块构建 | `go build ./...` | PASS 54.7s（2026-10-23） | 任何 `.go` 变更后重跑 |
| relaykit 独立构建 | `cd relaykit && GOWORK=off go build ./...` | 见 CI backend job | 任何 `relaykit/` 变更 |
| 前端类型检查 | `cd web && bun run typecheck` | PASS 4.9s（2026-10-23） | 任何 ts/tsx 变更 |
| 版本口径 | `node scripts/check-version-consistency.mjs` | consistent（2026-10-23） | 每次改 VERSION/CHANGELOG/tag/§0 |
| 版本门禁自测 | `... --self-test` | 9/9 PASS | 改门禁逻辑时 |
| 术语门禁 | `cd web && node scripts/check-terminology.mjs` | 0 违规（修复假绿后） | 改 locale |
| 漏译门禁 | `cd web && node scripts/check-missing-i18n.cjs` | 0 missing | 改 locale |
| CI 配置结构 | `Bun.YAML.parse(.github/workflows/ci.yml)` | 3 jobs / frontend 12 steps | 改 ci.yml |
| 真实 schema 导出 | 构建 → `SQLITE_PATH` 启动 → `scripts/gen-db-structure.mjs` | 55 表 / 637 列 | 任何 model/迁移变更 |
| axe 生产审计 | `node web/e2e-local/prod-axe-audit.cjs` | 0 critical/serious（v1.3.178 记载） | **任何 UI 改动后必须重跑**（本会话未重跑） |
| 三库 conformance | `make db-check` | PASS 103s（历史，v1.3.165 期） | 任何 schema/ORM/DSN 变更；本会话**未重跑** |
| 成本覆盖 | 生产 SQL 聚合 | 今日 99.14%（历史） | 改成本规则 |

## 已知噪声（不是回归）

| 现象 | 判定 |
|---|---|
| `go test ./controller/` 全量约 600s 超时（`pkg/jsplugin`/`grafana/sobek`） | 非回归；用 `-run` 缩小 |
| `bunx vitest run` 全量单进程 >25 分钟未完成（2026-10-23 实测） | 环境限制，非用例失败；用 `bun scripts/test-shards.mjs` |
| `web/e2e-local/*.cjs` 触发 oxlint `no-require-imports` | 未跟踪调试脚本，不在交付范围 |

## 2026-10-22 视觉体系批次（v1.3.182 / 185 / 186，本会话新增）

| 验证项 | 命令 | 结果 | 有效期条件 |
|---|---|---|---|
| 令牌层落地 | 浏览器 `getComputedStyle` 求值 | `--radius=.625rem`；`--surface-1/--rule` 解析正常；metric 值 `JetBrains Mono Variable`+`tabular-nums`；dashboard `rounded-2xl`=0；body 仍 `Public Sans` | 改 theme.css / 字体依赖后 |
| 视觉组件测试 | `cd web && bunx vitest run src/components/__tests__ src/components/ui src/features/dashboard` | 81/81（v1.3.185 时点）；163/163（v1.3.186 时点，24 文件） | 改这些目录后 |
| 预设对比度全扫 | axe `color-contrast`，`/keys` + `/dashboard/overview` × **11 预设** | 全 0（修 anthropic 后）；修复前 anthropic 2 页各 1 节点 2.9:1 | **任何预设/令牌改动后必须重扫** |
| 视觉基线 | `BASE=… PAGES=… PRESETS=default,anthropic,ink WIDTHS=1440,320 node web/e2e-local/visual-baseline.cjs` | 18 图 + `results.json`；axe 0（default/ink） | 任何 UI 改动后 |
| 动效红线 | `rg "transition-all" web/src` | 0 命中（41 处换 `.transition-quiet`；进度条保留 `transition-[width]`） | 新增动效时 |
| 生产健康 | 服务器侧 `curl -sI --resolve …` + `/healthz` + `/readyz` + `/api/status` | v1.3.182 / 185 / 186 三次部署均 200，0 ERROR/0 panic/0 restart | 每次部署后 |
| 本地压测 | 20 并发 / 20s 打 `/api/status`（本地实例） | 260,951 请求，0 错误，QPS 13,047，p50 1.23ms / p95 3.05ms / p99 3.70ms | 改热路径后重跑 |

## 新增已知噪声（本会话实测）

| 现象 | 判定 |
|---|---|
| `bun run test` 全量并行下 **18 个文件失败**（chat/model-pricing/models/onboarding/pricing/task-plugins/usage-logs/system-settings/subscriptions） | **非本会话引入**：在改动前提交 `6f284de93` 上用 `git worktree` 跑同样 18 个文件 → **14 文件 / 24 用例同样失败**。属仓库既有红基线 |
| `task-event-stream.test.tsx`、`delete-plan-dialog.test.tsx` 在整批并行下偶发 8s 超时 | 单跑全 PASS；属并发下的 flaky |
| `node scripts/knip-gate.mjs` 报 117 条"新增"死代码 | 基线文件陈旧导致的漂移，**无一条指向本会话文件**；未 `--update` 掩盖 |

## 2026-10-23 17:30 更新（前端套件全绿）

| 验证项 | 命令 | 结果 |
|---|---|---|
| 前端全量 | `cd web && node scripts/test-shards.mjs` | **1582/1582 通过，0 失败，5 片，567s，PASS** |
| 前端构建 | `cd web && bun run build` | clean，18.5s，产物 24.6MB（gzip 7.76MB） |
| 死代码门禁 | `cd web && node scripts/knip-gate.mjs` | 绿（586 存量 / 0 新增） |
| 令牌快照 | `cd web && node scripts/ds-tokens-check.mjs` | 绿（743 tokens） |
| 后端 | `go build ./...`、`go vet ./controller ./common` | clean |

测试池口径：**必须用 `threads`**（`vmThreads` 会同 worker 共享模块上下文导致 mock 串台）。
