# db_structure.md — 数据库结构权威参考

> **本文件由脚本生成，禁止手改。** 手抄 schema 一定漂移；改结构请改 `model/` 与迁移代码，
> 然后按下方命令重新生成并提交本文件。

## 0. 权威关系声明

| 问题 | 权威来源 |
|---|---|
| 表 / 字段 / 索引 / 约束 | **本文件**（由真实 AutoMigrate 结果生成） |
| 业务含义与字段语义 | `model/*.go` 的注释与校验逻辑 |
| 当前任务状态 | `project_specs.md` |
| 线上运行版本 | `workflow_status.md` §0 |
| 验证记录 / 噪声 | `test_ledger.md`、`计划书/audit-ledger.md` |

## 1. 生成方式（可复现）

```bash
# 1) 构建（不注入版本，只为跑一次迁移）
go build -o /tmp/new-api-dump .
# 2) 用一次性 SQLite 启动，触发 AutoMigrate（数据不落生产）
SQLITE_PATH=/tmp/spec.db PORT=3311 SESSION_SECRET=dump GIN_MODE=release /tmp/new-api-dump &
# 3) 生成
node scripts/gen-db-structure.mjs --db /tmp/spec.db --out db_structure.md
```

## 2. 本次生成事实

| 项 | 值 |
|---|---|
| 生成时间 | 2026-10-23T03:50:14.128Z |
| 引擎 | SQLite（AutoMigrate 真实执行） |
| 表数量 | 55 |
| 字段总数 | 637 |
| 生成命令 | `node scripts/gen-db-structure.mjs --db spec.db --out db_structure.md` |
| 源文件 | `spec.db`（一次性库，非生产） |

**已知边界（必须知道，否则会误判）**

1. SQLite 是「无服务即可跑」的代表方言，**不保证** MySQL / PostgreSQL 的类型字节级一致：
   例如 `bool` 在 SQLite 是整数、在 PG 是真布尔；`text` 长度限制由 GORM tag 决定，SQLite 不强制。
2. 索引名在各方言可能被截断或加后缀；本文件记录的是 SQLite 的实际命名。
3. 本文件只覆盖**主库**。日志库（`LOG_SQL_DSN`，可指向 ClickHouse）结构不同，见 §4。
4. 任何 schema 变更都必须按 `AGENTS.md` 的三库验证要求跑 `make db-check`。

## 3. 表清单

| # | 表 | 字段数 | 索引数 | 本次迁移后行数 |
|---|---|---|---|---|
| 1 | `abilities` | 7 | 5 | 0 |
| 2 | `action_approvals` | 11 | 5 | 0 |
| 3 | `audit_logs` | 19 | 8 | 0 |
| 4 | `auth_flows` | 11 | 5 | 0 |
| 5 | `authz_roles` | 9 | 1 | 2 |
| 6 | `banned_ips` | 6 | 2 | 0 |
| 7 | `casbin_rule` | 8 | 2 | 3 |
| 8 | `channel_combos` | 9 | 2 | 0 |
| 9 | `channels` | 31 | 2 | 0 |
| 10 | `checkins` | 5 | 1 | 0 |
| 11 | `custom_oauth_providers` | 21 | 1 | 0 |
| 12 | `event_deliveries` | 9 | 5 | 0 |
| 13 | `external_identity_claims` | 5 | 3 | 0 |
| 14 | `login_encryption_keys` | 3 | 1 | 0 |
| 15 | `logs` | 25 | 16 | 0 |
| 16 | `mcp_sessions` | 8 | 5 | 0 |
| 17 | `midjourneys` | 24 | 8 | 0 |
| 18 | `model_compression_stats` | 6 | 1 | 0 |
| 19 | `model_costs` | 11 | 1 | 0 |
| 20 | `models` | 14 | 3 | 0 |
| 21 | `open_ai_ops_objects` | 5 | 2 | 0 |
| 22 | `options` | 2 | 1 | 1 |
| 23 | `passkey_credentials` | 18 | 3 | 0 |
| 24 | `perf_metrics` | 11 | 2 | 0 |
| 25 | `prefill_groups` | 8 | 3 | 0 |
| 26 | `quota_data` | 12 | 8 | 0 |
| 27 | `redemption_usages` | 6 | 3 | 0 |
| 28 | `redemptions` | 13 | 4 | 0 |
| 29 | `setups` | 3 | 0 | 0 |
| 30 | `site_daily_visitors` | 10 | 3 | 0 |
| 31 | `subscription_orders` | 11 | 4 | 0 |
| 32 | `subscription_plan_grants` | 6 | 1 | 0 |
| 33 | `subscription_plans` | 27 | 0 | 0 |
| 34 | `subscription_pre_consume_records` | 8 | 5 | 0 |
| 35 | `system_instances` | 6 | 5 | 1 |
| 36 | `system_task_locks` | 5 | 5 | 0 |
| 37 | `system_tasks` | 12 | 7 | 2 |
| 38 | `task_events` | 7 | 4 | 0 |
| 39 | `task_plugins` | 12 | 3 | 0 |
| 40 | `tasks` | 19 | 12 | 0 |
| 41 | `tokens` | 19 | 4 | 0 |
| 42 | `top_ups` | 10 | 4 | 0 |
| 43 | `training_data_consents` | 8 | 1 | 0 |
| 44 | `training_data_export_downloads` | 11 | 1 | 0 |
| 45 | `training_data_exports` | 16 | 3 | 0 |
| 46 | `training_data_watermarks` | 10 | 1 | 0 |
| 47 | `two_fa_backup_codes` | 7 | 2 | 0 |
| 48 | `two_fas` | 10 | 3 | 0 |
| 49 | `user_oauth_bindings` | 5 | 2 | 0 |
| 50 | `user_sessions` | 16 | 5 | 0 |
| 51 | `user_subscriptions` | 23 | 6 | 0 |
| 52 | `users` | 32 | 16 | 0 |
| 53 | `vendors` | 8 | 2 | 0 |
| 54 | `web_request_logs` | 11 | 2 | 0 |
| 55 | `webhook_endpoints` | 8 | 2 | 0 |

## 4. 逐表结构

### `abilities`

| 字段 | 类型 | NOT NULL | 主键 | 默认值 |
|---|---|---|---|---|
| `group` | varchar(64) |  | yes |  |
| `model` | varchar(255) |  | yes |  |
| `channel_id` | INTEGER |  | yes |  |
| `enabled` | numeric |  |  |  |
| `priority` | INTEGER |  |  | `0` |
| `weight` | INTEGER |  |  | `0` |
| `tag` | TEXT |  |  |  |

索引：
- `idx_abilities_weight` on (weight)
- `idx_abilities_tag` on (tag)
- `idx_abilities_channel_id` on (channel_id)
- `idx_abilities_priority` on (priority)
- `sqlite_autoindex_abilities_1`（UNIQUE） on (group, model, channel_id)

### `action_approvals`

| 字段 | 类型 | NOT NULL | 主键 | 默认值 |
|---|---|---|---|---|
| `id` | INTEGER |  | yes |  |
| `user_id` | INTEGER |  |  |  |
| `action` | varchar(64) | yes |  |  |
| `resource` | varchar(191) | yes |  |  |
| `status` | varchar(20) | yes |  |  |
| `reason` | TEXT |  |  |  |
| `admin_id` | INTEGER |  |  |  |
| `created_at` | INTEGER |  |  |  |
| `updated_at` | INTEGER |  |  |  |
| `expires_at` | INTEGER |  |  |  |
| `consumed_at` | INTEGER |  |  |  |

