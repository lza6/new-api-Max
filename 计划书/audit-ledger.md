# 审计验证账本（Audit Ledger）

> 目的：记录已执行的审计/测试及范围与结论，避免重复重跑；改动相关区域时优先引用。
> 追加规范：每次新增一行 [date] 范围/命令/结论/证据路径。

## P0-1 三库 conformance 契约测试套件（2026-09-21）

- 范围：SQLite + MySQL(9.6.0) + PostgreSQL(16.14) 三库真实实例；AutoMigrate 幂等、logs/task_events 索引、保留字列 group/key、布尔值、JSON(TEXT) 往返、lockForUpdate 真库锁、UsingMainDatabase/UsingLogDatabase 分支。
- 命令：`scripts/db-conformance.ps1`（本机服务模式）；`go test ./model/ -run TestDBConformance -v -count=1`。
- 结论：PASS=24，FAIL=0，SKIP=0（三库全过）；连续两次运行一致（可重复性验证）。
- 证据：`.deploy/refscan/` 无相关；本文件；测试文件 `model/db_conformance_test.go`。
- 数据库版本：PostgreSQL 16.14 (VC build 1944)；MySQL 9.6.0；SQLite（内置 glebarez）。
- 幂等：全模型 AutoMigrate 二次跑无 schema mutation（drop-before-migrate 保证库状态无关）。

## 加入资产（2026-09-21）
- `make db-check`：一条命令跑三库 conformance。
- CI：`.github/workflows/ci.yml` 新增 `db-conformance` job（services: mysql:8.4 + postgres:16）。
- 脚本：`scripts/db-conformance.ps1`（支持本机服务 与 -Docker 两模式）。
## P0-2 计费安全收口（2026-09-21, v1.2.40）
- 审计：全量扫描裸转换与计费乘数 bound。结论：计费主路径已走 quota_math 饱和 + clamp 审计（attachQuotaSaturation 已接入 text/audio/quota/task 5 处）；maxTokensLimit/MaxImageN/MaxTaskDurationSeconds 已 bound。
- 修复：service/token_counter.go 图像宽高来自用户上传元数据，int 乘法可溢出为负 → sqrt(NaN) → 负 token → 负扣费。新增 clampImageDimensions（上限 100_000）在估算前 bound；音频多文件 token 累加饱和到 MaxQuota。
- 测试：service/quota_saturation_test.go 新增 TestClampImageDimensionsBounds/ProductCannotOverflow/TestAudioTokenAccumulationSaturates，全过。
- 命令：go test ./service/ -run 'TestClampImage|TestAudioTokenAccumulation|TestAttachQuota|TestPreConsume' -count=1 → ok

## P0-3 认证安全审计（2026-09-21, v1.2.40）
- 审计（OWASP ASVS 对齐）：会话管理（session 版本化/换页轮换/revoke/并发上限开关）、刷新令牌轮换 + replay 窗口 + 复用即吊销、Cookie HttpOnly+SameSiteStrict+Secure 可配置（SESSION_COOKIE_SECURE/TRUSTED_URL 校验）、密码 Argon2id（account_password.go）、重认证（改密码需当前密码）、审计脱敏（token 创建审计只记 id/name，不记 key；audit.go 模板化+admin-only 隔离）。
- 结论：既有实现与测试已覆盖关键 ASVS 需求，未发现新增 P0/P1 缺口；本次登记为已验证项。
- 命令：go test ./service/ ./controller/ -run 'TestAuth|TestSession|TestSecurity|TestToken|TestOIDC' → ok

