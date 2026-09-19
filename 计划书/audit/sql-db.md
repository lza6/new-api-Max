# SQL / 数据库只读审计报告（首跑：慢查询·索引·三库兼容·锁与安全）

- 仓库：`C:\Users\Administrator.DESKTOP-EGNE9ND\Desktop\公益服务网关\new-api`（Go+GORM，SQLite/MySQL/PostgreSQL 主库，日志库可独立 SQLite/MySQL/PostgreSQL/ClickHouse）
- 审计日期：2026-09-20
- 范围：logs/tasks/task_events/usage 聚合读路径、迁移幂等与三库兼容抽查、raw SQL 与锁风险
- 方式：纯只读（源码 + 本地 SQLite `$env:TEMP\e2e-data\one-api.db` 只读连接跑 EXPLAIN QUERY PLAN；未改任何源码/未迁移/未 commit）
- 结论计数：**13 条发现（P1×4，P2×9）**，另有 4 项"通过项"记录在案

> 说明：EXPLAIN 数据来自本地 e2e 库（logs 仅 481 行），执行计划形状可用于判断缺索引方向，不代表生产量级下的实际代价。

---

## 一、慢查询猎杀与缺失索引（logs/tasks/task_events/usage）

### F1 [P1] GetLogsTraffic：N 天 consume 日志**全量明细拉到 Go 内存**再按日聚合，且读取大 JSON 列 `other`
- 证据：`controller/log.go:166-194`（`LOG_DB.Model(&Log{}).Select("created_at","other").Where("type = ? AND created_at >= ?", LogTypeConsume, start)` 无 LIMIT）；`service/log_traffic.go:24-62` 逐行 `Unmarshal(other)` 后在 Go 内 `map[string]*DailyTraffic` 聚合。
- 实证 EXPLAIN（SQLite）：`SEARCH logs USING INDEX idx_created_at_id (created_at>?)` —— 按时间范围扫**所有类型**日志，`type=2` 在行内过滤；且每条都要读出整段 `other` JSON。
- 影响：`days=30/90` 时一次拉取数十万~百万行 × 每行 JSON，内存/网络/DB IO 全部放大；多管理员同时开仪表盘时叠加；ClickHouse 场景同样逐行传 `other`。
- 修复：聚合下推到 SQL（按日 `GROUP BY`，或物化每日流量表）；至少建 `(type, created_at)` 复合索引；`other` 只查需要的最小字段（流量字段建议拆独立列或物化）。

### F2 [P1] 管理端日志列表/统计缺 `(type, created_at)` 复合索引（现有 `idx_created_at_type` 列序相反）
- 证据：`model/log.go:59-81`（`CreatedAt gorm:"index:idx_created_at_id,priority:1;index:idx_created_at_type"`、`Type gorm:"index:idx_created_at_type"` → 实际 DDL `(created_at,type)`，`type` 等值无法作为前导）；查询 `GetAllLogs` `model/log.go:476-568`（type + model_name/username/token_name/request_id/upstream_request_id + created_at 区间 + channel + group，`ORDER BY created_at desc, id desc`）。
- 实证 EXPLAIN：`GetAllLogs`（type+model_name+username+时间区间）→ `SEARCH logs USING INDEX index_username_model_name (model_name=? AND username=?)` + **`USE TEMP B-TREE FOR ORDER BY`**；只带 type+时间时只能走 `idx_created_at_id` 范围扫。
- 影响：大日志表上管理端每页列表/统计都扫大区间 + 排序，`Count(*)` 与 `Find` 各扫一遍。
- 修复：新增复合索引 `(type, created_at, id)`（推荐顺序：等值 type → 区间 created_at → 排序 id，直接消掉 TEMP B-TREE）；通过 gorm tag 修改即可随 AutoMigrate 落地，但必须过三库验证矩阵。

### F3 [P1] 用户日志/画像查询缺 `(user_id, type, created_at)` 复合索引
- 证据：`GetUserLogs` `model/log.go:572-618`（`user_id` [+type] + model_name/… + created_at 区间，`ORDER BY id desc`）；画像聚合 `service/user_profile/profile.go:211-242`（`queryConsumeRows/queryErrorRows` 30 天明细全量拉取后在 Go 聚合）。
- 实证 EXPLAIN：`GetUserLogs` → `SEARCH logs USING INDEX idx_logs_user_id (user_id=?)` —— 仅用单列索引，把该用户**全部**日志行捞出再过滤/排序；高用量用户（数十万行/月）代价随历史线性增长。
- 影响：日志库越大，用户端翻页/统计/画像越慢；画像服务每次冷启动（60s 缓存过期）都会全量拉 30 天明细。
- 修复：新增 `(user_id, type, created_at)`（覆盖用户列表、`SumUsedQuota` 用户维度、画像聚合）；画像聚合建议一并下推 SQL `GROUP BY model_name/channel_id/date`（同 F1 模式）。