索引：
- `idx_action_approvals_created_at` on (created_at)
- `idx_action_approvals_expires_at` on (expires_at)
- `idx_action_approvals_user_id` on (user_id)
- `idx_action_approvals_action` on (action)
- `idx_action_approvals_status` on (status)

### `audit_logs`

| 字段 | 类型 | NOT NULL | 主键 | 默认值 |
|---|---|---|---|---|
| `id` | INTEGER |  | yes |  |
| `event_id` | varchar(64) |  |  |  |
| `user_id` | INTEGER |  |  |  |
| `username` | varchar(64) |  |  |  |
| `actor_role` | INTEGER |  |  |  |
| `created_at` | bigint |  |  |  |
| `category` | varchar(24) |  |  |  |
| `action` | varchar(128) |  |  |  |
| `token_ref` | varchar(64) |  |  |  |
| `auth_method` | varchar(24) |  |  |  |
| `ip` | varchar(64) |  |  |  |
| `user_agent` | varchar(512) |  |  |  |
| `method` | varchar(16) |  |  |  |
| `route` | varchar(255) |  |  |  |
| `status` | INTEGER |  |  |  |
| `success` | numeric |  |  |  |
| `request_id` | varchar(64) |  |  |  |
| `content` | TEXT |  |  |  |
| `other` | json |  |  |  |

索引：
- `idx_audit_logs_category` on (category)
- `idx_audit_logs_request_id` on (request_id)
- `idx_audit_logs_event_id`（UNIQUE） on (event_id)
- `idx_audit_created_event` on (created_at, event_id)
- `idx_audit_user_time` on (user_id, created_at)
- `idx_audit_logs_username` on (username)
- `idx_audit_token_time` on (token_ref, created_at)
- `idx_audit_logs_created_at` on (created_at)

### `auth_flows`

| 字段 | 类型 | NOT NULL | 主键 | 默认值 |
|---|---|---|---|---|
| `id` | INTEGER |  | yes |  |
| `token_hash` | char(64) | yes |  |  |
| `purpose` | varchar(32) | yes |  |  |
| `provider` | varchar(64) |  |  |  |
| `intent` | varchar(16) |  |  |  |
| `user_id` | INTEGER |  |  |  |
| `session_id` | varchar(64) |  |  |  |
| `payload` | TEXT |  |  |  |
| `created_at` | datetime |  |  |  |
| `expires_at` | datetime | yes |  |  |
| `consumed_at` | datetime |  |  |  |

索引：
- `idx_auth_flow_purpose_expiry` on (purpose, expires_at)
- `idx_auth_flows_user_id` on (user_id)
- `idx_auth_flows_session_id` on (session_id)
- `idx_auth_flows_consumed_at` on (consumed_at)
- `idx_auth_flows_token_hash`（UNIQUE） on (token_hash)

### `authz_roles`

| 字段 | 类型 | NOT NULL | 主键 | 默认值 |
|---|---|---|---|---|
| `id` | INTEGER |  | yes |  |
| `key` | TEXT | yes |  |  |
| `name` | TEXT | yes |  |  |
| `description` | TEXT |  |  |  |
| `built_in` | numeric |  |  |  |
| `enabled` | numeric |  |  |  |
| `sort` | INTEGER |  |  |  |
| `created_at` | INTEGER |  |  |  |
| `updated_at` | INTEGER |  |  |  |

索引：
- `idx_authz_roles_key`（UNIQUE） on (key)

### `banned_ips`

| 字段 | 类型 | NOT NULL | 主键 | 默认值 |
|---|---|---|---|---|
| `id` | INTEGER |  | yes |  |
| `ip` | varchar(64) | yes |  |  |
| `reason` | varchar(256) |  |  |  |
| `banned_at` | INTEGER |  |  |  |
| `expires_at` | INTEGER |  |  |  |
| `banned_by` | varchar(64) |  |  |  |

索引：
- `idx_banned_ips_ip`（UNIQUE） on (ip)
- `idx_banned_ips_expires_at` on (expires_at)

### `casbin_rule`

| 字段 | 类型 | NOT NULL | 主键 | 默认值 |
|---|---|---|---|---|
| `id` | INTEGER |  | yes |  |
| `ptype` | TEXT |  |  |  |
| `v0` | TEXT |  |  |  |
| `v1` | TEXT |  |  |  |
| `v2` | TEXT |  |  |  |
| `v3` | TEXT |  |  |  |
| `v4` | TEXT |  |  |  |
| `v5` | TEXT |  |  |  |

索引：
- `idx_casbin_rule` on (ptype, v0, v1, v2, v3, v4, v5)
- `idx_casbin_rule_unique`（UNIQUE） on (ptype, v0, v1, v2, v3, v4, v5)

### `channel_combos`

| 字段 | 类型 | NOT NULL | 主键 | 默认值 |
|---|---|---|---|---|
| `id` | INTEGER |  | yes |  |
| `name` | varchar(64) |  |  |  |
| `strategy` | varchar(20) |  |  | `"fallback"` |
| `models` | TEXT |  |  |  |
| `status` | INTEGER |  |  | `1` |
| `sticky` | INTEGER |  |  | `1` |
| `created` | INTEGER |  |  |  |
| `updated` | INTEGER |  |  |  |
| `deleted_at` | datetime |  |  |  |

索引：
- `idx_channel_combos_deleted_at` on (deleted_at)
- `idx_channel_combos_name`（UNIQUE） on (name)

### `channels`

| 字段 | 类型 | NOT NULL | 主键 | 默认值 |
|---|---|---|---|---|
| `id` | INTEGER |  | yes |  |
| `type` | INTEGER |  |  | `0` |
| `key` | TEXT | yes |  |  |
| `open_ai_organization` | TEXT |  |  |  |
| `test_model` | TEXT |  |  |  |
| `status` | INTEGER |  |  | `1` |
| `name` | TEXT |  |  |  |
| `weight` | INTEGER |  |  | `0` |
| `created_time` | INTEGER |  |  |  |
| `test_time` | INTEGER |  |  |  |
| `response_time` | INTEGER |  |  |  |
| `base_url` | TEXT |  |  | `""` |
| `other` | TEXT |  |  |  |
| `balance` | REAL |  |  |  |
| `balance_updated_time` | INTEGER |  |  |  |
| `models` | TEXT |  |  |  |
| `group` | varchar(64) |  |  | `"default"` |
| `used_quota` | INTEGER |  |  | `0` |
| `model_mapping` | TEXT |  |  |  |
| `status_code_mapping` | varchar(1024) |  |  | `""` |
| `priority` | INTEGER |  |  | `0` |
| `auto_ban` | INTEGER |  |  | `1` |
| `other_info` | TEXT |  |  |  |
| `probe_result` | TEXT |  |  |  |
| `tag` | TEXT |  |  |  |
| `setting` | TEXT |  |  |  |
| `param_override` | TEXT |  |  |  |
| `header_override` | TEXT |  |  |  |
| `remark` | varchar(255) |  |  |  |
| `channel_info` | json |  |  |  |
| `settings` | TEXT |  |  |  |

索引：
- `idx_channels_name` on (name)
- `idx_channels_tag` on (tag)

### `checkins`

