# Tasks：006 密钥自主管理 · 渠道 Key 运维 · 零停机热更新

**Prerequisites**: `spec.md`（US-1/2/3 + 非功能需求）、`plan.md`（AD-1 ~ AD-4）
**实现版本**：v1.3.59（主批）→ v1.3.60 / v1.3.61 / v1.3.62（收口批）
**组织方式**：先落地 US-1/US-2/US-3 主干，再由独立审查驱动三轮收口（v1.3.61 契约与错误码、v1.3.62 CI 阻断回归）。

> **口径说明**：本清单为**回填式**（功能先实现、后补 spec-kit 体例），所有 `[x]` 均对应仓库中已存在的代码 / 测试 / 证据。
> Evidence 列的行号已在 2026-09-30 用 `grep -n` 逐条核对。**未找到的引用如实标注「未找到」**，不代之以推测路径。

---

## Phase 1 — Foundation（配置与中间件）

- [x] **F-1** 新增 `TokenSetting.RequireVerificationToReadOwnKey bool`（JSON `require_verification_to_read_own_key`，默认 **false**），走既有 `config.GlobalConfig.Register("token_setting", ...)` 热更新注册机制。
  - **Depends on**: 无
  - **Requirement**: US-1（P0）
  - **Evidence**: `setting/operation_setting/token_setting.go:13`（字段）、`:24`（默认 false）、`:28`（init 注册）、`:44`（`IsOwnKeyReadVerificationRequired()`）
- [x] **F-2** 新增 `TokenSetting.RequireVerificationToReadChannelKey bool`（JSON `require_verification_to_read_channel_key`，默认 **false**）。
  - **Depends on**: F-1（同结构体 / 同注册点）
  - **Requirement**: US-2(e)
  - **Evidence**: `setting/operation_setting/token_setting.go:18`（字段）、`:49`（`IsChannelKeyReadVerificationRequired()`）
- [x] **F-3** `middleware.SecureTokenKeyVerificationRequired` 增加「归属成立 + 开关关闭 → 直接放行」分支；开关开启时保持原 step-up 语义。
  - **Depends on**: F-1
  - **Requirement**: US-1
  - **Evidence**: `middleware/secure_verification.go:63`（函数）、`:70`（`if !requireOwnTokenProofIfConfigured() { c.Next(); return }`）；辅助函数 `:54`
- [x] **F-4** `middleware.SecureTokenKeysBatchVerificationRequired` 同法放宽（批量 ≤100 条），并保留 id 集绑定 proof 的重放防护。
  - **Depends on**: F-1
  - **Requirement**: US-1（批量 reveal ≤100 条 → 200）
  - **Evidence**: `middleware/secure_verification.go:95`（函数）、`:105`（`len(body.Ids) > 100` 上限）
- [x] **F-5** `middleware.SecureVerificationRequired`（渠道 key 读）改为读 `IsChannelKeyReadVerificationRequired()`，默认放行。
  - **Depends on**: F-2
  - **Requirement**: US-2(e)
  - **Evidence**: `middleware/secure_verification.go:25`（函数）、`:32`（开关判定）
- [x] **F-6** 安全边界明确：放宽只作用于「自己的 token」；归属校验 `GetTokenByIds(id, userId)` 在 controller 层保持为唯一真边界，中间件放宽不替代它。
  - **Depends on**: F-3
  - **Requirement**: spec §3 安全（越权不得削弱）
  - **Evidence**: `middleware/secure_verification.go:49-53`（注释声明边界）、`controller/token.go:197`（`model.GetTokenByIds(id, userId)`）

## Phase 2 — Core Backend（密钥读取语义收口 + 渠道 key 运维）

- [x] **C-1** 越权 / 他人不存在统一返回 **404 + `TOKEN_NOT_FOUND`**（此前 200 + 错误文本、批量 200 + 空 map，可被用于枚举）。
  - **Depends on**: F-3、F-4
  - **Requirement**: US-1（越权仍被拦截）+ spec §3 安全
  - **Evidence**: `controller/token.go:200-206`（单条 404 分支）、`controller/token.go:563` 与 `:576`（批量 404）；i18n key `i18n/keys.go:57`
