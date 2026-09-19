# B4-1 任务事件流 SSE 方案（v1.2.27）

日期：2026-09-19
状态：已实现（v1.2.27），待部署 E2E 验证

## 1. 目标
任务从提交到结算的完整生命周期可被前端实时观测；断线重连从 sinceSeq 续传，no gap/dup。
事件类型：submitted / queued / claimed / progress / step_ratio / succeeded / failed / refunded / unconfirmed。

## 2. 现有架构事实（代码证据）
- 生命周期：POST /v1/video/generations（router/video-router.go）→ controller.RelayTask（controller/relay.go）→ relay.RelayTaskSubmit（relay/relay_task.go:197）→ 落库 Task → 轮询 service/task_polling.go updateVideoSingleTask 驱动状态。
- Task 模型：model/task.go L56-78（ID int64 主键 / TaskID 第三方 / Status / Progress / Data json / Properties json）。状态常量 L36-50：NOT_START/SUBMITTED/QUEUED/IN_PROGRESS/FAILURE/SUCCESS/UNKNOWN/UNCONFIRMED。
- 状态写入点（事件埋点锚）：
  a) submit 成功：controller/relay.go L872 `task.InsertWithContext`（durable barrier）→ submitted（含 immediate 终态分支 L856-870）
  b) unconfirmed 落库：controller/relay.go L794 `persistUnconfirmedTask` → unconfirmed
  c) 轮询状态机：service/task_polling.go L568-609（parsedStatus 分支）→ queued/claimed/progress/step_ratio/succeeded/failed
  d) 退款：service/task_billing.go `RefundTaskQuota` → refunded
- B5-3 结构化进度已在 task_polling.go applyStructuredTaskProgress（L760-775）写 Task.Data.progress {event_type,current,total,step} → step_ratio 事件数据源。
- 认证：middleware/auth.go classifyDashboardCredential 只认 Authorization Bearer/PAT（无 cookie 认证）→ SSE 客户端必须能带 header。

## 3. 设计决策

### 3.1 事件存储：独立表 task_events（三库兼容）
model.TaskEvent：
- ID int64 主键自增 = seq（DB 层全局单调，断线续传 `id > sinceSeq`，天然无 dup；事务回滚产生 gap 不影响正确性——不漏事件）
- InternalTaskID int64 index（Task.ID，稳定跟踪 key，轮询期 TaskID 会变）
- UserId int index（鉴权归属）
- Type varchar(30)
- CreatedAt int64
- Data json（gorm:"type:json"，与 Task.Properties 同模式，SQLite TEXT / MySQL json / PG json）
AutoMigrate 注册；无列变更。

### 3.2 事件写入：service.RecordTaskEvent（best-effort，失败仅 SysError 不阻塞主流程）
func RecordTaskEvent(ctx, internalTaskID int64, userID int, eventType string, data any)
- 事件类型常量：EventSubmitted/EventQueued/EventClaimed/EventProgress/EventStepRatio/EventSucceeded/EventFailed/EventRefunded/EventUnconfirmed
- data 为结构化对象（含 task_id/platform 等），json.RawMessage 落库

### 3.3 SSE 端点：GET /api/task/:task_id/events?since=<seq>
- 路由：apiRouter.Group("/task") 下加 `GET /:task_id/events`，middleware.UserAuth()
- 归属校验：model.GetByTaskId(userId, task_id) → 不存在 404
- 协议：Content-Type text/event-stream；每条 `id:<seq>\nevent:<type>\ndata:<json>\n\n`
- 轮询 DB 每 1s；无事件发 `: ping\n\n` heartbeat；客户端断开（ctx.Done）退出
- 任务终态且事件已全部推送 → 发 `event: done` 后关闭（前端可结束流）
- 支持 Last-Event-ID：取 query since 优先，其次 Last-Event-ID header

### 3.4 前端消费：fetch + ReadableStream（带 Authorization header）
- EventSource 无法带 Bearer header（认证只认 header）→ 用 fetch 读流 + 解析 event-stream 格式
- 任务日志详情加「实时事件流」区块：时间线展示 9 类事件 + 结构化数据；断线自动重连（since=lastSeq）避免 gap/dup
- 复用现有 Badge/Tooltip/Spinner 组件；全部文案 i18n（en/zh 双语 + sync）

## 4. 三库兼容
- 新表 AutoMigrate：SQLite/MySQL/PG 均支持自增主键 + json 列（与 Task.Properties 一致模式）
- 查询仅 GORM 标准 Select/Where/Order，无方言 SQL
- 验证：SQLite 单测；PostgreSQL 线上实跑（部署后 curl SSE 实证）；MySQL 受环境限制标注

## 5. 测试计划
- service 单测：RecordTaskEvent 落库 + ListEventsAfter(sinceSeq) 续传无 gap/dup；事件类型齐全
- controller SSE 测试：httptest 启动流 → 注入事件 → 读流断言 id/event/data 格式 + since 续传
- 既有回归：go test ./service/... ./controller/... ./relay/... 相关包

## 6. E2E 验收计划
- 后端：curl -N 带 token 订阅 SSE；模拟注入事件（直接写 task_events 或调用 RecordTaskEvent）观察推送；断线重连 since 续传 no gap/dup
- 真实任务：线上有可用视频渠道则走一单；无可用渠道则如实标注 + curl 模拟验证（验收允许）
- 前端：任务日志详情事件流渲染截图；JS 无报错；i18n sync 无漂移
- 部署：bump v1.2.27 → tag/release → CI → watchtower 自动部署 → /api/status 验证

## 7. 风险与对策
- 事件写入失败不能影响任务主流程：best-effort + SysError 日志
- SSE 长连接资源：单任务订阅，客户端断开即退；heartbeat 防代理超时
- 多节点部署：DB 自增 id 全局单调，多节点写无冲突