| 字段 | 类型 | NOT NULL | 主键 | 默认值 |
|---|---|---|---|---|
| `id` | INTEGER |  | yes |  |
| `user_id` | INTEGER | yes |  |  |
| `checkin_date` | varchar(10) | yes |  |  |
| `quota_awarded` | INTEGER | yes |  |  |
| `created_at` | INTEGER |  |  |  |

索引：
- `idx_user_checkin_date`（UNIQUE） on (user_id, checkin_date)

### `custom_oauth_providers`

| 字段 | 类型 | NOT NULL | 主键 | 默认值 |
|---|---|---|---|---|
| `id` | INTEGER |  | yes |  |
| `name` | varchar(64) | yes |  |  |
| `slug` | varchar(64) | yes |  |  |
| `icon` | varchar(128) |  |  | `""` |
| `enabled` | numeric |  |  | `false` |
| `client_id` | varchar(256) |  |  |  |
| `client_secret` | varchar(512) |  |  |  |
| `authorization_endpoint` | varchar(512) |  |  |  |
| `token_endpoint` | varchar(512) |  |  |  |
| `user_info_endpoint` | varchar(512) |  |  |  |
| `scopes` | varchar(256) |  |  | `"openid profile email"` |
| `user_id_field` | varchar(128) |  |  | `"sub"` |
| `username_field` | varchar(128) |  |  | `"preferred_username"` |
| `display_name_field` | varchar(128) |  |  | `"name"` |
| `email_field` | varchar(128) |  |  | `"email"` |
| `well_known` | varchar(512) |  |  |  |
| `auth_style` | INTEGER |  |  | `0` |
| `access_policy` | TEXT |  |  |  |
| `access_denied_message` | varchar(512) |  |  |  |
| `created_at` | datetime |  |  |  |
| `updated_at` | datetime |  |  |  |

索引：
- `idx_custom_oauth_providers_slug`（UNIQUE） on (slug)

### `event_deliveries`

| 字段 | 类型 | NOT NULL | 主键 | 默认值 |
|---|---|---|---|---|
| `id` | INTEGER |  | yes |  |
| `event_id` | varchar(128) | yes |  |  |
| `handler` | varchar(128) | yes |  |  |
| `event_type` | varchar(128) |  |  |  |
| `state` | varchar(32) |  |  |  |
| `attempts` | INTEGER |  |  |  |
| `last_error` | TEXT |  |  |  |
| `created_at` | INTEGER |  |  |  |
| `updated_at` | INTEGER |  |  |  |

索引：
- `idx_event_delivery_event_handler`（UNIQUE） on (event_id, handler)
- `idx_event_deliveries_event_type` on (event_type)
- `idx_event_deliveries_state` on (state)
- `idx_event_deliveries_created_at` on (created_at)
- `idx_event_deliveries_updated_at` on (updated_at)

### `external_identity_claims`

| 字段 | 类型 | NOT NULL | 主键 | 默认值 |
|---|---|---|---|---|
| `id` | INTEGER |  | yes |  |
| `provider` | varchar(32) | yes |  |  |
| `subject` | varchar(128) | yes |  |  |
| `user_id` | INTEGER | yes |  |  |
| `created_at` | datetime |  |  |  |

索引：
- `idx_external_identity_subject`（UNIQUE） on (provider, subject)
- `idx_external_identity_user`（UNIQUE） on (provider, user_id)
- `idx_external_identity_claims_user_id` on (user_id)

### `login_encryption_keys`

| 字段 | 类型 | NOT NULL | 主键 | 默认值 |
|---|---|---|---|---|
| `id` | INTEGER |  | yes |  |
| `slot` | varchar(32) | yes |  |  |
| `private_key_pem` | TEXT | yes |  |  |

索引：
- `idx_login_encryption_keys_slot`（UNIQUE） on (slot)

### `logs`

| 字段 | 类型 | NOT NULL | 主键 | 默认值 |
|---|---|---|---|---|
| `id` | INTEGER |  | yes |  |
| `user_id` | INTEGER |  |  |  |
| `created_at` | INTEGER |  |  |  |
| `type` | INTEGER |  |  |  |
| `content` | TEXT |  |  |  |
| `username` | TEXT |  |  | `""` |
| `token_name` | TEXT |  |  | `""` |
| `model_name` | TEXT |  |  | `""` |
| `quota` | INTEGER |  |  | `0` |
| `cost_quota` | INTEGER |  |  | `0` |
| `cost_rule_id` | INTEGER |  |  | `0` |
| `prompt_tokens` | INTEGER |  |  | `0` |
| `completion_tokens` | INTEGER |  |  | `0` |
| `use_time` | INTEGER |  |  | `0` |
| `is_stream` | numeric |  |  |  |
| `request_bytes` | INTEGER |  |  | `0` |
| `response_bytes` | INTEGER |  |  | `0` |
| `channel_id` | INTEGER |  |  |  |
| `channel_name` | TEXT |  |  |  |
| `token_id` | INTEGER |  |  | `0` |
| `group` | TEXT |  |  |  |
| `ip` | TEXT |  |  | `""` |
| `request_id` | varchar(64) |  |  | `""` |
| `upstream_request_id` | varchar(128) |  |  | `""` |
| `other` | TEXT |  |  |  |

索引：
- `idx_logs_user_id` on (user_id)
- `idx_logs_username` on (username)
- `idx_logs_model_name` on (model_name)
- `idx_logs_upstream_request_id` on (upstream_request_id)
- `idx_user_id_id` on (user_id, id)
- `idx_log_user_type_created` on (user_id, type, created_at)
- `index_username_model_name` on (model_name, username)
- `idx_logs_token_name` on (token_name)
- `idx_logs_group` on (group)
- `idx_logs_ip` on (ip)
- `idx_log_type_created_id` on (type, created_at, id)
- `idx_log_type_created_finance` on (type, created_at, quota, cost_quota, cost_rule_id)
- `idx_logs_channel_id` on (channel_id)
- `idx_logs_token_id` on (token_id)
- `idx_logs_request_id` on (request_id)
- `idx_created_at_id` on (created_at, id)

### `mcp_sessions`

| 字段 | 类型 | NOT NULL | 主键 | 默认值 |
|---|---|---|---|---|
| `id` | varchar(64) |  | yes |  |
| `user_id` | INTEGER |  |  |  |
| `token_id` | INTEGER |  |  |  |
| `protocol_version` | varchar(32) |  |  |  |
| `created_at` | INTEGER |  |  |  |
| `last_seen_at` | INTEGER |  |  |  |
| `expires_at` | INTEGER |  |  |  |
| `closed_at` | INTEGER |  |  |  |

索引：
- `idx_mcp_sessions_user_id` on (user_id)
- `idx_mcp_sessions_token_id` on (token_id)
- `idx_mcp_sessions_last_seen_at` on (last_seen_at)
- `idx_mcp_sessions_expires_at` on (expires_at)
- `sqlite_autoindex_mcp_sessions_1`（UNIQUE） on (id)

### `midjourneys`

