# workflow_status — 4.5.x/4.6.x/4.7.x/4.8.x/4.9.x 批量任务

> 创建：2026-10-04。用户要求「全部完成」。本文件记录任务图、完成度、验证证据、阻塞。
> 纪律：只有观察到交付物与验收证据才标 done；P3「远期」项若前置未就绪则标 deferred 并说明。

## 验收标准（总）
- 每项：代码可构建；有可执行验证（单测/三库/E2E）；`// 说明` 不伪造；文档同步。
- 风险 L2+（schema/路由/跨域/结算）：三库或真机验证 + 先红后绿（可行时）。

## 任务图与状态

| # | 任务 | 级别 | 状态 | 证据 |
|---|---|---|---|---|
| 4.9.1 | CORS 冲突修正（env 白名单） | P1 | **done** | `go test ./middleware/ -run TestCORS` 5/5；先红后绿（旧配置 4 失败） |
| 4.9.2 | 硬编码站点模型清理（3 处→useSiteModel） | P1 | **done** | typecheck 我改文件零错误；3 处改为动态 |
| 4.5.5 | 用量/成本聚合报表 + CSV 导出 | P2 | **done** | 三库 conformance `TestDBConformanceUsageReport` sqlite/mysql/pg PASS（连跑 2 次）；前端第 5 section |
| 4.5.6 | 孤儿端点 + 占位按钮清理 | P2 | **done** | 端点：恢复定价页 `SiteSubscriptionStatsCard`（README/specs 已文档化，选恢复而非删）；playground 附件/搜索假占位移除（删 `input-tool-utils.ts` + 组件改）；typecheck 干净 |
| 4.9.4 | 死代码/临时产物清理 | P2 | **done** | `git rm` 零引用 `coming-soon.tsx`/`auto-skeleton.tsx`；删 25MB gitignored e2e/one-api 一次性 db；logs/.codex/tmp 保留（体积小/易访问） |
| 4.9.3 | 前端 alert-dialog→confirm-dialog | P2 | **done(7/12)** | 迁移 7 处删除确认：deployments-table / passkey-card / announcements / api-info / faq / uptime-kuma / log-settings；typecheck+oxlint 干净。余 5 处为 trigger 式/复杂内容弹窗（log-settings 清理 trigger / performance / ollama-models / billing-history / payment-confirm / missing-models），有独立 loading/多按钮/custom 内容，属能力缺口，保留并注释 |
| 4.6.2 | 生成任务产物质检闸门 | P2 | **done** | `service/task_polling.go` `settleTaskBillingOnComplete` 前加 `taskArtifactLooksUsable` 判定：成功但产物 URL 缺失/非法 → 保留预扣不结算；fail-open 保守；`TestTaskArtifactLooksUsable` + 先红后绿；结算回归绿 |
| 4.8.1 | 任务/领域感知路由 | P2 | **done** | `service/domain_route.go`：`X-Route-Tag` 头 + 白名单 `DOMAIN_ROUTE_MAP` → 分组覆盖；**默认关、不越权**（目标分组须在用户可用分组内）；distributor 接入；4 用例 + 先红后绿（Once 修复后测试有牙齿） |
| 4.7.1 | 用户级记忆层（规则+KV） | P2 | **done** | `relaykit/dto/user_settings.go` 加 `LastUsedModel`（JSON 列，无需迁移）；`service/user_memory.go` 异步去重有界记录；接入 `service/text_quota.go` 成功路径；3 用例含 race；relaykit 独立构建绿 |
| 4.6.1 | 统一媒体 Provider 适配层 | P2 | **done(能力注册表)** | `service/media_provider_registry.go`：媒体能力目录（text_to_video/image_to_video/image_gen/tts/asr）× 渠道类型；init 登记既有 8 个媒体渠道；只读查询 + `SupportsMediaCapability` 门控 + 管理端点 `/api/system-info/media-providers`；零分发改动、零回归；2 用例。**说明**：本仓已有成熟 `Adaptor`/`TaskAdaptor` 接口+工厂，本项补的是「统一能力目录/注册入口」，非重写分发 |
| 4.9.5 | CI 增强 | P2 | **done(已存在)** | ci.yml 已含 frontend（oxlint/typecheck/knip/vitest）+ 三库 conformance job；release.yml 已含 `workflow_dispatch`；无缺口需改 |
| 4.8.2 | per-provider 熔断器 | P2 | **done** | `service/channel_circuit_breaker.go`；接入 `ChannelHealthProbe`+`DecideCooldown`/`RecordChannelOutcome`；`-race` 6/6；先红后绿 |
| 4.8.1 | 任务/成本感知路由 | P2 | **deferred(需接口决策)** | 路由 `model.GetRandomSatisfiedChannel` 无领域标签输入；实现需先定标签来源（header/用户组/模型标签），属接口设计 → 待用户确认后实施 |
| 4.7.1 | 用户级记忆层（规则+KV） | P2 | todo | |
| 4.6.1 | 统一媒体 Provider 适配层 | P2 | todo | |
| 4.6.2 | 生成任务产物质检闸门 | P2 | todo | |
| 4.6.3 | 平台自身产出成品 | P3 | deferred | 前置 4.6.1+4.6.2 |
| 4.7.2 | 技能沉淀机制 | P3 | deferred | 前置 4.7.1 |
| 4.8.3 | 模型目录签名分发 | P3 | deferred | 依赖 pricing_sync/model_sync |
| 4.9.6 | 文档/记忆保鲜 | 持续 | ongoing | 每批回填 |

