# 前后端契约 & 错误人话审计（只读子代理报告）

- 审计时间：2026-09-20
- 仓库：C:\Users\Administrator.DESKTOP-EGNE9ND\Desktop\公益服务网关\new-api
- 范围：只读审计（不改源码、不 commit/push）
- 抽查对象：usage-logs（list/stat/traffic）、认证/登录、token(密钥) 创建与列表、channel 增删改查、user 信息、错误人话映射
- 结论速览：**契约漂移 1 个（低危）** + **错误映射缺口 7 个（硬缺口 3 / 死模式 1 / 低危盲区 3）** + **usage-logs/user 缺契约测试**

---

## 一、契约防坑抽查（前后端字段一致性）

### 1.1 usage-logs 主链路：一致（无漂移）

| 前端（web/src/features/usage-logs） | 后端 | 字段 | 结论 |
|---|---|---|---|
| `api.ts:49-58` getLogsTraffic → `GET /api/log/traffic?days=30` | `controller/log.go:166-192` GetLogsTraffic（days 钳制 1..90） | days / total_requests / total_bytes / total_mb / by_day[] | 一致 |
| TrafficDaily `{date,requests,bytes,mb}`（api.ts:31-37） | `service/log_traffic.go:31-37` DailyTraffic `{date,requests,bytes,mb}` | 字段名一致 | 一致 |
| `api.ts:101-102` getAllLogs → `GET /api/log` | `controller/log.go:16-41` GetAllLogs | items/total/page/page_size（common/page_info.go:9-17） | 一致 |
| `api.ts:104-106` getUserLogs → `GET /api/log/self` | `controller/log.go:44-62` GetUserLogs | 同上分页结构 | 一致 |
| `api.ts:108-109` getLogStats → `GET /api/log/stat` | `controller/log.go:106-131` GetLogsStat | quota/rpm/tpm/concurrent_requests/completed_last_minute | 一致（types.ts:282-290） |
| `api.ts:111-113` getUserLogStats → `GET /api/log/self/stat` | `controller/log.go:133-155` GetLogsSelfStat | 仅 quota/rpm/tpm；前端 self 视图不渲染并发/完成徽章（common-logs-stats.tsx:126-143 仅 isAdmin 渲染） | 设计如此，无漂移 |
| 查询参数 p/page_size/type/model_name/token_name/group/channel/username/request_id/upstream_request_id/start_timestamp/end_timestamp | GetAllLogs 读取项（log.go:17-27）、GetUserLogs（log.go:47-54）、GetLogsStat（log.go:107-114） | 一致（buildQueryParams 过滤 undefined/null/''，lib/query-params.ts:8-20） | 一致 |
| type 枚举：LOG_TYPE_FILTERS（constants.ts:95-102,110-122） | `model/log.go:85-92` LogTypeTopup=1…Login=7 | 0=全部、1..7 全对齐 | 一致 |

### 1.2 认证/登录：一致（无漂移）

- 前端 `auth/api.ts`：`POST /api/user/login?turnstile=`、`POST /api/user/login/2fa`、`POST /api/user/auth/logout`、`GET /api/user/login/encryption-key`、密码加密字段 password_encrypted/encryption_key_id（auth/api.ts:47-81）。
- 后端 `controller/user.go:29-33` LoginRequest{username,password,password_encrypted,encryption_key_id}，Login 逻辑（user.go:53-105）与 `router/api-router.go:76-85` 完全对应。
- logout 的 `X-Auth-Session` 头 / 409 AUTH_SESSION_MISMATCH 恢复（auth/api.ts:99-133）与后端 session 语义一致（本审计不深挖安全细节，属认证专项）。

### 1.3 token（密钥）创建与列表：路径/参数一致（2 个兼容性注意项，非数据漂移）