### F4 [P1] task_events 表**无任何保留策略**，主库无限增长
- 证据：`model/task_event.go:38-95`（表定义 + 读函数，无删除）；写入点 `service/task_polling.go:575/580/588/607/621/839`、`service/task_event.go:49-66`；注册表 `model/main.go:356`（AutoMigrate `&TaskEvent{}`，主库表）。全仓 `rg` 未发现任何 `TaskEvent` 的 Delete/Cleanup/Retention 逻辑（实证 EXIT=1 无匹配）。
- 影响：任务每次状态变化（submitted/queued/claimed/progress/step_ratio/succeeded/failed/refunded…）都插一行；任务平台长时间运行后主库无界膨胀，SSE 断线续传只需最近事件（`ListTaskEventsAfter` `model/task_event.go:73-81` 只按 `internal_task_id + id` 读）。
- 修复：增加周期清理（建议保留 7 天，按 `id`/`created_at` 分批 `LIMIT` 删除避免长事务）；清理用时间条件 → 建议给 `created_at` 补索引（当前表只有 `internal_task_id/task_id/user_id` 三个单列索引，`created_at` 无索引）。

### F5 [P2] tasks 轮询/列表查询缺复合索引，轮询扫历史 `submit_time`
- 证据：`model/task.go:56-78`（Task 结构：11 个单列索引，无复合）；`GetAllUnFinishSyncTasks` `model/task.go:385-396`、`GetTimedOutUnfinishedTasks` `model/task.go:355-367`（`progress != '100%' AND status NOT IN (终态) AND submit_time < cutoff`）；`TaskGetAllUserTask` `model/task.go:274-308`（`user_id` + 过滤 + `ORDER BY id desc`）；`GetByTaskId` `model/task.go:459-472`。
- 实证 EXPLAIN：`GetAllUnFinishSyncTasks` → `SEARCH tasks USING INDEX idx_tasks_submit_time (submit_time<?)` —— 每次轮询都扫**历史上所有已提交**任务再过滤终态；`GetByTaskId` → 仅 `idx_tasks_user_id`。
- 影响：轮询是秒级高频路径，tasks 表越大每次扫越多；`NOT IN`/`!=` 使 status/progress 索引基本失效。
- 修复：轮询改正向条件 `status IN (活跃状态)` + 复合索引 `(status, submit_time)`；用户任务列表 `(user_id, id)`；`GetByTaskId` `(user_id, task_id)`。

### F6 [P2] SumUsedQuota/SumUsedToken：仪表盘每页两趟全区间聚合；SumUsedToken 为忽略错误的死代码
- 证据：`model/log.go:626-686`（SumUsedQuota 两趟 `COALESCE(SUM)`，rpm/tpm 仅 60s 窗口）；`SumUsedToken` `model/log.go:688-707`（`tx.Where(...).Scan(&token)` 忽略错误、参数 `tx` 未回接，调用点已注释）。`controller/log.go:106-162`（GetLogsStat/GetLogsSelfStat）。
- 影响：中。缺 `(type, created_at)` 时统计走大区间扫描；`SumUsedToken` 残留死代码。
- 修复：建 F2/F3 复合索引后基本可接受；建议长区间统计设最大天数上限；删除或修复 `SumUsedToken`。

### F7 [P2] 日志列表 COUNT(*) + OFFSET 深翻页
- 证据：`GetAllLogs` `model/log.go:511`、`GetUserLogs` `model/log.go:601`（`tx.Model(&Log{}).Count(&total)` 先全过滤集计数，再 `Offset(startIdx)`）；`logSearchCountLimit = 10000` `model/log.go:570` 仅用户端。
- 影响：深页（>几百页）在大表上重复扫描；管理端默认 100 条/页。
- 修复：keyset 分页（`WHERE (created_at, id) < (?,?)`），或限制可翻页深度。

---

## 二、三库兼容与迁移幂等抽查

