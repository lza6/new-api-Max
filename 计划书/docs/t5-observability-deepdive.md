# 专项分析 · T5 全链路可观测性解析（request-id / span / 日志关联）

> 定位：主指南 §T5 的**深挖文档**：从现状证据、已落地边界、剩余缺口到最小落地路径，全部只读整理。
> 生成：2026-09-25 · 未改业务代码。

## 1. 现状（代码证据，HEAD aced9a37d）

| 链路点 | 实现 | 证据 |
|---|---|---|
| 请求入口 | `middleware/request-id.go`：生成 id 写入 gin ctx + `X-Oneapi-Request-Id` 响应头 + request context | request-id.go:13-16 |
| 常量 | `common.RequestIdKey = "X-Oneapi-Request-Id"`、`common.UpstreamRequestIdKey = "X-Upstream-Request-Id"` | common/constants.go:206-207 |
| 日志关联 | `middleware/logger.go:25` 从 ctx 读 request-id；`relay/helper/common.go:175-180` 日志带 request-id | middleware/logger.go、relay/helper/common.go |
| 日志表字段 | `model/log.go` Log 有 `request_id` / `upstream_request_id`（索引） | db_structure.md（logs 表） |
| 上游透传 | `relay/channel/api_request.go:56-58`：`SetupApiRequestHeader` 默认带上网关 request-id；**Header Override 显式设置时覆盖** | api_request.go:50-60 |
| 回包 id 记录 | api_request.go:606-607：上游回包 `X-Oneapi-Request-Id` → `UpstreamRequestIdKey` | api_request.go:600-610 |
| 测试 | `relay/channel/api_request_test.go:239-351`：透传（DoApiRequest/DoFormRequest）、回包 id 记录、Override 覆盖 | api_request_test.go |
| 前端时间线 | `web/src/features/usage-logs` request-timeline / request-timeline-card（span 视图） | web/src/features/usage-logs |

## 2. 已落地边界（诚实陈述）
- 网关**每条请求**有 request-id，并贯穿 consume/error/task 日志与响应头（同一字段常量统一）。
- 上游请求**默认携带**网关 request-id，且可被 Header Override 显式覆盖（不新增开关，语义为「默认带上、可覆盖」）。
- 上游回包 id 被记录（`logs.upstream_request_id`），任一请求可经 `request_id` 串起网关日志 + 上游回包 id。
- 前端时间线 span 化已存在（commit 2457d7bf9 及后续）。

## 3. 剩余缺口（按价值排序）

| 缺口 | 现状 | 最小落地 | 优先级 |
|---|---|---|---|
| 多实例全局 trace | 单实例 request-id 已贯通；无节点 hop/全局 trace | 透传时追加 `X-Oneapi-Node`（NODE_NAME）或日志带节点名；不做全量 OTel | P2（可选） |
| 上游回包 id 的展示 | 日志已记录 `upstream_request_id`；前端时间线是否展示**待浏览器核对** | 前端时间线卡片补「上游请求 id」行 + i18n | P2（用户确认时） |
| 慢日志/告警深度 | 已有 slow SQL 阈值；无按 request-id 汇总慢链路 | 可选：慢请求采样日志按 request-id 关联 | P3 |
| 批量任务/插件链路 | `middleware/task_plugin.go:1244/1305` 已用 request-id | 插件任务 settle 日志带 request-id 即可（若缺则补） | P3 |

## 4. 落地纪律（若立项）
- 任何新头部/字段先加常量（common/constants.go），日志/模型字段回填 `db_structure.md`（若加列则三库矩阵）。
- 透传语义保持「默认带、可覆盖」，不引入新开关破坏现状；`relaykit` 若改 DTO 需 `GOWORK=off go build ./...`。
- 测试沿用 `api_request_test.go` 风格（DoApiRequest/DoFormRequest 透传、Override 覆盖、回包 id 记录）。
- 前端展示补 i18n（en/zh 为主，7 语言随 i18n:sync）。

## 5. 验证（建议命令）
- `go vet ./... && go test ./relay/channel/ ./middleware/ ./service/`
- `cd web && bun run typecheck && bunx vitest run src/features/usage-logs`
- 真实请求：本地 mock 上游回包带头 → grep `upstream_request_id` 日志。

## 闭环状态（2026-09-27，v1.3.44 回填）
- ✅ 前端展示上游 request-id：web/src/features/usage-logs/components/dialogs/details-dialog.tsx:787 展示 upstream_request_id；filter-bar 支持按 upstream_request_id 过滤。
- request-id 全链路（入口生成/日志/日志表字段/上游透传/回包 id 记录/前端时间线 span）此前已闭环（v1.3.35 前后）。