| 前端 `features/keys/api.ts` | 后端 `router/api-router.go:272-285` + `controller/token.go` | 结论 |
|---|---|---|
| `GET /api/token/?p=&size=`（api.ts:39-41） | GetAllTokens（token.go:130-143）→ GetPageQuery | **N1**：`size` 仅靠 page_size→ps→size 兜底（common/page_info.go:61-63），依赖隐式兼容，非公开契约字段 |
| `POST /api/token/`（api.ts:76） | AddToken | 一致 |
| `PUT /api/token/` body{id,...}（api.ts:84） | UpdateToken（token.go:383） | 一致 |
| `PUT /api/token/?status_only=true` body{id,status}（api.ts:105-108） | UpdateToken `status_only` 分支（token.go:391,423-425） | 一致 |
| `DELETE /api/token/:id/`（api.ts:90） | DeleteToken `/:id` | **N2**：尾斜杠 → Gin RedirectTrailingSlash 307 多一跳（行为可用，依赖 Gin 默认配置） |
| `POST /api/token/:id/key`、`POST /api/token/batch`、`POST /api/token/batch/keys`、`GET /api/token/auto-groups`、`GET /api/token/:id`、`GET /api/token/search?keyword&token&p&size` | GetTokenKey/DeleteTokenBatch/GetTokenKeysBatch/GetTokenAutoGroups/GetToken/SearchTokens | 全部一致 |
| ApiKeyFormData（含 rate_limit JSON 串） | model/token.go:35 RateLimit json:"rate_limit" | 一致 |

### 1.4 channel 增删改查：一致（无漂移）

- 前端 `features/channels/api.ts`（getChannels/searchChannels/getChannel/createChannel/updateChannel/deleteChannel/batchDeleteChannels/updateChannelStatus/batchUpdateChannelStatus/copyChannel/fixChannelAbilities/deleteDisabledChannels/getChannelKey/getTagModels/enableTagChannels/disableTagChannels/editTagChannels/fetchModels/deleteOllamaModel/testChannel/updateChannelBalance/fetchUpstreamModels/testAllChannels/updateAllChannelsBalance/getAllModels/getEnabledModels）
- 后端 `router/channel-router.go`（GET /、GET /search、GET /:id、POST /、PUT /、DELETE /:id、POST /batch、POST /:id/status、POST /status/batch、POST /copy/:id、POST /fix、DELETE /disabled、POST /:id/key、GET/PUT /tag*、POST /tag/enabled、POST /tag/disabled、POST /fetch_models、DELETE /ollama/delete、GET /test、GET /test/:id、GET /update_balance、GET /update_balance/:id、GET /fetch_models/:id、GET /models、GET /models_enabled）
- `UpdateChannel` 收 body{id,...}（channel.go:962 起）与前端一致；`SearchChannels` 读 keyword/group/model/status/type/id_sort/sort_by/sort_order/tag_mode/p/page_size（channel.go:277 起）与前端 SearchChannelsParams 一致。

### 1.5 user 信息：一致（无漂移）

- 前端 `features/users/api.ts`：GET/POST/PUT/DELETE `/api/user/`、GET `/api/user/search?keyword&group&role&status&p&page_size&sort_by&sort_order`、GET `/api/user/:id`、`/api/user/self`、PUT `/api/user/self`。
- 后端 `router/api-router.go:145-186`（adminRoute）+ selfRoute（91-143）+ `controller/user.go` GetAllUsers/SearchUsers（user.go:336-380 读 keyword/group/role/status/page/sort_by/sort_order）、GetUser（user.go:385）。
- usage-logs 的 `getUserInfo(userId)` → `GET /api/user/:id`（usage-logs/api.ts:115-120，user-info-dialog 使用）与 GetUser 一致；UserInfo 字段（usage-logs/types.ts）与 model/user.go:97-115 的 JSON 标签（display_name/used_quota/request_count/aff_code/aff_count/aff_quota）一致。

### 1.6 漂移发现（D1，低危）

