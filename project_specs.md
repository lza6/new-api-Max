# project_specs.md — 任务与状态权威源

> **权威关系声明（先读，避免又多一份互相矛盾的描述）**
>
> | 问题 | 权威来源 |
> |---|---|
> | 任务是否存在、优先级、状态 | **本文件** |
> | 表 / 字段 / 索引 | `db_structure.md`（脚本生成） |
> | 线上运行版本 | `workflow_status.md` §0 |
> | 施工细节（为什么这么做、怎么验收、怎么回滚） | `计划书/下一步改进指南.md`（编排）+ 尼尔森指南（交互）+ 视觉指南（视觉） |
> | 验证记录与已知噪声 | `test_ledger.md`、`计划书/audit-ledger.md` |
> | 版本口径 | `node scripts/check-version-consistency.mjs`（机器判定，非人眼） |
>
> 本文件**只记状态机与证据指针**，不重复施工细节。改状态必须同时给出证据路径，否则状态无效。

- 最后更新：2026-10-23
- 仓库基线：`VERSION=v1.3.187`（已发版）／最新 tag `v1.3.187`／线上 `v1.3.187`（2026-10-23 17:53 蓝绿，Caddy 上游 3001）
- 远端：`origin` = `lza6/new-api-Max`（**可推送**）；`upstream` = `QuantumNous/new-api`（**禁止推送**）

---

## 1. 状态图例

| 状态 | 含义（严格判定，不靠感觉） |
|---|---|
| `Done` | 有真实命令输出 + 证据路径；审查通过 |
| `Doing` | 已动手，证据未齐 |
| `Todo` | 未开始 |
| `Blocked` | 缺不可替代条件（环境/凭证/拍板），已写明缺什么 |
| `Dropped` | 决定不做，已写明理由 |

---

## 2. 任务状态

### 轨道 A — 工程可靠性

| ID | 目标 | 优先级 | 状态 | 依赖 | 证据 |
|---|---|---|---|---|---|
| T-A1 | 前端全量测试一条命令跑完（分片脚本） | P0 | **Done** | — | `web/scripts/test-shards.mjs`；**1582/1582 通过、0 失败、5 片、567s**（2026-10-23 17:2x） |
| T-A5 | 版本四口径机器校验 | P0 | **Done** | — | `scripts/check-version-consistency.mjs`；自测 9/9 PASS；真实仓库 `RESULT: consistent`；已进 CI |
| T-A6 | CI 补门禁 | P0 | **Done** | T-A5 | `.github/workflows/ci.yml` 新增 `i18n terminology gate` + `Version consistency gate`；`Bun.YAML.parse` 校验通过（frontend 12 steps / 3 jobs）；terminology 门禁本地 RED→GREEN 见 T-B9a |
| T-A2 | controller 全量 `-shuffle` 顺序依赖回归 | P1 | **Todo** | — | — |
| T-A3 | 三库 conformance 重跑 | P1 | **Todo** | 本机 MySQL/PG 服务 | — |
| T-A4 | ClickHouse 真跑验证 | P0 | **Blocked** | Docker 或 CH 二进制 | 缺：本机无 `docker`（`docker: command not found`）。需用户提供运行环境或批准下载 CH 二进制 |
| T-A7 | 工作区卫生清单（只列不删） | P2 | **Todo** | — | — |
| T-A8 | 三份规格文件补全 | P1 | **Done** | — | `db_structure.md` ✅（55 表/637 列，脚本生成）／`project_specs.md` ✅（本文件）／`.agent/memory/**` ✅（6 文件全部落盘：audit-history/change-index/decisions/known-issues/optimization-log/verified-scope） |

### 轨道 B — 交互契约（N 系列残余）