| 字段 | 类型 | NOT NULL | 主键 | 默认值 |
|---|---|---|---|---|
| `id` | INTEGER |  | yes |  |
| `code` | INTEGER |  |  |  |
| `user_id` | INTEGER |  |  |  |
| `action` | varchar(40) |  |  |  |
| `mj_id` | TEXT |  |  |  |
| `prompt` | TEXT |  |  |  |
| `prompt_en` | TEXT |  |  |  |
| `description` | TEXT |  |  |  |
| `state` | TEXT |  |  |  |
| `submit_time` | INTEGER |  |  |  |
| `start_time` | INTEGER |  |  |  |
| `finish_time` | INTEGER |  |  |  |
| `image_url` | TEXT |  |  |  |
| `video_url` | TEXT |  |  |  |
| `video_urls` | TEXT |  |  |  |
| `status` | varchar(20) |  |  |  |
| `progress` | varchar(30) |  |  |  |
| `fail_reason` | TEXT |  |  |  |
| `channel_id` | INTEGER |  |  |  |
| `quota` | INTEGER |  |  |  |
| `buttons` | TEXT |  |  |  |
| `properties` | TEXT |  |  |  |
| `token_id` | INTEGER |  |  | `0` |
| `billing_channel_id` | INTEGER |  |  | `0` |

索引：
- `idx_midjourneys_user_id` on (user_id)
- `idx_midjourneys_action` on (action)
- `idx_midjourneys_mj_id` on (mj_id)
- `idx_midjourneys_submit_time` on (submit_time)
- `idx_midjourneys_start_time` on (start_time)
- `idx_midjourneys_finish_time` on (finish_time)
- `idx_midjourneys_status` on (status)
- `idx_midjourneys_progress` on (progress)

### `model_compression_stats`

| 字段 | 类型 | NOT NULL | 主键 | 默认值 |
|---|---|---|---|---|
| `model_name` | varchar(191) |  | yes |  |
| `count` | INTEGER |  |  | `0` |
| `original_bytes` | INTEGER |  |  | `0` |
| `compressed_bytes` | INTEGER |  |  | `0` |
| `saved_bytes` | INTEGER |  |  | `0` |
| `updated_at` | INTEGER |  |  | `0` |

索引：
- `sqlite_autoindex_model_compression_stats_1`（UNIQUE） on (model_name)

### `model_costs`

| 字段 | 类型 | NOT NULL | 主键 | 默认值 |
|---|---|---|---|---|
| `id` | INTEGER |  | yes |  |
| `channel_id` | INTEGER |  |  |  |
| `model` | varchar(128) |  |  | `""` |
| `billing_mode` | varchar(16) |  |  | `"call"` |
| `cost_per_call_micros` | INTEGER |  |  | `0` |
| `cost_input_per_million_micros` | INTEGER |  |  | `0` |
| `cost_output_per_million_micros` | INTEGER |  |  | `0` |
| `effective_from` | INTEGER |  |  |  |
| `created_by` | INTEGER |  |  | `0` |
| `note` | varchar(255) |  |  | `""` |
| `created_at` | datetime |  |  |  |

索引：
- `idx_model_cost_lookup` on (channel_id, model, effective_from)

### `models`

| 字段 | 类型 | NOT NULL | 主键 | 默认值 |
|---|---|---|---|---|
| `id` | INTEGER |  | yes |  |
| `model_name` | TEXT | yes |  |  |
| `description` | TEXT |  |  |  |
| `icon` | varchar(128) |  |  |  |
| `tags` | varchar(255) |  |  |  |
| `vendor_id` | INTEGER |  |  |  |
| `endpoints` | TEXT |  |  |  |
| `status` | INTEGER |  |  | `1` |
| `sync_official` | INTEGER |  |  | `1` |
| `created_time` | INTEGER |  |  |  |
| `updated_time` | INTEGER |  |  |  |
| `deleted_at` | datetime |  |  |  |
| `groups` | varchar(255) |  |  | `"default"` |
| `name_rule` | INTEGER |  |  | `0` |

索引：
- `idx_models_vendor_id` on (vendor_id)
- `idx_models_deleted_at` on (deleted_at)
- `uk_model_name_delete_at`（UNIQUE） on (model_name, deleted_at)

### `open_ai_ops_objects`

| 字段 | 类型 | NOT NULL | 主键 | 默认值 |
|---|---|---|---|---|
| `id` | INTEGER |  | yes |  |
| `user_id` | INTEGER |  |  |  |
| `kind` | varchar(16) | yes |  |  |
| `upstream_id` | varchar(191) | yes |  |  |
| `created_at` | INTEGER |  |  |  |

索引：
- `idx_open_ai_ops_objects_user_id` on (user_id)
- `idx_ops_object_kind_id`（UNIQUE） on (kind, upstream_id)

### `options`

| 字段 | 类型 | NOT NULL | 主键 | 默认值 |
|---|---|---|---|---|
| `key` | TEXT |  | yes |  |
| `value` | TEXT |  |  |  |

索引：
- `sqlite_autoindex_options_1`（UNIQUE） on (key)

### `passkey_credentials`

| 字段 | 类型 | NOT NULL | 主键 | 默认值 |
|---|---|---|---|---|
| `id` | INTEGER |  | yes |  |
| `user_id` | INTEGER | yes |  |  |
| `credential_id` | varchar(512) | yes |  |  |
| `public_key` | TEXT | yes |  |  |
| `attestation_type` | varchar(255) |  |  |  |
| `aa_guid` | varchar(512) |  |  |  |
| `sign_count` | INTEGER |  |  | `0` |
| `clone_warning` | numeric |  |  |  |
| `user_present` | numeric |  |  |  |
| `user_verified` | numeric |  |  |  |
| `backup_eligible` | numeric |  |  |  |
| `backup_state` | numeric |  |  |  |
| `transports` | TEXT |  |  |  |
| `attachment` | varchar(32) |  |  |  |
| `last_used_at` | datetime |  |  |  |
| `created_at` | datetime |  |  |  |
| `updated_at` | datetime |  |  |  |
| `deleted_at` | datetime |  |  |  |

索引：
- `idx_passkey_credentials_user_id`（UNIQUE） on (user_id)
- `idx_passkey_credentials_credential_id`（UNIQUE） on (credential_id)
- `idx_passkey_credentials_deleted_at` on (deleted_at)

### `perf_metrics`

| 字段 | 类型 | NOT NULL | 主键 | 默认值 |
|---|---|---|---|---|
| `id` | INTEGER |  | yes |  |
| `model_name` | TEXT |  |  |  |
| `group` | TEXT |  |  |  |
| `bucket_ts` | INTEGER |  |  |  |
| `request_count` | INTEGER |  |  | `0` |
| `success_count` | INTEGER |  |  | `0` |
| `total_latency_ms` | INTEGER |  |  | `0` |
| `ttft_sum_ms` | INTEGER |  |  | `0` |
| `ttft_count` | INTEGER |  |  | `0` |
| `output_tokens` | INTEGER |  |  | `0` |
| `generation_ms` | INTEGER |  |  | `0` |

索引：
- `idx_perf_model_group_bucket`（UNIQUE） on (model_name, group, bucket_ts)
- `idx_perf_bucket_ts` on (bucket_ts)

### `prefill_groups`

| 字段 | 类型 | NOT NULL | 主键 | 默认值 |
|---|---|---|---|---|
| `id` | INTEGER |  | yes |  |
| `name` | TEXT | yes |  |  |
| `type` | TEXT | yes |  |  |
| `items` | json |  |  |  |
| `description` | varchar(255) |  |  |  |
| `created_time` | INTEGER |  |  |  |
| `updated_time` | INTEGER |  |  |  |
| `deleted_at` | datetime |  |  |  |

索引：
- `uk_prefill_name`（UNIQUE） on (name)
- `idx_prefill_groups_type` on (type)
- `idx_prefill_groups_deleted_at` on (deleted_at)

### `quota_data`