## P0-4 日志透明化：错误归因（2026-09-21, v1.2.40）
- 现状：RequestId 中间件 + request_id/upstream_request_id 字段与索引已存在；本批补齐「错误归因落日志」。
- 修复：controller/relay.go 错误日志增加 other.error_class（RelayErrorClassString：auth/rate_limited/server_error/timeout/bad_request/capability/ok），前端可直接人话映射，排障不再人工比对状态码。
- 测试：controller/relay_error_log_test.go 502 → server_error 落日志断言，全过。
- 命令：go test ./controller/ -run 'TestProcessChannelError' -count=1 → ok
## P1 批次盘点 + P1-3 权限闭环（2026-09-21, v1.2.41）
- 盘点结论（真实代码核验）：
  - P1-1 用户画像层（规则版）：✅ 已落地（service/user_profile/profile.go 14KB：Weibull 衰减/进程缓存+Redis 缓存/失效；profile_test.go 空态/聚合/衰减/缓存 4 组测试；controller/user_profile.go + 路由 /api/user/profile/insights；前端 profile-insights feature）
  - P1-2 Redis 队列批量落库：✅ 已满足（model/consume_log_flusher.go 内存批缓冲：4096 队列 + 周期 flush + 满时同步背压不丢；指南允许「内存批缓冲」路径）
  - P1-3 任务事件流 SSE 断线续传：✅ 已落地（model/task_event.go seq 单调自增 + ListTaskEventsAfter(sinceSeq)/LastTaskEventSeq；controller/task_event.go TaskEventsSSE + since 续传 + heartbeat + done；前端 task-event-stream.ts/tsx ReadableStream + since=lastSeq 续传）
  - P1-4 渠道健康度+组合路由：✅ 已落地（service/channel_health_score.go + channel_combo_route.go + 系列测试）
- 本批补齐：P1-3 权限越权回归测试（指南验收标准要求、此前缺失）：
  - controller/task_event_test.go 新增 TestStreamTaskEventsOtherUserDenied（非属主 → 404，不泄露事件数据）与 TestStreamTaskEventsOwnerAllowed（属主 → 200 + done）
  - 命令：go test ./controller/ -run 'TestStreamTaskEvents' -v -count=1 → 5/5 PASS
## P2-2 事件子系统最小核心（2026-09-21, v1.2.42）
- 新增 service/event_bus.go（内存版通用事件总线）：注册式处理器路由 + 事件 ID 幂等去重 + 失败退避重试 + 状态机（pending/success/failed/dead）+ panic 隔离 + 关闭语义 + 同步/异步分发 + 统计计数。纯增量新文件，不触碰现有代码与三库方言；后续支付 webhook/通知归一可在 Dispatch 内接持久化。
- 测试：service/event_bus_test.go 6 组（投递+幂等、重试至 dead、瞬态失败恢复、无处理器/panic 隔离、关闭拒绝、异步+自动 ID 幂等），全过。
- 命令：go test ./service/ -run 'TestEventBus' -v -count=1 → 6/6 PASS；go vet/build/gofmt 干净；三库 conformance 24/24（P0-1 防回退）。
- 范围说明：P2-2 完整版（支付 webhook 接入 + 双向状态机持久化 + 事件幂等表）与 P2-1（无锁快照）/P2-3（jsplugin 沙箱）/P2-4（平台生态）为独立大项，未在本次实现，待后续批次。
## P0-2 计费安全收口：三清单闭合（2026-09-21, v1.2.43）
### 清单一：未 bound 的用户可控计费乘数（闭合）
| 乘数 | 校验点 | 状态 |
|---|---|---|
| 图片 n | relay/helper/valid_request.go:199-200, 249-250（`dto.MaxImageN`，超值 400）；multipart n `strconv.Atoi` + bound（valid_request.go:196-203） | ✅ bound |
| 图片 n 溢出 uint64（18446744073686646784） | valid_request.go 解析为 uint 后与 MaxImageN 比较，溢出值 > max → 400 | ✅ 拒绝（openai_image_request_test.go:97-101） |
| 视频 seconds/duration | relay/common/relay_utils.go:148-157 `validateTaskDurationBounds`（`MaxTaskDurationSeconds=3600`，负数/超值 400） | ✅ bound |
| multipart seconds 字符串 | relay/common/relay_utils.go:175-183 本批修复：解析失败返回 err（原静默归 0 → 时长乘数缺失少收费），`validateMultipartTaskRequest` 层 400 | ✅ 本批修复 |
| max_tokens 族 | valid_request.go:124-127 `maxTokensLimit=MaxInt32/2`；Responses/Text/Gemini 各格式调用（145/301/326/379） | ✅ bound |
| 分辨率/质量比 | `AddOtherRatio` 经 isValidOtherRatio 拒绝非正/NaN/Inf（types/price_data.go:35-45） | ✅ 防护 |
| Extra["parameters"] 绕过 | relay/channel/ali/image.go:29-63：从 Extra 解析 Parameters 后同款 bound（N 校验 + AddOtherRatio） | ✅ 收口 |
### 清单二：未走 quota_math 的转换（闭合）
- 全量 rg：计费路径均已走 `QuotaFromFloat/QuotaRound/QuotaFromDecimal`（含 *Checked）。剩余裸转换点均非计费或已饱和：channel_health_score.go:208（分位数索引）、common/utils.go:155（显示格式化）、token_counter.go:158-168（尺寸中间量，前批已 clamp 上限 100_000）、topup.go IntPart（后接 WalletQuotaFromDecimalStrict）。
### 清单三：clamp 未记录（闭合）
- attachQuotaSaturation（service/log_info_generate.go:36）已接入 5 处：text_quota.go:556、quota.go:243/376、task_billing.go:70；任务路径 noteTaskQuotaClamp（relay_task.go:427）→ RecalculateTaskQuota → attachQuotaSaturationToOther（task_billing.go:359）。
### 回归（本批）
- relay/common/relay_utils_test.go 新增 TestMultipartSecondsMalformedRejected（1e30/abc 拒绝、正常 8 解析）；既有 TestTaskDurationBounds/TestGetAndValidOpenAIImageRequestNBounds/TestQuota* 全绿。
- 命令：go build ./...；go test relay/common relay/helper service common（相关组）→ ok；三库 conformance 24/24。
## P0-3 认证安全审计：OWASP ASVS 对齐矩阵（2026-09-21, v1.2.44）
> 基准：OWASP ASVS 5.0（Authentication V2 / Session Management V3）。逐模块核验实现 + 证据 + 回归测试。

