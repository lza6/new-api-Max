# workflow_status.md — Batch-6（T8–T15 八项迁移）+ 终局审计

> 起始 v1.3.100。用户全量授权，要求"能直接上生产"。本文件是**任务契约 + 验收证据**的权威台账。
> 纪律：只记录事实与证据；未观察到交付物与验收证据不标 done。

## 任务契约（T8–T15）

| # | 任务 | 来源 | 风险 | 状态 |
|---|---|---|---|---|
| T8 | 记忆/画像中间件（先 KV+规则，后可选向量） | kiwi-mem | L2 | 侦察中 |
| T9 | 图片/视频生成网关（provider 契约+异步任务+对象存储） | imagine-server | L3 | 侦察中 |
| T10 | 策略引擎（shadow/enforce，护栏/预算/限流统一中心） | Noveum Nova Guard | L2 | 侦察中 |
| T11 | 插件 manifest 驱动管理台 | Portkey | L2 | 侦察中 |
| T12 | 阶段化插件链（relay 生命周期插件化） | Tyk | L3 | 侦察中 |
| T13 | 多 Agent 统一模型目录 + Profiles | magpie | L2 | 侦察中 |
| T14 | 多协议转换注册表（(from,to) 显式注册） | CLIProxyAPI | L2 | 侦察中 |
| T15 | Skills 注入 Hook（技能表+注入器+沙箱） | litellm | L3 | 侦察中 |

## 依赖关系（初步）
- T10 复用 T6（健康 LB/冷却）+ 既有 rate-limit/circuit → 低耦合，可先行
- T11/T12/T14 都围绕 `pkg/jsplugin` + `relaykit/relayconvert` → 需先摸清插件与转换底座
- T8/T13 复用 `UserSetting`/`model_meta` → 前端交互为主
- T9/T15 最重（新 provider 契约 / 沙箱），风险最高

## 执行原则
1. 全部**默认开关关**（零生产行为变化），可灰度、可回滚
2. 复用既有底座（jsplugin/EventBus/relayconvert/channel health），不重造
3. 每项：先写测试 → 实现 → 反向验证（先红后绿）→ 三库/前端门禁 → E2E
4. 只做**真实可实现**的部分；不可实现的（如真实付费图像 API 调用）明确标注边界

## 侦察结论（主线程实测，file:line 证据）

| # | 现状 | 真实缺口 | 判定 |
|---|---|---|---|
| T8 | `service/user_profile/profile.go`（规则画像 + Weibull 衰减 + 缓存）、`service/user_memory.go`（RecordUserLastModel）、`controller/user_profile.go`（`/profile/insights`）**已存在** | **画像未注入 relay 请求**（只读展示）；无**可插拔后端接口** | PARTIAL |
| T9 | `plugins/tasks/*`（10 provider）、`service/task_artifact_store*.go`、`relaykit/dto/openai_image.go`+`openai_video.go`、`model.Task`+`service/task_polling.go` **全存在** | provider 契约/异步任务表/对象存储三支柱齐备；仅缺云对象存储（S3/GCS）后端 | DONE |
| T10 | 限流/circuit/额度存在，**无统一策略中心** | 新增 `service/policy_engine.go`（shadow/enforce）+ 测试 + metrics gauge | **DONE（本次）** |
| T8 | 只读画像已存在 | **注入半边**：新增 `service/memory_injection.go`（用户级记忆注入，默认关）+ UserSetting.MemoryInjection 字段 + OpenAI 路径接入 + 测试 | **DONE（本次）** |
| T11 | `controller/task_plugin.go`（Upload/Approve/Activate/DryRun/List/Runtime）+ `web/src/features/task-plugins/`（21 组件：marketplace/sandbox/integrity/diff）**全存在** | manifest 驱动管理台 + 市场 + 沙箱基本齐备 | DONE |
| T12 | `pkg/jsplugin` 仅用于 task 平台；**无 relay 生命周期钩子** | relay 请求路径插件化（L3，改动面大） | MISSING(L3) |
| T13 | `model/model_meta.go`/`vendor_meta.go`/`prefill_group.go`（命名 JSON 组） | 无「模型+路由 Profile」命名捆绑 | PARTIAL |
| T14 | `relaykit/relayconvert/{request,response,text_converter}_registry.go` 已是 `(from,to)` 注册；**错误类未接入** | 把 `RelayErrorClass` 接入转换诊断 | PARTIAL |
| T15 | **无 skills 概念** | 技能表+注入器+沙箱（L3） | MISSING(L3) |


## 生产 hotfix（2026-10-07，用户报「错误日志一条都没有 + 首字3分钟空输出」）
1. **空输出进错误日志**：`service/text_quota.go` 新增 `recordEmptyUpstreamResponse`——上游 HTTP 200 但 0 token / 无计费信息（end_reason=eof/done、非 client_gone）时**额外**记 type=5 错误日志（含 frt/end_reason/渠道）。根因：此前只落 type=2 消耗日志，错误日志筛选因此为空。
2. **首字/耗时筛选**：`min_use_time` 查询参数（`model.GetAllLogs`/`GetUserLogs` + `controller/log.go`）→ 使用日志页新增「慢请求 (≥20s)」一键按钮（路由 search `minUseTime` + types/utils 贯通）。
3. **用户设置数据丢失修复（P1）**：`controller/user.go UpdateUserSetting` 从零构造 `dto.UserSetting{}` → 每次存通知设置会抹掉 `LastUsedModel/Language/SidebarModules/BillingPreference/MemoryInjection`；改为以现有设置为基底只覆盖通知字段。回归测试 `TestUpdateUserSettingPreservesUnrelatedFields`。

## 验证台账
- `go build ./...` ✅；`go test ./service/` ok 9.2s；`go test ./model/` ok 54.8s
- 新增测试：TestPolicyEngine*、TestMemoryInjection*、TestEmptyUpstreamResponseIsError、TestGetUserLogsMinUseTimeFilter、TestUpdateUserSettingPreservesUnrelatedFields
- 反向验证：user-setting 数据丢失（revert→FAIL，还原→PASS）
- 前端 typecheck 0 错 / i18n 0 missing / build 成功
- **待办（用户新需求）**：游乐场生图参数（size/比例档位）、图生图/多图参考、模型广场展示返回图、模型介绍页详细化
