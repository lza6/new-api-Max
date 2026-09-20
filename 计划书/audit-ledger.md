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