**D1：管理员「只看自己（Only Mine）」视图下，渠道/Channel 过滤静默失效（task/drawing 日志）**
- 证据（前端）：`web/src/features/usage-logs/lib/utils.ts:145-149` buildBaseParams 无条件发送 `channel_id`（不按 isAdmin 收敛）；而 common 日志在 utils.ts:190-192 有 `isAdmin &&` 门控。管理员在「All」视图设置渠道后切到「Only Mine」，URL 上的 `?channel=` 保留 → task/drawing 请求带 `channel_id`。
- 证据（后端）：`controller/task.go:385-394` GetUserTask 的 queryParams **不读 channel_id**（仅 GetAllTask 于 task.go:378 读）；`controller/midjourney.go:306-313` GetUserMidjourney **不读 channel_id**（仅 GetAllMidjourney 于 midjourney.go:286 读）。
- 影响：管理员/带 URL 参数的纯用户看到的是「过滤了但实际没过滤」的静默 no-op 列表；普通用户无法经 UI 触发（渠道输入框仅 isAdmin 渲染，common-logs-filter-bar.tsx:445-462），影响低但真实存在。

### 1.7 兼容性注意项（不计入漂移数）
- **N1** token 分页用 `size` 兜底参数（见 1.3，page_info.go:61-63）。
- **N2** `DELETE /api/token/:id/`、`/api/user/:id/` 尾斜杠依赖 Gin RedirectTrailingSlash（keys/api.ts:90、users/api.ts deleteUser）。

---

## 二、错误人话映射覆盖率

### 2.1 前端已覆盖的分类（web/src/lib/server-error-message.ts）

A. 正则映射 FRIENDLY_ERROR_PATTERNS（server-error-message.ts:155-166，顺序=优先级）：
1. 额度不足（insufficient quota/balance/credit，或任意含 `quota`）
2. 密钥无效（key invalid / invalid api key / 401）
3. 限流（rate limit / 429）
4. 渠道冷却（cooldown / cooling down / is cooling）
5. IP 封禁（ip banned）
6. 模型不可用（model not found / no model / model unavailable）
7. 上游不可用（upstream / bad gateway / 502 / 503）
8. 内容安全（content filter / safety / moderation / prompt block）

B. 安全专用键 serverErrorMessageKeys（server-error-message.ts:26-104）：Telegram OAuth/Bind 系列、AUTH_INTERNAL_ERROR、SECURITY_VERIFICATION_*（失败/流程要求/锁定）、OAUTH_ACCOUNT_MISMATCH、2FA（code invalid/already enabled/not enabled/setup invalid）、PASSKEY_NOT_FOUND、AUTH_FLOW_INVALID、SECURITY_PROOF_*、AUTH_SESSION_LIMIT、AUTH_SESSION_ISSUANCE_LIMIT 等（safeServerErrorMessage 保护，不被人话正则覆盖）。

C. 后端机器可读 error.type/error.code 稳定枚举（relaykit/types/error.go:94-98：insufficient_quota/key_invalid/rate_limited/upstream_unavailable/content_filtered；B6-2 归一化在 controller/relay.go:458-492）→ 前端 `getFriendlyErrorMessage` 读取 response.data.error.type/code 命中 A。5 类有配套单测（web/src/lib/__tests__/friendly-error-mapping.test.ts）。

### 2.2 后端会产生、前端未映射/弱映射的场景（缺口）

