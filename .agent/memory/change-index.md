# change-index.md — 变更索引（文件 → 为什么变）

> 目的：接手者能回答「这个文件为什么长这样」。只记**本会话及近期**有实质影响的变更。
> 逐提交明细看 `git log`；这里记**意图**，不是 diff。

## 2026-10-23（本会话，T-A 轨道）

| 文件 | 类型 | 变更与原因 |
|---|---|---|
| `scripts/check-version-consistency.mjs` | 新增 | T-A5：VERSION / CHANGELOG / tag / §0 / README 五处口径机器校验。含 `--self-test`（9 例）与 `--require-tag` 发版严格模式 |
| `scripts/gen-db-structure.mjs` | 新增 | T-A8：从真实 AutoMigrate 结果生成 `db_structure.md`，禁止手抄 schema |
| `db_structure.md` | 新增（生成物） | T-A8：55 表 / 637 列，来自一次性 SQLite 实例 |
| `project_specs.md` | 新增 | T-A8：任务状态权威源（只记状态机与证据指针） |
| `.agent/memory/*.md` | 新增 6 份 | T-A8：audit-history / verified-scope / known-issues / optimization-log / change-index / decisions |
| `web/scripts/test-shards.mjs` | 新增 | T-A1：197 个测试文件按体积贪心切成 N 片，一条命令给出总通过数 |
| `web/scripts/check-terminology.mjs` | **修 bug** | 豁免正则 `/rate\|limit\|concurren/` 会命中 `Unlimited` 里的 `limit`，导致「无限配额」被跳过 → 门禁假绿。改为 `\b(...)\b` 词边界 |
| `web/src/i18n/locales/zh.json` / `zh-TW.json` | 修文案 | T-B9a：`无限配额` / `無限配額` → `无限额度` / `無限額度`（各 2 处） |
| `.github/workflows/ci.yml` | 新增 2 step | T-A6：`i18n terminology gate`、`Version consistency gate`（其余 4 个门禁与 db-conformance job 由并行会话先行加入） |
| `计划书/下一步改进指南.md` | 重写 | 基线 v1.3.166 → v1.3.181，含 v1.3.167→181 完成度实测盘点 |
| `计划书/README.md` | 修事实段 | 版本口径改为机器判定（脚本），并更新到 v1.3.182 未发版状态 |

## 他会话在途（本会话**不提交、不改动**，仅登记）

| 文件 | 内容 |
|---|---|
| `web/src/styles/theme.css`、`index.css`、`main.tsx` | 视觉 V1 令牌层（`--radius` 0.625rem、`--numeric-font`、surface/rule 令牌） |
| `web/src/config/fonts.ts`、`context/font-provider.tsx` | V0 删除 |
| `web/src/components/metric.tsx` + `__tests__/metric.test.tsx`、`components/ui/card.tsx`、`features/dashboard/**/stat-card.tsx` | V2/V3 |
| `web/scripts/ds-tokens-*.mjs`、`check-i18n-coverage.cjs`、`knip-gate.mjs`、`package.json`、`bun.lock` | 设计令牌快照门禁、i18n 覆盖率门禁、JetBrains Mono 依赖 |
| `web/src/i18n/locales/{fr,ja,ru,vi,zh-TW}.json` | 补 95/66 条漏译 |

## 他会话遗留、仅换行级改动（不得代提交）

`controller/webhook.go`、`model/shadow_price.go`、`controller/site_stats_test.go`、`model/{channel_constraint_test,shadow_price_test,subscription_plan_delete_test}.go` —— 已由 `workflow_status.md` §13 登记为「其它会话未提交文件」。

## 2026-10-23 17:30（前端套件转绿批次）

| 文件 | 变更与原因 |
|---|---|
| `web/vitest.config.ts` | 池 `vmThreads` → `threads`：前者同 worker 共享 VM 上下文，`vi.mock` 串台导致「单跑绿、合跑红」 |
| `web/src/test-setup.ts` | 注入 Node WebCrypto（jsdom 无 `crypto.subtle`，摘要校验用例会走不到真实分支） |
| `web/src/lib/handle-server-error.ts` | 新增 `resetServerErrorDedupe()`：去重表是模块级状态，测试需按例清空 |
| `web/src/features/task-plugins/lib/plugin-icon-file.ts` | 摘要校验 fail-open → **fail-closed**（pin 了 sha256 就必须校验通过） |
| `web/src/features/system-settings/security/sections.ts` | 补登记 `policy-center`：分区已存在但导航元数据缺失 |
| 25 个前端文件 | 去掉 40 个「仅本文件使用」的多余 export（行为等价） |
| `web/src/features/ops-panel/{api,types}.ts`、`pricing/components/site-subscription-stats-card.tsx` | 删除未使用的 API 封装 / 类型 / 组件 |
| `web/package.json` + `bun.lock` | 移除死依赖 `i18next-browser-languagedetector` |
| `web/scripts/knip-gate.mjs` | 不再把 `e2e-local/`、`scripts/` 算死代码，忽略仅探针使用的 `playwright` |
| `web/.ai/ds/ds-tokens.json` | 重生成：117 条漂移对应 ink 预设（V13） |
| 8 个测试文件 | 断言对齐真实契约（Base UI 的 `<a role=button>`、`100dvh`、具名 combobox、表达式不外显策略） |

## 2026-10-24（改进指南 v1.3.195 重写批次，**仅文档**）

| 文件 | 类型 | 变更与原因 |
|---|---|---|
| `计划书/下一步改进指南.md` | **整篇重写** | 基线 v1.3.181 → v1.3.195；新增 §1.3「CI 门禁真相」、§7「自我反驳与盲点」、轨道 E/F/G；P0 3→5、任务卡 32→34、路线 v1.3.196–209 + 砍版规则 |
| `计划书/archive/plans/20261024/下一步改进指南-v1.3.181基线.md` | 新增（归档） | 旧版留档，sha256 `56bbcb0b…d79fb9`，80646 B |
| `计划书/archive/plans/20261024/archive-manifest.md` | 新增 | 归档 manifest（含恢复命令） |
| `project_specs.md` | 重写 | 对齐 v1.3.195；7 轨道 / 34 卡状态表；据实修 T-A2/A3/A4/A7 与 B 轨残余数量 |
| `计划书/tasks/riper5_20261024_184000.md` | 新增 | 本轮 RIPER-5 记录 |
| `计划书/tasks/weekly.md` | 追加 | 第 44 周本轮摘要 |
| `计划书/尼尔森十项可用性原则-界面设计规范与整改指南.md` | **首次纳入 git** | 此前未跟踪（P1-5），47.1KB 唯一可用性施工蓝图 |
| `logger/zz_dryrun_test.go` | **首次纳入 git** | 未跟踪但参与 `go test ./logger/` 编译；有效手工 dry-run 工具 |
| `.agent/memory/{known-issues,verified-scope,change-index,decisions,audit-history}.md` | 追加 | 本轮事实与决策 |

**未改动（明确）**：无任何 `.go` / `.ts` / `.tsx` / `.yml` / 测试 / 脚本变更；他会话 12 处源码改动未动。
