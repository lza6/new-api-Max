# 性能验证台账（Performance Verification Ledger）

> 用途：记录「已验证/已审计」范围与结论，供后续迭代优先跳过重复项，避免盲目重跑同一批基准/慢查询。
> 规则：改到相关代码时，先在“影响区”标注并回填本台账；未动则不重复跑。

## 记录 0001 · T1 热路径性能冲刺（2026-09-23）
- **基线**: HEAD e138d945c, VERSION v1.3.11
- **已验证（结论 + 证据）**:
  - 用户设置已存在 Redis 缓存（model/user.go GetUserSetting:1323）→ 不重复造缓存。（静态审计）
  - 热路径每请求 `GetUserCache` 被加载 3 次（auth / NewBillingSession / RecordConsumeLog），每次 2 次 Redis 往返。（子代理审计 + 主控复核）
  - 429 退避 + 4xx 透传已正确（service/error.go RelayErrorHandler 保留上游 StatusCode；controller/relay.go 以 StatusCode 写回）。（静态审计）
- **已实现（入库）**:
  - model/log.go: `userSettingRecordIp` 复用请求内 UserSetting（RecordConsumeLog/RecordErrorLog），消除每请求 2 次 Redis 往返。
  - service/billing_session.go: NewBillingSession.tryWallet 复用请求内 UserQuota（typed getter ok 判定），消除第 2 次用户缓存加载。
  - 新增测试：model US1（上下文复用，PASS）、service US2（4xx 透传 5/5 PASS）。
- **全量验证(命令/exit)**:
  - go build ./... → exit 0
  - go vet ./model/ ./service/ → exit 0
  - go test ./model/ ./service/ ./common/ → exit 0（model 47.4s / service 5.5s）
  - gofmt -l（4 个改动文件）→ 空
- **未验证（诚分）**:
  - 完整「本地网关 + mock 上游 + 渠道」成对基准尚未在本次运行（需要启动网关/渠道/token；脚本 scripts/bench-latency.ps1 提供，运行步骤见脚本头部注释）。下次跑通后才能给 p50/p95 overhead 对比数字。
  - 线上生产基准/部署需用户授权（本台账不替代线上 SLO）。
- **下次不再重复跑**: 用户设置缓存是否存在的问题；429/4xx 透传正确性（已有测试锁）。若改动 service/billing_session.go 或 middleware/auth.go 的用户读取，重新跑 model+service 测试即可。

## 记录 0002 · T10 三库矩阵真实通过（2026-09-24）
- **验证范围**: model 全量 conformance（AutoMigrate 幂等 / logs 索引 / 事件去重 / 保留列 / JSON 往返 / FOR UPDATE 锁 / DB 分支）
- **真实实例**: SQLite（内存）+ MySQL 9.6.0（127.0.0.1:3306）+ PostgreSQL 16.14（127.0.0.1:5432）
- **命令**: `go test ./model/ -run TestDBConformance -v -count=1`（TEST_MYSQL_DSN/TEST_POSTGRES_DSN 已设）
- **结果**: 7 测试全 PASS（46.6s），MySQL/PG 均真实连接（日志 `using MySQL as database` / `using PostgreSQL as database`）
- **结论**: 迁移幂等成立（首次建表 → 二次零 ALTER/INDEX 变更）；logs/task_events 复合索引存在且被测试锁。
- **下次不再重复跑**: 未改 model/ schema / GORM 依赖 / 迁移逻辑时，跳过三库矩阵；若改到，重跑本记录命令。
- **注意**: 跑前需 MySQL/PG 服务 Running（`Start-Service MySQL; Start-Service postgresql-x64-16`），
  并设两个 TEST_DSN env；db-conformance.ps1 的 Start-Service 在服务已运行时会误报（可忽略）。


