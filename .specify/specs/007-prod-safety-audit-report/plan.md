# 007 — Plan（实施计划）

> 配套 spec.md / tasks.md。所有改动均为本地，未部署。

## 技术方案

### US-1 暴力破解防护
- `setting/operation_setting/login_rate_limit_setting.go`：`Enabled: false → true`（安全默认）。
- `model/log.go`：`RecordLoginLog` 增 `success bool, status int` 参数；新增 `RecordLoginFailureLog`。
- `controller/user.go`：`Login` 每个失败分支调 `recordLoginFailureAudit`（reason ∈ invalid_params/decrypt_failed/database_error/invalid_credentials）；`recordLoginAudit` 传 `(true, 200)`。
- 测试 `controller/login_audit_test.go`（3 用例，含去敏断言）。

### US-2 私网判定一致
- `common/ip.go` `IsPrivateIP`：补 CGNAT/link-local/保留段 + IPv6 ULA 兜底。
- 测试 `common/ip_test.go`。

### US-3 用量报表
- `model/usage_report.go`：`GetUsageReport(groupBy, start, end)`，整数日键 `x-(x%86400)`（禁 `/`）；别名 `dim_key` 避 `key` 保留字。
- `controller/usage_report.go`：JSON `GetUsageReport` + 流式 CSV `ExportUsageReportCSV`。
- `router/api-router.go`：`/api/log/report`、`/api/log/report/export`（AdminAuth）。
- 三库 conformance `TestDBConformanceUsageReport`。
- 前端 dashboard 第 5 section `usage-report-section.tsx` + i18n 14 键 × 7 语言。

### US-4 CORS
- `middleware/cors.go`：env `CORS_ALLOWED_ORIGINS` 白名单；消除冲突组合。
- `middleware/cors_test.go`。

### US-5 优雅关闭 + 索引
- `model/utils.go` + `main.go`：`FlushBatchUpdate`/`StopBatchUpdater`。
- `service/background_loop.go` + 4 loop：`Stop*`。
- `model/main.go`：`migrateLogTrafficIndex` 显式 DropIndex。

## 依赖与顺序
1（安全基线，最快止血）→ 2（安全一致性）→ 4（CORS）→ 3（报表）→ 5（稳定性）。
依赖关系：3 依赖三库环境；5 与 1/2/4 无耦合，可并行。

## 风险
- 报表日键：MySQL `/` 是小数除法 → 必须 `%` 方案（已实测）。
- 持久库 conformance 用例须开头清表（已踩坑修复）。
- 登录审计签名变更影响面：`go build` 已证无其他 caller 断裂。
- 未部署：需用户显式指令。

## 验证矩阵（节点验收）
| 节点 | 验收 | 命令 |
|---|---|---|
| 构建 | 全树编译 | `go build ./...` |
| 静态 | vet/gofmt | `go vet ./...`、`gofmt -l` |
| 安全单测 | 登录审计 + 私网 | `go test ./controller/ ./common/` |
| 三库 | 报表 conformance | `TEST_MYSQL_DSN=... TEST_POSTGRES_DSN=... go test ./model/ -run TestDBConformance` |
| 前端 | typecheck + 我改文件零错误 | `cd web && bun run typecheck` |
| i18n | 7 语言键集一致 | node 校验脚本 |
| 反向 | 先红后绿 | 逐项临时回退验证 |
