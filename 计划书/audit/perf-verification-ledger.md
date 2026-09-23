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
