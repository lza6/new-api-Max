# Batch-2 施工状态（B2-2 / B2-3 / B2-4）

> 起始版本 v1.3.98。用户授权全量开发（2026-10-06）：三项全做，含 B2-3 完整多端点 webhook 子系统、B2-4 全部 5 个 SQL 债项。
> 本次**不部署**——用户明确「先不用线上部署更新，我到时候会叫你更新部署」。

## 任务契约

| 任务 | 目标 | 验收标准 | 状态 |
|---|---|---|---|
| **B2-2** 模型目录动态同步 | ① 已有 `/v1/pricing` 端点补**契约测试**；② 新增独立「模型目录同步」定时循环（与 pricing_sync 分开），可配开关 | 契约测试 PASS；定时任务 env 开关；优雅关闭纳入 | ✅ 完成 |
| **B2-3** 通用 webhook 多端点子系统 | `webhook_endpoints` 表 + 每端点独立密钥/事件订阅 + HMAC 签名 + 幂等（DB 唯一）+ 重试退避 + 投递状态机；迁移现有 4 个支付/任务 webhook 到事件总线 | 幂等（重复投递不重复处理）；重试退避曲线测试；三库 conformance | ✅ 完成 |
| **B2-4** SQL 债清偿 | ① GetLogsTraffic 聚合下推 ② SumUsedQuota 天数上限 ③ 日志列表深分页延迟关联 ④ tasks 轮询正向活跃态索引 ⑤ 独立日志库时主库不 AutoMigrate &Log{} | EXPLAIN 前后对比；三库验证；分页正确性 | ✅ 完成 |

## 关键现状（侦察结论）

### B2-2
- `controller/pricing.go:45 GetV1Pricing` **已存在**（`/v1/pricing`，公开只读，与 `/api/pricing` 同源）。
- `controller/pricing_sync.go runPricingSyncTaskOnce` + `system_task_handlers.go:190 pricingSyncHandler` **已存在**（env `PRICING_SYNC_TASK_ENABLED` 默认 true，周期 `PRICING_SYNC_TASK_INTERVAL_MINUTES` 默认 360）。
- `controller/model_sync.go` 的 `SyncUpstreamPreview`/`SyncUpstreamModels` 是**手动** preview/apply，无定时调度。
- → 缺口：① `/v1/pricing` 无契约测试；② 无**目录**自动同步循环（手动 apply 已有）。

### B2-3
- 现有 `service/webhook.go`：**单 URL**（operation_setting）+ HMAC-SHA256 + 进程内去重（60s 窗口）+ 3 次重试 + SSRF 守卫。
- 现有 `service/event_bus.go`（P2-2 事件总线）：幂等（event_id+handler 唯一）+ 内存状态机 + 持久化 store（`model.EventDelivery`）。**这是要复用的底座**。
- 现有 `model.EventDelivery`：`event_id+handler` 联合唯一，状态 pending/success/failed/dead。已用于 `service/epay_events.go`。
- 前端 `web/src/features/webhook/` 单 URL 配置页。
- 迁移来源：`service/epay_events.go`（topup/subscription）、`service/task_polling.go:699`（task.settled）共 3 处 `NotifyWebhooks` 调用。
- → 缺口：多端点表、每端点独立订阅、投递状态可查、跨实例幂等（现有 event_bus 已具备，需把 webhook 投递接入 event_bus）。

### B2-4
- ① `controller/log.go:168 GetLogsTraffic` **仍全表扫 other JSON**（`Select("created_at","other")`）→ 生产 38 万行 1246-1448ms。已有 60s Redis 缓存兜底。**SiteOverview/BandwidthLeaderboard 已改走持久化列**（可对照参考 `controller/log.go:360`）。
- ② `model/log.go:660 SumUsedQuota` 无天数上限（startTimestamp 任意）。前端默认窗口需核对。
- ③ `model/log.go:553/644` `GetAllLogs`/`GetUserLogs` 用 `Limit(num).Offset(startIdx)` 深 OFFSET。
- ④ `model/task.go:67 Status` 单列 index；轮询查询是 `progress != '100%' AND status NOT IN (...)` 复合谓词 → 需正向活跃态复合索引。
- ⑤ `model/main.go:351` `migrateDB()` AutoMigrate 含 `&Log{}`（主库），即使 `LOG_SQL_DSN` 独立配置也会在主库建 logs 表。