| 字段 | 类型 | NOT NULL | 主键 | 默认值 |
|---|---|---|---|---|
| `id` | INTEGER |  | yes |  |
| `user_id` | INTEGER |  |  |  |
| `username` | TEXT |  |  | `""` |
| `model_name` | TEXT |  |  | `""` |
| `created_at` | INTEGER |  |  |  |
| `use_group` | TEXT |  |  | `""` |
| `token_id` | INTEGER |  |  | `0` |
| `channel_id` | INTEGER |  |  | `0` |
| `node_name` | TEXT |  |  | `""` |
| `token_used` | INTEGER |  |  | `0` |
| `count` | INTEGER |  |  | `0` |
| `quota` | INTEGER |  |  | `0` |

索引：
- `idx_quota_data_use_group` on (use_group)
- `idx_quota_data_token_id` on (token_id)
- `idx_quota_data_channel_id` on (channel_id)
- `idx_quota_data_node_name` on (node_name)
- `idx_quota_data_user_id` on (user_id)
- `idx_qdt_model_user_name` on (model_name, username)
- `idx_qdt_created_at` on (created_at)
- `idx_qdt_created_at_2` on (created_at)

### `redemption_usages`

| 字段 | 类型 | NOT NULL | 主键 | 默认值 |
|---|---|---|---|---|
| `id` | INTEGER |  | yes |  |
| `redemption_id` | INTEGER |  |  |  |
| `user_id` | INTEGER |  |  |  |
| `quota` | INTEGER |  |  |  |
| `created_time` | INTEGER |  |  |  |
| `deleted_at` | datetime |  |  |  |

索引：
- `idx_redemption_usages_redemption_id` on (redemption_id)
- `idx_redemption_usages_user_id` on (user_id)
- `idx_redemption_usages_deleted_at` on (deleted_at)

### `redemptions`

| 字段 | 类型 | NOT NULL | 主键 | 默认值 |
|---|---|---|---|---|
| `id` | INTEGER |  | yes |  |
| `user_id` | INTEGER |  |  |  |
| `key` | char(32) |  |  |  |
| `status` | INTEGER |  |  | `1` |
| `name` | TEXT |  |  |  |
| `quota` | INTEGER |  |  | `100` |
| `created_time` | INTEGER |  |  |  |
| `redeemed_time` | INTEGER |  |  |  |
| `used_user_id` | INTEGER |  |  |  |
| `deleted_at` | datetime |  |  |  |
| `expired_time` | INTEGER |  |  |  |
| `max_uses` | INTEGER |  |  | `0` |
| `plan_id` | INTEGER |  |  | `0` |

索引：
- `idx_redemptions_key`（UNIQUE） on (key)
- `idx_redemptions_name` on (name)
- `idx_redemptions_deleted_at` on (deleted_at)
- `idx_redemptions_plan_id` on (plan_id)

### `setups`

| 字段 | 类型 | NOT NULL | 主键 | 默认值 |
|---|---|---|---|---|
| `id` | INTEGER |  | yes |  |
| `version` | varchar(50) | yes |  |  |
| `initialized_at` | bigint | yes |  |  |

### `site_daily_visitors`

| 字段 | 类型 | NOT NULL | 主键 | 默认值 |
|---|---|---|---|---|
| `id` | INTEGER |  | yes |  |
| `day` | INTEGER |  |  |  |
| `ip_hash` | varchar(64) |  |  |  |
| `first_seen_at` | INTEGER |  |  |  |
| `last_seen_at` | INTEGER |  |  |  |
| `hits` | INTEGER |  |  |  |
| `source` | varchar(24) |  |  |  |
| `referrer_host` | varchar(255) |  |  |  |
| `path` | varchar(255) |  |  |  |
| `is_bot` | numeric |  |  |  |

索引：
- `idx_site_visitor_day_ip`（UNIQUE） on (day, ip_hash)
- `idx_site_visitor_day` on (day)
- `idx_site_visitor_bot` on (is_bot)

### `subscription_orders`

| 字段 | 类型 | NOT NULL | 主键 | 默认值 |
|---|---|---|---|---|
| `id` | INTEGER |  | yes |  |
| `user_id` | INTEGER |  |  |  |
| `plan_id` | INTEGER |  |  |  |
| `money` | REAL |  |  |  |
| `trade_no` | varchar(255) |  |  |  |
| `payment_method` | varchar(50) |  |  |  |
| `payment_provider` | varchar(50) |  |  | `""` |
| `status` | TEXT |  |  |  |
| `create_time` | INTEGER |  |  |  |
| `complete_time` | INTEGER |  |  |  |
| `provider_payload` | TEXT |  |  |  |

索引：
- `idx_subscription_orders_user_id` on (user_id)
- `idx_subscription_orders_plan_id` on (plan_id)
- `idx_subscription_orders_trade_no` on (trade_no)
- `sqlite_autoindex_subscription_orders_1`（UNIQUE） on (trade_no)

### `subscription_plan_grants`

| 字段 | 类型 | NOT NULL | 主键 | 默认值 |
|---|---|---|---|---|
| `id` | INTEGER |  | yes |  |
| `plan_id` | INTEGER |  |  |  |
| `title` | TEXT |  |  |  |
| `subtitle` | TEXT |  |  |  |
| `total_amount` | INTEGER |  |  |  |
| `payload` | TEXT |  |  |  |

索引：
- `idx_subscription_plan_grants_plan_id` on (plan_id)

### `subscription_plans`

| 字段 | 类型 | NOT NULL | 主键 | 默认值 |
|---|---|---|---|---|
| `id` | INTEGER |  | yes |  |
| `title` | varchar(128) | yes |  |  |
| `subtitle` | varchar(255) |  |  | `''` |
| `price_amount` | decimal(10,6) | yes |  |  |
| `currency` | varchar(8) | yes |  | `'USD'` |
| `duration_unit` | varchar(16) | yes |  | `'month'` |
| `duration_value` | INTEGER | yes |  | `1` |
| `custom_seconds` | bigint | yes |  | `0` |
| `enabled` | numeric |  |  | `1` |
| `sort_order` | INTEGER |  |  | `0` |
| `allow_balance_pay` | numeric |  |  | `1` |
| `allow_wallet_overflow` | numeric |  |  | `1` |
| `stripe_price_id` | varchar(128) |  |  | `''` |
| `creem_product_id` | varchar(128) |  |  | `''` |
| `waffo_pancake_product_id` | varchar(128) |  |  | `''` |
| `max_purchase_per_user` | INTEGER |  |  | `0` |
| `first_purchase_discount_percent` | decimal(5,2) | yes |  | `0` |
| `upgrade_group` | varchar(64) |  |  | `''` |
| `downgrade_group` | varchar(64) |  |  | `''` |
| `total_amount` | bigint | yes |  | `0` |
| `quota_reset_period` | varchar(16) |  |  | `'never'` |
| `quota_reset_custom_seconds` | bigint |  |  | `0` |
| `concurrency_limit` | INT |  |  | `0` |
| `rpm_limit` | INT |  |  | `0` |
| `models` | TEXT |  |  | `''` |
| `created_at` | bigint |  |  |  |
| `updated_at` | bigint |  |  |  |

### `subscription_pre_consume_records`

