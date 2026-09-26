# new-api 安全审计复核（OWASP ASVS V2/V3/V4）— 2026-09-26

> 范围：登录/找回防枚举、OAuth state、会话撤销传播、密钥披露、审计去敏、重放/过期。
> 结论：[已满足] 5 项核心 + [缺口] 4 项（2 中高 + 2 中），本批已修复其中 1 项（G2）。

## 一、逐项核查结论

### 1. 登录/找回失败响应防枚举 — 部分满足
- [已满足] 登录：controller/user.go:73,94 统一 `MsgUserUsernameOrPasswordError`；model/user.go:1078-1096 一律 `ErrInvalidCredentials`。
- [已满足] 找回发送：controller/misc.go:249-272 对不存在邮箱也返回 success:true。
- [缺口 G3] 重置成功返回明文临时密码（controller/misc.go:298,312），GenerateVerificationCode(12) 熵低于完全随机。
- [缺口 G4] 注册/邮箱验证区分「邮箱被占用」（controller/user.go:251,269；controller/misc.go:228）→ 枚举。

### 2. OAuth state 校验 — [已满足]
- controller/oauth.go:45-136 state 32 字节随机 + HMAC，明文不落库；TTL 10 分钟；
- 回调校验 purpose+provider+会话一致（oauth.go:157-193）；ConsumeAuthFlowWithAction 原子消费防重放（auth_flow.go:240-281）。

### 3. 会话撤销传播 — [已满足]
- JWT 携带 sid/uv/sv，校验比对 DB（auth_session.go:141,148）；改密/封禁触发 AuthVersion 递增 + RevokeAllUserSessions + Redis 缓存失效；
- refresh token 轮换 + 30s 重放检测窗口（auth_session.go:230-278）。

### 4. 密钥页面披露默认 — 部分满足
- [已满足] PAT 一次性披露：controller/access_token.go:24-49 仅创建时返回；前端一次性弹窗 + 卸载清 state。
- [缺口 G1] relay API key 明文查看无 step-up：POST /api/token/:id/key（controller/token.go:188-200）与 /batch/keys 未挂 SecureVerificationRequired（与 channel key 不对称）。

### 5. 审计事件去敏 — [已满足]
- model/audit_log.go:57-60 明确「raw URLs/credentials/bodies 永不入表」；middleware/audit.go:209 仅 allowlist 元数据；
- 只记成功/失败不做 body；PAT/token 只记指纹（audit.go:276-293）。

### 6. 重放/过期 — 基本满足
- [已满足] TOTP 30s + 失败 5 次锁 300s；备用码哈希 + CAS 单次；passkey sign_count；security proof 1 分钟 + 单次。
- [缺口 G2 - 本批已修] 邮箱验证码/重置链接非一次性：VerifyCodeWithKey 只检查不消费；注册路径成功后不 DeleteKey。
  → 已新增 VerifyCodeWithKeyConsume（校验即删除），controller/user.go 注册验证改用之（common/verification.go:73-92）。
- [缺口 G5] 验证码存进程内存 map（上限 10 条），多实例不共享——运维风险，建议 Redis。

## 二、缺口清单（按严重度）

| # | 严重度 | 项 | 状态 |
|---|--------|-----|------|
| G2 | 中高 | 验证码可重放（注册窗口） | ✅ 本批已修（VerifyCodeWithKeyConsume） |
| G1 | 中高 | API key 明文查看无 step-up | ⏳ Gap（需新增验证 scope + 前端弹窗） |
| G3 | 中 | 重置返回明文临时密码 | ⏳ Gap |
| G4 | 中 | 注册/邮箱验证存在性枚举 | ⏳ Gap |
| G5 | 低 | 验证码内存存储不跨实例 | ⏳ Gap（建议 Redis） |

## 三、未修复项说明（诚实标注，不宣称合规）
- G1/G3/G4/G5 未在本批修复：G1 涉及新增安全验证 scope + 前端交互（较大改动需独立评审）；
  G3/G4 涉及改重置/注册流程 UX（需独立授权）；G5 需 Redis 改造。