## 变更文件（Batch A/B）
- `middleware/cors.go`（改）、`middleware/cors_test.go`（新）
- `web/src/hooks/use-site-model.ts`（新）、`web/src/features/tool-setup/tool-integration-section.tsx`、`web/src/features/docs/index.tsx`、`web/src/routes/model-test.tsx`
- `model/usage_report.go`（新）、`controller/usage_report.go`（新）、`router/api-router.go`
- `model/db_conformance_test.go`（+2 用例：LogsTrafficIndexDropped/UsageReport）
- `web/src/features/dashboard/api.ts`、`section-registry.tsx`、`index.tsx`、`components/reports/usage-report-section.tsx`（新）
- `web/src/i18n/locales/*.json`（14 键 × 7 语言）

## 关键决策/注意
- **CORS**：`AllowAllOrigins`+`AllowCredentials` 规范冲突；改 env `CORS_ALLOWED_ORIGINS` 白名单，默认仅同源。
- **报表日键**：整数 `x - (x % 86400)`（x=created_at+offset），三库一致；**不用** `x/86400`（MySQL 是小数除法）。别名避 `key` 保留字（用 `dim_key`）。
- **conformance 持久库必须状态独立**：MySQL/PG 是持久库，用例开头清表（logs）。
- **i18n**：纯文本行插入（末尾 `}` 前），未重序列化；混淆键 `footer.newapi...` 保持。

## 环境
- 三库本机可用：MySQL 9.6（3306）、PG 16（5432，进程存活）；DSN 见 memory ledger。
- 本机 vitest 启动超时（台账既有噪声）→ 用 node 直接跑一致性契约。

## 批次 E（2026-10-04，安全 + 文档 + 报告）
| 项 | 状态 | 证据 |
|---|---|---|
| 4.11.3 登录限流默认开 + 失败审计 | **done** | `controller/login_audit_test.go` 3/3 + 先红后绿；`go build` OK |
| 4.11.5 私网判定补 CGNAT | **done** | `common/ip_test.go` + 先红后绿（删 CGNAT 行→红） |
| 4.11.5 文档断裂修正 | **done** | `计划书/project_specs.md`（T12 证据路径）、`计划书/docs/t8-security-asvs-deepdive.md`（G4→部分闭环） |
| README.md 同步 | **done** | env 表 +4 项（CORS/CIRCUIT×3）+ 特性节「007 批次」 |
| Spec Kit 规范 | **done** | `.specify/specs/007-prod-safety-audit-report/{spec,plan,tasks}.md` |
| HTML 变更报告 + 测验 | **done** | `计划书/reports/007-change-report.html`（8 题，须 8/8） |
| 独立审查线程 | **done** | 子代理 6 维审查 → 报 3 阻塞项，全部修复 + 先红后绿（见下） |
| 视频/图像端点适配 | **done** | `/v1/videos/generations` 等价别名（提交+查询，`router/video-router.go`）；`/v1/messages/count_tokens` 重新启用（`router/relay-router.go`，controller 早已实现+单测）；**新增** `/v1/sub2api/billing`（`controller/sub2api_billing.go`，返回分组倍率）；3 路由注册测试 + 1 controller 测试全 PASS |
| 图床 + 5 分钟清理 | **done** | `service/task_artifact_store_local.go`（本地磁盘图床）+ `task_artifact_cleanup.go`（5min loop）；6 用例含 race + 防穿越 |
| 图床防盗刷 + 接口安全 | **done** | 签名 capability URL（既有 HMAC）+ 下载限流 10/min（`DownloadRateLimit`）+ attachment/nosniff + 防目录穿越 + CRLF 头注入防护；单文件上限 |