- [x] **C-2** 审计事件 `token.key_view` / `token.key_view_batch` 保留（含失败尝试）。
  - **Depends on**: C-1
  - **Requirement**: US-1 验收（审计仍记录）
  - **Evidence**: `controller/token_test.go:702`、`:707`（`action: "token.key_view"` / `"token.key_view_batch"` 断言）
- [x] **C-3** 渠道多 key 新增：`manage` 增加 `add_keys` action，**追加**语义（不覆盖既有）+ 去重 + 拒绝空输入。
  - **Depends on**: 无
  - **Requirement**: US-2(a)
  - **Evidence**: `controller/channel.go:1925`（`case "add_keys"`）、请求结构体字段 `controller/channel.go:1507`
- [x] **C-4** key 状态查询沿用既有 `get_key_status`（有效 / 手动禁用 / 自动禁用 / 原因 / 时间）。
  - **Depends on**: 无
  - **Requirement**: US-2(b)
  - **Evidence**: `controller/channel.go:1578`（`case "get_key_status"`）、`:1506`（status 过滤语义注释）
- [x] **C-5** 单 key 测试：`POST /api/channel/:id/key/test?key_index=N`，复用渠道测试链路并指定第 N 把 key。
  - **Depends on**: C-4
  - **Requirement**: US-2(c)
  - **Evidence**: 路由 `router/channel-router.go:55`；handler `controller/channel-test.go:1205`；key 注入 `controller/channel-test.go:188-198`（`ContextKeyChannelKey` / `ContextKeyChannelMultiKeyIndex`）
- [x] **C-6** 批量测试：`POST /api/channel/:id/keys/test`，并发上限 3，返回 `{total, ok_count, fail_count, results[]}`。
  - **Depends on**: C-5
  - **Requirement**: US-2(d)
  - **Evidence**: 路由 `router/channel-router.go:56`；handler `controller/channel-test.go:1234`；并发常量 `controller/channel-test.go:1277`
- [x] **C-7** 批量测试防御：key 数上限 200、整批总超时（默认 400s，`timeout_seconds` 可覆盖并钳制 ≤3600）、超时响应带 `timed_out`。
  - **Depends on**: C-6
  - **Requirement**: US-2(d) 安全性（不因一次点击打出无界外部请求）
  - **Evidence**: `controller/channel-test.go:1252`（`maxKeysPerBatchTest = 200`）、`:1267-1273`（总超时）、`:1324-1325`（`timed_out` / `timeout_seconds`）
- [x] **C-8** **key 测试为只读诊断，不改动渠道健康分**（不写 health score / 不触发自动禁用）。
  - **Depends on**: C-5、C-6
  - **Requirement**: plan AD-2 实现说明
  - **Evidence**: `controller/channel-test.go:1349-1377`（`runSingleKeyTest` 全函数体内无任何 health / disable 写入调用；`grep` 该区间零命中）

## Phase 3 — Frontend

- [x] **F-1(web)** reveal 流程改为「先不带 proof 请求 → 命中 `SECURITY_PROOF_*` 才带 proof 重试」，站点开关任意配置都能工作（无需能力探测端点）。
  - **Depends on**: F-3、F-4
  - **Requirement**: US-1 前端 + spec §3 兼容性边界
  - **Evidence**: `web/src/features/keys/hooks/use-token-key-disclosure.ts:44`（`readServerCode(error)`）、`:65`（`readServerCode` 定义）
- [x] **F-2(web)** **axios 错误码读取顺序纪律**：必须先读 `response.data.code`，再排除 `ERR_` 前缀的传输层码——否则整条「强制验证时回退弹窗」路径是死代码。
  - **Depends on**: F-1(web)
  - **Requirement**: US-1 前端（真实可用，非静默失效）
  - **Evidence**: `web/src/features/keys/hooks/use-token-key-disclosure.ts:60-63`（注释写明 axios 对 4xx 一律设 `error.code='ERR_BAD_REQUEST'`）
- [x] **F-3(web)** reveal API 层透传 `X-Security-Proof`（单条 / 批量）。
  - **Depends on**: F-1(web)
  - **Requirement**: US-1
  - **Evidence**: `web/src/features/keys/api.ts:114`（`fetchTokenKey`）、`:125`（`fetchTokenKeysBatch`）