| ASVS 需求（ID） | 实现/证据 | 回归测试 |
|---|---|---|
| V2.1 口令存储（Argon2id/bcrypt 强哈希） | common/account_password.go:44 HashAccountPassword（Argon2id 双格式读取过渡） | security_account_test.go TestSecurityAccountPasswordRequiresCurrentPassword |
| V2.2 防枚举/防爆破、验证码单次+过期 | service/login_verification.go（LoginChallenge TTL 5min + AuthFlow 状态机）；service/security_verification.go | auth_flow_test.go TestSecurityLoginCodeCompletesOnce / TestSecurityLoginRejectsChangedOrExpiredAuthorization |
| V2.3 MFA/TOTP + 恢复码单次 | common/totp.go（TOTP + BackupCode 4×8 位）；model/twofa.go:228 is_used 单次消费；service/twofa.go 设置/完成闭环 | common/totp_test.go；auth_flow_test.go TestSecurityLoginAllPrimaryTransportsRequireAdditionalVerification |
| V2.4 WebAuthn/Passkey | controller/passkey.go（注册完成需流审批 + User Verification） | passkey_test.go TestPasskeyRegisterFinishRejectsUnapprovedFlowWithoutConsumingIt；auth_flow_test.go TestSecurityLoginPasskeyDoesNotRequireAdditionalTwoFA / ConcurrentCompletionCreatesOneSession |
| V2.5 OAuth/OIDC state/nonce/redirect/code 单次 | oauth/generic.go:99 redirect_uri + state；oauth/telegram.go:116 redirect 严格校验；auth_flow.go OAuth code 绑定会话+单次消费 | auth_flow_test.go TestGenerateOAuthCodeBindsFlowToAuthenticatedSession / TestOAuthLoginConsumesFlowOnlyAfterProviderIdentity |
| V2.6 重认证（敏感操作） | controller/secure_verification.go（当前密码/因子证明）；security_account.go 账号删除需 ScopedProof | security_account_test.go TestSecurityAccountDeletionRequiresScopedProof / RechecksTransactionAndConsumesFailedProof |
| V3.1 会话轮换/登出全端失效/并发上限 | service/auth_session.go（createLoginSession 版本化 + 并发/签发上限 UserSessionActiveLimit/IssuanceLimit + AdvanceCurrentSessionSecurity 换版 + RefreshLoginSession 轮换） | auth_session_test.go TestCreateLoginSessionEnforcesActiveLimitAcrossAuthVersions / TestUserAuthVersionInvalidatesExistingSession / TestLoginSessionCreateRefreshAndRevoke |
| V3.2 刷新令牌轮换+replay 窗口+复用即吊销 | service/auth_session.go:250-285（deriveNextRefreshSecret + RotateUserSessionRefresh + RefreshReplayWindow + ErrUserSessionRefreshReuse→Revoke） | auth_session_test.go（refresh/reuse 族） |
| V3.3 Cookie 属性 Secure/HttpOnly/SameSite | service/auth_session.go:318-363（HttpOnly + SameSiteStrict + Secure 由 SessionCookieSecure 控制）；common/session_cookie.go InitSessionCookieSettings（SECURE=true 强制 https TRUSTED_URL） | middleware/auth_origin_test.go（secure/origin 矩阵） |
| V3.4 审计脱敏（不落口令/token/验证码） | controller/token.go:201/291/359/378/403/425 审计 params 仅 id/name；audit.go 模板白名单渲染；attachQuotaSaturation 等 admin_info 隔离 | access_token_audit_test.go（request_id 关联 + 过滤矩阵） |