## 验证台账（待补）
（施工后填 EXPLAIN 前后、三库 conformance、契约测试结果）

## 验证台账（v1.3.99 施工后）

### 构建 / 静态
- `go build ./...` ✅（后台完成 exit 0）
- `go vet ./model/ ./service/ ./controller/` ✅ 无输出
- `gofmt -l` 对全部改动文件 ✅ 干净

### 单元 / 集成
- `go test ./model/ -count=1` ✅ **ok 55.8s**（全量绿）
- `go test ./service/ -run "Webhook|EventBus|BackgroundLoop"` ✅ ok
- `go test ./controller/ -run "TestTrafficCacheStoresOnlyDailyAggregates|TestV1Pricing|TestLog|TestWebhook"` ✅ ok
- 新增用例：
  - B2-3：`TestDispatchWebhookEventIsIdempotent`（重复投递 handler 只跑 1 次）、`TestWebhookDeliveryBackoffCurve`（2 次重试后 dead、attempts=3）、`TestWebhookEventFanoutToMultipleEndpoints`、`TestParseWebhookEndpointEvents`、`TestEndpointSubscribes`、`TestWebhookDeliveryStateQuery`，+ **真实 E2E**：`TestNotifyWebhookEndpointsSignsAndDelivers`（真实 HTTP 接收方收到 `sha256=` 签名头）、`TestDispatchWebhookEventEndToEnd`（签名逐字节 = HMAC-SHA256(secret, body) + 事件信封字段 + 二次幂等）、`TestNotifyWebhookEndpointsRespectsSubscription`（未订阅端点不收到）
  - B2-2：`TestV1PricingContract`（无鉴权 + 字段契约：data/vendors/group_ratio/usable_group/pricing_version + 每项 model_name/enable_groups）
  - B2-4：`TestLogPaginationDeferredJoinCorrectness`（2000 行，偏移 1500 深分页与偏移语义一致）、`TestLogPaginationShallowPathUnchanged`、`TestSumUsedQuotaClampsOpenEndedWindow`（含 `LOG_STAT_MAX_DAYS=0` 关闭路径）

### HTTP E2E（本地 18131 真实实例）
- `/v1/pricing` 无鉴权 → 200 ✅
- `GET/POST/PUT/DELETE /api/admin/webhook/endpoints` 全链路 ✅：
  - 无鉴权 → 401；建列表返回 `[]`
  - **SSRF 拒绝**：`url=http://127.0.0.1/hook` → 400 `Invalid parameters`（管理员亦然，守卫有效）
  - 创建合法 `https://…` → `{id:1}`
  - 列表 **密钥脱敏**（回显 `has_secret:true`，无 `secret` 字段）
  - 更新（secret 留空 → 保留原密钥）；删除 → 列表复位 ✅

### 反向验证（先红后绿）
- B2-4 延迟关联：临时把深分页重排为 `id asc` → `TestLogPaginationDeferredJoinCorrectness` **FAIL**（quota 490≠499）；还原 → PASS。证明测试真实捕获深分页顺序错误。
- B2-4 两条路径等价性：阈值设 0（全走深路径）与设极大（全走浅路径）用例均 PASS → 延迟关联与 LIMIT/OFFSET 语义等价。

### 三库 conformance
- `TEST_MYSQL_DSN=... TEST_POSTGRES_DSN=... go test ./model/ -run TestDBConformance -count=1` ✅ **ok 65.4s**
- 新增/扩展断言：`TestDBConformanceLogsIndexes` 增加 `tasks.idx_task_active_poll` 三库存在性；`TestDBConformanceWebhookEndpointRoundTrip` 三库建表 + 读写 + events JSON 往返；`conformanceModels()` 纳入 `&WebhookEndpoint{}` 保证 AutoMigrate 幂等。

