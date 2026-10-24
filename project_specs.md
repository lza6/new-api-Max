# project_specs.md — 任务与状态权威源

> **权威关系声明（先读，避免又多一份互相矛盾的描述）**
>
> | 问题 | 权威来源 |
> |---|---|
> | 任务是否存在、优先级、状态 | **本文件** |
> | 表 / 字段 / 索引 | `db_structure.md`（脚本生成） |
> | 线上运行版本 | `workflow_status.md` §0 |
> | 施工细节（为什么这么做、怎么验收、怎么回滚） | `计划书/下一步改进指南.md`（编排）+ 尼尔森指南（交互）+ 视觉指南（视觉） |
> | 验证记录与已知噪声 | `test_ledger.md`、`计划书/audit-ledger.md`、`.agent/memory/verified-scope.md` |
> | 版本口径 | `node scripts/check-version-consistency.mjs`（机器判定，非人眼） |
>
> 本文件**只记状态机与证据指针**，不重复施工细节。改状态必须同时给出证据路径，否则状态无效。

- 最后更新：**2026-10-24**（对齐 `计划书/下一步改进指南.md` 基线 v1.3.195）
- 仓库基线：`VERSION=v1.3.195`（已发版）／最新 tag `v1.3.195`／**线上 `v1.3.195`（2026-10-24 生产实测，与仓库零落后）**
- 远端：`origin` = `lza6/new-api-Max`（**可推送**）；`upstream` = `QuantumNous/new-api`（**禁止推送**）
- 上一版本记录：`v1.3.187` 基线（见 git 历史）

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

### 轨道 A — 工程可信

| ID | 目标 | 优先级 | 状态 | 依赖 | 证据 |
|---|---|---|---|---|---|
| T-A1 | 前端全量测试一条命令跑完（分片脚本） | P0 | **Done** | — | `web/scripts/test-shards.mjs`；2026-10-24 实测 **1599/1601、707.4s**（2 条失败归因并行会话在途改动，见指南 §1.6.1） |
| T-A5 | 版本四口径机器校验 | P0 | **Done（脚本）／失效（触发器）** | — | `scripts/check-version-consistency.mjs`；自测 9/9；**但 CI 只在 PR 触发 → 从未拦截过，2026-10-24 实测为 RED** |
| T-A6 | CI 补门禁 | P0 | **半完成** | T-A5 | 6 条门禁 step 已在 `ci.yml`；`Test` 步骤仍 `bun run test`（单进程 `maxWorkers:1`）+ `timeout 10min` → 跑不完 |
| T-A8 | 三份规格文件补全 | P1 | **Done（已过期一次）** | — | `db_structure.md`（55 表/637 列）／`project_specs.md`／`.agent/memory/*`；本文件已重新对齐 v1.3.195 |
| **T-A9** | CI 门禁真正被触发（加 `push: main`、Test 改分片、放宽超时） | **P0** | **Todo** | — | — |
| **T-A10** | 修 3 处门禁红项（`计划书/README` 版本声明 / `workflow_status` §0 / 镜像清单矛盾） | **P0** | **Todo** | — | 2026-10-24 实测 `check-version-consistency` = inconsistent |
| **T-A11** | 三库 conformance 重跑（v1.3.189–195 改过 `model/` 后必做） | **P1** | **Todo** | 本机 MySQL/PG 临时实例 | 最后证据是 `evidence/three-db-conformance-20261018.json`（v1.3.16x 期） |
| **T-A12** | `controller -shuffle` 顺序依赖回归 | P1 | **Todo** | — | — |
| **T-A13** | 工作区卫生落地（提交未纳管文档、e2e-local 归档分类） | P1 | **Doing** | — | 2026-10-24：尼尔森指南已提交；`web/e2e-local/` 73 个未跟踪脚本待迁 `archive/` |
| **T-A14** | 台账对齐（本文件 + `workflow_status` §0 + `计划书/README`） | P1 | **Done（2026-10-24）** | T-A10 | 三处均已对齐 v1.3.195；`workflow_status` §0 已按生产实测把「线上 192→195」纠正，并补了主机暴露面/备份两行 |
| **T-A15** | `relaykit` 独立构建复验（`GOWORK=off`） | P2 | **Todo** | — | — |
| T-A4 | ClickHouse 真跑验证 | P0 | **Blocked** | Docker 或 CH 二进制 | 缺：本机无 `docker`；从未执行 |
| T-A7 | 工作区卫生清单（只列不删） | P2 | **Dropped** | — | 已被 T-A13 取代（本轮直接给了处置决策） |

### 轨道 B — 交互契约