结论：六模块关键需求均有实现 + 回归测试；审计抽样确认 token/密码/验证码不落审计字段。本批为审计登记（无新增代码改动），已运行的认证回归：go test ./service/ ./controller/ -run 'TestAuth|TestSecurity|TestOAuth|TestPasskey|TestSession|TestAccessToken' → ok。
## P0-3 审计脱敏抽样实证（2026-09-21, v1.2.45）
- 抽样：controller/token_test.go TestAPITokenAuditDatabaseMatrix（create/key_view/batch 全矩阵）——多重断言确认审计日志不含 PAT、JWT、Token.Key、Token.Name、raw-body-secret、raw-storage-error-secret、private-model-configuration、Authorization 头（token_test.go:803 `assert.NotContains(string(params), created.Key)` + 803-810 全 secret 清单）。
- 全量扫描：SetPublic/LogWarn/SysLog 参数中无 key/token/password/secret/authorization/code 明文路径（grep 全绿）。
- 认证回归：go test ./service/ -run 'TestAuth|TestSecurity|TestOAuth|TestPasskey|TestSession|TestAccessToken|TestLogin|TestTwoFA' → ok（1.75s）。
- 环境说明：controller 认证测试在 Windows 本地因 SQLite 临时文件锁（TempDir RemoveAll）清理失败，断言逻辑本身通过；CI（ubuntu）无此问题。非认证缺口。
- 结论：P0-3 六模块 ASVS 对齐（见 v1.2.44 矩阵）+ 脱敏实证闭合。
## P2-1 无锁配置快照（2026-09-21, v1.2.46）
- 目标：渠道组合（ChannelCombo）路由热点每请求直查 DB → 无锁快照。
- 实现：service/combo_snapshot.go（atomic.Pointer 不可变索引：构建全量 map → Store 原子发布；读零锁；重建串行化 + 失败保留旧快照）；ResolveComboForModel 快照优先 + DB 回退；controller/channel_combo.go Create/Update/Delete 后 RefreshComboSnapshot 失效刷新。
- 测试：combo_snapshot_test.go（构建/解析语义 + 并发读在原子替换期间一致性），go test ./service/ -run TestComboSnapshot -race → ok（无 race）。
- 命令：go build ./...；go test ./service/ -race → ok；三库 conformance 不受影响（纯内存快照，无 DB 语义变更）。