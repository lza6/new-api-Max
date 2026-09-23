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