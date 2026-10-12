# 认证/会话/凭据链路盲点扫描（只读审计）

- 审计员：只读安全审计员（未修改任何文件，未对生产发起任何请求）
- 日期：2026-10-12
- 仓库根：`new-api`（Go + React）
- 范围：登录（密码/OAuth/OIDC/Passkey/2FA）、会话（`UserSession`/JWT/refresh）、API token、step-up 再验证（`middleware/secure_verification.go`）、密钥管理（`CRYPTO_SECRET` / `SESSION_SECRET` / 渠道密钥加密）
- 依据：`AGENTS.md:49-55`（Authentication Security, OWASP Mandatory）。已实际抓取 OWASP Cheat Sheet 原文（Authentication / Session Management）作为条款出处，未凭印象。

---

## 1. 结论（一句话）

未发现可直接利用的越权（IDOR）、会话绕过或凭据明文落日志/响应；会话失效与资源归属校验链路整体扎实。发现 **2 条 P1**（登录族账号枚举、无账号级爆破防护）与 **5 条 P2**；其中 P1-1 在代码注释中被自认为「有意的可用性取舍」，但与 `AGENTS.md:53` 的强制 OWASP 合规要求直接冲突。

---

## 2. 问题清单

### P1-1 登录对「账号存在但未设密码」返回精确文案 → 账号枚举

- **证据**
  - `controller/user.go:101-111`：`ErrUserNoPassword` 分支返回 `i18n.MsgUserNoPasswordUseOAuth`（「请用 X 登录」），且带出 provider 名称；源码注释 `controller/user.go:104` 自述「这会透露『该账号存在且未设密码』」。
  - `model/user.go:1130-1134`：账号存在但 `Password == ""` 时单独返回 `ErrUserNoPassword`。
  - 对照：用户名不存在走 `controller/user.go:112-115` 的通用文案 `MsgUserUsernameOrPasswordError`。
- **影响**：以「用户名 + 任意密码」即可判定账号是否存在，并进一步推断其注册/登录方式（OAuth 供应商）。属于 OWASP 明令禁止的账号枚举（user enumeration）。
- **复现方式**
  1. `POST /api/user/login`，body `{"username":"<仅 OAuth 注册的账号>","password":"x"}` → 响应 message 含 provider 引导文案。
  2. 换成不存在的用户名 → 响应 message 为通用「用户名或密码错误」。
  3. 两者文案不同即构成枚举判定因子。
- **建议修法**：统一返回通用文案；「用什么方式登录」的引导改到认证成功路径或在注册邮箱/账号设置页下发，不在失败响应里体现。
- **OWASP 条款**：Authentication Cheat Sheet → *Authentication and Error Messages / Authentication Responses*。原文：「An application should respond (both HTTP and HTML) in a generic manner. ... an application must respond with a generic error message regardless of whether: The user ID or password was incorrect. The account does not exist. The account is locked or disabled.」

### P1-2 登录/2FA 限流仅按 IP，无账号级失败计数与锁定

- **证据**
  - `middleware/rate-limit.go:232-241`：`LoginRateLimit` 以 `c.ClientIP()` 为限流键。
  - `setting/operation_setting/login_rate_limit_setting.go:32-39`：默认 `20 次 / 1200s`，**每 IP**。
  - `controller/user.go:186-204`：失败登录只写审计日志，不做计数、不锁定。
  - `router/api-router.go:91-97`：`/login`、`/login/2fa`、`/login/verify`、`/login/passkey/*`、`/passkey/login/*` 全部只挂 `LoginRateLimit()`（IP 桶）。
  - 对照：2FA 有账号级 `locked_until`（`model/twofa.go:92-104`），**密码登录无对应机制**。