### F8 [P2] 迁移整体幂等合规（通过项，记录在案）
- 证据：`migrateDB` `model/main.go:320-398` 采用「先查后改 + 幂等 re-check」：`migrateTokenKeyUniqueness`（仅 PG，`model/token_migration.go:129+`，事务 + `LOCK TABLE` + 约束检查）、`migratePrefillGroupUniqueness`（`model/prefill_group_migration.go:10+`，按库分支 + 冲突白名单校验）、`migrateOptionPrimaryKey`（`model/option_primary_key_migration.go:20-44`，`optionsKeyIsUnique` 前后双检 + MySQL GET_LOCK/PG advisory lock）、`migrateTokenModelLimitsToText` / `migrateSubscriptionPlanPriceAmount`（`model/main.go:583-694`，HasTable/HasColumn + 类型比对 + SQLite 直接跳过 + PG `USING ::decimal`）。
- 结论：未发现幂等缺陷；`ensureSubscriptionPlanTableSQLite`（`model/main.go:504-578`）对 SQLite 手工建表 + ADD COLUMN 增量补齐也合规。

### F9 [P2] 主库无条件 AutoMigrate `&Log{}`：配置独立 LOG_SQL_DSN 时主库仍会创建 logs 表/索引
- 证据：`model/main.go:351`（migrateDB 的 AutoMigrate 列表含 `&Log{}`，不区分是否配置了独立日志库）；`migrateLOGDB` `model/main.go:400-408` 再对日志库 AutoMigrate 同一结构。
- 影响：独立日志库部署下，主库多一张无用 logs 表 + 14 个索引，每次重启重复 DDL（幂等但多余）；容易误写（`LOG_DB=DB` 与独立库的分支见 `model/main.go:230-239`）。
- 修复：主库 AutoMigrate 列表按「是否配置 LOG_SQL_DSN」条件移除 `&Log{}`（保留 `MigrateAuditLogs` 已按日志库处理）。

### F10 [P2] JSON 列三库兼容模式合规，但新字段必须沿用（PG simple protocol bytea 陷阱有注释示范）
- 证据：`model/task.go:104-115`（`Properties.Value()` 显式返回 string 并注明 "PG simple protocol 下 []byte 按 bytea 编码,写 json 列会触发 SQLSTATE 22P02"）；`Task.Data`/`Task.PrivateData`、`TaskEvent.Data`（`model/task_event.go:45`）、`AuditLog.Other`（`model/audit_log.go:45`）均为 `gorm:"type:json"` + `json.RawMessage`。
- 影响/结论：`json.RawMessage` 走 GORM 内建 JSON 序列化，三库可用（实证 e2e SQLite DDL `data json`）；新增 JSON 列必须继续用 `json.RawMessage` 或显式 Valuer 返回 string，禁止裸 `[]byte`。
- 备注（影子价）：B5-2 `api_equivalent_usd` 写在 `Log.Other` JSON 内（`model/log.go:351-356`），无独立列 → 无迁移负担；但跨库无法对 JSON 内字段建索引做聚合（SQLite/MySQL 无 jsonb 路径），若后续要按影子价统计，应物化为列/独立表（前瞻，P2）。

### F11 [P2] web_request_logs：uint64 主键三库映射有差异；其余表定义合规
- 证据：`model/web_request_log.go:7-19`（`ID uint64 gorm:"primaryKey"`；`ip/window_start` 有索引；`DeleteWebRequestLogsBefore` 7 天保留 `:27-33`，批量写入 `RecordWebRequestLogs` `:35-42` 事务化）。
- 实证：e2e SQLite DDL `id integer PRIMARY KEY AUTOINCREMENT`。MySQL 映射 `BIGINT UNSIGNED AUTO_INCREMENT`；PostgreSQL 下 GORM 按 signed bigint+序列生成 —— 实际序列远小于 int64 上限，风险低，但跨库类型不一致。
- 修复：如要严格一致改为 `int64` 主键；保留 7 天策略合规（与 F4 的 task_events 形成对比）。

---

## 三、SQL 安全、锁与死锁

### F12 [P2] 未发现 raw SQL 注入点（通过项，记录在案）
- 证据：raw SQL 全部参数绑定（`model/db_time.go:12-16`、`model/main.go:541/576/605-628`、`model/web_request_log.go:63-88`）；用户可控排序全部白名单化（`NewChannelSortOptions` `model/channel.go:90-105`、`NewUserSortOptions` `model/user.go:48-62`、`AggregateWebRequestLogs` 的 `sort` switch `model/web_request_log.go:72-82`）；LIKE 模式经 `sanitizeLikePattern` + `ESCAPE '!'`（`model/token.go:142-156`、`model/log.go:33-47`，ClickHouse 单独 sanitize）；`logGroupCol`/`logKeyCol` 按库引号化（`model/main.go:58-62`）。
- 结论：`applyExplicitLogTextFilter` 的列名来自内部字面量集合，不来自用户输入。

