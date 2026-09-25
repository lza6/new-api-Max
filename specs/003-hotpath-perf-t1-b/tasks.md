# Tasks：热路径性能冲刺（T1 二期）

**Prerequisites**: spec.md（本目录）· 基线 HEAD aced9a37d
**Organized by user story**（可独立实现/测试）。

## US1 — 渠道运行时快照预计算
- **[T1-B1] model/channel_cache.go**：新增 `ChannelRuntimeSnapshot` 结构 + `buildChannelRuntimeSnapshotsLocked` + `CacheGetChannelRuntimeSnapshot`；`InitChannelCache` 锁内重建。
- **[T1-B2] middleware/distributor.go** `SetupContextForSelectedChannel`：改用快照（Setting/Other/Param/Header/ModelMapping/StatusCodeMapping/AutoBan/BaseURL），快照 nil 时回退原方法。
- **测试**：`model/channel_cache_runtime_test.go` TestChannelRuntimeSnapshotParsedOnce（已绿）。

## US2 — 冷却恢复索引修复
- **[T1-B3] model/channel_cache.go** `CacheUpdateChannelStatus`：启用分支调 `rebuildGroupIndexesLocked`（按优先级排序）；非启用保留删除逻辑。
- **测试**：TestCacheUpdateChannelStatusReenableRestoresIndex（已绿）。

## US3 — 健康分秒级快照缓存
- **[T1-B4] service/channel_health_score.go**：`GetChannelHealthSnapshot` 1s TTL 缓存包装；新样本/冷却事件/冷却清除写入置空缓存。
- **[T1-B5] service/channel_cooldown.go**：`recordChannelCooldown`/`clearExpiredChannelCooldowns` 写后置空健康快照缓存（锁序：先释放 cooldown 锁再取 health 锁）。
- **测试**：`service/channel_health_score_test.go` 新增缓存失效/TTL 用例（已绿 7/7）。

## US4 — 真实基准 + 慢 SQL 台账
- **[T1-B6] 基准**：`scripts/bench-latency.ps1`（pwsh 7 跑，含并发）；真实 mock 上游 + 网关 + 渠道/定价/token。
- **[T1-B7] 台账**：`计划书/audit/perf-verification-ledger.md` 追加记录 0011（优化后数字 + 慢 SQL 清单）。

## 质量门（真实运行）
- [x] `go build ./model/ ./service/ ./middleware/` exit 0
- [x] `go test ./model/ -run TestChannelRuntimeSnapshot|TestCacheUpdateChannelStatusReenable` PASS
- [x] `go test ./service/ -run TestChannelHealth` PASS（7/7）
- [ ] 全量 `go vet ./...` / `go test ./...` / relaykit `GOWORK=off` 独立构建（进行中）
- [x] 真实 E2E：网关 3000 → 渠道 → mock 上游 18080 转发成功
- [x] 基准 JSON 到 `计划书/e2e-evidence/`（3 份，最终 014749）

## 诚实边界
- 10ms 目标：warm 连接池下 overhead_p50=-28.27ms（达成）；冷连接/限流干扰轮次如实记录，不挑数。
- 剩余同步 DB 写（token/user/channel/log 账本写）不缓存，属 T1-B 预扣异步化授权批。
- 不声称改动计费语义（本批零计费变更）。
