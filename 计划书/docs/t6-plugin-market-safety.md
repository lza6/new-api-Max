# 专项分析 · 插件/沙箱/市场安全与任务结算（T6 纵深）

> 定位：主指南 §T6 的**深挖文档**：现有边界、缺口、落地路径；只读整理（未改业务代码）。
> 生成：2026-09-25 · 锚点：`pkg/jsplugin`（Sobek）、`service/task_plugin_audit.go`、`service/task_polling.go`、`middleware/task_plugin.go`。

## 1. 现状（证据）
- 沙箱：宿主对象不可达 + import 编译阻断；marketplace URL 协议白名单（https only + 1MB + SHA-256）。
- 审批：写审批（`service/task_plugin_audit.go`）；任务轮询/结算在 `service/task_polling.go`。
- request-id：`middleware/task_plugin.go:1244/1305` 已用（T5 贯通）。
- 测试：sandbox 2 测试 + marketplace 225 测试（主指南基线）。

## 2. 缺口（主指南 §T6）
1. `applyStructuredTaskProgress`（task_polling.go:774）缺「progress+refund 共存 / 纯字符串零破坏 / 非法 JSON 零破坏」验收测试。
2. 通用 webhook 子系统（MISSING，需用户优先级确认，见 `docs/t15-roadmap-brainstorm.md` 方向 B）。
3. 定时同步（B5-4）：无定时调度与 `/v1/pricing` 路由（见方向 A/C）。
4. Marketplace 签名/registry 校验增强（伪造签名拒绝测试）。

## 3. 落地路径（建议）
1. **applyStructuredTaskProgress 测试**：直接在 `service/task_polling_test.go` 补 3 用例（现风格），不新建散测试文件。
2. **webhook（若立项）**：幂等键=任务ID+事件；重试退避；URL 白名单（沿用 marketplace https-only 模式）+ SSRF 校验（`service/url_guard.go`）；默认关开关。
3. **定时同步（若立项）**：复用 maintenance loop 模式（`service/*cleanup.go`）；dry-run 先跑，生产开关默认关；涉订阅/定价路径先走 T15 立项。
4. **签名校验**：publisher key 白名单 或 包签名（Ed25519）→ 伪造签名拒绝用例；不弱化现有协议白名单。

## 4. 纪律
- 沙箱不因加功能弱化：宿主对象边界/import 阻断/大小/协议白名单保持。
- 一切外呼走 URL 白名单 + SSRF 校验；任务结算语义（轮询）不被 webhook 替代（webhook 只通知）。
- 测试用 require/assert；市场/插件测试集中在 `pkg/jsplugin/` 与 `service/`，不散。

## 5. 验证
- `go test ./pkg/jsplugin/... ./service/ ./relay/channel/task/...`
- `cd web && bun run typecheck && bunx vitest run src/features/task-plugins`

## 闭环状态（2026-09-27，v1.3.44 回填）
- ✅ applyStructuredTaskProgress 验收测试已存在：service/task_polling_test.go:1033 TestApplyStructuredTaskProgressMergesWithoutClobbering（结构化合并保留 refund、纯字符串零破坏、nil 零破坏 3 用例）。
- ⚪ 通用 webhook / 定时同步：属 T15 立项方向 B/C，非本批；沙箱边界与市场协议白名单未弱化。
