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
- ⏳ 仍为 Gap（诚实标注，不宣称合规）：G1 API key 明文查看无 step-up（需新增 token.key.read scope + SecureVerificationRequired 挂载 + 前端弹窗，涉认证流程独立评审）；G4 邮箱枚举（改注册 UX）；G5 验证码内存存储（建议 Redis）。
- 验证：go test ./service/ ./middleware/ -run "TestAuditLogSensitiveFieldsAbsent|TestAccountSecurity|TestSecurity" PASS；go test ./middleware/ -run TestAudit PASS。