### F13 [P2] 锁模式合规，但多事务跨表加锁顺序不统一（待评审）；SQLite 无 FOR UPDATE 属预期
- 证据：统一 helper `lockForUpdate` `model/locking.go:20-25`（MySQL/PG 发 `FOR UPDATE`，SQLite 跳过——注释说明 single-writer 冲突事务失败）；全仓无遗留 `Set("gorm:query_option","FOR UPDATE")`（rg 实证）；配额预扣为原子条件更新/Redis Lua：`reserveUserQuotaDB` `model/quota_reserve.go:144-149`（`WHERE id=? AND quota >= ?`）、`TryReserveUserQuota` `:165-199`、Lua 脚本 `:20-66`；使用点 `WalletFunding.PreConsume` `service/funding_source.go:42-55`、回滚 `service/billing_session.go:187-245`。
- 影响/结论：
  - 计费/钱包、令牌、充值回调、auth_flow、2FA、model_meta 均走原子更新或 `lockForUpdate`，未发现 check-then-act 竞态；
  - 但跨表加锁顺序不统一（如 `twofa.go:171` 锁 User 后锁 TwoFA，auth_flow 锁 AuthFlow→UserSession，topup 锁 TopUp→User 等），理论上有死锁可能；建议一次代码评审统一"主资源→从资源"顺序（P2）。
  - `DeleteOldLogBatch` `model/log.go:717-751` 每批 `LIMIT 100` 小事务删除 + `CountOldLog`，无长事务/锁表风险；WebRequestLog 批量写走事务但每批 200 行（`model/web_request_log.go:35-42`），风险低。

---

## 四、实证：EXPLAIN QUERY PLAN（本地 SQLite 只读，`$env:TEMP\e2e-data\one-api.db`）

| 查询 | 计划 | 结论 |
|---|---|---|
| GetLogsTraffic（type=2 + created_at>=） | `SEARCH logs USING INDEX idx_created_at_id (created_at>?)` | 时间范围扫全类型 → 缺 (type, created_at) |
| GetUserLogs（user_id + type + 区间） | `SEARCH logs USING INDEX idx_logs_user_id (user_id=?)` | 单列索引 → 缺 (user_id, type, created_at) |
| GetAllLogs（type+model+user+区间） | `index_username_model_name` + `USE TEMP B-TREE FOR ORDER BY` | 排序无法用索引 → 缺 (type, created_at, id) |
| SumUsedQuota（username+区间+type） | `SEARCH logs USING INDEX idx_logs_username` | 可用但区间聚合代价随数据增长 |
| GetAllUnFinishSyncTasks | `SEARCH tasks USING INDEX idx_tasks_submit_time (submit_time<?)` | 轮询扫历史 → 缺 (status, submit_time) |
| GetByTaskId | `SEARCH tasks USING INDEX idx_tasks_user_id` | 缺 (user_id, task_id) |
| TaskEventsAfter | `SEARCH task_events USING INDEX idx_task_events_internal_task_id` | 可接受；表无清理策略（F4） |

## 五、修复优先级速览

| 优先级 | 条目 | 一句话 |
|---|---|---|
| P1 | F1 | GetLogsTraffic 改 SQL 聚合/物化，勿全量拉 other |
| P1 | F2 | logs 加 (type, created_at, id) 复合索引 |
| P1 | F3 | logs 加 (user_id, type, created_at) 复合索引；画像聚合下推 SQL |
| P1 | F4 | task_events 增加保留策略 + created_at 索引 |
| P2 | F5 | tasks 轮询改 status IN 活跃态 + (status, submit_time)；GetByTaskId 加 (user_id, task_id) |
| P2 | F6 | 统计聚合上限天数；清理 SumUsedToken 死代码 |
| P2 | F7 | 日志列表 keyset 分页替代深 OFFSET |
| P2 | F8 | 迁移幂等合规（通过项，无需动作） |
| P2 | F9 | 独立 LOG_SQL_DSN 时主库不再 AutoMigrate &Log{} |
| P2 | F10 | JSON 列继续用 json.RawMessage/显式 Valuer；影子价若需聚合则物化列 |
| P2 | F11 | web_request_logs uint64 PK 跨库差异（低风险） |
| P2 | F12 | 注入面干净（通过项，无需动作） |
| P2 | F13 | 统一跨表加锁顺序评审；其余锁/配额原子化合规 |

> 只读审计：未修改源码、未执行迁移、未 commit/push。