| 字段 | 类型 | NOT NULL | 主键 | 默认值 |
|---|---|---|---|---|
| `id` | INTEGER |  | yes |  |
| `request_id` | varchar(64) |  |  |  |
| `user_id` | INTEGER |  |  |  |
| `user_subscription_id` | INTEGER |  |  |  |
| `pre_consumed` | bigint | yes |  | `0` |
| `status` | varchar(32) |  |  |  |
| `created_at` | INTEGER |  |  |  |
| `updated_at` | INTEGER |  |  |  |

索引：
- `idx_subscription_pre_consume_records_updated_at` on (updated_at)
- `idx_subscription_pre_consume_records_request_id`（UNIQUE） on (request_id)
- `idx_subscription_pre_consume_records_user_id` on (user_id)
- `idx_subscription_pre_consume_records_user_subscription_id` on (user_subscription_id)
- `idx_subscription_pre_consume_records_status` on (status)

### `system_instances`

| 字段 | 类型 | NOT NULL | 主键 | 默认值 |
|---|---|---|---|---|
| `node_name` | varchar(128) |  | yes |  |
| `info` | TEXT |  |  |  |
| `started_at` | INTEGER |  |  |  |
| `last_seen_at` | INTEGER |  |  |  |
| `created_at` | INTEGER |  |  |  |
| `updated_at` | INTEGER |  |  |  |

索引：
- `idx_system_instances_started_at` on (started_at)
- `idx_system_instances_last_seen_at` on (last_seen_at)
- `idx_system_instances_created_at` on (created_at)
- `idx_system_instances_updated_at` on (updated_at)
- `sqlite_autoindex_system_instances_1`（UNIQUE） on (node_name)

### `system_task_locks`

| 字段 | 类型 | NOT NULL | 主键 | 默认值 |
|---|---|---|---|---|
| `type` | varchar(64) |  | yes |  |
| `task_id` | varchar(64) |  |  |  |
| `locked_by` | varchar(128) |  |  |  |
| `locked_until` | INTEGER |  |  |  |
| `updated_at` | INTEGER |  |  |  |

索引：
- `idx_system_task_locks_locked_until` on (locked_until)
- `idx_system_task_locks_updated_at` on (updated_at)
- `idx_system_task_locks_task_id` on (task_id)
- `idx_system_task_locks_locked_by` on (locked_by)
- `sqlite_autoindex_system_task_locks_1`（UNIQUE） on (type)

### `system_tasks`

| 字段 | 类型 | NOT NULL | 主键 | 默认值 |
|---|---|---|---|---|
| `id` | INTEGER |  | yes |  |
| `task_id` | varchar(64) |  |  |  |
| `type` | varchar(64) |  |  |  |
| `status` | varchar(32) |  |  |  |
| `active_key` | varchar(64) |  |  |  |
| `payload` | TEXT |  |  |  |
| `state` | TEXT |  |  |  |
| `result` | TEXT |  |  |  |
| `error` | TEXT |  |  |  |
| `locked_by` | varchar(128) |  |  |  |
| `created_at` | INTEGER |  |  |  |
| `updated_at` | INTEGER |  |  |  |

索引：
- `idx_system_tasks_type` on (type)
- `idx_system_tasks_status` on (status)
- `idx_system_tasks_active_key`（UNIQUE） on (active_key)
- `idx_system_tasks_locked_by` on (locked_by)
- `idx_system_tasks_created_at` on (created_at)
- `idx_system_tasks_updated_at` on (updated_at)
- `idx_system_tasks_task_id`（UNIQUE） on (task_id)

### `task_events`

| 字段 | 类型 | NOT NULL | 主键 | 默认值 |
|---|---|---|---|---|
| `id` | INTEGER |  | yes |  |
| `internal_task_id` | INTEGER |  |  |  |
| `task_id` | varchar(191) |  |  |  |
| `user_id` | INTEGER |  |  |  |
| `type` | varchar(30) |  |  |  |
| `created_at` | INTEGER |  |  |  |
| `data` | json |  |  |  |

索引：
- `idx_task_events_internal_task_id` on (internal_task_id)
- `idx_task_events_task_id` on (task_id)
- `idx_task_events_user_id` on (user_id)
- `idx_task_events_created_at` on (created_at)

### `task_plugins`

| 字段 | 类型 | NOT NULL | 主键 | 默认值 |
|---|---|---|---|---|
| `id` | INTEGER |  | yes |  |
| `key` | TEXT | yes |  |  |
| `api_version` | INTEGER | yes |  |  |
| `version` | TEXT | yes |  |  |
| `source` | TEXT |  |  |  |
| `source_hash` | TEXT | yes |  |  |
| `icon` | TEXT |  |  |  |
| `enabled` | numeric | yes |  |  |
| `active` | numeric | yes |  |  |
| `approval_status` | TEXT |  |  |  |
| `created_at` | INTEGER | yes |  |  |
| `remark` | TEXT |  |  |  |

索引：
- `uk_task_plugin_key_version`（UNIQUE） on (key, version)
- `idx_task_plugins_active` on (active)
- `idx_task_plugins_approval_status` on (approval_status)

### `tasks`

| 字段 | 类型 | NOT NULL | 主键 | 默认值 |
|---|---|---|---|---|
| `id` | INTEGER |  | yes |  |
| `created_at` | INTEGER |  |  |  |
| `updated_at` | INTEGER |  |  |  |
| `task_id` | varchar(191) |  |  |  |
| `platform` | varchar(30) |  |  |  |
| `user_id` | INTEGER |  |  |  |
| `group` | varchar(50) |  |  |  |
| `channel_id` | INTEGER |  |  |  |
| `quota` | INTEGER |  |  |  |
| `action` | varchar(40) |  |  |  |
| `status` | varchar(20) |  |  |  |
| `fail_reason` | TEXT |  |  |  |
| `submit_time` | INTEGER |  |  |  |
| `start_time` | INTEGER |  |  |  |
| `finish_time` | INTEGER |  |  |  |
| `progress` | varchar(20) |  |  |  |
| `properties` | json |  |  |  |
| `private_data` | json |  |  |  |
| `data` | json |  |  |  |

索引：
- `idx_tasks_submit_time` on (submit_time)
- `idx_tasks_finish_time` on (finish_time)
- `idx_tasks_progress` on (progress)
- `idx_tasks_task_id` on (task_id)
- `idx_tasks_platform` on (platform)
- `idx_tasks_channel_id` on (channel_id)
- `idx_tasks_start_time` on (start_time)
- `idx_tasks_created_at` on (created_at)
- `idx_tasks_user_id` on (user_id)
- `idx_tasks_action` on (action)
- `idx_tasks_status` on (status)
- `idx_task_active_poll` on (status, progress, submit_time)

### `tokens`

| 字段 | 类型 | NOT NULL | 主键 | 默认值 |
|---|---|---|---|---|
| `id` | INTEGER |  | yes |  |
| `user_id` | INTEGER |  |  |  |
| `key` | varchar(128) |  |  |  |
| `status` | INTEGER |  |  | `1` |
| `name` | TEXT |  |  |  |
| `created_time` | INTEGER |  |  |  |
| `accessed_time` | INTEGER |  |  |  |
| `expired_time` | INTEGER |  |  | `-1` |
| `remain_quota` | INTEGER |  |  | `0` |
| `unlimited_quota` | numeric |  |  |  |
| `model_limits_enabled` | numeric |  |  |  |
| `model_limits` | TEXT |  |  |  |
| `allow_ips` | TEXT |  |  | `""` |
| `used_quota` | INTEGER |  |  | `0` |
| `group` | TEXT |  |  | `""` |
| `cross_group_retry` | numeric |  |  |  |
| `auto_groups` | TEXT |  |  |  |
| `rate_limit` | TEXT |  |  |  |
| `deleted_at` | datetime |  |  |  |

