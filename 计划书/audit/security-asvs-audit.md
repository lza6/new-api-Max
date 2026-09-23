# OWASP ASVS 安全审计报告（Security ASVS Audit）

> 生成：2026-09-24 · 只读审计（代码证据 文件:行号）+ 引用 ASVS 4.0.3 requirement id。
> 范围：认证（V2）、会话管理（V3）、访问控制（V4）、数据保护（V6）。
> 结论先行：**现有实现已覆盖大部分核心 ASVS 要求**；未发现「明文凭据落库/无防爆破/无
> 会话失效」级高危。剩余差距以 P1/P2 标注，不宣称完全合规（未做渗透测试/生产实测）。

## V2 认证（Authentication）

| ASVS | 要求 | 现状 | 证据 | 状态 |
|---|---|---|---|---|
| 2.1.1 | 密码使用自适应哈希（argon2id/bcrypt） | ✅ argon2id 默认 + bcrypt 兼容读 | common/account_password.go:44-54 | 通过 |
| 2.1.7 | 密码长度策略 | ✅ 8-128 校验 | model/user.go:94 | 通过 |
| 2.2.1 | 防暴力破解（限流+锁定） | ✅ 登录限流 + Turnstile | router/api-router.go:84-86、middleware/login_rate_limit | 通过 |
| 2.2.3 | 防账号枚举（统一响应） | ⚠️ 部分：登录失败统一文案由后端控制；需人工核验 | middleware/auth.go | 待核验 |
| 2.4.1 | 忘记密码防枚举+防爆破 | ✅ 找回限流 + Turnstile | router/api-router.go:50-51 | 通过 |
| 2.5.x | MFA/TOTP | ✅ TOTP + 备用码 rejection sampling | common/totp_test.go | 通过 |
| 2.7.x | OAuth/OIDC state 校验 | ✅ state 生成+校验 | controller/oauth.go:45,157 | 通过 |
| 2.8.x | WebAuthn/Passkeys | ✅ 已实现 | service/webauthn* | 通过 |

## V3 会话管理（Session Management）

| ASVS | 要求 | 现状 | 证据 | 状态 |
|---|---|---|---|---|
| 3.1.1 | 会话令牌不可预测 | ✅ 服务端生成随机 SID | model/user_session.go | 通过 |
| 3.2.1 | 会话 Cookie 安全属性 | ✅ Secure/HttpOnly/SameSite + OriginGuard | middleware/auth_origin.go、SESSION_COOKIE_SECURE | 通过 |
| 3.2.3 | 会话超时 | ✅ expires_at | model/user_session.go | 通过 |
| 3.4.1 | 会话撤销 | ✅ 登出/改密/会话管理撤销 | controller/user.go、auth_session_test.go | 通过 |
| 3.5.1 | 会话 ID 轮换 | ✅ refresh token 轮换 | service/*session* | 通过 |

## V4 访问控制（Access Control）

| ASVS | 要求 | 现状 | 证据 | 状态 |
|---|---|---|---|---|
| 4.1.1 | 服务端强制访问控制 | ✅ Casbin authz | service/authz/ | 通过 |
| 4.2.1 | 最小权限（角色） | ✅ RoleCommonUser/Admin/Root | common/constants.go | 通过 |
| 4.3.1 | 管理端点鉴权 | ✅ AdminAuth 403 测试（T3） | controller/web_protection_test.go | 通过 |

## V6 数据保护（Data Protection）

| ASVS | 要求 | 现状 | 证据 | 状态 |
|---|---|---|---|---|
| 6.1.1 | 传输加密（TLS） | ✅ 生产 Caddy HTTPS | docs/installation | 通过 |
| 6.2.1 | 敏感数据最小化存储 | ✅ 不存明文密码/可逆密钥；token 落指纹 | middleware/audit.go（Fingerprint） | 通过 |
| 6.3.1 | 审计日志去敏 | ✅ 不落请求体/密码/验证码/恢复码/可用 token | middleware/audit.go:108-178 | 通过 |

## 差距与建议

### P1（建议本批修复）
1. **登录/找回防枚举统一文案**：虽有限流，但需确认登录失败、用户不存在、密码错误的响应是否
   完全一致（ASVS 2.2.3）。建议补一个 controller 级测试锁定三态同文案。
2. **API 密钥部分披露**：默认是否隐藏完整 token（仅尾部可见）需确认 `features/keys` 披露面；
   子代理侦察确认「创建后仅显示一次 + revoke」已实现。

### P2（后续批次）
3. axe-core a11y 扫描（T7 C3）
4. 渗透测试/生产环境安全基线（需授权）
5. OAuth 绑定/解绑的 CSRF state 重放窗口复核（已有 state 校验，建议补重放测试）

## 结论
- 认证/会话/访问控制核心达标；审计去敏以「后端不写敏感字段 + token 指纹」实现，无显式 redact 管线
  但效果等价。
- 不宣称 ASVS 完全合规：未做渗透测试、生产 TLS 配置复核、以及 P1 项人工核验。