| # | 后端错误场景 | 后端产出（证据） | 前端匹配 | 状态 |
|---|---|---|---|---|
| 1 | 额度不足（relay） | code `insufficient_quota`（service/billing_session.go:234） | 命中 `quota` 模式 | ✅ 覆盖 |
| 2 | 额度不足（业务 i18n） | `quota.insufficient` → zh「额度不足」/ en「Insufficient quota」（i18n/locales/zh-CN.yaml:116） | en 命中 / **zh 不命中** | 🟡 半覆盖 |
| 3 | 上游限流/速率限制 | middleware/rate-limit.go:140-150 type=rate_limited；web_protection 同（service/web_protection_tracker.go:200,221）；relay code rate_limited | 命中 `rate_limit`/`429` | ✅ 覆盖 |
| 4 | 速率限制（业务 i18n） | `rate_limit.reached` → zh「请求数限制…」/ en「request limit…」（zh-CN.yaml:206） | **都不含 "rate limit"** | ❌ 未映射（原文人话） |
| 5 | IP 封禁 | web_protection `ip_banned`（web_protection_tracker.go:200） | 命中 | ✅ 覆盖 |
| 6 | 账号封禁 | `auth.user_banned` → zh「用户已被封禁」/ en「User has been banned」（zh-CN.yaml:34） | 需 ip 上下文，不命中 | ❌ 未映射（原文人话） |
| 7 | 渠道冷却（cooldown） | 仅 service/channel_cooldown.go 内部 + `logger.LogWarn`（controller/relay.go:496）；**后端从不对客户端输出 cooldown/cooling down 文案** | 有 cooldown 正则（server-error-message.ts:160）但不可达 | ⚠️ 死模式 |
| 8 | 冷却/无渠道的实际症状 | `get_channel_failed` + zh「分组 X 下模型 Y 的可用渠道不存在（retry）」（controller/relay.go:392）；task 路径 `channel_no_available_key`（relay/relay_task.go:103） | **不命中任何模式** | ❌ 硬缺口 |
| 9 | 模型暂不可用 | code `model_not_found` / 「no available model for this request」 | 命中 `model[_ ]?not[_ ]?found`/`no.*model` | ✅ 覆盖 |
| 10 | 模型不可用（zh 业务） | zh「模型不存在/暂不可用」 | 不命中 | 🟡 半覆盖 |
| 11 | 密钥无效（relay 归一化） | 401 → code `key_invalid`（controller/relay.go:474） | 命中 | ✅ 覆盖 |
| 12 | token 无效（业务） | `token.invalid` → zh「无效的令牌」（zh-CN.yaml:45）；`auth.access_token_invalid`「access token 无效」（zh-CN.yaml:29） | 无 code、zh 不命中；serverErrorMessageKeys 无 access_token 条目 | ❌ 未映射（原文人话） |
| 13 | channel 域错误码保留 | processChannelError 对 channel error 不改写（controller/relay.go:462-470）：`channel:invalid_key`、`channel:response_time_exceeded`、`channel:param_override_invalid` 等（relaykit/types/error.go:52-65） | 正则方向不匹配（"invalid_key" 反向） | ❌ 硬缺口 |
| 14 | 内部技术码 | `pre_consume_token_quota_failed`（service/billing_session.go:202）、`count_token_failed`、`model_price_error`、`do_request_failed`、`read_response_body_failed`、`bad_response_status_code`、`empty_response`、`update_data_error`、`query_data_error`（relaykit/types/error.go:16-86） | 无任何 pattern | ❌ 硬缺口（playground 直出） |
| 15 | 上游不可用 | code `upstream_unavailable` / bad gateway / 502/503 | 命中 | ✅ 覆盖 |
| 16 | 内容安全 | code `content_filtered` / prompt blocked / moderation（controller/relay.go:486-491） | 命中 | ✅ 覆盖 |
| 17 | 会话/2FA/Passkey/Telegram/安全验证 | 专用 code（AUTH_SESSION_LIMIT 等） | serverErrorMessageKeys | ✅ 覆盖 |

### 2.3 缺口汇总（共 7）

- **硬缺口 3**：
  - G1 无可用渠道/模型（#8：get_channel_failed / channel_no_available_key，冷却的真实出口）
  - G2 channel 域保留码（#13：channel:invalid_key 等，后端明确不改写为 B6-2 枚举）
  - G3 内部技术码（#14：pre_consume_token_quota_failed 等，playground 会裸显示英文技术串）
