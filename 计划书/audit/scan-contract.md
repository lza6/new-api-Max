# 前端 ↔ 后端契约一致性审计（只读）

- 审计范围：`web/src` 前端 ↔ Go 后端（controller / model / middleware / common / setting）
- 审计方式：静态代码路径核对（file:line 两侧对照），未启动前端、未实机复现、未连生产库
- 结论标签：`静态确认` = 读代码确认；`待验证` = 缺环境/权限

---

## 1. 结论一句话

**有一处会直接产生用户可见错误**：任务日志的「操作类型」标签因前后端枚举值不对齐而全部退化显示为 `Unknown`（P1）。另有一处静默截断（`page_size>100`）和若干静默回退放大器（P2）。分页参数拼写、错误码、webhook / web-protection 请求体经逐项核对后**未发现不一致**。

---

## 2. 问题清单

### P1-1 任务日志 action 枚举前后端不对齐 → 标签静默显示 Unknown

**问题**：后端在 DTO 层把 action 归一化为新词表（snake_case），前端映射表仍是 legacy camelCase 键，两者键集不相交，标签查不到即回退默认 `Unknown`。

**证据（后端）**
- `relay/relay_task.go:656` — `Action: constant.NormalizeTaskAction(task.Action)`，产出 `image_to_video` / `text_to_video` / `first_tail_to_video` / `reference_to_video` / `remix`
- `constant/task.go:11-16` — canonical 常量定义；`constant/task.go:18-24` — `legacyTaskActionAliases` 把 `generate` → `image_to_video` 等
- `controller/task.go:422` — `relay.TaskModel2Dto(task)`，用于 `/api/task` 列表返回
- `plugins/tasks/kling/plugin.js:32-33` — 插件写入的 action 已是 canonical
- `relay/channel/task/jsplugin/adaptor.go:1159` — `info.Action = descriptor.Action`（canonical）

**证据（前端）**
- `web/src/features/usage-logs/constants.ts:182-193` — `TASK_ACTIONS` 仍为 `generate` / `textGenerate` / `firstTailGenerate` / `referenceGenerate` / `remixGenerate`；注释写着 “Must match backend constants in constant/task.go”（已不成立）
- `web/src/features/usage-logs/constants.ts:293-310` — `TASK_ACTION_MAPPINGS` 仅注册上述 legacy 键
- `web/src/features/usage-logs/lib/status.ts:29-31` — `getLabel` 查不到即回退默认 `'Unknown'`
- `web/src/features/usage-logs/components/columns/task-logs-columns.tsx:246` — `t(taskActionMapper.getLabel(log.action))`
- `web/src/features/usage-logs/components/dialogs/task-details-dialog.tsx:206` — 同一 mapper

**影响**：所有新任务（插件写入 canonical）在任务日志列与详情弹窗里，操作类型一律显示 `kling · Unknown` 等，而非 `Image to Video`。静默、无报错、无测试失败——前端测试用的是 legacy 值（`web/src/features/usage-logs/components/__tests__/task-artifacts.test.tsx:75`），与后端真实契约脱节。

**复现方式**：本地起前端 → 用量日志 → Task 分类 → 任意 Kling/Sora 视频任务行，观察操作类型标签为 `Unknown`。（静态确认，未实机复现）

**建议修法**（二选一，勿双改）
1. 前端 `TASK_ACTIONS` / `TASK_ACTION_MAPPINGS` 改用 canonical 值，并保留 legacy 键作为兼容别名；
2. 后端 `TaskModel2Dto` 不再归一化，直接回传库中原值。

推荐 ①，因为 DTO 归一化是为对外一致性服务的。

---

### P2-1 前端请求 `page_size=1000`，后端硬上限 100，静默截断

**问题**：前端拉「全量供应商」用于筛选/下拉，后端 `GetPageQuery` 硬性截断到 100，超出的供应商静默缺失。

**证据（前端）**
- `web/src/features/models/api.ts:155` — 默认 `{ page_size: 1000 }`
- `web/src/features/models/components/models-table.tsx:89` — `getVendors({ page_size: 1000 })`
- `web/src/features/models/components/drawers/model-mutate-drawer.tsx:128` — 同上
- `web/src/features/models/components/dialogs/vendor-operation-dialog.tsx:59` — 同上

**证据（后端）**
- `common/page_info.go:77-79` — `if pageInfo.PageSize > 100 { pageInfo.PageSize = 100 }`
- `controller/vendor_meta.go:18` — `SearchVendors` 走 `common.GetPageQuery(c)`

**影响**：供应商超过 100 个时，供应商筛选下拉静默缺项，无任何报错。当前内置供应商约 23 个（`model/pricing_default.go:113` 起 `defaultVendorDisplayIDs`），但 `model/model_metadata_sync.go:240` 会随元数据同步新增。

**复现方式**：库中造 >100 条 `vendors` → 打开模型列表页 → 供应商筛选下拉仅 100 项。

**建议修法**：前端改成分页循环拉全量（取到 total 为止），或后端新增明确的「全量元数据」端点，避免依赖被截断的 `page_size`。

**待验证**：生产库 `vendors` 实际行数是否已 >100。

---

### P2-2 静默回退点清单（契约不一致的放大器）