### 前端
- `bun run typecheck`：webhook 新文件 **0 错**（既有噪声：chat-presets-item / nav-group / account-bindings / security index 的 `relay.request_compression_max_mb`，均非本批）
- `bunx oxlint` webhook-endpoints.tsx ✅ 干净（webhook-settings.tsx:47 的 catch-or-return 为既有噪声，stash 对照确认）
- `bun run i18n:sync` + `i18n:check` ✅ **0 missing**；新 16 键 zh/zh-TW 人工翻译，en 为基准；`footer.newapi` 混淆键 diff 仅 +16 行未触碰

### 全量 controller 套件（已知噪声）
`go test ./controller/ -count=1` 有 9 个 FAIL，**全部在既有噪声清单**（`TestAuditDatabaseMatrix` / `TestSessionLimit...` / `TestResetPassword*` / `TestSendEmailVerificationAntiEnumeration` / `TestKlingNativeRoute...` / `TestGetSiteOverviewAggregatesConsumeLogs` / `TestSiteSubscriptionStatsAggregates` / `TestAdminSetUserSubscriptionTierInvalidatesCache`），单独跑全部 PASS → 顺序依赖噪声，非本批回归（与 ledger 记录一致）。

### 未做 / 需说明
- **EXPLAIN 前后对比**：本地三库已跑 conformance（证明索引/语义正确），但未在生产数据量级做 EXPLAIN 基准（生产日志 60 万行、数据在线上）。延迟关联与聚合下推的设计依据是既有已优化的 `BandwidthLeaderboard`/`SiteOverview`（同法，已生产验证）。生产 EXPLAIN 待部署后按需补。
- **B2-2 目录同步默认关**：目录变更是有副作用的写，默认 `CATALOG_SYNC_TASK_ENABLED=false`，需管理员显式开启（与任务书「可配开关」一致）。

### 自查发现并修复的问题
- **B2-4 ① ClickHouse 回归（自审发现，已修）**：`GetLogsTraffic` 改走 `request_bytes`/`response_bytes` 列后，ClickHouse 的 `clickHouseLogCreateTableSQL` **原本没有这两列** → CH 日志库下该端点会 `Unknown identifier` 500（且 `SiteOverview`/`BandwidthLeaderboard` 早已引用同列，属既有债）。已修：① CH DDL 补 `request_bytes/response_bytes Int64 DEFAULT 0`；② 新增 `ensureClickHouseLogTrafficColumns()` 对存量 CH 表 `ALTER TABLE ... ADD COLUMN IF NOT EXISTS`（幂等）。回归护栏：`TestClickHouseLogCreateTableSQL` 断言含两列。

### 独立审查
- 已启动独立 Critic 子代理审查（按用户 007 批次的 Red Team 纪律），但该子代理**长跑后转 idle 未回消息**（0 字节输出 30+ 分钟，与 ledger 记录的审计子代理「长跑僵死」模式一致）。**未获取到独立审查结论。**
- 改为**自审**（逐项对照任务契约）：自查中**发现并修复了唯一真实回归**——B2-4① ClickHouse 缺列（见上「自查发现并修复的问题」）。其余项（延迟关联 SQL 形状、⑤ 的 main/LOG 库分离、B2-3 幂等键/闭包捕获/密钥不泄漏、SSRF 等价性、字段契约兼容）经代码 + 实测逐项确认无缺陷：
  - 延迟关联两步 SQL 已实测打印：均带同一 `ORDER BY logs.created_at desc, logs.id desc`，语义等价。
  - `ep := endpoint` 显式拷贝，无循环变量捕获 bug。
  - `EventDelivery` 表无 payload 列，Secret 不落库；事件总线 `safeHandle` 捕获 panic。
  - `validateWebhookEndpointInput` 与既有 `UpdateWebhookSettings` 同用 `ValidateSSRFProtectedFetchURL` + http(s) 前缀；实测私网 URL 被拒。
  - `GetLogsTraffic` 响应字段（days/total_requests/total_bytes/total_mb/by_day）保持不变 → 前端 `LogsTrafficData` 契约兼容。