- [x] **F-4(web)** 多 key 面板：行内单 key 测试 + 一键批量测试 + 新增密钥弹窗；`time_ms` 与后端 `keyTestResult.time_ms` 对齐。
  - **Depends on**: C-5、C-6
  - **Requirement**: US-2(c)(d)
  - **Evidence**: `web/src/features/channels/api.ts:704`（`testChannelKey`，`:712` 注释对齐 `time_ms`）、`:732`（`testChannelKeys`）；对话框 `web/src/features/channels/components/dialogs/multi-key-manage-dialog.tsx:194`（`time_ms: res.time_ms ?? 0`）、`:538-539`
- [x] **F-5(web)** 对话框切换渠道时重置 `testResults` / `testingIndex`，避免显示上一渠道的 OK/FAIL 标记。
  - **Depends on**: F-4(web)
  - **Requirement**: US-2 运维正确性
  - **Evidence**: `web/src/features/channels/components/dialogs/multi-key-manage-dialog.tsx:129-130`
- [x] **F-6(web)** 系统设置 → 安全 → Token 限制 增加两个验证开关（表单 schema + 字段渲染）。
  - **Depends on**: F-1、F-2
  - **Requirement**: spec §3 可回滚（配置开关）
  - **Evidence**: `web/src/features/system-settings/request-limits/token-limit-section.tsx:49-50`（zod）、`:151`（own key 字段）、`:176`（channel key 字段）
- [x] **F-7(web)** 复制按钮补 `aria-label`（axe `button-name` critical 缺陷）。
  - **Depends on**: 无
  - **Requirement**: spec §3 可维护 / a11y
  - **Evidence**: `web/src/features/keys/components/api-keys-cells.tsx:129`

## Phase 4 — Hardening（v1.3.60-62 收口）

- [x] **H-1** 上游网络层失败（connection refused / unexpected EOF / DNS / reset / broken pipe）不再误报 500，归类 **502 `upstream_unreachable`**；超时仍 504。
  - **Depends on**: 无（跨批修复）
  - **Requirement**: 生产投诉（v1.3.60）
  - **Evidence**: `relaykit/types/error.go:55`（错误码定义）、`relay/channel/api_request.go:522`（`isUpstreamUnreachable`）、`:560`（`summarizeNetworkError`）、`:662`（映射 `http.StatusBadGateway`）、`controller/relay.go:497`（不降格为 `upstream_unavailable`）
- [x] **H-2** 关键词归一化保留精确分类：`upstream_unreachable` 不得被降格覆盖为笼统的 `upstream_unavailable`（丢失「可安全重试」语义）。
  - **Depends on**: H-1
  - **Requirement**: v1.3.60 回归防护
  - **Evidence**: `controller/relay.go:493-499`；测试 `controller/relay_error_log_test.go:477`、`:488-492`
- [x] **H-3** 数据库导出 / 导入灾备：`GET /api/system/db/export[/info]` + `POST /api/system/db/import`（RootAuth）。纯 Go + gzip + JSON Lines 流式，导入语义为**只插入缺失行（冲突跳过）、绝不删除覆盖**，可安全重放。
  - **Depends on**: 无（跨批新增）
  - **Requirement**: 生产灾备（v1.3.60）
  - **Evidence**: 路由 `router/api-router.go:358-360`；handler `controller/db_backup.go:30`（Export）/ `:59`（Import）/ `:100`（Info）；服务 `service/db_backup.go:73`（`StreamDatabaseBackup`）/ `:206`（`ImportDatabaseBackup`）/ `:317`（`backupTableIndex`，未知表跳过）
- [x] **H-4** 备份表清单有单一来源：`model.BackupTables()` / `model.LogTables()`，改 AutoMigrate 时必须同步。
  - **Depends on**: H-3
  - **Requirement**: 可维护性
  - **Evidence**: `model/main.go:404`、`:420`；消费点 `service/db_backup.go:91`、`:95`、`:319`、`:329`
