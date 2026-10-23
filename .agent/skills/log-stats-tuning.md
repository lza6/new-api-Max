# Skill：日志与统计性能调优（含 OOM 事故三段式）

## 适用场景
- 统计接口变慢（`/api/log/traffic`、带宽排行、`/api/site/stats`、`/api/log/report`）
- PostgreSQL 容器被 OOM killer 杀进程 / 崩溃恢复 / 统计信息被清空
- `logs` 表持续增长，需要判断「加索引 / 加缓存 / 做预聚合」哪一层才对症

## 前置读取
1. 记忆 `new-api-pg-oom-and-sql-tuning-v13187.md`（事故全貌与配置矩阵）
2. 记忆 `new-api-log-daily-stats-preactivation-v13190.md`（预聚合设计与 `migrateLOGDB` 未解根因）
3. `controller/log.go`（4 个统计接口）、`model/log_daily_stats.go`
4. `test_ledger.md` 的已知噪声表（避免误判回归）

## 三段式处理（**顺序不能颠倒**）

### 第 1 段：止血 —— 不改代码，只调环境
1. 确认 OOM：`cat /sys/fs/cgroup/.../memory.events` 看 `oom_kill`；`dmesg -T | grep oom`
2. 从 **Postgres 日志**捞元凶 SQL：`docker logs postgres | grep "Failed process was running"`
3. 扩容：容器 `mem_limit`、`shared_buffers`、`work_mem`、`maintenance_work_mem`
4. 关键三改：
   - `jit = off`（256MB 容器里 JIT 是纯负收益，且是 OOM 的最后一根稻草）
   - `random_page_cost = 1.1`（**默认 4 是机械盘值**，会让 index-only scan 成本虚高而被放弃）
   - `shm_size: 256m`（**否则 `maintenance_work_mem > 64MB` 时 VACUUM 报 `could not resize shared memory segment`**，且 autovacuum 会永久失败）
5. `ANALYZE` 重建统计信息（崩溃恢复后会清空）

### 第 2 段：对症 —— 覆盖索引
1. 先 EXPLAIN 看清是 `Seq Scan` 还是已经在用索引
2. 只有「列不在索引里导致回表」时，加 `CREATE INDEX CONCURRENTLY ... INCLUDE (...)`
3. **建完必须 `VACUUM`**：index-only scan 依赖 visibility map，`relallvisible` 从 78% → 99.99% 才能把 `Heap Fetches` 从 28 万降到 81
4. 选择率 >30% 时不要指望索引能赢全表扫——那是预聚合的信号

### 第 3 段：根治 —— 预聚合
1. 建汇总表，维度**只选字符串**（数字维度要 CAST，三库语法不同）
2. 刷新用「删窗口 + 整批重插」（GORM `OnConflict` 在 MySQL 上会渲染空子句）
3. 查询侧必须做**覆盖判断**：`MinDay <= fromDay && MaxDay >= today-86400`，否则回退原扫表路径
4. 首次部署表空时回填，回填上限要有封顶（400 天）
5. **把 `AutoMigrate` 兜底到使用点**，不要只依赖 `migrateLOGDB`

## 验收标准
| 项 | 门槛 |
|---|---|
| 汇总值与原始聚合 | **逐位相等**（`WITH t AS (...) SELECT ... UNION ALL SELECT ...` 对比） |
| 重复刷新 | 行数不变、数值不重复累加（幂等） |
| `type=5` 错误日志 | 不得计入消费统计 |
| 覆盖判断 | 空表返回 false；未回填的历史返回 false |
| 接口耗时 | 分页统计 <100ms，历史对比记录在案 |

## 证据要求
- EXPLAIN / EXPLAIN ANALYZE 前后对比（含 `Buffers` 与 `Heap Fetches`）
- 汇总 vs 原始聚合的对比 SQL 与实际输出
- 接口 curl 实测耗时（≥3 次）
- 事故时间线：OOM 时刻 ↔ 503 分钟的对应关系

## 回滚方式
- 预聚合：`DROP TABLE log_daily_stats`，查询自动回退（`DailyStatsCovers` 返回 false）
- 索引：`DROP INDEX CONCURRENTLY`
- 配置：compose 有 `.bak-*`；`ALTER SYSTEM` 值在 `postgresql.auto.conf`

## 常见坑
1. `docker exec` 里 `grep` 二进制要加 `-a`，否则假 0 命中
2. `pg_reload_conf()` 不改变**已存在会话**的 `pg_settings` 显示值，必须新连接验证
3. compose `command: -c xxx=` 优先级高于 `postgresql.auto.conf`，改参数必须改 compose
4. `pg_stat_user_indexes.idx_scan` 崩溃恢复后会被重置，**不能**据此判定索引无用
5. 删行不释放磁盘，`VACUUM` 只更新 FSM 供复用；`VACUUM FULL` 会锁表
6. 统计数字「变小」优先查汇总表 min/max `day_key` 是否覆盖查询窗口，而不是查查询逻辑