- **影响**：攻击者用代理池/僵尸网络轮换出口 IP 时，可对单一账号进行无上限密码爆破；2FA 与 passkey 登录端点同样只受 IP 桶约束。
- **复现方式**：以不同出口 IP 轮流 `POST /api/user/login` 同一用户名。每个 IP 在 20 分钟内打满 20 次后换新 IP，服务端永远不会返回 429，失败次数无上限累积。
- **建议修法**：新增以 `userID/username` 为键的失败计数与指数退避锁定（`model/twofa.go` 已有可复用范式）；锁定仅作用于该账号的密码因子并触发告警，避免被用作针对他人账号的 DoS。
- **OWASP 条款**：Authentication Cheat Sheet → *Account Lockout*。原文：「The counter of failed logins should be associated with the account itself, rather than the source IP address, in order to prevent an attacker from making login attempts from a large number of different IP addresses.」

### P2-3 登录时序侧信道（账号存在性）

- **证据**：`model/user.go:1123-1135`。账号不存在时在 `model/user.go:1125-1127` 直接返回，**跳过** `common.ValidatePasswordAndHash`（bcrypt/argon2 高昂计算）；账号存在时才在 `model/user.go:1135` 执行。
- **影响**：响应耗时差异可区分「账号不存在」与「密码错误」，构成枚举旁路（即使文案已统一）。
- **复现方式**：对不存在用户名与存在用户名各发若干次 `POST /api/user/login`，比较服务端处理时长分布（存在账号的分支显著更慢）。
- **建议修法**：账号不存在或无密码时，先对固定 dummy hash 执行一次 `ValidatePasswordAndHash` 再返回，拉平时延。
- **OWASP 条款**：Authentication Cheat Sheet → *Authentication and Error Messages* 末段。原文：「It is interesting to note that the business logic itself can bring a discrepancy factor related to the processing time taken.」

### P2-4 注册接口泄露用户名是否已存在

- **证据**：`controller/user.go:328-336` → `i18n.MsgUserExists`。邮箱已实现防枚举统一响应（`controller/user.go:311-319`、`342-352`），用户名路径未做，二者策略不一致。
- **影响**：可通过注册接口枚举已存在的用户名。
- **复现方式**：`POST /api/user/register` 使用已存在用户名 + 未注册邮箱 → 返回「用户已存在」；换未使用用户名 → 走向成功路径。响应可区分。
- **建议修法**：与邮箱路径对齐——统一成功响应 + 异步通知，或至少不在同步响应中区分。
- **OWASP 条款**：Authentication Cheat Sheet → *Authentication Responses*。原文：「The account registration feature should also be taken into consideration, and the same approach of a generic error message can be applied regarding the case in which the user exists.」

### P2-5 改密/重置密码后 dashboard PAT（access_token）仍长期有效

- **证据**
  - 改密只递增 `AuthVersion` 并撤销 `UserSession`：`model/account_security.go:50`（`IncrementUserAuthVersionWithTx`）；`service/auth_session.go:202-217` → `model/user_session.go:720-782`（仅撤销 `user_session` 表）；重置密码路径 `model/user.go:1252-1277`。
  - PAT 鉴权不比对 `AuthVersion`：`middleware/auth.go:174-186` 仅按 `access_token` 查库后取 `GetUserCache`；`model/user.go:1293-1309`（`ValidateAccessToken` 只按 token 列匹配）。
  - `access_token` 列不被上述流程清除，且无 TTL：`model/user.go:139-140`；`model/audit_log.go:215-223` 中的 `access_token_created_at` 仅用于展示，未参与过期判定。
- **影响**：攻击者一旦持有 PAT（如从浏览器/桌面端取出），「改密码」这一标准恢复动作不会将其踢出，访问持续有效直至用户主动撤销。
- **复现方式**（静态可判，未实机执行）：取一个账号的 PAT → 调用改密接口成功 → 之后仍用该 PAT 请求 `AdminAuth/UserAuth` 保护的接口 → 仍鉴权通过。
- **建议修法**：在密码变更/重置、2FA 开关、角色变更处调用现成的 `model/user.go:210-225 RevokeUserAccessToken`；或为 PAT 引入 TTL（`access_token_created_at` 已具备字段基础）。
- **OWASP 条款**：Session Management Cheat Sheet → *Renew the Session ID After Any Privilege Level Change*。原文：「The session ID must be renewed or regenerated by the web application after any privilege level change ... Common scenarios to consider include; password changes, permission changes, or switching from a regular user role to an administrator role. ... any previous session IDs must be ignored, only the current session ID must be assigned to every new request received for the protected resource, and the old or previous session ID must be destroyed.」
  - ASVS 具体条款编号（V2.2.x / V3.5.x / V3.7.x）**未定位**，见 §4。

