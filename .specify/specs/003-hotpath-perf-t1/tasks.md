# Tasks：热路径性能冲刺（T1）

**Prerequisites**: spec.md（本 spec 003）、plan.md
**Organized by user story**（可独立实现/测试/部署）。

## US1 — 请求内用户缓存复用
- **[T1-1] [US1] `model/log.go` 新增 `userSettingRecordIp(c,userId) bool`**：读 `ContextKeyUserSetting`（typed，ok 判定）→ 命中即返回 `RecordIpLog`；否则回退 `GetUserSetting(userId,false)`；c==nil 不回退 break。文件：`model/log.go`。
- **[T1-2] [US1] `model/log.go` RecordConsumeLog + RecordErrorLog 用 1 行替换两处 `needRecordIp` 块** → `needRecordIp := userSettingRecordIp(c, userId)`。
- **[T1-3] [US1] `service/billing_session.go` NewBillingSession.tryWallet 复用请求内额度**：`GetContextKeyType[int](c, ContextKeyUserQuota)`，有值用之；否则 `model.GetUserQuota`。文件：`service/billing_session.go`。

## US2 — 4xx 透传回归
- **B-1 [US2] `service/error_test.go` 增表测试**：RelayErrorHandler 对 400/401/403/404/429（body 通用错误）返回 `StatusCode == 上游`（非 500）。
- **[P] 可并行**：与 T1-3 不冲突文件。

## US3 — 基准与台账
- **C-1 [US3] `scripts/bench-latency.ps1`**：直连 mock 上游 vs 网关，顺序 N=60 + 并发 20/50，输出 JSON 到 `计划书/e2e-evidence/`。
- **C-2 [US3] 验证台账**：`计划书/audit/perf-verification-ledger.md` 记录本次已验证项/结论，避免下次重复慢查询/基准。

## 测试清单（质量门 Entry）
- [ ] 新增：`model` US1 上下文用例（RecordIpLog true/false，id 不存在证明不走 DB）
- [ ] 新增：billing wallet 上下文额度复用用例
- [ ] 新增：service 4xx 透传表驱动用例
- [ ] 全量：`go build/vet/test` 绿
- [ ] relaykit `GOWORK=off` 独立构建