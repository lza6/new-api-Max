# workflow_status.md — 本轮闭环施工台账

> 起始基线：`v1.3.147` / `5d1aa17d9`。用户已全量授权本轮拆解、验证、提交、发布与部署。
> 纪律：只记录事实与证据；未见交付物与验收证据不标 Done。

## 1. 任务契约

**目标**：把《下一步改进指南》及其后续计划中的开放项尽可能真实闭环；完成真实 E2E/压测/独立审计；提交、发布并部署。
**成功标准**：每项有可执行代码 + 生产链路调用点 + 可复现验证；无“库全绿但零调用”。
**停止条件**：全部批次完成且验证通过，或遇到不可替代的凭证/环境阻塞。

## 2. 需求追踪矩阵

| # | 需求 | 实现位置 | 状态 | 证据 | 缺口 |
|---|---|---|---|---|---|
| 1 | 邮箱防枚举 | `controller/user.go` / `controller/oauth.go` | Done | 既有测试 + 本轮审计确认 | — |
| 2 | 渠道密钥加密 | `common/channel_key_crypto.go`、`model/channel_key_migration.go`、`main.go` | Done（默认关） | 启动接线 + README 启用步骤 + 测试 | 生产保持关闭（用户决定） |
| 3 | CSP/安全头 | `deploy/Caddyfile.example` | Done | 文件存在 + README | 生产 Caddy 由运维应用 |
| 4 | 错误文案全覆盖 | `web/src/lib/server-error-message.ts` | Done | 既有测试 | — |
| 5 | 订阅续费提醒 | `service/subscription_reset_task.go` | Done（3 天/1 天） | 新增 3 用例 PASS | — |
| 6 | 首购优惠 | — | **未实现（业务决策）** | — | 折扣规则/支付渠道差异需业主定义 |
| 7 | 省费可视化 | — | **未实现（口径决策）** | — | “对比按量付费”的基准需定义 |
| 8 | 模型目录同步 + `/v1/pricing` | `controller/model_sync.go`、`system_task_handlers.go`、`controller/pricing.go` | Done | 既有契约测试 | — |
| 9 | 通用 Webhook | `model/webhook_endpoint.go`、`service/webhook*`、管理端/前端 | Done | 幂等/重试/三库 conformance 既有证据 | — |
| 10 | SQL 债 | `model/log.go`、`controller/log.go`、`model/task.go`、`model/main.go` | Done | 既有测试 | keyset 用延迟关联替代 |
| 11 | `/metrics` | `common/metrics.go`、`middleware/metrics_auth.go`、`router/main.go` | Done（默认关） | 鉴权测试 | 生产 `METRICS_ENABLED` 未开 |
| 12 | `/readyz` 依赖检查 | `controller/health.go` | Done | 既有测试 | — |
| 13 | `TRUSTED_PROXIES` | `middleware/trusted_proxies.go`、`common/trusted_proxies.go` | Done | 6 组测试 | — |
| 14 | a11y / 响应式 | 前端 | Done | Playwright 7/7 PASS | — |
| 15 | 游乐场侧边栏/图片能力 | `web/src/features/playground`、`components/chat-image.tsx` | Done | 代码 + 既有测试 | — |
| 16 | Agent loop / ask_options / 画廊 | — | **未实现（明确延后）** | — | 独立产品面 |
| 17 | SEO sitemap/robots/canonical/内容注入 | `controller/spa_seo.go`、`controller/misc.go` | Done | 既有 + 本轮测试 | — |
| 18 | 登录页 noindex | `spaNoIndexPath` | Done | 新增测试 | — |
| 19 | Organization / BreadcrumbList | `spaJSONLDScript` | Done | 新增测试 | — |
| 20 | hreflang + 语言路径前缀 | `spa_seo.go`、`misc.go`、`web/src/i18n/languages.ts`、`main.tsx` | Done（zh/en 声明） | 新增后端/前端测试 | fr/ru/ja/vi 服务端文案未翻译，不声明 hreflang |
| 21 | 运营者信息 | `spaJSONLDScript` | Done（仅站点名/URL） | 测试 | 真实主体信息未提供 |
| 22 | 法务页路由 | 既有 `/user-agreement`、`/privacy-policy` + 后端配置 | Done | 路由/页面存在 | 正文由后台配置 |
| 23 | Key 创建后 curl 示例 | `api-keys-mutate-drawer.tsx` | Done | vitest 3/3 | — |
| 24 | OpenAI 兼容端点补全 | `controller/relay.go`、`relay/openai_ops_handler.go`、`router/relay-router.go` | Done | 新增单测 | 真实上游 E2E 未跑 |
| 25 | G4 路由 enforce | `controller/relay.go`、`service/routing_policy.go` | Done（默认 off） | 既有 + 新增校验 | 生产需显式开启 |
| 26 | G5-3 动作级审批 | `model/action_approval*`、`service/action_registry.go`、管理端/前端 | Done | 新增 3 组测试 | — |
| 27 | MCP Gateway | `controller/mcp.go`、`router/main.go` | Done（完整版，默认关） | 新增 5 组测试 | 真实 MCP 客户端握手未跑 |
| 28 | Claude 计费两处 | `service/text_quota.go`、`relay/channel/claude/relay-claude.go` | Done（修复） | 复现测试转为修复断言 | 生产计费变更需观察 |
| 29 | 多实例采集协调 | — | **未实现** | — | 需跨实例租约设计 |
| 30 | G8 drawer 拆分 | — | **未做** | — | 5054 行大重构，按约束不做 |
| 31 | G12 死码/噪声清理 | — | 部分 | 既有台账 | 未系统清理 |
| 32 | 文档清理 | 已删除过期蓝图/状态/spec | Done | git 变更 | — |
| 33 | 提交/推送/tag/release | 见第 5 节 | 执行中 | — | — |
| 34 | 生产部署热更新 | 见第 5 节 | 待执行 | — | — |
| 35 | 压测 | 本地 mock 上游 | 待执行 | `evidence/stress-*.json` | 真实渠道未跑 |