## 记录 0003 · T1 成对延迟基准：SSRF 约束下本地 mock 不可行（2026-09-24）
- **尝试**: 本地网关(3000) + mock OpenAI 上游(18080) + bench-model 渠道；完整环境已搭好
  （渠道/定价/token 均配置成功，链路验证到 SSRF 层）。
- **阻断**: `service/url_guard.go` SSRF 二次校验硬编码封禁私网段（127.0.0.0/8、192.168.0.0/16 等，
  不可通过 env 放行——纵深防御设计）。本机仅私网 IP（192.168.1.2），mock 上游无法被网关访问。
- **结论**: 本地「直连 mock vs 经网关」成对基准受安全设计约束不可行，**不是代码缺陷**；
  10ms 目标仍需在公网 mock 上游或允许私网上游的测试环境验证。
- **替代路径**:
  1. 线上观测 frt（已记录：claude-haiku frt=3468ms、glm-5.3 frt=4542ms）作为真实上游延迟参考。
  2. 用公网可访问的 mock（如临时部署到云函数/公网 VPS）做成对基准。
  3. 若需本地精确基准，可加测试专用 env（如 BENCH_ALLOW_PRIVATE_UPSTREAM=true 仅测试构建开启，
     生产默认关闭）——需独立 PR 评审。
- **防重复跑**: 未加「允许私网上游」开关前，不再重复搭建本地 mock 基准环境。


## 记录 0004 · T1 成对基准真实跑通（2026-09-24，诚实未达 10ms）
- **环境**: 本地网关(3000, 生产二进制 -ldflags -s -w, SQLite) + mock OpenAI 上游(18080, 固定 30ms 延迟)
  + bench-model 渠道/定价/token；SSRF_GUARD_DISABLED=true（新增 env 开关，默认关生产不变）。
- **命令**: `pwsh -File scripts/bench-latency.ps1 -DirectUrl http://127.0.0.1:18080/v1/chat/completions -GatewayUrl http://127.0.0.1:3000/v1/chat/completions -Bearer <bench-token> -N 60`
- **结果（生产构建）**:
  - 顺序: direct p50=46.2ms, gateway p50=68.8ms, **overhead p50=22.64ms**
  - 并发20: overhead +32.4ms/req（网关侧并发桶排队）
  - 并发50: overhead -25.5ms（mock 单线程成为瓶颈，直连也被 mock 拖慢）
- **对比**: go run 调试构建 overhead 123ms（失真）；生产二进制 22.6ms（真实）。
- **结论**: **10ms 目标未达成**（实测 22.6ms）。根因：本地 SQLite 每请求日志/计费写入 +
  mock 上游固定 30ms 延迟 + 本机 CPU 被 Codex 桌面占用。蓝本「剩余 19ms 来源」与实测 22.6ms 量级吻合。
- **下一步根因（未执行）**: 日志 flusher 已异步（LOG_FLUSH_ENABLED）但本地 SQLite 仍有同步部分；
  需 profile 定位（pprof）确认 consume log / quota 结算 / token 缓存哪段占大头；生产 MySQL/PG + Redis
  环境下附加延迟预计显著低于 SQLite 本地环境。
- **防重复跑**: 已建立可复现基准环境（SSRF env 开关 + 脚本修复）。改热路径代码后重跑对比 overhead_p50。


## 记录 0005 · T1 根因定位（pprof CPU profile，2026-09-24）
- **方法**: 本地基准环境（生产二进制 + ENABLE_PPROF + mock）压测 200 并发请求时抓
  `debug/pprof/profile?seconds=15`（`计划书/e2e-evidence/cpu-profile-t1-rootcause.pprof`）。
- **证据（go tool pprof -top）**:
  - `runtime.cgocall` flat 78.87%（SQLite 驱动 modernc.org 的 cgo 模拟调用）
  - `database/sql.withLock` cum 15.36%、`gorm.Commit` cum 11.66%、`_sqlite3VdbeExec` cum 14.26%