- [x] **H-5** 首页 3D 能力展示（纯 CSS 3D，零 WebGL），并修正「`initial={{opacity:0}}` + `whileInView` 在 IO 未触发时内容永久不可见」的可用性缺陷。
  - **Depends on**: 无（跨批新增）
  - **Requirement**: v1.3.60
  - **Evidence**: `web/src/features/home/components/sections/hero-3d-showcase.tsx`；引用点 `web/src/features/home/components/sections/hero.tsx:27`
- [x] **H-6** **i18n nil-bundle 保护**：`i18n.Translate` 在 `bundle == nil` 时直接返回 key，避免 `NewLocalizer(nil,...)` panic 掉整个 controller 测试包。
  - **Depends on**: C-1（404 分支引入 `i18n.T`）
  - **Requirement**: v1.3.62 P0-A（CI 阻断）
  - **Evidence**: `i18n/i18n.go:100`（函数）、`:101-103`（`if bundle == nil { return key }` + 契约注释）；调用侧 `controller/token.go:205`
- [x] **H-7** 测试链路显式初始化 i18n，贴近生产语义。
  - **Depends on**: H-6
  - **Requirement**: v1.3.62 P0-A（根治，而非只靠 nil 保护）
  - **Evidence**: `controller/main_test.go:35`（`_ = i18n.Init()` + 注释说明「Init 失败不阻断」与 main.go 同契约）
- [x] **H-8** 状态码语义变更后的测试对齐：`foreign key view` / `batch keys no matches` 由期望 200 改为期望 **404**（表格驱动加 `status` 字段）。
  - **Depends on**: C-1
  - **Requirement**: v1.3.62 P0-B
  - **Evidence**: `controller/token_test.go:702`、`:707`
- [x] **H-9** 批量测试新逻辑补齐覆盖：上限 / 空渠道 / 超时参数钳制。
  - **Depends on**: C-7
  - **Requirement**: v1.3.62 P1-A（本批最大新逻辑原为零覆盖）
  - **Evidence**: `controller/channel_test_internal_test.go:609`（`TestTestChannelKeysRejectsTooManyKeys`）、`:620`（`...RejectsEmptyChannel`）、`:638`（`...HonorsTimeoutParam`）

## Phase 5 — Verification & Delivery

- [x] **V-1** 单测：中间件开关矩阵（归属 × 开关 × 单条/批量）。
  - **Requirement**: US-1
  - **Evidence**: `middleware/secure_verification_test.go:20`（`TestOwnTokenKeyReadSkipsStepUpByDefault`）、`:80`（`TestChannelKeyReadSkipsStepUpByDefault`）
- [x] **V-2** 单测：越权 / 不存在 → 404 的归属与状态契约。
  - **Requirement**: US-1（越权拦截）
  - **Evidence**: `controller/token_key_test.go:23`（`TestTokenKeyDisclosureOwnershipAndStatus`）、`controller/security_enrollment_test.go:1675`（`TestChannelKeyReadWithoutVerificationByDefault`）
- [x] **V-3** 单测：`add_keys` 追加 / 去重 / 拒空 / 拒非多 key。
  - **Requirement**: US-2(a)
  - **Evidence**: `controller/channel_test_internal_test.go:490`（`TestManageMultiKeysAddKeys`）
- [x] **V-4** 单测：key 预览不泄露完整密钥 + 越界索引明确报错而非 panic。
  - **Requirement**: US-2(c) 安全 + 边界
  - **Evidence**: `controller/channel_test_internal_test.go:532`（`TestPreviewKeyNeverLeaksFullKey`）、`:543`（`TestRunSingleKeyTestRejectsOutOfRangeIndex`）、`:564`（`TestPreviewKeyMasksShortAndLong`）
- [x] **V-5** 单测：备份往返 + 幂等重放 + 非本格式拒绝 + 未来版本拒绝。
  - **Requirement**: H-3
  - **Evidence**: `service/db_backup_test.go:52`（`TestBackupRoundTrip`）、`:91`（`TestBackupImportRejectsGarbage`）、`:110`（`TestBackupImportHigherVersionRejected`）