## 3. 实际改动

- 后端：SEO/hreflang、OpenAI ops relay、路由 enforce、动作审批、MCP、Claude 计费、密钥加密接线、续费提醒双阈值。
- 前端：语言路径前缀、Key curl、动作审批页面、侧边栏入口、7 语言文案。
- 文档：README、计划书/README、audit-ledger、policy、test_ledger。
- 删除：过期施工蓝图/状态/spec、本地构建残留（详见 git diff）。

## 4. 验证

见 `test_ledger.md`。本轮已实际运行构建、vet、定向测试、前端 typecheck/build/i18n/lint、真实 Playwright a11y。

## 5. 发布与部署

（执行后回填：commit SHA、tag、release URL、镜像 tag、部署时间、线上验收结果、回滚点。）

## 6. 独立审查与修复

- 安全：C1 编码路径计费绕过、R1 开放重定向、R2 ops 跨用户 IDOR 已修并有回归测试。
- Go：审批绑定请求参数、请求体流式化（避免 OOM）已修。
- 前端：失败误报成功、错误当空列表、存量语言与 canonical、未翻译前缀 lang 已修。
- 详见 `计划书/audit-ledger.md` 第六节。

## 7. 未闭环 / 阻塞（如实）

- 首购优惠、省费可视化：业务规则未定义，未实现。
- 渠道冗余：需用户提供上游渠道。
- 多实例采集协调：未实现（已有单实例约束文档）。
- G8 大文件拆分：按“不做大幅重构”约束未做。
- MySQL/PostgreSQL conformance：本机无环境。
- 真实上游 fine-tune/batch、真实 MCP 客户端、真实渠道 p95：缺环境/渠道。
