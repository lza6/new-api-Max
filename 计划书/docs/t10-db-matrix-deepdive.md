# 专项分析 · 数据库三库矩阵与迁移幂等（T10 纵深 + AGENTS 硬规则）

> 定位：主指南 §T10 与 AGENTS 数据库纪律的**深挖文档**：矩阵方法、常见坑、锚点；只读整理。
> 生成：2026-09-25 · 证据：perf-ledger 0002/0010（SQLite+MySQL 9.6.0+PG 16.14，7/7 PASS，76.3s）。

## 1. 硬规则（AGENTS）
1. 所有 DB 代码必须 SQLite / MySQL ≥5.7.8 / PostgreSQL ≥9.6 同时可用；真实实例验证，单库/单测不算。
2. 迁移：全新库 + 既有库各跑两次（幂等）；`AutoMigrate` 重复 ALTER 要避免（布尔 default 等）。
3. 行锁：`SELECT ... FOR UPDATE` 必须走 `model.lockForUpdate(tx)`；禁用 GORM v1 `Set("gorm:query_option", ...)`。
4. raw SQL 方言：PG `"col"` vs MySQL/SQLite `` `col` ``；保留字 `group/key` 用 `commonGroupCol/commonKeyCol`；布尔用 `commonTrueVal/commonFalseVal`；分支用 `common.UsingMainDatabase/UsingLogDatabase`。
5. 主键由 GORM 生成；禁 `AUTO_INCREMENT`/`SERIAL` 直写；JSON 列有 TEXT 回退；SQLite 用 `ADD COLUMN`。
6. GORM core + dialect 视为兼容版本集，升级需整套矩阵。

## 2. 矩阵脚本与复验（现有）
- `scripts/db-conformance.ps1`；命令 `go test ./model/ -run TestDBConformance -v -count=1`。
- 本机实例：MySQL 9.6.0（127.0.0.1:3306）、PG 16.14（127.0.0.1:5432，服务 State=Stopped 但进程存活——`pg_ctl status` 确认）；Start-Service 在已运行时可能误报。
- 覆盖：AutoMigrate 幂等 / logs 索引 / 事件去重 / 保留列 / JSON 往返 / FOR UPDATE 锁 / DB 分支（7 测试）。

## 3. 待办（主指南 T10）
- CI 三库矩阵 job（容器 PG/MySQL + SQLite）或固化本地脚本证据；
- 迁移幂等脚本输出明确「全新+升级各两次」；
- 慢查询热点（用户/日志/任务/订阅）EXPLAIN 记录；
- 日志归档配置（对齐 TASK_EVENT_RETENTION_DAYS），可配开关 + 测试。

## 4. 常见坑清单（执行 AI 对照）
- 改了 model/GORM 依赖/迁移逻辑 → 重跑矩阵（perf-ledger 下次不再重复跑规则）。
- `gorm:"default:true"` 会造成 MySQL/PG 重复 ALTER → 用代码层默认。
- `*uint` 巨大数回绕 → 上界；`int(float64)` 裸转换 → quota_math。
- 软删除索引/复合索引缺失 → 慢查询；先 EXPLAIN 再建索引。

## 5. 验证命令
- `powershell -File scripts/db-conformance.ps1`（本机需 MySQL/PG 服务与 TEST_DSN env）。
- 未改 schema 时跳过（台账防重复跑）；改到则必须跑并回填 `db_structure.md` + ledger。

## 闭环状态（2026-09-27，v1.3.44 回填）
- ✅ CI 三库矩阵已存在（.github/workflows/ci.yml db-conformance job：SQLite+MySQL8.4+PG16，TestDBConformance）。
- ✅ 线上真实 PG EXPLAIN 核对：perf-ledger 0012（S7 索引全生效：banned_ips.expires_at / top_ups.create_time / task_events.created_at）。
- ✅ 日志归档：service/task_event_cleanup.go TASK_EVENT_RETENTION_DAYS（默认 7，0=禁用）+ 每小时主节点清理。
