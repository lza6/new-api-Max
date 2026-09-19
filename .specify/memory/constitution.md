# new-api-Max 项目宪法（Project Constitution）

> 适用对象：本仓库（AI API 网关/中转，Go + React，已生产部署，多租户计费）。
> 本宪法约束一切功能开发、缺陷修复、终局审计与交付。

## 核心原则

### I. 生产优先（Production-First）
功能必须"真实可运行、真实可调用、真实可计费、真实可审计"才能算完成。
- 任何实现都必须在真实链路（或等价真实 E2E）上验证，禁止以"静态审查/mock/单元测试通过"冒充完成。
- 明确标注证据等级：本地单测 / 本地 E2E / 线上验收，禁止伪造成功。

### II. 计费与资金安全不变量（NON-NEGOTIABLE）
- 任何计费路径不得产生负扣费、溢出回绕；所有配额转换必须走 `common/quota_math.go` 的 Checked 变体并记录饱和事件。
- 预扣→结算→退费全链路安全；用户可控的计费乘数（n/seconds/duration/ratio）必须先限界再计算。
- 影子价/口径新增不得改变既有计费数值；改计费必须全量回归。

### III. 安全基线（OWASP ASVS）
- 认证、会话、MFA、OAuth、密码类改动必须遵守 ASVS 并在服务端强制；前端检查不得替代服务端。
- SSRF 防护（渠道 base_url 二次校验）、web 防护、限流/冷却为不可降级防线。
- 认证审计日志不得记录密钥/令牌/验证码等秘密。

### IV. 三库兼容（SQLite / MySQL / PostgreSQL）
- 所有 schema、迁移、原始 SQL 必须三库同时成立；迁移幂等；验证需真实三库实例。
- 优先 GORM 方法；必须用锁时走 `lockForUpdate(tx)`；布尔默认值不得用 `default:true` 业务规则。

### V. 性能与低延迟（中转网关本质）
- 网关不得给用户请求增加可感知的首字延迟；每请求固定开销目标 < 10ms 级（热路径禁 DB 同步写/禁每行日志）。
- 流式透传不得缓冲首包；异步/连接池/缓存优先；禁止"看似透传实则加税"。

### VI. 前端体验与国际化
- 复用 `web/src/components/` 既有组件优先；新增需说明能力缺口。
- 所有新增文案走 i18n 且全语言无漂移（`bun run i18n:sync`）；键盘可达、色弱友好、移动端 320px+。
- 空态/加载/错误/成功反馈必须齐全；禁止"点了没结果"。

### VII. 保守改动与工作保护
- 不做无关重构；不 reset/clean/force push；按主题拆 commit；保留用户脏工作区。

## 质量门（Quality Gates）
- 后端：`go build ./...`、相关包 `go test`；relaykit 独立构建（`cd relaykit && GOWORK=off go build ./...`）。
- 前端：`bun run typecheck`（tsgo -b）、`bun run build`、vitest、`i18n:sync` 无漂移。
- E2E：真实请求（本地 mock 上游 + 生产 HTTPS）留证据 JSON 于 `计划书/e2e-evidence/`。
- 交付：主题 commit → push → tag → GitHub Release → 生产部署（备份 + 回滚命令）。

## 治理
- 本宪法优先于其他临时实践；改动需在 `workflow_status.md` 留痕并更新证据账本。
- 复用验证记录：已跑过的审计/基准（慢查询、压测、契约）记入 `计划书/audit-ledger.md`，避免重复无意义重跑；改动相关区域时优先引用账本。
- 复杂任务按 Spec-Kit 7 阶段：Constitution → Specify → Clarify → Plan → Tasks → Analyze → Implement。

**Version**: 1.0.0 | **Ratified**: 2026-09-20 | **Last Amended**: 2026-09-20
