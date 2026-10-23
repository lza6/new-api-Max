# decisions.md — 决策记录（含理由与代价）

> 只记**已经影响代码形状**的决策。每条必须有「代价」，没有代价的决策多半是没想清楚。

## D-2026-10-23-01｜版本口径改由脚本判定，不再靠人眼

- **背景**：`计划书/README.md` 曾写「最新 tag = v1.3.180」而实际 v1.3.181 已发版。手写事实必漂移。
- **决策**：新增 `scripts/check-version-consistency.mjs`，比较 5 处来源；进 CI；发版时加 `--require-tag`。
- **代价**：每次改 VERSION 都要顺带更新 README 声明，多一步手工。

## D-2026-10-23-02｜「VERSION 先于 tag」不算错

- **背景**：门禁第一版把 tag≠VERSION 当硬错误，结果正常发版窗口（bump 后未打 tag）天天红。
- **决策**：默认把「VERSION 领先 tag」记为 note；只有「tag 新于 VERSION」（不可能态）才报错；`--require-tag` 用于发布流水线。
- **代价**：门禁默认宽松，必须靠发版流程记得加 `--require-tag`。

## D-2026-10-23-03｜`db_structure.md` 由脚本从真实库生成

- **背景**：手抄 55 张表必然错；且 `AGENTS.md` 要求三库兼容，手抄无法体现方言差异。
- **决策**：`scripts/gen-db-structure.mjs` 读一次性 AutoMigrate 后的 SQLite 文件生成。
- **代价**：schema 变更后必须重跑生成并提交，否则文档过期；SQLite 不能代表 MySQL/PG 的类型细节（已在文件头声明边界）。

## D-2026-10-23-04｜术语门禁豁免条改词边界

- **背景**：`Unlimited Quota` 因 `limit` 子串被当成限流类豁免，门禁假绿 2 处违规。
- **决策**：`EXEMPT_KEY` 改为 `\b(rate|rates|limit|limits|limited|concurrency|concurrent)\b`。
- **代价**：若将来出现「限额」语义的新 key（如 `credit limit`）仍会被豁免——那是**有意的**，因为限流语境里「配额」才是准确词。

## D-2026-10-23-05｜不代提交他会话的在途改动

- **背景**：工作区内同时存在视觉 V0–V3、i18n 补译、CI/门禁脚本等他人改动。
- **决策**：只提交本会话产出（脚本 + 规格文件 + 文档 + N1 术语修复），不 `git add -A`，不 `git clean`。
- **代价**：本轮提交不构成「完整可发布状态」，发版需等他会话收口或明确接管。

## D-2026-10-23-06｜推送目标固定 `origin`，禁用 `upstream`

- **背景**：`main` 的 upstream 被设为 `QuantumNous/new-api`（非本仓库）。
- **决策**：一切推送显式写 `git push origin main`；禁止 `--force`。
- **代价**：依赖执行者记住这一点（已写入 `known-issues.md` 与本文）。
