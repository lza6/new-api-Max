# Plan：热路径性能冲刺（T1）

## 阶段划分（依赖顺序）
- **Phase 1【Spec/计划】**: 本 spec + plan + tasks（已建）→ 质量门：暂无代码。
- **Phase 2【实现·热路径复用】**:
  - 2a `model/log.go`：`userSettingRecordIp(c,userId)` 复用请求上下文 UserSetting（RecordConsumeLog/RecordErrorLog 落到 `needRecordIp`）。
  - 2b `service/billing_session.go`：`NewBillingSession.tryWallet` 复用上下文 UserQuota（typed `ok` 判定，无权值回退 `GetUserQuota`）。
- **Phase 3【测试】**：US1 上下文复用（model）、US2 4xx 透传（service/error_test.go）、2b 上下文额度测试。
- **Phase 4【基准与台账】**：`scripts/bench-latency.ps1` + 真实本地 E2E 基准 + `计划书/e2e-evidence/` 证据 + 台账。
- **Phase 5【质量门/交付】**：gofmt → go vet → go build → 相关 go test → relaykit 独立构建 → 主题 commit → push → tag → Release → （生产部署待授权）。

## 依赖关系
- Phase 2 依赖 Phase 1；2b 依赖 2a 模式确认后独立。
- Phase 3 依赖 Phase 2；Phase 4 依赖 2a/2b（要测优化后对比）；Phase 5 依赖 3/4。
- US1/US2 无交叉文件；US3 独立脚本。

## 并行机会
- 2a 与 2b 可并行（不同文件 model/log.go vs service/billing_session.go）。
- US2 测试（service/error_test.go）可与 2b 并行。
- US3 脚本与 2 并行编写，验证依赖实现完成。

## 风险与缓解
| 风险 | 缓解 |
|---|---|
| 上下文 UserSetting 缺值兜底错误 | 用 typed Get (ok) 判定，缺时回退 GetUserSetting（与现行为一致）；RecordConsumeLog 全路径保留原 fallback |
| UserQuota=0 合法性与 not-set 混淆 | 仅用上下文 typed `ok`（能区分是否存在），不存在才回退 Redis/DB |
| 性能提升量级小 | 这是行为等价的安全收口；真实收益由 phase4 基准量化，不夸大 |
| 生产部署 | 本批不自动部署；交付后在用户授权下按既有 SOP 部署 |

## 质量门禁
- `gofmt -l . relaykit`（排除 web/）为空
- `go vet ./...` + `go vet ./relaykit/...` exit 0
- `go build ./...` + `GOWORK=off go build ./...`（relaykit）
- `go test ./model/... ./service/... ./relay/... ./common/...` 绿
- US3 证据 JSON 落到 `计划书/e2e-evidence/`