### P2-6 `CRYPTO_SECRET` 未显式配置时，渠道密钥加密不可逆（数据可丢失）

- **证据**
  - `common/init.go:67-71`：`CRYPTO_SECRET` 为空时回落 `CryptoSecret = SessionSecret`。
  - `common/constants.go:36`：`SessionSecret = uuid.New().String()` 是**进程启动时随机**生成；`common/init.go:59-66` 只打 warning，不中止。
  - `common/channel_key_crypto.go:43`：`deriveChannelKeyAESKey()` 用 `CryptoSecret` 派生 AES 密钥；`common/channel_key_crypto.go:60-78` `EncryptChannelKey` 无任何密钥稳定性守卫。
  - 写入路径：`model/channel.go:452-455`（保存渠道即加密）。
  - 唯一防护 `model/channel_key_migration.go:89-92` **仅打日志，不阻断启动/不拒绝加密**。
- **影响**：`CHANNEL_KEY_ENCRYPTION=true` 且 `CRYPTO_SECRET`/`SESSION_SECRET` 均未配置时，重启后派生密钥改变，库中 `enc:v1:` 渠道密钥**永久不可解密**（不可逆数据损失，比同源讨论中的「访客统计失效」严重得多，且直接导致全渠道 401）。注意 `common/channel_key_crypto.go:28-33` 已明确：关闭开关对已加密库不是安全降级而是服务中断。
- **复现方式**（静态可判）：不设 `CRYPTO_SECRET`/`SESSION_SECRET`，开 `CHANNEL_KEY_ENCRYPTION=true`，保存任一渠道 → 重启进程 → 渠道鉴权全部失败且密钥无法恢复。
- **建议修法**：加密开启且 `CryptoSecret` 不稳定（空或等于随机 `SessionSecret`）时，启动即 `log.Fatal`，对齐 `common/init.go:52-55` 对 `SESSION_SECRET=random_string` 的既有先例。
- **OWASP 条款**：Cryptographic Storage Cheat Sheet（关键管理/密钥持久性）方向；未抓取该页原文，条款名标注为待验证。

### P2-7 支付 webhook 缺密钥时全量 body 落日志

- **证据**：`controller/topup_creem.go:38-42`。密钥为空时 `logger.LogWarn(..., fmt.Sprintf("Creem webhook secret 未配置 test_mode=%t signature=%q body=%q", ...))` 把**完整 webhook body** 写入日志。
- **影响**：webhook body 含支付订单/用户关联信息；与 `AGENTS.md:54`「Authentication audit events MUST exclude passwords, verification codes, recovery codes, private keys, and usable session or authentication tokens」同源风险（日志中出现原始凭据材料）。
- **复现方式**：未配置 `CREEM_WEBHOOK_SECRET` 时向 `/api/creem/webhook` 发送任一带 `body` 的请求，观察后端日志出现完整 body。
- **建议修法**：仅记录 body 长度与签名前缀（如前 8 位），不落原文；或仅在 DEBUG 下记录。
- **OWASP 条款**：Logging Cheat Sheet（敏感数据不入日志）；未抓取原文，标注为待验证。

---

## 3. 核对过并确认正确的

