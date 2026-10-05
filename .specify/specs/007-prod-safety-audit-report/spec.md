# 007 — 生产安全加固 · 登录审计 · 报表导出 · 稳定性闭环

> Ratified: 2026-10-04 ｜ 依赖：001/005/006 ｜ 目标版本：下一批（未部署）
> Spec Kit 规范（本文件为 spec，配套 plan.md / tasks.md）

## 1. 问题陈述（Problem）

上线后终局审计发现以下真实生产缺口，均已在代码中核验：

1. **登录默认不限流 + 失败零审计（P1 安全）**：`router/api-router.go` 的 `/api/user/login` 挂了 `LoginRateLimit()`，但 `setting/operation_setting/login_rate_limit_setting.go` 的 `Enabled` 默认 `false` → 默认配置下暴力破解无速率限制；`model/log.go` 的 `RecordLoginLog` 硬编码 `Success: true` → 失败登录无审计记录（OWASP 反暴力破解基线缺失）。
2. **私网判定两套并存且不一致（P1 安全）**：`common/ssrf_protection.go` 的 `isPrivateIP` 含 CGNAT `100.64.0.0/10`，但 `common/ip.go` 的 `IsPrivateIP`（被 `service/web_protection_tracker.go` 使用）**缺 CGNAT 与 link-local/保留段** → 两处判定不一致，存在绕过面。
3. **文档断裂（P2）**：`计划书/project_specs.md` 引用不存在的 `ops/ai-compliance-checklist.md`；`计划书/docs/t8-security-asvs-deepdive.md` 声称「G4 邮箱枚举已闭环」，但注册/OAuth 绑定侧仍可枚举。
4. **报表导出缺失（P2）**：站点用量/成本无按模型/渠道/日的聚合与 CSV 导出（企业审计诉求）。
5. **CORS 非法组合（P1）**：`middleware/cors.go` 同时 `AllowAllOrigins=true` + `AllowCredentials=true`（规范禁止组合）。
6. **停机/稳定性**：批量额度更新与后台 loop 未纳入优雅关闭；废弃索引 `idx_logs_traffic` 只增写放大。

## 2. 用户故事（User Stories）

### US-1 暴力破解防护（P0 安全）
作为**站点运维者**，在默认配置下，登录接口应有速率限制，且每次失败尝试都留下可审计的记录（去敏，不含密码/凭据）。
- 验收：`IsLoginRateLimitEnabled()` 默认为 true
- 验收：失败登录写 `audit_logs`，`success=false`、`status=401`（或 400）、`other.op.params.reason` 记稳定短标识
- 验收：审计内容绝不包含密码明文
- 验收：成功登录仍记 `success=true, status=200`

### US-2 私网判定一致（P0 安全）
作为**安全审计者**，`common.IsPrivateIP` 与 SSRF 的 `isPrivateIP` 判定集合一致，覆盖 CGNAT/链路本地/保留段。
- 验收：`100.64.0.1`、`169.254.1.1`、`0.0.0.0` 均判为 private；`100.128.0.1`、`8.8.8.8` 判为 public

### US-3 用量/成本报表（P1）
作为**企业管理员**，我能按模型/渠道/日聚合查看用量与成本，并导出 CSV。
- 验收：`GET /api/log/report?group_by=model|channel|day&start=&end=` 返回聚合行 + totals
- 验收：`GET /api/log/report/export` 流式返回 UTF-8 BOM 的 CSV
- 验收：三库（SQLite/MySQL/PG）日键分桶一致

### US-4 CORS 合规（P1）
作为**部署者**，跨域响应绝不同时出现 `Access-Control-Allow-Origin: *` 与 `Allow-Credentials: true`；白名单由 env 控制，默认仅同源。

### US-5 优雅关闭（P2）
作为**运维者**，SIGTERM 后后台写任务先停、不丢账、日志顺序正确。

## 3. 非目标（Out of Scope）
- 不做自动涨价/自动风控闭环。
- 不做多租户隔离（另行设计）。
- 不动生产部署（用户明确：等指令）。

## 4. 未闭环项（诚实披露）
- **邮箱枚举注册/OAuth 侧**：注册与 OAuth 绑定仍可枚举（P2，未修）。
- **主站 CSP/安全头**：仓库内无 Caddyfile，安全头在宿主机（不在可审计范围）。
- **渠道 Channel.Key 明文存库**：迁移/备份须按敏感数据处理（未改加密列）。
- **视频/图像端点适配 与 图床+5分钟清理**：见 plan.md 后续批次。
