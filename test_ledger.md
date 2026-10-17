# test_ledger.md — 验证台账（防重复跑 / 防误判回归）

> 只记录实际执行的命令与观察到的结果。最后更新：2026-10-17。

## 已 PASS（本轮实际运行）

| 范围 | 命令 | 结果 |
|---|---|---|
| 后端构建 | `go build ./...` | PASS（exit 0） |
| relaykit 独立构建 | `cd relaykit && GOWORK=off go build ./...` | PASS（exit 0） |
| 静态检查 | `go vet ./controller/ ./relay/ ./relay/helper/ ./model/ ./service/ ./middleware/ ./router/` | PASS（无输出） |
| SQLite schema 幂等 | `go test ./model/ -run TestDBConformanceAutoMigrateIdempotent -count=1` | PASS（34s） |
| 模型层定向 | `go test ./model/ -run 'ActionApproval\|McpSession\|ChannelKey' -count=1` | PASS |
| 服务层定向 | `go test ./service/ -run 'ActionRegistry\|ExpiryReminder\|RoutingPolicy\|Repro_Claude' -count=1` | PASS |
| relay ops | `go test ./relay/ ./relay/helper/ -run 'OpenAIOps' -count=1` | PASS |
| controller 定向 | `go test ./controller/ -run 'TestMcp\|TestSPASEO\|TestGetSitemap\|TestGetRobots' -count=1` | PASS |
| router | `go test ./router/ -count=1` | PASS |
| 前端类型 | `cd web && bun run typecheck` | PASS（exit 0） |
| 前端构建 | `cd web && bun run build` | PASS（dist 生成） |
| i18n | `cd web && bun run i18n:check` | PASS（0 missing） |
| 前端 lint | `npx oxlint -c .oxlintrc.json <changed files>` | PASS（0 error） |
| Key 抽屉测试 | `npx vitest run src/features/keys/components/__tests__/api-keys-mutate-drawer.test.tsx` | 3/3 PASS |
| 语言前缀测试 | `npx vitest run src/i18n/__tests__/route-locale.test.ts` | 5/5 PASS |
| 真实浏览器 a11y/响应式 | `node e2e-local/v1388-a11y-responsive.cjs` | 7/7 PASS |

## 已知噪声（非本轮回归）

| 范围 | 现象 | 判定 |
|---|---|---|
| `go test ./controller/` 全量 | 约 600s 超时，栈在 `pkg/jsplugin` / `sobek` | 既有噪声；用 `-run` 缩小范围 |
| `web/e2e-local/*.cjs` | `bun run lint` 报 `no-require-imports` | 未跟踪的临时脚本，不在交付范围 |
| `controller/TestAuditDatabaseMatrix` 等 | 既有失败 | 与既有台账一致，非本轮引入 |

## 未验证 / 缺口

- MySQL / PostgreSQL conformance：本机无 Docker/MySQL/PG，未执行。
- 真实上游 fine-tune/batch 创建：无可用上游渠道，未执行。
- 真实 MCP 客户端握手（Claude Code/Cursor）：未执行。
- 生产 p95 压测：本地 mock 上游压测见 `evidence/stress-*.json`；真实渠道压测未执行。
- 浏览器 E2E：本轮新增页面（动作审批、Key curl）需真实登录态；见 `evidence/` 中的执行记录。

## 2026-10-18 独立审查后新增验证

| 范围 | 命令 | 结果 |
|---|---|---|
| 编码路径计费 | `go test ./relay/ -run 'TestOpenAIOpsPaidUsesDecodedPath\|TestOpenAIOpsPathSupported'` | PASS |
| 对象归属/IDOR | `go test ./model/ -run TestOpenAIOpsObjectOwnership` + `go test ./relay/ -run 'ParseOpenAI\|FilterOwned'` | PASS |
| 开放重定向 | `go test ./controller/ -run TestSPASEODefaultLocaleRedirectRejectsProtocolRelative` | PASS |
| `$` 替换展开 | `go test ./controller/ -run TestSPASEOReplacementDollarIsLiteral` | PASS |
| 前端审批错误处理 | `npx vitest run src/features/action-approvals/__tests__/api.test.ts` | 3/3 PASS |
| 最终真实浏览器 E2E | `node web/e2e-local/v13148-full-e2e.cjs` | 7/7 PASS |
| 最终 50 并发压测 | `node stress.cjs`（SQLite + mock 上游） | 0 错误；QPS 19.9；P95 5.80s；P99 6.41s |

## 独立审查结论与修复
- 安全：1 Critical（编码路径计费绕过）、2 High（开放重定向、ops IDOR）→ 已修。
- Go：2 P1（审批未绑定参数、请求体多次物化）→ 已修。
- 前端：4 P1（失败误报成功、错误当空列表、存量语言与 canonical、未翻译前缀 lang）→ 已修。
- 仍未验证：MySQL/PostgreSQL conformance、真实上游 fine-tune/batch、真实 MCP 客户端、生产环境。
