# Spec 003：热路径性能冲刺（T1）· 请求内用户缓存复用 + 4xx 透传回归 + 基准化

**Feature Branch**: `3-hotpath-perf-t1`
**Created**: 2026-09-23
**Status**: Draft
**基线**: HEAD `e138d945c` · VERSION v1.3.11
**依据**: 项目宪法（.specify/memory/constitution.md）§V 性能与低延迟、§VII 保守改动；AGENTS.md 计费/relaykit 硬规则。

## 1. 背景与目标（第一性）

网关本质是透明中转：**每请求固定开销目标 <10ms**（宪法 V）。
历史基准（workflow_status）显示单并发网关附加 ~19ms、并发 20/50 放大到数百 ms。异步 consume-log flusher 已移走日志 INSERT；热路径剩余**同步 Redis 往返**与同步 DB 写仍是瓶颈。

**审计结论（只读子代理 + 主控复核，2026-09-23）**：
- `GetUserSetting` 已 Redis 缓存（model/user.go:1323）——任务卡里「用户设置未缓存」的前提**已过时**，不重复造缓存。
- 每请求用户缓存被完整加载 **3 次**（每次 = Redis HGETALL + auth-version MGET = 2 往返）：
  1) `middleware/auth.go:457` TokenAuth → `GetUserCache`
  2) `service/billing_session.go:378` NewBillingSession → `model.GetUserQuota`
  3) `model/log.go` RecordConsumeLog/RecordErrorLog → `GetUserSetting`（仅为 `RecordIpLog`）
- 每次 `GetUserCache` = `cacheGetUserBase`(Redis HGETALL) + `getUserAuthVersionFloor`(Redis MGET)，行程 6 往返。
- 429 退避、4xx 透传**已正确**（`service/error.go:87` RelayErrorHandler 保留上游 StatusCode；`controller/relay.go:102/107` 最终用 StatusCode 写回）；**缺 4xx 透传回归测试**。

**目标**：用**请求内复用**（不改一致性、不动计费）消除第 2、3 处冗余加载；补 4xx 透传回归测试；建立可复现基准脚本与验证台账（避免重复跑同一批 SQL/慢查询测试）。

## 2. 约束（Constitution / AGENTS）
- **不得**改变计费数值：预扣 Lua、settle 差额写、quota_math 全不动。
- **不得**改动 Redis 缓存语义：仅复用请求上下文已加载的同一份缓存值（行为等价）。
- **不得**新增基础设施、不改 `docs/`；新基准脚本放仓库 `scripts/`。
- Go 风格遵守本项目 `AGENTS.md`（early return、`for i := range n` 等）；只用 `testify`。
- 前端本批不改（T1 无 UI 面）。结束前 `gofmt` + `go vet` + `go build` + 相关 `go test`。

## 3. User Stories（按优先级、独立可测）

### User Story 1 — 消除热路径冗余用户缓存加载（Priority: P1）
中转网关不向 auth 上下文已加载后又去 Redis/DB 重复读同一用户设置/额度。

**Why P1**: 每请求省 2~4 次 Redis 往返，直接降热路径同步 I/O，是宪法 §五的硬目标；行为完全等价，风险最低。

**Independent Test**: 构造 gin 上下文携带 `user_setting`/`user_quota` 缓存，断言 RecordConsumeLog/NewBillingSession 命中上下文（不触发 Redis/DB），可在 `go test ./model/ ./service/` 独立验证。

**Acceptance Scenarios**:
1. Given 上下文有 `ContextKeyUserSetting`（记录 IP开关=true）且 DB 无该用户，When 调用 `userSettingRecordIp(c, userId)`，Then 返回 true（证明走上下文缓存而非 DB 回退——DB 回退 id 不存在返回 false）。
2. Given 上下文有 `UserSetting`（false），When 同上，Then 返回 false。
3. Given 上下文有 `UserQuota=int`，When NewBillingSession wallet 路径，Then 不做第二次 `GetUserQuota` Redis 读（上下文无权值时回退原逻辑，行为不变）。

### User Story 2 — 上游 4xx 状态码透传回归测试（Priority: P2）
网关对上游 4xx（400/401/403/404/429）原样透传客户端，不被包装 500。

**Why P2**: 行为已正确，缺回归锁；防未来重构回归。

**Independent test**: `go test ./service/ -run RelayErrorHandler` 表驱动断言每个 4xx 的 StatusCode 原样。

**Acceptance Scenarios**:
1. Given 上游非 200（400/401/403/404/429）且 body 为 OpenAI/通用错误，When RelayErrorHandler，Then 返回 StatusCode == 上游状态（≠500）。

### User Story 3 — 可复现基准 + 验证台账（Priority: P2）
脚本只跑一次，记录相关；让下次不再盲目重跑同批 SQL/基准。

**Why P2**: 提升后续迭代效率与可审计性（宪法 §一 证据化）。

**Independent test**: 脚本 `scripts/bench-latency.ps1` 生成 JSON；台账记录范围与结论。

**Acceptance Scenarios**:
1. Given 本地网关连通 mock 上游，When 跑脚本，Then 输出直连 vs 网关对照 JSON 到 `计划书/e2e-evidence/`。
2. 验证台账写明本次已验证项与结论，避免重复。

## 4. 开放问题（不阻塞，记录）
- Q1 预算异步预扣（pre-consume）为 L3 计费改造，需单独批准——本批不实施（列为 Non-Goal）。
- Q2 本批是否顺带把统计写（used_quota/request_count/channel used_quota）改默认批量？——成本较高，本批不实施，列为 P2 建议（写入 checkpoint 台账）。
- Q3 生产 Redis 往返实测是否能测：本机 Redis 可用；线上需用户授权。

## 5. 非目标
- 不动异步 pre-consume；不改 quota 结算写；不引入 Kafka/新队列；
- 不做三库 schema 变更（无 DB schema 改动）；不做前端改动。