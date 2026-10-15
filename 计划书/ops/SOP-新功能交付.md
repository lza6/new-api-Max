# SOP — 新功能交付（从需求到上线）

> 面向「要在这个仓库里加一个功能」的开发者或 AI。目标是让每一步都有**可执行的检查点**，
> 而不是靠记忆。配套：`计划书/ops/SOP-排障-*.md`（出问题怎么查）、`计划书/ops/SOP-回滚.md`（怎么安全退）、
> `.claude/skills/add-gateway-feature/SKILL.md`（同一流程的 skill 形态）。
>
> **本文所有路径/命令都来自本仓库真实文件并标注 `file:line`；核实不了的标「待验证」。**
> 最后核对：2026-10-16（对照 `VERSION` = `v1.3.147`）。

---

## 阶段 0：侦察与判定（动手前，不可跳过）

1. 读根 `AGENTS.md`（用 Read 工具，不靠 `@import`）、`web/AGENTS.md`（碰前端时）。
2. 读 `计划书/workflow_status.md` + `memory/*.md` + `.claude/skills/new-api-add-feature/SKILL.md`，
   判断要做的主题**是否已闭环**、有没有「别重复跑/改过」的记录。
3. **判过时**：文档里每句「路径/命令/默认值」都要能在当前代码 grep 到；对不上先更新文档。
4. 产出：一句话任务契约 + 验收标准 +（复杂时）任务依赖图。

检查点：能否指出「这件事已经有/没有实现」的 file:line 证据。

---

## 阶段 1：规范（Spec）

- 产出 `specs/NNN-<feature>/spec.md` + `plan.md`（用户故事 / 验收场景 / FR / 成功标准 / 假设）。
- 有歧义先问（≤5 问），不沉默选择。
- 涉三库/计费/认证，在 spec 里显式列出所引用的 AGENTS 规则编号。

检查点：每条验收场景都能翻译成一条可执行断言。

---

## 阶段 2：数据层

| 动作 | 依据 |
|---|---|
| 新表用显式 `TableName()` | `model/training_data.go:54-57` |
| 新表登记进 `AutoMigrate` | `model/main.go:384-385,436` |
| JSON 走 `common.*`，禁 `encoding/json` | `AGENTS.md:80-90`、`common/json.go:40-66` |
| 行锁用 `lockForUpdate(tx)` | `AGENTS.md:101` |
| 保留字列用 `commonGroupCol/commonKeyCol` 等 | `AGENTS.md:102-106` |

检查点：

```
go build ./... && go vet ./...
make db-check          # Makefile:53-54 → scripts/db-conformance.ps1
```

迁移**至少跑两次**（fresh + 上一版升级）证明幂等（`AGENTS.md:97`）。

---

## 阶段 3：接口与路由

- **管理端路由放 `/api/admin/...`**，独立 router 文件 + `/api`→`/admin` 分组 + `AdminAuth()`。
  范式：`router/web-protection-router.go:10-26`、`router/training-data-router.go:29-51`。
  **不要**放进 `api-router.go` 的 `adminRoute`（其父组是 `/api/user`，前缀会变成 `/api/user/...`：
  `router/api-router.go:89,167-168`）。
- 在 `router/main.go` 登记新 router（`router/main.go:32-33`）。
- 读接口用 `middleware.DisableCache()`（`router/training-data-router.go:35-50`）。

检查点：

```
go test ./router/ -run TestTrainingDataAdminRoutes   # 路由声明已注册且在 /api/admin 下
go test ./router/ -run TestFrontend.*PathsExistOnBackend   # 前端路径都能在后端找到
```

> 路由契约测试是本仓库的「防 404」防线：它**扫描前端源码**里的 `/api/...` 路径逐个比对后端路由表
> （`router/training_data_router_test.go:107-133`）。新增前端 API 文件后，把文件路径加进该测试的扫描列表
> （`router/training_data_router_test.go:115-118`）。

---

## 阶段 4：鉴权与安全

- 认证/授权/绑定/会话改动 → 先读 OWASP ASVS 与对应 Cheat Sheet（`AGENTS.md:49-55`）。
- 归属校验用 `model.Get*ByIds(id, userId)`；迁移/放行类 **fail-closed**（无证据即拒绝）。
- 服务端强制授权，前端开关只是 UI（`service/training_capture/consent.go:14-37`）。

检查点：安全回归用例覆盖失败/过期/重放/绕过；越权路径仍被覆盖。

---

## 阶段 5：前端

- 先搜 `web/src/components/` 与既有 feature 复用（`web/AGENTS.md:3.3`，强制）。
- 文案走 i18n；新 key **纯文本行插入**（见 `SOP-回滚.md` 同名红线与 ADR 说明）。

检查点（`web/package.json` 实有脚本）：

```
cd web && bun run typecheck && bun run lint && bun run format:check
bun run i18n:check && bun run copyright:check
bunx vitest run <受影响目录>
bun run build
```

---

## 阶段 6：性能（碰热路径/公开端点则强制）

- 热路径禁 DB、禁秒级聚合；缓存要 SWR + 单飞。
- 复制截断文本用 `strings.Clone`（`service/training_capture/hotpath.go:198-224`）。
- 常驻 loop 用 `service/background_loop.go`（可停可重启）。

检查点：公开端点**真压测**（单元测试测不到），脚本入 `计划书/e2e-evidence/<batch>/`；热路径报微秒级耗时。

---

## 阶段 7：验证、证据与审查

- TDD：先 RED（能复现失败）→ 实现 → GREEN；**RED 反向验证**：破坏修复点 → 测试变红 → 还原。
- 起**独立审查**子代理（不继承实现者结论），按需求完整性/边界/安全/测试缺口逐条复验。
- 断言四级标签：`已验证` / `静态确认` / `合理推断` / `待验证`。

检查点：§验收清单（见 skill §4）逐条有**真实输出**。

---

## 阶段 8：发布与回滚

- 发布仅在用户明确授权时进行（记忆纪律：按需部署）。
- 零停机用 `/opt/new-api/deploy-zero-downtime.sh <tag>`（**不要**用 `deploy.sh`，见记忆与
  `.claude/skills/new-api-add-feature/SKILL.md` §7、§12）。
- 部署脚本 `docker run` **必须**含 `--memory 900m --memory-swap 900m`（2026-10-07 全局 OOM 事故）。
- 回滚路径见 `计划书/ops/SOP-回滚.md`。

---

## 交付模板（实施类任务）

```markdown
# 结果
- 结论：complete / partial / blocked / failed
# 实际修改
- 关键文件与行为变化（接口/数据/配置/依赖）
# 验证
- 实际运行的命令与结果摘要；未运行的验证及原因
# 独立审查
- 审查轮次、已关闭问题、剩余事项
# 剩余风险
- 未验证项、外部限制、合理假设、需用户决策事项
```