| 位置 | 行为 |
|---|---|
| `common/page_info.go:47-75` | 未知拼写（如 `page`）不报错，静默回退 `ItemsPerPage`；`p` 非数字同样静默回退 1 |
| `common/page_info.go:77-79` | `page_size>100` 静默截断为 100 |
| `controller/web_protection.go:174-176` | `size<=0 \|\| size>100` 静默回退 20 |
| `controller/log.go:20-30`（及 50-58、111-117、142-147） | `type` / `start_timestamp` / `end_timestamp` / `channel` / `min_use_time` 全部 `Atoi`/`ParseInt` 忽略错误 → 非法值静默变 0（0 在多数筛选里等于“不过滤”），查询范围被悄悄放大 |
| `controller/usedata.go:32-34` | 同上，时间戳非法静默变 0 |
| `model/midjourney.go:49-54`（及 80 / 226 / 243） | `StartTimestamp` 是未校验的字符串，直接进 `submit_time >= ?` 比较 |
| `model/log.go:538`、`model/log.go:640` | `logType==Unknown(0)` 时不加 type 过滤（有意的“全部”语义，前端 `LOG_TYPE_ALL_VALUE='0'` 与之对齐，见 `web/src/features/usage-logs/constants.ts:69-72`）——列此仅为提示语义脆弱，非缺陷 |

---

## 3. 我核对过但确认一致的

- **分页拼写**：`common/page_info.go:41-82` 接受 `p` → `page_size` → `ps` → `size`，不认 `page`。全仓前端分页请求实际使用 `p`+`page_size`（users / channels / redemption / wallet / usage-logs / vendors / deployments / audit / subscription）、`p`+`size`（`web/src/features/keys/api.ts:40`）、或 `page`+`size`（`web/src/features/web-protection/api.ts:73/78/83` —— 这三个端点是独立实现，后端 `controller/web_protection.go:169-170,210-211,228-229` 正是读 `page`/`size`，自洽）。**未发现前端把 `page` 传给 `GetPageQuery` 端点。**
- **`pageIndex+1` 的 1-based 换算**：`web/src/features/usage-logs/components/usage-logs-table.tsx:143`、`web/src/features/keys/components/api-keys-table.tsx:294`、`web/src/features/channels/components/channels-table.tsx:242` 等均 +1，与后端 `GetStartIdx()` 从 1 计一致。
- **错误码**：`SECURITY_PROOF_*` 前端 7 个码（`web/src/lib/secure-verification.ts:74-82`）与后端产出集完全吻合（`middleware/secure_verification.go:147-175`、`controller/secure_verification.go:88-92`）；`AUTH_SESSION_MISMATCH` 前端要求 HTTP 409（`web/src/features/auth/api.ts:110-115`）与 `service/auth_session.go:454-455` 一致；axios 读取顺序 `response.data.code` 优先、排除 `ERR_` 前缀（`web/src/lib/secure-verification.ts:92-103`）与后端 `securityProofError` 写顶层 `code` 的形态（`middleware/secure_verification.go:185-193`）匹配。
- **请求体字段**：
  - webhook settings / endpoints —— `controller/webhook_endpoint.go:22-28` ↔ `web/src/features/webhook/api.ts:52-69`；事件值 `epay.topup.success` / `epay.subscription.success` / `task.settled` 与 `setting/operation_setting/webhook_setting.go:28-32` 逐字一致
  - web-protection settings —— `setting/operation_setting/web_protection_setting.go:15-42` ↔ `web/src/features/web-protection/api.ts:21-34`
  - wallet 支付体 `amount` / `payment_method` —— `controller/topup.go:130-131`
  - aff 转账 `quota` —— `controller/user.go:494-496`
  - users `sort_by` / `sort_order` —— `controller/user.go:407-409`
  - deployments `price-estimation` 的 `gpus_per_container` / `duration_qty` / `hardware_qty` —— `pkg/ionet/types.go:186-195`
  - `/api/site/visitors` 全字段 —— `service/site_visitor_tracker.go:498-513` ↔ `web/src/features/home/types.ts:58-87`
  - audit 的 `category` 四值与 `model/audit_log.go:18-21` 一致
- **任务状态枚举**：`TASK_STATUS`（`web/src/features/usage-logs/constants.ts:198-206`）与 `model/task.go:37-49` 八个值逐字一致；`MJ_TASK_TYPES` / `MJ_TASK_STATUS` 与 `constant/midjourney.go:9-26` 一致。
- **日志 type 语义**：`LOG_TYPE_ENUM`（`web/src/features/usage-logs/constants.ts:56-65`）与 `model/log.go:94-101` 的 0-7 逐值一致。

---

## 4. 我没能验证的

- **供应商实际数量是否 >100**（决定 P2-1 是否已实际发生）—— 需连生产库统计 `vendors` 表。
- **`action` 在历史数据中的分布**（若库中既有 legacy 又有 canonical，则前端不论改哪边都需保留兼容别名）—— 需查库。
- **P1-1 未实机复现**：本任务为只读审计，前端未启动；判定依据为静态代码路径（`TaskModel2Dto` 归一化 + mapper 键集不相交），证据充分但未经浏览器实测。
- **部分认证错误码无专属文案**：`AUTH_TOKEN_EXPIRED` / `AUTH_SESSION_REVOKED` / `AUTH_REFRESH_RACE` 等在 `web/src/lib/server-error-message.ts` 无专属映射，走通用分支，是否属有意 —— **待验证**，不影响功能。
