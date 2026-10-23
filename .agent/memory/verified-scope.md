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
