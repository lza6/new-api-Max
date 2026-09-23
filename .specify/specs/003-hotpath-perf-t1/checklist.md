# Checklist : T1 验收门
## Blocking（不达不可进交付）
- [ ] `gofmt -l . relaykit`（排除 web/）为空
- [ ] `go vet ./...`、`go vet ./relaykit/...` exit 0
- [ ] `go build ./...` 与 `GOWORK=off go build ./...` 通过
- [ ] `go test ./model/... ./service/... ./relay/... ./common/...` 绿
- [ ] T1-1/T1-2/T1-3 变更通过评审（行为等价，无计费改动）
- [ ] US1/US2 验收场景通过（见 spec §3）

## 证据（不可省略）
- [ ] 基准 JSON 到 `计划书/e2e-evidence/`
- [ ] 验证台账 `计划书/audit/perf-verification-ledger.md`
- [ ] workflow_status.md 追加 T1 记录
- [ ] 主题 commit + push + tag（Release）

## 诚守边界
- 不声称「预扣异步化已做」（L3 未做）
- 线上部署/线上基准需用户授权，未授权不线下自证