- [x] **V-6** 单测：上游不可达分类矩阵（refused / EOF / DNS / reset / broken pipe 为真；超时 / 业务错误 / nil 为假）。
  - **Requirement**: H-1
  - **Evidence**: `relay/channel/api_request_getbody_test.go:639`（`TestIsUpstreamUnreachable`）
- [x] **V-7** 前端单测：reveal 不再触发验证弹窗 + 真实 axios 错误形状（含 `code:'ERR_BAD_REQUEST'`）覆盖回退路径。
  - **Requirement**: US-1 前端
  - **Evidence**: `web/src/features/keys/hooks/__tests__/token-key-disclosure.test.tsx`；契约重写后 `web/src/features/keys/components/__tests__/a11y-keys-stepup.test.tsx`、`api-key-listing.test.tsx`
- [x] **V-8** 真实浏览器 E2E（本地，部署冻结期替代方案）：备份分区可见 / 导出按钮 / 导出下载为有效 gzip / 防错提示 / 密钥列表非空（防假阳性）/ reveal 未弹二次验证 / 正向证据「已解锁」toast / 控制台无致命错误。
  - **Requirement**: US-1、US-2、H-3、H-5
  - **Evidence**: `计划书/e2e-evidence/v1.3.60/results.json`（13 项全 `ok: true`，`base=http://127.0.0.1:3000`，导出 `1964 bytes`，`rows=2`）；截图目录 `计划书/e2e-evidence/v1.3.60/`（含 `c1-fallback-dialog.png`）
- [x] **V-9** 生产端 US-2 密钥运维 E2E（单 key 4/4、批量 `ok_count=4 fail_count=0`、查看渠道 key 200 无 proof、`add_keys` 幂等拒绝重复）。
  - **Requirement**: US-2
  - **Evidence**: `计划书/change-report-v1.3.59.html`（第 5 节验证表）；**注意：仓库内无 v1.3.59 的 `e2e-evidence/` JSON 归档**（该目录下最早的生产 E2E 归档为 `v1.3.46` / `v1.3.47`），证据以该变更报告为唯一载体 —— **未找到**对应的原始结果文件。
- [x] **V-10** 零停机发布验证（蓝绿 + 端口交替 + Caddy reload + 失败回滚）。
  - **Requirement**: US-3
  - **Evidence**: `计划书/ops/deployment-sop.md:12`、`:20`、`:39`（实测基线 25/25=200、切流 4 秒）、`:48`、`:55`；`计划书/workflow_status.md` 第九节「九-补 US-3」。**脚本 `/opt/new-api/deploy-zero-downtime.sh` 位于生产机、不在仓库内**（仓库中仅有 `./.codex/tmp/rollback.sh`，与之无关）；v1.3.59 变更报告曾写「已上线」但 SOP 写入「生产未切」 —— **两处文档口径互相矛盾，以 SOP + 部署纪律为准（生产部署按需进行）**。
- [x] **V-11** 质量门（如实口径）：
  - `go build ./...` / `go vet ./...` exit 0 —— ✅
  - `middleware` / `common` / `setting` 全量 PASS —— ✅
  - `controller` **全量仍有 9 项既有噪声失败**（经 `git stash` 基线对照确认与 006 无关，含顺序依赖项 `TestSiteSubscriptionStatsAggregates`、`TestAdminSetUserSubscriptionTierInvalidatesCache`：隔离跑 PASS / 全量 FAIL）；v1.3.62 已消除本批引入的 i18n panic（此前 `go test ./controller/` 全量 FAIL 433s，现 0 panic）—— ⚠️ **不得声称「controller 全绿」**
  - 前端 `bun run typecheck` exit 0、oxlint 改动文件无 error、vitest keys+channels 119/119 —— ✅
  - i18n `bun run i18n:sync` 无漂移 + 一致性测试 2/2 —— ✅
  - **Requirement**: 全部
  - **Evidence**: `计划书/audit/perf-verification-ledger.md` 记录 0025（含 2026-09-30 更正）/ 0027 / 0028

---

## Notes — 实现与初稿 `plan.md` 的偏差（重要）

以下偏差均已在文档中回改，记录在此以便追溯「承诺 vs 交付」的差异：

