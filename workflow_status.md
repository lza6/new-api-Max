# Batch-5 进度跟踪（B5-2 hover-why + B5-3 任务进度结构化）

## 目标
1. B5-2 hover-why 状态提示：渠道状态徽章 hover 显示冷却原因；任务列表失败/取消 hover 显示 fail_reason 人话版。
2. B5-3 任务进度结构化（向后兼容）：Task.Data 约定 `{"progress":{event_type,current,total,step}}`；adaptor 解析出就写、否则维持原字符串（零破坏）；前端详情分段条+步骤名、纯字符串原样显示。

## 现状（已侦察确认）
- **B5-2 渠道冷却**：`GetChannelHealthSnapshot` 已有 `cooling_down`/`cool_until`/`cool_count`；`ChannelHealthCell`（健康列）已在 Popover 内显示 "Cooling down · 相对时间"，且冷却时徽章 `pulse`。**但渠道状态列（status column）在 auto-disabled 时才显示 tooltip，且只显示 other_info.status_reason/time，没有冷却原因**。需要：状态列（ENABLED 且冷却中）加 Tooltip 显示冷却原因 + 到期时间。
- **B5-2 任务 fail_reason 人话**：`TaskDetailsCell` 已内联显示 `fail_reason` 截断文本；`getFriendlyErrorMessage`（B6-2 人话映射）已在 `web/src/lib/server-error-message.ts` 导出，但未在任务列表 hover 使用。任务详情 dialog 已显示原始 `fail_reason`。需要：任务列表 fail_reason hover 用 Tooltip 显示人话映射 + 原始原因。
- **B5-3 前端**：`readTaskStructuredProgress`（types.ts）与 `TaskStructuredProgressRow`（task-details-dialog.tsx）**已存在**并已在任务详情渲染分段条+步骤名；纯字符串进度原样显示。**缺口：无组件测试覆盖**。
- **B5-3 后端**：`Task.Data` 是 `json.RawMessage`，轮询路径目前统一 `task.Data = redactVideoResponseBody(responseBody)` 覆盖（丢失插件返回的 taskData）；`AppendTaskRefundMarker`（service/task_billing.go:267）已示范「合并写 map → SetData」的追加模式。**缺口：轮询时若插件返回结构化 progress，应合并进 Data 而非覆盖丢失；refund 标记也应与之共存**。
- 任务 adaptor 是 JS 插件（kling/sora → jsplugin），`ParseTaskResult` 有 `Progress string` 与 `State` 字段；`parseBatchResult` 有 `Data` 字段。

## 待办
### B5-2
- [ ] 渠道状态列（channels-columns.tsx status cell）：冷却中（health snapshot coolingDown）时用 Tooltip 显示冷却原因 + 到期相对时间；auto-disabled 分支保留原 reason/time tooltip
- [ ] 任务列表 `TaskDetailsCell`：fail_reason hover 显示人话映射（`getFriendlyErrorMessage`）+ 原始原因
- [ ] 相关 i18n key（冷却原因/人话标题）
- [ ] 组件测试：渠道状态列冷却 Tooltip；任务 fail_reason hover 人话

### B5-3
- [ ] 后端 `service/task_polling.go`：轮询解析到插件返回的结构化 progress 时，`task.Data` 合并写入 `progress`（不覆盖 refund/其他字段）；解析不出维持原样
- [ ] 后端 `jsplugin ParseTaskResult`：支持 `taskData`/`progress` 结构返回（parseSubmitResponse 已有 TaskData；轮询 ParseTaskResult 目前丢弃 data —— 补 `Data` 字段透传）
- [ ] 后端单测：合并写入行为（progress + refund 共存；纯字符串零破坏）
- [ ] 前端：任务详情/列表结构化进度组件测试（已有 `readTaskStructuredProgress`/`TaskStructuredProgressRow`，补用例）

## 验收
- [ ] 后端 `go build ./...` + 相关单测
- [ ] 前端 `bun run typecheck` + 新增测试
- [ ] E2E 截图（渠道冷却 hover、任务 fail_reason hover、结构化进度条）—— 需要可用渠道/浏览器，当前不可用则如实报告
- [ ] 提交 + 推送 + 版本 bump v1.2.5

## 关键文件
- `web/src/features/channels/components/channels-columns.tsx`（状态列 Tooltip）
- `web/src/features/usage-logs/components/columns/task-logs-columns.tsx`（TaskDetailsCell hover）
- `web/src/features/usage-logs/components/dialogs/task-details-dialog.tsx`（进度条）
- `web/src/features/usage-logs/types.ts`（readTaskStructuredProgress）
- `service/task_polling.go`（轮询 Data 合并）
- `relay/channel/task/jsplugin/adaptor.go`（ParseTaskResult Data 透传）


---

# 2026-09-19 新任务：参考项目全量对标（Phase A）

> 本段为当日新任务状态，追加于旧 B5 记录之后，互不覆盖。
> 完整状态源见 `参考的结果计划指南.md`。

## Task Contract
- 原始目标：深挖 D:\参考项目（1178 项）全部项目，提炼可借鉴优点；围绕「AI 更懂用户/小白易用/日志全开/用户 skills 沉淀/扩展品类」推动 new-api 演进。
- 当前阶段：Phase A（ANALYSIS_ONLY）完成 → 等待用户批准 Phase B 批次 1（P0）。
- 当前授权：只读分析；只允许写 参考的结果计划指南.md 与本追加段。
- 成功标准：9 节报告完成；1178 项全覆盖；高价值项附证据；路线图 P0/P1/P2；无 P0 遗漏。
- 停止条件：用户批准具体批次后进入 Phase B。