索引：
- `idx_tokens_user_id` on (user_id)
- `idx_tokens_key`（UNIQUE） on (key)
- `idx_tokens_name` on (name)
- `idx_tokens_deleted_at` on (deleted_at)

### `top_ups`

| 字段 | 类型 | NOT NULL | 主键 | 默认值 |
|---|---|---|---|---|
| `id` | INTEGER |  | yes |  |
| `user_id` | INTEGER |  |  |  |
| `amount` | INTEGER |  |  |  |
| `money` | REAL |  |  |  |
| `trade_no` | varchar(255) |  |  |  |
| `payment_method` | varchar(50) |  |  |  |
| `payment_provider` | varchar(50) |  |  | `""` |
| `create_time` | INTEGER |  |  |  |
| `complete_time` | INTEGER |  |  |  |
| `status` | TEXT |  |  |  |

索引：
- `idx_top_ups_user_id` on (user_id)
- `idx_top_ups_trade_no` on (trade_no)
- `idx_top_ups_create_time` on (create_time)
- `sqlite_autoindex_top_ups_1`（UNIQUE） on (trade_no)

### `training_data_consents`

| 字段 | 类型 | NOT NULL | 主键 | 默认值 |
|---|---|---|---|---|
| `id` | INTEGER |  | yes |  |
| `user_id` | INTEGER |  |  |  |
| `granted` | numeric |  |  |  |
| `granted_at` | INTEGER |  |  |  |
| `revoked_at` | INTEGER |  |  |  |
| `policy_version` | varchar(32) |  |  |  |
| `created_at` | datetime |  |  |  |
| `updated_at` | datetime |  |  |  |

索引：
- `idx_training_consent_user`（UNIQUE） on (user_id)

### `training_data_export_downloads`

| 字段 | 类型 | NOT NULL | 主键 | 默认值 |
|---|---|---|---|---|
| `id` | INTEGER |  | yes |  |
| `export_id` | INTEGER |  |  |  |
| `downloader_id` | INTEGER |  |  |  |
| `source` | varchar(16) |  |  |  |
| `bytes_sent` | INTEGER |  |  |  |
| `range_header` | varchar(128) |  |  |  |
| `completed` | numeric |  |  |  |
| `started_at` | INTEGER |  |  |  |
| `completed_at` | INTEGER |  |  |  |
| `created_at` | datetime |  |  |  |
| `updated_at` | datetime |  |  |  |

索引：
- `idx_training_export_dl` on (export_id, downloader_id)

### `training_data_exports`

| 字段 | 类型 | NOT NULL | 主键 | 默认值 |
|---|---|---|---|---|
| `id` | INTEGER |  | yes |  |
| `status` | varchar(16) |  |  |  |
| `date_from` | varchar(10) |  |  |  |
| `date_to` | varchar(10) |  |  |  |
| `user_id` | INTEGER |  |  |  |
| `model` | varchar(128) |  |  |  |
| `archive_format` | varchar(16) |  |  |  |
| `object_path` | varchar(255) |  |  |  |
| `size_bytes` | INTEGER |  |  |  |
| `part_count` | INTEGER |  |  |  |
| `record_count` | INTEGER |  |  |  |
| `error_msg` | TEXT |  |  |  |
| `created_by` | INTEGER |  |  |  |
| `download_count` | INTEGER |  |  |  |
| `created_at` | datetime |  |  |  |
| `updated_at` | datetime |  |  |  |

索引：
- `idx_training_export_status` on (status)
- `idx_training_export_scope` on (user_id)
- `idx_training_export_creator` on (created_by)

### `training_data_watermarks`

| 字段 | 类型 | NOT NULL | 主键 | 默认值 |
|---|---|---|---|---|
| `id` | INTEGER |  | yes |  |
| `user_id` | INTEGER |  |  |  |
| `conversation_id` | varchar(128) |  |  |  |
| `uploaded_count` | INTEGER |  |  |  |
| `prefix_fingerprint` | varchar(64) |  |  |  |
| `request_text_bytes` | INTEGER |  |  |  |
| `request_text_hash` | varchar(64) |  |  |  |
| `last_uploaded_at` | INTEGER |  |  |  |
| `created_at` | datetime |  |  |  |
| `updated_at` | datetime |  |  |  |

索引：
- `idx_training_watermark_user_conv`（UNIQUE） on (user_id, conversation_id)

### `two_fa_backup_codes`

| 字段 | 类型 | NOT NULL | 主键 | 默认值 |
|---|---|---|---|---|
| `id` | INTEGER |  | yes |  |
| `user_id` | INTEGER | yes |  |  |
| `code_hash` | varchar(255) | yes |  |  |
| `is_used` | numeric |  |  |  |
| `used_at` | datetime |  |  |  |
| `created_at` | datetime |  |  |  |
| `deleted_at` | datetime |  |  |  |

索引：
- `idx_two_fa_backup_codes_deleted_at` on (deleted_at)
- `idx_two_fa_backup_codes_user_id` on (user_id)

### `two_fas`

| 字段 | 类型 | NOT NULL | 主键 | 默认值 |
|---|---|---|---|---|
| `id` | INTEGER |  | yes |  |
| `user_id` | INTEGER | yes |  |  |
| `secret` | varchar(255) | yes |  |  |
| `is_enabled` | numeric |  |  |  |
| `failed_attempts` | INTEGER |  |  | `0` |
| `locked_until` | datetime |  |  |  |
| `last_used_at` | datetime |  |  |  |
| `created_at` | datetime |  |  |  |
| `updated_at` | datetime |  |  |  |
| `deleted_at` | datetime |  |  |  |

索引：
- `idx_two_fas_user_id` on (user_id)
- `idx_two_fas_deleted_at` on (deleted_at)
- `sqlite_autoindex_two_fas_1`（UNIQUE） on (user_id)

### `user_oauth_bindings`

| 字段 | 类型 | NOT NULL | 主键 | 默认值 |
|---|---|---|---|---|
| `id` | INTEGER |  | yes |  |
| `user_id` | INTEGER | yes |  |  |
| `provider_id` | INTEGER | yes |  |  |
| `provider_user_id` | varchar(256) | yes |  |  |
| `created_at` | datetime |  |  |  |

索引：
- `ux_user_provider`（UNIQUE） on (user_id, provider_id)
- `ux_provider_userid`（UNIQUE） on (provider_id, provider_user_id)

### `user_sessions`

| 字段 | 类型 | NOT NULL | 主键 | 默认值 |
|---|---|---|---|---|
| `sid` | varchar(64) |  | yes |  |
| `user_id` | INTEGER | yes |  |  |
| `version` | bigint | yes |  | `1` |
| `user_auth_version` | bigint | yes |  |  |
| `status` | varchar(16) | yes |  |  |
| `refresh_hash` | char(64) | yes |  |  |
| `previous_refresh_hash` | varchar(64) |  |  |  |
| `previous_valid_until` | bigint | yes |  | `0` |
| `login_method` | varchar(32) | yes |  |  |
| `ip` | varchar(64) |  |  |  |
| `user_agent` | TEXT |  |  |  |
| `created_at` | INTEGER |  |  |  |
| `last_active_at` | bigint | yes |  |  |
| `expires_at` | bigint | yes |  |  |
| `revoked_at` | bigint | yes |  | `0` |
| `revoked_reason` | varchar(64) |  |  |  |