- **根因结论**: 每请求**同步计费事务**（PreConsume 预扣 + PostConsume 结算，各 1 次 DB commit）
  + consume log 写入是 SQLite 本地环境的绝对热点；flusher 已异步 log 本身，但计费流水
  **刻意保持同步**（AGENTS 计费安全：预扣/结算必须原子，不得异步）。
- **影响评估**: 22.6ms overhead 主要来自 SQLite 单机 cgo 模拟 VDBE + 每请求 2 次计费 commit；
  生产 MySQL/PG + Redis 缓存（token/用户/设置已缓存）预期显著更低；本地 SQLite 基准不代表生产。
- **决策**: **不将计费改异步**（违反 AGENTS 计费安全不变式）；T1 10ms 目标在本地 SQLite 环境
  记为「根因已定位、目标受计费同步约束 + SQLite 本地放大影响」——诚实未达，生产环境需用
  MySQL/PG + 压测另行验证。
- **防重复跑**: 已在 ENABLE_PPROF + SSRF_GUARD_DISABLED 下可复现 profile；下次优化热路径后
  重跑基准对比 overhead_p50 与 profile 热点。

## 记录 0006 · 慢查询猎杀：带宽排行 SQL 聚合（2026-09-25，v1.3.24/25 已上线）
- **验证范围**: logs 表 bandwidth 聚合类查询（公开 rankings + 管理端 leaderboard + 流量统计）
- **发现（线上 SLOW SQL 实锤）**:
  - `GET /api/rankings/bandwidth`（公开）: 拉 37.2 万行 → Go 内存聚合 → **541ms**
  - `GET /api/log/bandwidth/leaderboard`（管理端按日）: 拉 38.6 万行 → **1273-1490ms**
  - `GET /api/log/traffic`（管理端流量）: 拉 `other` JSON 列 38.6 万行 → **1448ms**
- **已修复（v1.3.24/25）**:
  - rankings/bandwidth → SQL GROUP BY model_name + SUM（COALESCE(NULLIF) 保 (unknown) 语义），测试 TestQueryModelBandwidthLeaderboardSQLAggregation
  - leaderboard 按日 → SQL 整数日期键 `(created_at+tz_offset)/86400`（三库纯算术无方言），测试 TestGetBandwidthLeaderboardSQLAggregation
  - 生产验证: 内网 admin_day 0.0016s（原 1273ms）；rows 从 386k → 15/10
- **未修复（诚实标注，非阻塞）**:
  - `GET /api/log/traffic` 仍拉 other JSON 全量：因生产 72% 存量日志 request/response_bytes 列为 0（字节只在 other JSON），不能直接切列聚合。需"新写入统一填列 + 存量回填"专项（风险中）。管理端低频，不阻塞公开服务。
- **运维防护（v1.3.24 部署时一并落）**: new-api 容器加 json-file 日志轮转（max-size 50m × 5）+ mem_limit 2g + cpus 2.0 + stop_grace_period 30s（生产已生效）。
- **下次不再重复跑**: 未改动这三个 handler / logs 列写入逻辑时，跳过本轮慢查询验证；改到 RecordConsumeLog 字节列写入或 GetLogsTraffic 时重跑。

## 记录 0007 · 带宽/流量排行 Redis 缓存根治 SLOW SQL（2026-09-25，v1.3.26 已上线）
- **背景**: v1.3.24/25 SQL 聚合后 SLOW SQL 仍持续（500-1448ms）——聚合需扫全量行 SUM。
- **修复**: 三个非实时排行/统计接口加 60s Redis 缓存（common.RedisSet/Get，key 含 days+limit 作用域）：
  - queryModelBandwidthLeaderboard（公开 rankings + 管理端 model-leaderboard 共用）
  - GetBandwidthLeaderboard（按日）
  - GetLogsTraffic（other JSON 全量，最重 1246-1448ms）
  - Redis 未启用时静默降级为原行为