| ID | 目标 | 优先级 | 状态 | 证据（2026-10-24 实测） |
|---|---|---|---|---|
| T-B1 | `ApiErrorMsg(...err.Error())` 改稳定码 | P1 | **Done** | `8055b8789` 收口；复测 = **0** |
| T-B2 | 清原生 `window.confirm` + 删死 hook | P1 | **Done** | 3 处命中全为注释，真实调用 **0** |
| T-B9a | quota 术语统一为「额度」 | P1 | **Done** | 术语门禁 PASS |
| T-B3 | 数据区空态带下一步动作 | P1 | **Todo** | `"No data"\|"No results"\|No data yet` = **18 处**（需先分类：真缺动作 vs 下拉面板不该有） |
| T-B4 | 错误 toast 带可点动作 | P1 | **Todo** | `web/src/lib/handle-server-error.ts:57,65,72` 三处无 action |
| T-B5 | 长任务进度/可取消 | P1 | **Todo** | `toast.loading` 仍 **2 处**，同在 `channel-test-dialog.tsx` |
| T-B6 | 轮询随可见性停 | P1 | **Todo** | `refetchInterval` **22 处 / 17 文件**；`useDocumentVisible` 仅 **5 文件** → 门控覆盖不足（口径见指南 §7.1 R4） |
| T-B7 | 日志/订阅表多选（**范围已缩小到 2 张表**） | P1 | **Todo** | `usage-logs-table.tsx:179`、`subscription-logs-table.tsx:176`；`audit-log-viewer.tsx:100` 的 false 是正确设计 |
| T-B8 | 高危设置分区内联帮助 | P1 | **Todo** | 用量未量化（`LearnMore`/`HelpTooltip` 使用面待清点） |
| T-B9b | 表单校验时机 `onChange`→`onBlur` | P2 | **Todo** | `mode: 'onChange'` = **12 处** |
| T-B10 | Service Worker 静态资源 cache-first 策略 | P2 | **Todo** | `web/public/sw.js:28-49` |
| T-B11 | GET 去重键纳入关键 config | P2 | **Todo** | `web/src/lib/http-client.ts:58-75` |
| T-B12 | 落地页第三方取图自托管 | P2 | **Todo** | `features/home/components/sections/hero.tsx:284-296` |

### 轨道 C — 视觉（V0–V13）

| ID | 目标 | 状态 | 说明 |
|---|---|---|---|
| T-C0…T-C11 | V0–V13 | **Done** | v1.3.182/185/186 已发版；token 门禁绿；axe 11 预设 × 2 页 0 违规；`transition-all` 0 命中 |
| — | V7 三态视觉 | **◐ 半完成** | error/loading 无单测、截图 0 张 → 并入 T-B3 验收 |
| — | 维护要求 | **生效中** | 任何 UI 改动后必须重跑 `visual-baseline.cjs` + axe + `ds-tokens-check` |

### 轨道 D — 财务与运营

| ID | 目标 | 优先级 | 状态 | 前置 / 证据 |
|---|---|---|---|---|
| T-D1a | Claude 计费只读审计 | P1 | **Doing** | `evidence/audit-20261023-billing.json`（9 条，2 条 P1 已修）；**`计划书/ops/DECISION-claude-billing.md` 尚未读 → 结论待确认** |
| T-D1b | 计费口径修复（仅当审计判"存在"） | P1 | **Todo** | 需先读上述 DECISION；禁止改生产表达式 |
| T-D2 | 消耗排行榜 / 累计充值 | P2 | **Todo** | **需拍板**（谁的排行榜、隐私边界） |
| T-D3 | 成本覆盖率历史缺口 | P2 | **Done（结论：不可追溯补真）** | 面板已标注起始日；禁止伪造回填 |
| T-D4 | 真实上游 p95 压测 | P2 | **Blocked** | 预算红线（默认 0 次付费调用）需批准 |

### 轨道 E — 生产运维（新增）