## Task Graph（摘要）
| ID | Owner | Goal | Status |
|----|----|----|----|
| A0 | 主协调 | 工作区检查 | DONE |
| A1 | 主协调 | 主项目识别 | DONE |
| A2-1/2 | 子代理×2 | agents 188 项深扫 | DONE |
| A2-3 | 主协调 | 其余 990 项全覆盖（清单+README） | DONE |
| A3-A5 | 主协调 | 筛选/差距/路线图 | DONE |
| A6 | 主协调 | 交付物落盘 | IN_PROGRESS |
| A7 | 主协调 | 输出报告等确认 | PENDING |
| Critic-1 | 独立审查 | 六维审查 | PENDING |

## 待办（Phase B 候选，需用户批准）
- P0-1 stream_fallover 灰度→默认开
- P0-2 channel_health_score API 前端接入
- P0-3 Web 防护可配置化 + 实时服务器状态页
- P1 费用解释用户版 / trace 时间线 / 插件写审批 / skills 仓库 v1 / 离线评测
- P2 任务品类扩展 / 模型目录同步 / 记忆层

---

# 2026-09-20 追加段：Go 全面评审 P1 修复批（不覆盖上述记录）

## Task Contract
- 目标：落地 `计划书/GO-全面代码评审报告.md` 的 P1 修复项；行为保持（支付/任务路径仅加日志不改控制流）。
- 当前状态：本地实现完成，验证通过；**提交/推送/发布待用户确认**（生产契约审批边界）。

## Nodes
| ID | Name | Status | Acceptance | Files | Verification |
|----|----|----|----|----|----|
| N1 | 默认 root 口令日志去敏 | PASSED | 日志不再打印口令 | model/main.go | gofmt/vet ok |
| N2 | zhipu/mistral stub panic→error | PASSED | ConvertClaudeRequest 返回 error 不 panic | relay/channel/zhipu/adaptor.go, mistral/adaptor.go | go test ./relay/channel/ PASS |
| N3 | TOTP 备用码 rejection sampling | PASSED | 格式/字符集回归测试通过 | common/totp.go, common/totp_test.go | go test ./common/ PASS |
| N4 | 支付/任务/余额吞错加日志（不改行为） | PASSED | 编译通过，行为不变 | controller/subscription_payment_epay.go, channel-billing.go, codex_usage.go, relay/relay_task.go | go vet + go build PASS |

## Verification Evidence
- `go vet ./common/... ./relay/... ./controller/... ./model/...` → exit 0
- `go test ./common/ -run "Backup|TOTP"` → PASS (0.03s)
- `go test ./relay/channel/ -run "UnimplementedClaude"` → PASS (mistral+zhipu)
- `go build ./relay/... ./controller/... ./common/... ./model/...` → exit 0
- 新增测试文件：common/totp_test.go、relay/channel/adaptor_stub_test.go

## Next
- 待用户确认后：主题提交 + 推送分支 + （与代码批一起打 tag 发版）。
---

# 2026-09-20 追加段：签到奖励设置 "Invalid input" Bug 修复（不覆盖上述记录）

## Root Cause
- `web/src/features/system-settings/general/checkin-settings-section.tsx` schema 对 min/max 用了 `z.coerce.number().int()`，但字段值是**显示货币金额**（如 ¥0.5 = 0.5 小数）→ 小数必被 `.int()` 拒绝 → zod 默认报 "Invalid input"。

## Fix
- schema 改为 `z.coerce.number().min(0)`（允许小数）；整数 quota 换算由 `parseQuotaFromDollars` 完成（0.5 → 250000、0.8 → 400000 @ quotaPerUnit=500000）。

## Verification
- 新增回归测试 `web/src/features/system-settings/general/__tests__/checkin-settings-section.test.tsx`：小数 0.5/0.8 提交成功，换算值正确（min_quota=250000/max_quota=400000），无 "Invalid input"。
- `bunx vitest run .../checkin-settings-section.test.tsx` → 1 passed (19.9s)
- `bun run typecheck`（tsgo -b）→ exit 0

## Status
- 本地修复完成、测试通过；提交/推送/发布待用户确认（生产契约审批边界）。
---

# 2026-09-20 追加段：首字延迟对标 sub2api（不覆盖上述记录）

## 结论
- 主病灶：`MEMORY_CACHE_ENABLED` 默认 false → 渠道/模型选择每请求回源 DB（channel_cache.go:124）。
- sub2api 可借鉴：HTTP 客户端池化（我们已有）、用量/配额异步 flusher（我们为同步预扣，L3 待审批）、热路径轻量化。

## 落地（仓库内）
- docker-compose.yml：新增 MEMORY_CACHE_ENABLED=true + SYNC_FREQUENCY=60（含注释）。
- 文档：`计划书/首字延迟对标sub2api与落地.md`（对标表 + 根因排序 + 服务器验证方法）。
- 校验：pyyaml 解析 OK（本地无 docker CLI）。

## 待用户确认
- 服务器应用 compose 并重启 new-api（生产变更）；验证方法见文档 §6。
- 预扣费异步化（对标 flusher）为 L3 计费改造，需方案审批后另起批次。
---

# 2026-09-20 追加段：本地真实 E2E 证据（不覆盖上述记录）

## 结果
- 流式透传：mock 上游首字 1.5s，网关首字 1.52-1.57s（额外 22-68ms），9 chunks，DONE，内容完整。
- 签到设置后端：min=250000 / max=400000 / enabled=true 复读通过。
- 证据：`计划书/e2e-evidence/v1.2.32-local-e2e.md`。

## 结论
- 网关流式透传零缓冲；首字额外开销毫秒级（本地）。生产端 1.5-2s 放大来源为 MEMORY_CACHE 未开的 DB 回源 + 2C CPU 争抢 + 上游波动，已由 compose 落地（MEMORY_CACHE_ENABLED=true）定向修复，待生产部署后实测复核。