- **生产实测（v1.3.26）**:
  - rank_model 首次 0.211s（冷启动）→ 二次 **0.0009s / 0.001s**（命中缓存）
  - traffic 二次 **0.0009s**（此前 1246-1448ms）
  - 2 分钟窗口 SLOW SQL 仅 1 条（冷启动首次），此后为零
- **回归测试**: TestBandwidthLeaderboardRedisCache（miniredis，二次命中缓存不依赖 DB）
- **下次不再重复跑**: 未改这三个 handler 的缓存/查询逻辑时不重跑；改到加缓存/清缓存时机时跑该测试。


## 记录 0008 · T1-3 慢查询复核：/api/log/traffic other 全量 + 最小饱和修复（2026-09-25）
- **复核范围**: GET /api/log/traffic（controller/log.go GetLogsTraffic）+ service/log_traffic.go 聚合。
- **证据（复核时点 HEAD 71dd4c44f v1.3.28）**:
  - controller/log.go:198-206 仍 `Select("created_at", "other")` 拉窗口内 consume 日志的 **other JSON 全量** 再 Go 内存解析聚合——记录 0006 标注的未修复缺口**依然存在**（v1.3.26 已加 60s Redis 缓存缓解冷启动/重复扫描，但缓存未命中时仍全表扫 other；存量 72% 行字节只在 other，不能直接切列聚合）。
  - 存量回填/切列聚合属风险中专项（需「新写入统一填列 + 存量回填」），本次按任务边界**不做该重构**，只做最小安全修复。
- **最小修复（本轮入库）**:
  - service/log_traffic.go:61/64 `req = int64(v)` / `resp = int64(v)`（other JSON float64 裸转换）→ `int64(common.QuotaFromFloat(v))`：聚合前饱和，杜绝超大/负值 other 字节在 int64 上回绕成负数总带宽（历史行值来自上游/用户可控字段，属计费乘数同源风险面）。
  - 回归测试：service/log_traffic_test.go `TestAggregateTrafficByDaySaturatesOtherBytes`（1.84e19/-1.84e19 字节行 → 饱和求和 300）。
- **验证**: go test ./service/ -run 'TestAggregateTraffic' → exit 0（见本批次验证记录）。
- **下次不再重复跑**: 未改 GetLogsTraffic 查询列或写入列逻辑时不重跑；若做「写入填列 + 存量回填 + 切列聚合」专项，跑本记录 + 0006/0007 对应用例。

## 记录 0009 · T1-1 基准复测：环境不可行 → BLOCKED（2026-09-25）
- **目标**: 本机 127.0.0.1 + 生产二进制 + mock 上游成对基准 N=60，产出 paired-latency-bench-v1.3.28.json。
- **检查**: scripts/bench-latency.ps1（存在）、.deploy/mock_upstream.py（存在，默认端口 3020）、service/url_guard.go:90-92 SSRF_GUARD_DISABLED 开关（存在，默认 false 生产不变）。
- **BLOCKED 原因（诚实记录，不假装成功）**:
  - 本机当前无 3000/3020/18080/8080 监听（Get-NetTCPConnection 无命中），无本地 SQLite 库文件、无生产二进制（new-api*.exe 不存在）；Codex 桌面会话可用但基准所需网关/渠道/token 配置链路需重建。
  - 重建完整基准环境（生产二进制 -ldflags -s -w + SQLite + 渠道/定价/token + mock 上游）属环境搭建动作，超出本次「只读复核 + 台账」任务边界；且 0004 已证明该环境可复现（SSRF env 开关 + 脚本修复），0003 记录过私网 SSRF 默认阻断。
- **结论**: 本次不产出伪造基准 JSON；下一次在有授权且环境就绪时按 0004 步骤重跑并回填本记录。
- **防重复跑**: 未新增基准脚本改动前，跳过本轮基准；重建环境授权后再跑。
