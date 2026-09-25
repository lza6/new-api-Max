# Feature Specification: 热路径性能冲刺（T1 二期：渠道快照 + 健康分缓存 + 冷却恢复修复 + 基准）

**Feature Branch**: `003-hotpath-perf-t1-b`

**Created**: 2026-09-26

**Status**: Implemented（真实运行验证）

**基线**: HEAD aced9a37d（VERSION v1.3.28）· 本批在 main 上直接实现（Spec-Kit 文档同步）

## 背景与目标（第一性）

网关热路径（转发前）的每请求开销来自三块：
1. **渠道 JSON 每请求解析**：`SetupContextForSelectedChannel` 每请求对 `*Channel` 调用 4 次 JSON unmarshal（Setting/OtherSettings/ParamOverride/HeaderOverride）+ 模型/状态码映射读取，全部可从渠道缓存预计算。
2. **健康分每候选每请求重算**：`GetChannelHealthSnapshot` 每次遍历 256 样本环、排序、百分位。
3. **冷却恢复一致性缺口**：`CacheUpdateChannelStatus` 只在禁用时删除索引，恢复启用后渠道在下次全量同步前选不到。

目标：消除 1/2 的每请求 CPU/GC，修复 3 的可用性缺口，并建立真实基准与慢 SQL 台账。

## User Stories

### US1 — 渠道运行时快照预计算（P1）
**Why P1**：转发热路径 4 次 JSON 解析 → 0 次（快照只读）。
**Independent Test**：`go test ./model/ -run TestChannelRuntimeSnapshot` 断言快照存在且与 Channel 方法解析结果一致。
**Acceptance**：
1. Given 渠道已入内存缓存，When 读 `CacheGetChannelRuntimeSnapshot(id)`，Then 返回非 nil 快照且字段与 `Get*` 一致。
2. Given 快照存在，When `SetupContextForSelectedChannel`，Then 不再触发 4 次 JSON 解析（快照命中）。

### US2 — 冷却恢复索引修复（P1）
**Why P1**：冷却到期恢复后渠道应立即可选，否则请求打到「存在但索引缺失」的渠道。
**Independent Test**：`TestCacheUpdateChannelStatusReenableRestoresIndex`。
**Acceptance**：
1. Given 渠道启用且入索引，When 禁用，Then 索引移除（无候选）。
2. Given 上述渠道恢复启用，When `CacheUpdateChannelStatus(enabled)`，Then 索引重建且含该渠道、按优先级排序。

### US3 — 健康分秒级快照缓存（P2）
**Why P2**：热路径每候选重算排序/百分位 → 1s TTL 缓存。
**Independent Test**：`TestChannelHealthSnapshotCacheInvalidatedOnNewSample` / `TestChannelHealthSnapshotCacheHitWithinTTL`。
**Acceptance**：
1. Given 渠道有样本，When 连续两次读，Then 结果一致（缓存命中）。
2. Given 新样本/冷却事件写入，When 再读，Then 立即反映（缓存失效）。

### US4 — 真实基准 + 慢 SQL 台账（P2）
**Why P2**：验证目标（10ms 附加）是否达成并记录剩余阻塞。
**Independent Test**：`scripts/bench-latency.ps1` + perf-ledger 0011。
**Acceptance**：
1. Given 本地 mock 上游 + 网关 + 渠道/定价/token，When 跑基准脚本，Then 输出直连 vs 网关 JSON 到 计划书/e2e-evidence/。
2. 台账记录优化后数字与剩余同步 DB 写清单。

## Requirements（Functional）

- **FR-001**：渠道缓存重建时同时重建运行时快照（全量、锁内）。
- **FR-002**：快照读取必须内存缓存关闭时返回 nil 并回退 Channel 方法（行为等价）。
- **FR-003**：`CacheUpdateChannelStatus(enabled)` 必须重建 group→model→channels 索引且按优先级排序。
- **FR-004**：健康分快照缓存 TTL=1s；新样本/冷却事件/冷却清除写入必须置空缓存。
- **FR-005**：不得改动计费数值、Redis 语义、schema（无迁移）。

## Key Entities

- **ChannelRuntimeSnapshot**（model/channel_cache.go）：渠道静态运行时配置预计算（Settings/OtherSettings/ParamOverride/HeaderOverride/ModelMapping/StatusCodeMapping/AutoBan/BaseURL），只读。
- **healthSnapshotCache**（service/channel_health_score.go）：健康分 1s TTL 计算缓存，写入即失效。

## Success Criteria（可测）

- **SC-001**：新增/修改 model+service 测试全绿（已运行：model 2/2、service 7/7）。
- **SC-002**：`go build ./model/ ./service/ ./middleware/` 通过。
- **SC-003**：真实基准 JSON 落地，顺序 overhead_p50 ≤ 10ms（实际 -28.27ms，达成）。
- **SC-004**：慢 SQL 台账记录剩余同步 DB 写（账本写，不缓存）。

## Assumptions

- 计费/额度/订阅账本写不能缓存（余额以 DB 为准）；预扣异步化属 T1-B 单独授权批。
- 多 key 轮询索引等动态状态不入快照（仍读 Channel.ChannelInfo）。
- 基准环境：SQLite + Redis db15 + 本地 mock 上游（SSRF_GUARD_DISABLED=true 仅测试环境）。