索引：
- `idx_user_sessions_user_status_expiry` on (user_id, status, expires_at)
- `idx_user_sessions_user_created` on (user_id, created_at)
- `idx_user_sessions_status_revoked` on (status, revoked_at)
- `idx_user_sessions_expires_at` on (expires_at)
- `sqlite_autoindex_user_sessions_1`（UNIQUE） on (sid)

### `user_subscriptions`

| 字段 | 类型 | NOT NULL | 主键 | 默认值 |
|---|---|---|---|---|
| `id` | INTEGER |  | yes |  |
| `user_id` | INTEGER |  |  |  |
| `plan_id` | INTEGER |  |  |  |
| `rpm_override` | INTEGER |  |  | `0` |
| `concurrency_override` | INTEGER |  |  | `0` |
| `amount_total` | bigint | yes |  | `0` |
| `amount_used` | bigint | yes |  | `0` |
| `price_amount` | decimal(10,6) | yes |  | `0` |
| `period_price` | decimal(10,6) | yes |  | `0` |
| `currency` | varchar(8) | yes |  | `"USD"` |
| `start_time` | INTEGER |  |  |  |
| `end_time` | INTEGER |  |  |  |
| `status` | varchar(32) |  |  |  |
| `reminder_days_notified` | INTEGER |  |  | `0` |
| `source` | varchar(32) |  |  | `"order"` |
| `last_reset_time` | bigint |  |  | `0` |
| `next_reset_time` | bigint |  |  | `0` |
| `upgrade_group` | varchar(64) |  |  | `""` |
| `prev_user_group` | varchar(64) |  |  | `""` |
| `downgrade_group` | varchar(64) |  |  | `""` |
| `allow_wallet_overflow` | numeric |  |  |  |
| `created_at` | INTEGER |  |  |  |
| `updated_at` | INTEGER |  |  |  |

索引：
- `idx_user_subscriptions_user_id` on (user_id)
- `idx_user_sub_active` on (user_id, status, end_time)
- `idx_user_subscriptions_plan_id` on (plan_id)
- `idx_user_subscriptions_end_time` on (end_time)
- `idx_user_subscriptions_status` on (status)
- `idx_user_subscriptions_next_reset_time` on (next_reset_time)

### `users`

| 字段 | 类型 | NOT NULL | 主键 | 默认值 |
|---|---|---|---|---|
| `id` | INTEGER |  | yes |  |
| `username` | TEXT |  |  |  |
| `password` | TEXT | yes |  |  |
| `display_name` | TEXT |  |  |  |
| `role` | INTEGER |  |  | `1` |
| `status` | INTEGER |  |  | `1` |
| `email` | TEXT |  |  |  |
| `github_id` | TEXT |  |  |  |
| `discord_id` | TEXT |  |  |  |
| `oidc_id` | TEXT |  |  |  |
| `wechat_id` | TEXT |  |  |  |
| `telegram_id` | TEXT |  |  |  |
| `access_token` | char(32) |  |  |  |
| `access_token_created_at` | bigint |  |  |  |
| `quota` | INTEGER |  |  | `0` |
| `used_quota` | INTEGER |  |  | `0` |
| `request_count` | INTEGER |  |  | `0` |
| `group` | varchar(64) |  |  | `"default"` |
| `aff_code` | varchar(32) |  |  |  |
| `aff_count` | INTEGER |  |  | `0` |
| `aff_quota` | INTEGER |  |  | `0` |
| `aff_history` | INTEGER |  |  | `0` |
| `inviter_id` | INTEGER |  |  |  |
| `deleted_at` | datetime |  |  |  |
| `linux_do_id` | TEXT |  |  |  |
| `source` | varchar(64) |  |  | `""` |
| `setting` | TEXT |  |  |  |
| `remark` | varchar(255) |  |  |  |
| `stripe_customer` | varchar(64) |  |  |  |
| `created_at` | INTEGER |  |  |  |
| `last_login_at` | INTEGER |  |  | `0` |
| `auth_version` | bigint | yes |  | `1` |

索引：
- `idx_users_linux_do_id` on (linux_do_id)
- `idx_users_username` on (username)
- `idx_users_email` on (email)
- `idx_users_discord_id` on (discord_id)
- `idx_users_aff_code`（UNIQUE） on (aff_code)
- `idx_users_oidc_id` on (oidc_id)
- `idx_users_access_token`（UNIQUE） on (access_token)
- `idx_users_deleted_at` on (deleted_at)
- `idx_users_stripe_customer` on (stripe_customer)
- `idx_users_source` on (source)
- `idx_users_git_hub_id` on (github_id)
- `idx_users_we_chat_id` on (wechat_id)
- `idx_users_telegram_id` on (telegram_id)
- `idx_users_inviter_id` on (inviter_id)
- `idx_users_display_name` on (display_name)
- `sqlite_autoindex_users_1`（UNIQUE） on (username)

### `vendors`

| 字段 | 类型 | NOT NULL | 主键 | 默认值 |
|---|---|---|---|---|
| `id` | INTEGER |  | yes |  |
| `name` | TEXT | yes |  |  |
| `description` | TEXT |  |  |  |
| `icon` | varchar(128) |  |  |  |
| `status` | INTEGER |  |  | `1` |
| `created_time` | INTEGER |  |  |  |
| `updated_time` | INTEGER |  |  |  |
| `deleted_at` | datetime |  |  |  |

索引：
- `uk_vendor_name_delete_at`（UNIQUE） on (name, deleted_at)
- `idx_vendors_deleted_at` on (deleted_at)

### `web_request_logs`

| 字段 | 类型 | NOT NULL | 主键 | 默认值 |
|---|---|---|---|---|
| `id` | INTEGER |  | yes |  |
| `ip` | varchar(64) |  |  |  |
| `path` | varchar(512) |  |  |  |
| `method` | varchar(16) |  |  |  |
| `status` | INTEGER |  |  |  |
| `bytes_sent` | INTEGER |  |  |  |
| `bytes_received` | INTEGER |  |  |  |
| `user_agent` | varchar(512) |  |  |  |
| `request_count` | INTEGER |  |  |  |
| `rate_per_second` | REAL |  |  |  |
| `window_start` | INTEGER |  |  |  |

索引：
- `idx_web_request_logs_ip` on (ip)
- `idx_web_request_logs_window_start` on (window_start)

### `webhook_endpoints`

| 字段 | 类型 | NOT NULL | 主键 | 默认值 |
|---|---|---|---|---|
| `id` | INTEGER |  | yes |  |
| `name` | varchar(128) | yes |  |  |
| `url` | varchar(512) | yes |  |  |
| `secret` | varchar(256) | yes |  |  |
| `enabled` | numeric |  |  |  |
| `events` | TEXT |  |  |  |
| `created_at` | INTEGER |  |  |  |
| `updated_at` | INTEGER |  |  |  |

索引：
- `idx_webhook_endpoints_enabled` on (enabled)
- `idx_webhook_endpoints_created_at` on (created_at)

## 5. 日志库

日志表由 `LOG_SQL_DSN` 指向的独立库承载（支持 SQLite / MySQL / PostgreSQL / ClickHouse）。
其结构不在本文件范围内：**从未验证过 ClickHouse 这条路径**（见 `计划书/下一步改进指南.md` 任务卡 T-A4）。

