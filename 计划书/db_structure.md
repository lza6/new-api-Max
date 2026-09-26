# 数据库结构权威源（db_structure.md）

> 基线版本：2026-09-25 · 从 git HEAD（a0d0589f5, v1.3.28）恢复，内容与 HEAD 版本一致（生成 2026-09-24，从 `model/` GORM 标签抽取）。
> 更新纪律：任何 schema 变更（模型字段/索引/迁移）后**必须回填本文件**并跑三库矩阵。
> 兼容矩阵：SQLite / MySQL ≥5.7.8 / PostgreSQL ≥9.6（AGENTS 硬性要求）。

## 约定
- 主键：GORM 默认 `id`（bigint/自增，跨库由 GORM 处理，禁用 AUTO_INCREMENT/SERIAL 直写）。
- 软删除：`deleted_at`（gorm.DeletedAt，已索引）。
- 保留字列（group/key）经 `commonGroupCol/commonKeyCol` 引用。

## 核心表

### users（model/user.go:91）
| 列 | 类型 | 约束/索引 |
|---|---|---|
| id | bigint PK | auto |
| username | varchar(20) | **unique** + index（user.go:93） |
| password | 变长 | not null；argon2id 哈希（不存明文） |
| display_name | varchar(20) | index |
| email | varchar(50) | index |
| github_id/discord_id/oidc_id/wechat_id/telegram_id/linux_do_id | varchar | index |
| access_token | char(32) | **uniqueIndex**（系统管理 token） |
| aff_code | varchar(32) | **uniqueIndex** |
| inviter_id | int | index |
| deleted_at | datetime | index（软删除） |
| source | varchar(64) | index |
| stripe_customer | varchar(64) | index |

### logs（model/log.go:61）
| 列 | 类型 | 约束/索引 |
|---|---|---|
| id | bigint PK | idx_created_at_id / idx_user_id_id / idx_log_type_created_id |
| user_id | int | index + idx_user_id_id(1) + idx_log_user_type_created(1) |
| created_at | bigint | idx_created_at_id(1) / idx_log_type_created_id(2) / idx_log_user_type_created(3) |
| type | int | idx_log_type_created_id(1) / idx_log_user_type_created(2) |
| username | varchar | index + index_username_model_name(2) |
| token_name | varchar | index |
| model_name | varchar | index + index_username_model_name(1) |
| request_bytes/response_bytes | bigint | idx_logs_traffic(2/3) |
| channel_id | int | index |
| token_id | int | index |
| group | varchar | index（保留字走 commonGroupCol） |
| ip | varchar | index |
| request_id | varchar(64) | idx_logs_request_id（T5 全链路追踪） |
| upstream_request_id | varchar(128) | idx_logs_upstream_request_id |

### channels（model/channel.go:23）
- 核心列：id、name、type、key、base_url、models、group、status、weight、priority、probe_result（text，B4 probe 报告 JSON）。
- 说明：渠道-模型-分组映射经 `abilities` 表（model/ability.go:18，group+model+enabled+priority 复合查询）。

### tokens（model/token.go:14）
- 核心列：id、user_id、key（哈希/指纹）、name、status、expired_time、unlimited_quota、models。
- 约束：user_id 索引；key 唯一。

### subscription_plans（model/subscription.go:148）
- 核心列：id、name、price、duration、concurrency_limit、rpm_limit、models（SQLite 迁移含 create+alter，见 v1.3.10 修复）。

### user_subscriptions（model/subscription.go:263）
- 核心列：id、user_id、plan_id、start_at、end_at、status。

### system_instances（model/system_instance.go）
- 核心列：id、node_name、started_at、last_seen_at、info（节点规格/负载/磁盘上报，供 server-stats 状态页）。
- 约束：node_name 唯一；stale 判定 SystemInstanceStaleAfterSeconds。

### banned_ips（model/banned_ip.go:14，Web 防刷封禁）
- 核心列：id、ip、reason、banned_at、expires_at、banned_by。
- 约束：ip 唯一（uniqueIndex）；expires_at 索引（v1.3.41 S7：DeleteExpiredBannedIPs 按 expires_at 范围清扫）。

### top_ups（model/topup.go:15，充值/兑换入账）
- 核心列：id、user_id、amount、money、trade_no、payment_method、payment_provider、create_time、complete_time、status。
- 约束：user_id 索引；trade_no 唯一；create_time 索引（v1.3.41 S7：user_id + create_time 范围分页查询）。

## 索引策略（慢查询猎杀结论）
- logs 复合索引覆盖高频查询：按时间（idx_created_at_id）、按用户（idx_user_id_id）、
  按类型+时间（idx_log_type_created_id）、按用户+类型+时间（idx_log_user_type_created）、
  按流量（idx_logs_traffic）、按 request_id 追踪（idx_logs_request_id）。
- users 唯一索引防重复注册：username / aff_code / access_token。

## 验证
- HEAD 版记录：三库矩阵真实通过 2026-09-24 SQLite + MySQL 9.6.0 + PG 16.14，7/7 conformance PASS（ledger 0002）。
- v1.3.41：新增 banned_ips.expires_at / top_ups.create_time 索引，SQLite AutoMigrate 冒烟
  `go test ./model/ -run TestS7IndexAutoMigrateSmoke -count=1` PASS（HasIndex 断言）。
- 本次复验：2026-09-25 v1.3.28 重新跑 `go test ./model/ -run TestDBConformance -v -count=1`，7/7 PASS（76.3s），
  MySQL 9.6.0 + PG 16.14 真实连接（含 AutoMigrate 幂等：首次建表 → 二次零变更）。见 perf-verification-ledger.md 记录 0010
  与原始输出 `计划书/audit/dbconformance-2026-09-25.log`。
- schema 未变更：本次文档治理轮次不产生迁移；后续 schema 改动后必须重跑矩阵并回填本文件。