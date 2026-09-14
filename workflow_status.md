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