1. **单/批量 key 测试不改动渠道健康分**（plan AD-2 已于初稿后补注）。
   测试定位为**只读诊断**：若把测试失败写入 health score，一次误测就会误伤生产渠道。
   代码事实：`controller/channel-test.go:1349-1377` 的 `runSingleKeyTest` 全函数体不含任何 health / disable 写入。
   对比：健康巡检走的是另一条链路 `testChannelForHealthCheck`（`controller/channel-test.go:941`）+ `performChannelTests`（`:1080`），二者不复用。

2. **`keyTestResult.TimeMs` 从死字段改为实测值**（v1.3.61 契约修复）。
   初版 `TimeMs` 从未赋值（恒 0），且 `keyTestResult` 曾含 `keyIndex` 死字段、前端读的是不存在的 `res.time`。
   现统一为：后端 `time.Now()` 实测 → `json:"time_ms"`（`controller/channel-test.go:1190` 字段 / `:1361` 起测 / `:1363` 取耗时），前端读 `res.time_ms`（`web/src/features/channels/api.ts:712`、`multi-key-manage-dialog.tsx:194`）。

3. **批量测试新增两项初稿未写的防御**：key 数上限 200、整批总超时（默认 400s，`timeout_seconds` 可覆盖且钳制 ≤3600）+ 响应 `timed_out` 标记。
   动因：并发只限「同时在跑几个」，不限总时长；慢上游会让 handler 长期挂住 goroutine 与连接。

4. **越权语义从「200 + 错误文本」升级为「404 + `TOKEN_NOT_FOUND`」**。
   初稿只要求「越权仍被拦截」，未指定状态码。实测原实现可用状态码区分「存在但无权」与「不存在」，构成枚举面；v1.3.60 统一 404。该变更连带要求 v1.3.62 修正两条仍然期望 200 的旧测试（见 H-8）。

5. **前端「回退弹窗」曾整条是死代码**（v1.3.61 C-1）。
   初稿假定读 `error.code` 即可拿到业务码；实际上 **axios 对所有 4xx 一律设 `error.code='ERR_BAD_REQUEST'`**，业务码只在 `response.data.code`。
   修正后需先读 `response.data.code` 并排除 `ERR_` 前缀；且测试必须使用**真实 axios 形状**（带 `code:'ERR_BAD_REQUEST'`），否则会造出假阳性。

6. **a11y 契约连带重写**：本批改变 reveal 流程后，`a11y-keys-stepup.test.tsx` / `api-key-listing.test.tsx` 因断言旧 step-up 流程而变红，已按新契约重写；顺带修复 `api-keys-cells.tsx:129` 复制按钮缺 `aria-label` 的真实 a11y 缺陷。

7. **spec §3 兼容性边界系实现后补记**：站点把 `require_verification_to_read_own_key` 设为 true 时，**新前端**会回退弹验证，**旧版本前端**（仅「带 proof 才请求」）会直接 403 —— 即强制模式下不兼容旧前端；默认 false 下新旧前端都正常。此边界已在 `spec.md:38-42` 如实记录。

8. **US-2(e) 的开关命名**：plan 写作 `ChannelSetting.RequireVerificationToReadKey`，实际落在 `TokenSetting.RequireVerificationToReadChannelKey`（`setting/operation_setting/token_setting.go:18`）—— 归入 token_setting 而非新开 ChannelSetting，避免为一个开关新建设置分组。

## 未找到 / 口径不一致项（如实登记）

| 项 | 状态 |
|---|---|
| v1.3.59 的 `e2e-evidence/` 原始结果 JSON | **未找到**（`计划书/e2e-evidence/` 下无 v1.3.59 归档）；证据仅存于 `计划书/change-report-v1.3.59.html` |
| `/opt/new-api/deploy-zero-downtime.sh` | **不在仓库内**（生产机文件）；仓库内无同源副本 |
| v1.3.59「已上线」 | v1.3.59 变更报告写「已上线」，`计划书/ops/deployment-sop.md` 写「生产未切」——**两处矛盾**，以部署纪律（按需部署）与 SOP 为准 |
| `controller` 包「全绿」 | **不成立**：全量仍有 9 项既有噪声失败（台账 0025 已于 2026-09-30 更正、0028 复核） |