- **死模式/对齐 1**：G4 前端 cooldown 正则（#7）后端不产出对应文案 → 与 G1 错位；建议把冷却症状映射挂到 G1 的 code 上。
- **低危盲区 3**：G5 zh 业务文案不触发任何模式（#2/#4/#10，原文已人话，回退可接受）；G6 access_token/token.invalid 无专门映射（#12，#12 部分）；G7 账号封禁（#6）。触发面：前端 getFriendlyErrorMessage 只认英文/code；后端 i18n 已按语言返回，故 zh 用户永远走不到人话映射（但回退文案本身人话，危害低）。

---

## 三、契约测试现状

### 3.1 已有（半契约/URL 级断言）

- `web/src/features/keys/components/__tests__/api-key-listing.test.tsx:403` 断言 `PUT /api/token/?status_only=true` 及 payload；`:437` 断言 `POST /api/token/7/key`。
- `web/src/features/keys/components/__tests__/api-keys-mutate-drawer.test.tsx:68-80` 断言 `GET /api/token/auto-groups` 与 `POST /api/token/`。
- `web/src/features/channels/hooks/__tests__/channel-key-disclosure.test.tsx:87-100` 断言 `POST /api/channel/123/key`。
- `web/src/features/dashboard/components/overview/__tests__/setup-guide.test.tsx:57` 断言 `GET /api/token/?p=1&size=10`。
- `web/src/lib/__tests__/friendly-error-mapping.test.ts` + `server-error-message.test.ts`：错误映射模式有单测（含 5 类 relay 枚举、cooldown、IP ban、model not found、zh? 无）。
- `web/src/features/usage-logs/components/__tests__/common-logs-stats.test.tsx:37-63`：mock `/api/log/traffic` 与 `/api/log/stat(+/self)` 响应体 → 渲染断言。**不校验请求参数，也不校验分页/字段契约**。

### 3.2 缺口（缺契约测试）

- `/api/log`、`/api/log/self` 列表：无 items/total/page/page_size 字段级断言；无对 p/page_size/type/start_timestamp/end_timestamp 等查询参数的断言。
- `/api/log/traffic`：by_day 的 date/requests/bytes/mb 结构无 schema 校验（组件测试仅用空 by_day）。
- user 列表/信息（`/api/user`、`/api/user/:id`）、channel 列表与 CRUD（除 key 外）无 wire-format 契约测试。
- 无「前端 DTO ↔ 后端 DTO」对照测试（无 zod/schema 守卫、无基于真实后端响应的 fixture）。
- 后端契约侧：`controller/relay_error_log_test.go`、`service/log_traffic_test.go` 只测后端自身，不覆盖前端字段期望。

> 结论：token/channel 有零星 URL 断言（半契约），usage-logs 与 user 缺契约测试；D1 这类「前端发、后端不收」的静默漂移正是因为没有字段级双向对照测试而漏网。

---

## 四、建议（最小落地，不作实施）

1. 给 usage-logs api 层补 1 个契约测试文件：断言 `/api/log`、`/api/log/self`、`/api/log/stat`、`/api/log/self/stat`、`/api/log/traffic` 的 URL/查询参数与响应字段（items/total/page/page_size、quota/rpm/tpm、by_day 字段）。
2. 修 D1：`buildBaseParams` 增加 isAdmin 门控（与 buildApiParams 对齐），或后端 self 分支补 channel_id 解析——二选一，避免静默 no-op。
3. 补 G1/G2/G3 映射：在 FRIENDLY_ERROR_PATTERNS 增加 get_channel_failed / channel:no_available_key / channel:invalid_key / pre_consume_token_quota_failed 等 code 模式（含「无可用渠道→请稍后/换模型」人话），并把 cooldown 正则（死模式）收口到 G1 的 code。
