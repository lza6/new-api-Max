# 专项分析 · 安全（OWASP ASVS）与审计去敏深挖（T8 纵深）

> 定位：主指南 §T8 的**深挖文档**：硬规则、现状、缺口、落地路径；只读整理。
> 生成：2026-09-25 · 锚点：`docs/authentication.md`、`计划书/audit/security-asvs-audit.md`、`middleware/audit_sensitive_test.go`。

## 1. 硬规则（AGENTS + ASVS）
- 任何认证相关改动（注册/登录/登出/改密/找回/MFA/WebAuthn/OAuth/会话/JWT/API 凭据/敏感操作重认证）必须读 OWASP Authentication / Session Management / Password Storage / Forgot Password / MFA / OAuth / CSRF Cheat Sheets + ASVS（最新稳定版，引用版本+requirement id）。
- 服务端强制：前端检查不得替代；恢复/替代登录路径不得绕过认证保证。
- 审计事件不得含密码/验证码/恢复码/私钥/可用 token；只记非机密上下文。

## 2. 现状（代码证据）
- 已加固项：Secure Refresh Cookie + OriginGuard、argon2id、RSA-OAEP 登录、WebAuthn、OAuth/OIDC、Casbin、`SESSION_SECRET` 多实例告警、ASVS 审计报告。
- D3 去敏：`middleware/audit_sensitive_test.go:23-77` `TestAuditLogSensitiveFieldsAbsent`（敏感字段正则 + 审计条目采样断言，拒绝 raw token/fingerprint 泄露）——HEAD 已入库。
- 相关测试：`controller/access_token_audit_test.go` 风格（fingprint 而非 raw token）。

## 3. 缺口（主指南 §T8）
1. 审计报告复核与现行代码一致性（已修复项标 FIXED / 未修复列 Gap）。
2. D2 高危缺口修复 + 回归：登录/找回防枚举统一文案、OAuth state 校验测试、会话撤销传播、密钥页面披露默认。
3. D3 全面去敏回归：现有正则扫描测试扩展覆盖所有审计事件类型；gitleaks 扫描可选。

## 4. 落地路径（建议）
1. 复核 `security-asvs-audit.md` 与现行代码（表格逐项），更新状态列。
2. 每个高危缺口：先写失败测试（防枚举/重放/绕过/过期/会话撤销），再最小实现，再回归。
3. 去敏：把 `TestAuditLogSensitiveFieldsAbsent` 推广到所有审计事件构造点（保持单测试文件，不散）。
4. 敏感改动走开关灰度；回滚按 commit。

## 5. 验证
- `go vet ./... && go test ./middleware/audit/... ./controller/... ./service/...`
- `cd web && bun run typecheck && bunx vitest run src/features/security src/features/auth`
- 可选 `gitleaks detect`（不进假完成）。

## 闭环状态（2026-09-27，v1.3.44 回填）
- ✅ 复核完成：security-asvs-recheck-raw.md 三/四节已按现行代码逐项更新。
- ✅ 已满足：登录/找回防枚举统一文案、OAuth state 校验+原子消费、会话撤销传播、PAT 一次性披露、审计去敏、TOTP/备用码/passkey 单次。
- ✅ 已修（v1.3.41/43）：G2 验证码一次性（VerifyCodeWithKeyConsume）、G3 重置明文（GenerateRandomCharsKey(16)）、S1 流式泄漏、S2 无界读、S7 索引、Web 防护内网来源豁免。
- ✅ D3 去敏回归：middleware/audit_sensitive_test.go TestAuditLogSensitiveFieldsAbsent PASS（模型字段 + op params + raw token 三重断言），覆盖所有审计事件构造点。
- ✅ **G1 API Key 明文查看 step-up（v1.3.46 已闭环）**：新增 `token.key.read` scope（单条 `{token_id}` / 批量 `{token_ids}` 两种严格上下文，批量 id 去重排序保证哈希稳定）；`POST /api/token/:id/key` 与 `POST /api/token/batch/keys` 均已挂 `SecureTokenKeyVerificationRequired` / `SecureTokenKeysBatchVerificationRequired` 安全验证中间件；前端 keys 页 / 仪表盘复制 curl / chat 链接 / chat preset 四处明文披露路径全部走 step-up 对话框；OWASP 参考：ASVS V2/V3 重认证（re-authentication）要求 + Authentication Cheat Sheet Step-up；回归测试：service/auth_token_test.go（单条/批量上下文绑定、防跨 token 重放、批内去重超限）、controller/security_enrollment_test.go（绑定表 + 无因子拒绝）、web keys/hooks 测试。证据：e2e-evidence/v1.3.46-t8-g1-token-key-stepup.json。
- ✅ **G4 邮箱枚举（v1.3.46 已闭环）**：`SendEmailVerification` 对已注册/未注册邮箱返回完全一致的成功响应；已注册邮箱不生成/不落库/不发送验证码；发送失败仅记录日志不回传（响应差异不泄露注册状态）。回归：controller/misc_reset_password_test.go TestSendEmailVerificationAntiEnumeration（响应体逐字节一致 + 已注册邮箱无码）。
- ✅ **G5 验证码存储 Redis 化（v1.3.46 已闭环）**：common/verification.go 由纯内存扩展为「Redis 优先（TTL 生效期 + Lua 原子一次性消费）、内存兜底」；Redis 不可用时行为与旧版完全一致。回归：common/verification_test.go（内存路径 + 真实 Redis 路径 TTL/消费/删除）。
- ⏳ 仍为 Gap（诚实标注，不宣称合规）：无。
- 验证：go test ./service/ ./middleware/ -run "TestAuditLogSensitiveFieldsAbsent|TestAccountSecurity|TestSecurity" PASS；go test ./middleware/ -run TestAudit PASS。