| ID | 目标 | 优先级 | 状态 | 证据 |
|---|---|---|---|---|
| T-B1 | 39 处 `ApiErrorMsg(...err.Error())` 改稳定码 | P1 | **Done** | 基线清点：39 处 / 8 个 controller；2026-10-23 复测 `grep -rn "ApiErrorMsg(.*err\.Error()" controller/ middleware/` = **0** | `evidence/T-B1.json` |
| T-B2 | 清最后 2 处原生 `window.confirm` + 删死 hook | P1 | **Done** | 2026-10-23 复测：`grep -rn "window\.confirm" web/src` = 3 处但**全为注释**（form-navigation-guard.tsx:35 / checkin-settings-section.tsx:124 / waffo-pancake-settings-section.tsx:83，均写明「改用 ConfirmDialog」），真实调用点 **0**；死 hook `use-form-dirty-guard.ts` 已删 | `evidence/T-B2.json` |
| T-B3 | 12 处数据区空态带动作 | P1 | **Todo** | 基线：`grep "No data\|No results"` = 17，其中 12 处需动作 |
| T-B4 | 错误 toast 带可点动作 | P1 | **Todo** | 基线：`handle-server-error.ts` 3 处 `toast.error` 无 action |
| T-B5 | 长任务进度/可取消 | P1 | **Todo** | 基线：`toast.loading` 全仓 2 处 |
| T-B6 | 轮询随可见性停 + 新鲜度 | P1 | **Todo** | 基线：`refetchInterval` 15 处 / `use-document-visible` 6 文件 |
| T-B7 | 日志/订阅表多选与批量 | P1 | **Todo** | 基线：3 处 `enableRowSelection: false` |
| T-B8 | 高危设置分区内联帮助 | P1 | **Todo** | 基线：帮助组件仅 2 处在用 |
| T-B9a | N1 术语残余（quota→额度） | P1 | **Done** | 门禁假绿已修（`\b` 词边界）；zh/zh-TW 各 2 处改「额度」；RED 4 处 → GREEN 0 处 |
| T-B9b | 表单校验时机 `onChange`→`onBlur` | P2 | **Todo** | 基线：`mode:'onChange'` 11 处 / `onBlur` 1 处 |

### 轨道 C — 视觉（V0–V13）

| ID | 目标 | 状态 | 说明 |
|---|---|---|---|
| T-C0…T-C3 | V0 删死配置 / V1 令牌层 / V2 `Card.tone` / V3 `Metric` | **Doing（他会话实施，验收未完成）** | 工作区已有实现（未提交）；本会话负责验收：typecheck / build / 体积 / 截图 / axe |
| T-C4…T-C11 | V4–V13 | **Todo** | 待 T-C0…C3 验收提交后开工 |

### 轨道 D — 财务与运营

| ID | 目标 | 状态 | 前置 |
|---|---|---|---|
| T-D1a | Claude 计费疑似缺陷只读审计 | **Todo** | 无（只读，可随时开） |
| T-D1b | 计费修复 | **Todo** | T-D1a 判「存在」+ 计费红线流程 |
| T-D2 | 消耗排行榜 / 累计充值 | **Todo** | 需求确认 |
| T-D3 | 成本覆盖率历史缺口 | **Done（结论：不可追溯补真）** | 面板已标注起始日；禁止伪造回填 |
| T-D4 | 真实上游 p95 压测 | **Blocked** | 预算红线（默认 0 次付费调用）需批准 |

---

## 3. 本会话（2026-10-23）已验证事实

| # | 事实 | 命令 | 结果 |
|---|---|---|---|
| 1 | Go 根模块构建 | `go build ./...` | PASS，54.7s |
| 2 | 前端类型检查 | `bun run typecheck` | PASS，4.9s |
| 3 | 版本口径 | `node scripts/check-version-consistency.mjs` | `consistent`（3 条 pending-release note） |
| 4 | 版本门禁自测 | `--self-test` | 9/9 PASS |
| 5 | 术语门禁 | `node scripts/check-terminology.mjs` | 修复假绿后 RED 4 → 修复文案后 GREEN 0 |
| 6 | 漏译门禁 | `node scripts/check-missing-i18n.cjs` | 0 missing |
| 7 | CI 配置结构 | `Bun.YAML.parse(ci.yml)` | 3 jobs，frontend 12 steps |
| 8 | 真实 schema | 构建 → `SQLITE_PATH` 启动 AutoMigrate → `node:sqlite` 导出 | 55 表 / 637 列 |
| 9 | 全量前端测试单进程 | `bunx vitest run` | **>25 分钟未完成**（P0-2 证据）→ 必须分片 |

---

## 4. 更新规则

1. 改状态必须同批更新「证据」列，且证据是可复跑的命令输出或文件路径。
2. 新增任务必须先给出「验收标准 + 回滚方式」，否则不入表。
3. 每次发版后同步 `workflow_status.md` §0；每次 schema 变更后重生 `db_structure.md`。
4. 本文件不写「计划中」当成「已完成」；两者混写一次，整份文件就不可信。