## 独立审查发现 → 修复（2026-10-04，verdict: Request Changes → 已闭环）
| 阻塞项 | 根因 | 修复 | 验证 |
|---|---|---|---|
| C1 熔断半开探针在**过滤谓词**中被消费 | `ChannelCircuitAllows` 有 `probing=true` 副作用，被 `ChannelHealthProbe→channelMatchesFilter` 每候选/每轮（affinity+主选）多次调用 → 未选中的候选静默烧令牌，渠道 Open 窗口内永不恢复 | 拆分：`ChannelCircuitAllows` 改为**纯判定**（Open 未到期→false 剔除；半开候选→true 保留，**不消费**）；新增 `AcquireCircuitProbe` 在**真正发请求处**（`middleware/distributor.go` 选定后）单飞消费 | `TestChannelCircuitAllowsIsPurePredicate` / `TestAcquireCircuitProbeSingleFlight` + 先红后绿 |
| C2 CGNAT 被当**信任来源**豁免防护 | Web 防护用 `common.IsPrivateIP` 判信任；加 CGNAT 后 → CGNAT 外部来源被豁免限流/封禁（安全反向扩大） | 拆分语义：新增 `common.IsTrustedSourceIP`（不含 CGNAT）；`isWebProtectionTrustedSource` 改用它 | `TestIsTrustedSourceIPExcludesCGNAT` / `TestWebProtectionCGNATIsNotTrustedSource` + 调用点级先红后绿 |
| C3 失败登录审计未闭环 + 死代码 | 禁用用户/DecodeJson 分支无审计；`RecordLoginFailureLog` 零调用点（死代码）；注释不实 | 删除死代码；`DecodeJson` 失败 + `setupLoginAtAuthVersion` 的禁用/加载失败分支补 `recordLoginFailureAudit`；注释更正 | `TestLoginDisabledUserIsAudited` + 4 登录用例 PASS |
| 建议级 | CSV 公式注入 / start>end 静默交换 / DST 日键注释 / SysError 泄 DB 错 | 记录为后续（低危，管理端） | — |

## 诚实边界
- **未修**：邮箱枚举（注册/OAuth 侧）、主站 CSP 头（宿主机 Caddy）、渠道 Key 明文列。
- **未部署**：用户明确「等我说线上部署更新你再更新」。
- **既有噪声**（非本批回归）：`TestSessionLimitDoesNotRecordRejectedLoginAsSuccessful`、`TestAuditDatabaseMatrix`（Windows 文件锁）、本机 vitest 超时。

## 收尾批次（2026-10-04，审查建议级 + Observability）
| 项 | 状态 | 证据 |
|---|---|---|
| CSV 公式注入防护 | **done** | `controller/usage_report.go` `csvSafeCell`（=+-@/tab/CR 前置单引号）+ `TestCsvSafeCell` |
| start>end 显式 400（不静默交换） | **done** | `parseUsageReportParams` 显式校验 |
| DB 内部错误不泄漏到响应 | **done** | 记录 `SysError` 详情，返回通用错误 |
| 熔断器观测端点 | **done** | `controller/channel_health.go` `GetChannelHealthScores` 附 circuit_state + consecutive_failures；路由 `/api/system-info/channel-health`（RootAuth） |
| 服务探活 Health Checks | **done** | `controller/health.go` `/healthz`（存活，200）+ `/readyz`（就绪，DB ping，503）；根级无认证；2 用例 + 2 路由注册测试 |
| `/v1/responses/input_tokens` | **done** | `controller/relay.go` `CountResponsesInputTokens`（复用 responses 校验器 + CountRequestToken）；路由注册测试 |