| ID | 目标 | 优先级 | 状态 | 前置 / 证据 |
|---|---|---|---|---|
| T-E1 | 每日 `pg_dump` + 7 天轮转 + **restore 演练** | **P0** | **Done（2026-10-24）** | 脚本加固 + 恢复演练通过（pg_restore exit 0 / 1m31s / 表 62=62 / users 1625=1625 / tokens 1452=1452 / daily 415=415）+ 异地副本 sha256 一致 → `evidence/T-E1.json`、`计划书/ops/SOP-备份与恢复.md` |
| T-E2 | 主机加固（SSH 密钥化 / 关密码登录 / 关公网端口 / ufw） | **P0** | **Done（2026-10-24）** | 密钥登录+关密码（实测 `Permission denied (publickey)`）、MaxAuthTries 3、ufw 仅 22/80/443、postfix 只听 loopback、compose 端口锁 loopback；外网仅 22/80/443 可达，站点 200 → `evidence/T-E2.json`、`计划书/ops/SOP-主机加固.md` |
| T-E3 | Docker 日志轮转上限 | P1 | **Todo** | `docker-compose.yml` 无 `logging:` 段（示例文件；生产以 `LogConfig` 为准） |
| T-E4 | 连接池 `SQL_MAX_OPEN_CONNS` 64 → ≤ PG `max_connections` | P1 | **Todo** | `model/main.go:212,266` 默认 64；审计称生产 PG 上限 50 |
| T-E5 | Caddy 健康检查 `/api/status` → `/readyz` | P1 | **Todo** | 需真实演练：停 PG 后 `/readyz` 必须非 200 |
| T-E6 | 部署脚本脱敏（无明文 DB/Redis 密码，权限 700） | P1 | **Todo** | `计划书/ops/deploy-zero-downtime.example.sh` |

### 轨道 F — 数据层性能（新增）

| ID | 目标 | 优先级 | 状态 | 证据位置 |
|---|---|---|---|---|
| T-F1 | `/api/log/report` 单飞 + SWR + `start` 下限 | P1 | **Todo** | `model/usage_report.go:117-135`、`controller/log.go:184-441` |
| T-F2 | `web_request_logs` IP 聚合加时间窗 | P1 | **Todo** | `model/web_request_log.go:55-88` |
| T-F3 | 用户画像聚合下推（不再拉 30 天明细进内存） | P1 | **Todo** | `service/user_profile/profile.go:214-247` |
| T-F4 | `quota_data` 唯一约束 + 单事务 | P2 | **Todo** | `model/usedata.go:11-24,104-124` |
| T-F5 | 冗余索引清理（约 190MB 可无损回收） | P2 | **Todo** | `db_structure.md:689-691`、`model/audit_log.go:35-36`、`model/log.go:62-95` |
| T-F6 | `GetOpsDailySeries` 合并为一次 GROUP BY | P2 | **Todo** | `controller/ops_panel.go:556-575`（最多 90 次串行） |

### 轨道 G — 计费正确性（新增，**独立成版**）

| ID | 目标 | 优先级 | 状态 | 证据位置 |
|---|---|---|---|---|
| T-G1 | `BATCH_UPDATE` 落库失败不再静默丢弃增量 | **P1** | **Todo** | `model/utils.go:147-157`（返回值被丢弃） |
| T-G2 | `BillingSession.Refund` 进优雅关闭排空 | P1 | **Todo** | `service/billing_session.go:109-125`（`gopool.Go` 即发即忘）、`main.go:361-371` |
| T-G3 | 任务结算 `usageFacts` 上界钳制 | P1 | **Todo** | `service/task_polling.go:738-752`、`service/task_billing.go:322-430` |

---

## 3. 本轮（2026-10-24）已验证事实

| # | 事实 | 命令 | 结果 |
|---|---|---|---|
| 1 | 版本口径 | `node scripts/check-version-consistency.mjs` | **inconsistent**（`计划书/README.md` 声明 v1.3.192） |
| 2 | 术语门禁 | `cd web && node scripts/check-terminology.mjs` | PASS |
| 3 | 漏译门禁 | `cd web && node scripts/check-missing-i18n.cjs` | OK（0 missing） |
| 4 | 令牌门禁 | `cd web && node scripts/ds-tokens-check.mjs` | OK（743 tokens） |
| 5 | Go 构建 | `go build ./...` | PASS，10.2s（缓存热） |
| 6 | 前端全量 | `cd web && node scripts/test-shards.mjs` | **1599/1601，2 failed，707.4s**（失败归因并行会话在途改动） |
| 7 | CI 触发条件 | 读 6 个 workflow | **`ci.yml` 仅 `on: pull_request`** → 全部门禁在本仓库工作流下从未运行 |
| 8 | 未纳管文档 | `git -c core.quotepath=false ls-files 计划书` | 尼尔森指南**不在**版本控制（现已补交） |

---

## 4. 更新规则

1. 改状态必须同批更新「证据」列，且证据是可复跑的命令输出或文件路径。
2. 新增任务必须先给出「验收标准 + 回滚方式」，否则不入表。
3. 每次发版后同步 `workflow_status.md` §0；每次 schema 变更后重生 `db_structure.md`。
4. 本文件不写「计划中」当成「已完成」；两者混写一次，整份文件就不可信。
5. 与 `计划书/下一步改进指南.md` 的卡 ID 必须一一对应；新增卡先改指南再改本文件。