- **会话失效链路**：`service/auth_session.go:132-152`（`ValidateLoginSession`）同时比对 `session.Version`、`session.UserAuthVersion`、`user.AuthVersion` 与账号状态。封禁/降权/删除经 `authChanged → IncrementUserAuthVersionWithTx → RevokeAllUserSessions`：`model/user.go:851-871`、`model/user.go:916-960`、`controller/user.go:794`、`controller/user.go:1331`、`controller/user.go:1343-1348`。未找到「原有 web session 与 refresh token 在安全变更后仍有效」的路径。
- **Refresh 轮换与重放**：`service/auth_session.go:250-270` 一次性轮换 + 重放检测（`ErrUserSessionRefreshReuse`）+ 宽限窗（`RefreshReplayWindow`），并绑定 SID（`service/auth_session.go:224-226`）。
- **凭据不入审计/日志**：
  - 契约 `model/audit_log.go:72-73`「raw URLs, query strings, credentials and response/request bodies must never enter this table」。
  - token 审计只记 id/name/count/returned_ids：`controller/token.go:190-220`、`controller/token.go:521-570`。
  - PAT 以 SHA-256 指纹入库：`model/audit_log.go:61-70`。
  - 登录失败审计不含密码：`controller/user.go:186-204`。
  - GORM 日志开启参数化并脱敏驱动错误：`model/gorm_logger.go:36-52`、`model/gorm_logger.go:66-96`。
  - 密码重置不回落明文临时密码到响应：`controller/misc.go:526-536`。
- **资源归属校验**：token 读取/批量读取/批量删除均带 `user_id` 过滤：`model/token.go:282-289`、`model/token.go:495-500`、`model/token.go:467-486`。任务/SSE/产物：`controller/task.go:234-250`、`controller/task_event.go:61-70`；匿名产物能力凭证先验签再读库（`middleware/task_artifact_access.go:183-200`）。
- **OAuth/OIDC**：state 存服务端 `AuthFlow` 并绑定 provider/intent；bind/verify 强制发起时的 session，且需 step-up proof：`controller/oauth.go:45-113`、`controller/oauth.go:158-205`。一次性消费为原子事务：`model/auth_flow.go:240-277`。
- **限流覆盖**：注册、重置密码、邮箱验证码、OAuth 回调、`/api/verify` 均有独立限流（`router/api-router.go:56-97`、`router/api-router.go:83`；`middleware/email-verification-rate-limit.go:10-14`）。
- **step-up 再验证**：`middleware/secure_verification.go:144-193` 统一校验 session identity + `X-Security-Proof`，区分 EXPIRED / SCOPE_MISMATCH / CONSUMED / CONTEXT_MISMATCH，错误码完备。
- **前端凭据存储**：access token 仅存内存 zustand（`web/src/stores/auth-store.ts:118`），refresh 走 HttpOnly + `SameSite=Strict` + 受限 `Path=/api/user/auth` 的 cookie（`service/auth_session.go:310-329`）；CSRF 由 `middleware/auth_origin.go:13-18` 守卫。
- **注册邮箱防枚举**：`controller/user.go:311-319`、`controller/user.go:342-352`、`controller/misc.go:444-473` 均已做统一响应。
- **忘记密码响应**：`controller/misc.go:474-506` 对「已注册/未注册」返回完全一致的 `200 {"success":true}`。

---

## 4. 未能验证的及原因

1. **ASVS 具体条款编号**（如 V2.2.x / V3.5.x / V3.7.x）：本会话无 WebFetch 工具，只能以 `node -e` + `https` 抓取 OWASP Cheat Sheet 页面（Authentication / Session Management 已成功取到原文并引用）；ASVS 文档未抓取，故上文只引用 Cheat Sheet 章节名，ASVS 编号一律标注为待验证。
2. **PAT 无 TTL 是否为书面决策**：未在 `docs/` 穷尽检索，无结论；P2-5 只陈述事实（改密后旧 session 失效、旧 PAT 不失效）。
3. **`/api/user/login/encryption-key`（`router/api-router.go:90`）与各支付 webhook 端点无独立限流**：仅受 `GlobalAPIRateLimit`（默认 360 次 / 180s / IP，`common/init.go:160-162`）兜底。是否可接受取决于部署形态，未实测其滥用成本。
4. **本机 curl 全局无法 HTTPS**：所有 HTTPS 探测均通过 `node -e` 完成；未对生产发起任何攻击性请求，P1/P2 的「复现方式」均为静态推导，未在运行实例上执行。
5. **Cryptographic Storage / Logging Cheat Sheet 原文**：未抓取，P2-6、P2-7 的条款出处为方向性引用，未逐字核对。
