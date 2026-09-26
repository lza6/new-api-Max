# 专项分析 · 性能热路径深挖与基准策略（T1 纵深）

> 定位：主指南 §T1 的**深挖文档**：根因、现状证据、基准策略、优化候选与诚实边界；只读整理。
> 生成：2026-09-25 · 锚点：perf-verification-ledger 0001-0010、`计划书/e2e-evidence/paired-latency-bench-v1.3.28.json`。

## 1. 根因与已落地（证据）
- 热路径每请求曾 3 次 `GetUserCache`（auth / NewBillingSession / RecordConsumeLog），每次 2 次 Redis 往返 → 已复用请求内 UserSetting/UserQuota（perf-ledger 0001）。
- 异步 consume-log flusher（`model/consume_log_flusher.go`）已落地；带宽/流量排行走 Redis 缓存。
- T1-3：`service/log_traffic.go:58-68` 流量聚合 `other.request_bytes/response_bytes` 改走 `common.QuotaFromFloat`（饱和），杜绝 int64 回绕负值（log_traffic_test.go:111-130 已锁）。
- 基准：`计划书/e2e-evidence/paired-latency-bench-v1.3.28.json` 诚实标注 BLOCKED（本机无 3000/3020/18080 监听、无生产二进制、无 SQLite db，未伪造数字）。目标 10ms 未达成，**待授权重建环境**（perf-ledger 0004 步骤：`SSRF_GUARD_DISABLED=true` 仅测试环境 + 公网 mock 上游）。

## 2. 基准策略（授权后）
1. 按 perf-ledger 0004 重建：生产二进制（`-ldflags -s -w`）+ SQLite + bench-model 渠道/定价/token + mock 上游（固定 30ms 延迟）。
2. `powershell -File scripts/bench-latency.ps1 -DirectUrl <mock> -GatewayUrl <gw> -Bearer <token> -N 60`，跑直连 vs 网关，记 min/p50/p90/p95/max + 并发 20/50。
3. 结果回填 perf-ledger 新记录，标注与 19ms 历史、10ms 目标差距；不达标不宣称完成。
4. 生产观测：线上 frt 记录（claude-haiku 3468ms、glm-5.3 4542ms）仅参考；需授权后线上复核。

## 3. 优化候选（按风险排序）
| 候选 | 风险 | 说明 |
|---|---|---|
| A. 只读热路径缓存 | 低 | 用户设置/渠道列表/定价快照 Redis+内存短 TTL；写路径不变；开关默认关可回滚 |
| B. 预扣费异步化 | 高 | 结算语义变更，需幂等/对账/灰度；**本期不做**，单独授权 |
| C. 连接池/HTTP 复用收敛 | 中低 | 上游 transport 复用/dialer 调优；已部分完成，做剩余项 |
| D. 上游流式首字节优化 | 中 | SSE 首字节路径检查（流式透传时不做缓冲/聚合）；需真实流式基准 |

## 4. 纪律
- 任何配额/计费改动必须 `quota_math` 纪律（AGENTS 附录 C）；预扣费饱和须在 pre-consume 失败而非回绕。
- 基准不伪造：环境不可复现时写 BLOCKED JSON 并说明原因（已有先例）。
- 涉 DB 索引/慢查询改动：三库矩阵 + EXPLAIN 记录。

## 闭环状态（2026-09-27，v1.3.44 回填）
- ✅ 基准已达成：perf-ledger 0011（v1.3.28 优化后）overhead_p50=-28.27ms、并发20 -20.1ms、并发50 -20.6ms（warm 连接池）。10ms 目标实质达成，历史 19ms 参照不再成立。
- 本文件「待授权重建环境」为 2026-09-25 旧状态，已由 0011 取代。
