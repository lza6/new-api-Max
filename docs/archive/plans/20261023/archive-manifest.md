# 归档清单 — 计划文档归档批次 `20261023`

> 本目录是**删除前的备份**，不是 `docs/` 的常规新增内容。
> `AGENTS.md` 规定「不得在 `docs/` 下新增文件」，本次归档属**用户明确授权的例外**，
> 并以本 manifest 留痕（来源、判定依据、删除时间、回滚方式）。

## 1. 批次信息

| 项 | 值 |
|---|---|
| 归档批次 | `20261023` |
| 执行时间 | 2026-10-23T21:00+08:00 |
| 授权 | 用户在 2026-10-23 会话中明确授权「删除已完全闭环的计划文档」（仅限下列 2 份） |
| 审计报告 | [`evidence/audit-20261023-docs.json`](../../../../evidence/audit-20261023-docs.json)（57 份文档 / 121 任务单元：保留 53 / 可删除 2 / 需决策 2） |
| 删除前 commit | `664a8489b866607e896771bbccf3575c746740c0` |

## 2. 归档明细

| 文档路径（删除前） | 任务总数 | 已闭环 | 已过时 | 不适用 | 用户决策 | 归档路径 | 是否可删除 |
|---|---|---|---|---|---|---|---|
| `前端改进计划.md`（仓库根） | 5 | 5 | 0 | 0 | 0 | `docs/archive/plans/20261023/前端改进计划.md` | 是 |
| `计划书/ops/测试分片矩阵.md` | 7 | 7 | 1（核心结论被 T-A1 推翻） | 0 | 0 | `docs/archive/plans/20261023/测试分片矩阵.md` | 是 |

字节校验（归档前 `diff` 与 `sha256sum`，两份均一致）：

| 文件 | SHA-256 |
|---|---|
| `前端改进计划.md` | `278cb332a442b00093199ccc7a09e08d54da19a7994eaa702cdd983f1eeead83` |
| `测试分片矩阵.md` | `a410de31e86343921d8a3f5b97e7efdf9cac991ea400ee046352d3e30c2d2b23` |

## 3. 复核结论（执行者二次确认，非仅采信审计）

- `前端改进计划.md`：§14–§18 全部为 v1.3.166–170 已执行事项并附验证记录；
  §17 明写「T-B4 留待单独一轮」，§18 已记 v1.3.170 T-B4 闭环；
  §15.5 记录的既有缺陷（导航以中文原文当 key）已在 `web/src/hooks/use-top-nav-links.ts`
  改为 `t('Home')` 等英文 key（v1.3.168 补齐 key），属**已过时**，非未闭环。
  代码侧抽验通过：`settings-page-context.tsx:104` 统一渲染 `FormNavigationGuard`、
  `theme.css:127/265` 存在 `--theme-color` 令牌、`test-setup.ts:135` 已补 `ReadableStream`、
  `uptime-panel.tsx:50-59` 的 `StatusDot` 有 `role='img'` + `aria-label`/`title`、
  `rsbuild.config.ts` 已无 `vendor-react` 死配置。→ **零未闭环，可删除**
- `计划书/ops/测试分片矩阵.md`：7 行分片均为已执行实测记录，无任务残留；
  其「本机无法单进程跑完全量 vitest」的结论已被 T-A1 推翻
  （`web/package.json:19` `"test:shards": "bun scripts/test-shards.mjs"`，
  `test_ledger.md:697` 记 1582/1582 PASS / 5 片 / 566.9s）。保留只会误导下一轮。→ **可删除**

## 4. 删除前的引用处理

删除前已把这些位置改为不依赖被删文件（详见 `evidence/doc-archive-20261023.json`
的 `references_fixed`）：`CHANGELOG.md`、`计划书/下一步改进指南.md`（§3.1 目录树、§3.3 漂移表、
§5.3、§6 P2-3、§13.2 清理清单）、`计划书/README.md`（索引行）、`workflow_status.md`（B2 段）、
`计划书/尼尔森十项可用性原则-界面设计规范与整改指南.md`（§引用与 H8.4 证据行）。
历史审计/任务快照（`计划书/audit/*`、`计划书/tasks/*`、`evidence/audit-20261023-docs.json`）
保留原文不动——它们是带时间戳的历史记录，不改写。

## 5. 回滚

```bash
# 单份恢复（推荐，删除发生在 commit 664a8489 之后的工作区/后续提交）
git checkout 664a8489b866607e896771bbccf3575c746740c0 -- "前端改进计划.md"
git checkout 664a8489b866607e896771bbccf3575c746740c0 -- "计划书/ops/测试分片矩阵.md"

# 或直接从本归档目录拷回（字节一致）
cp -a docs/archive/plans/20261023/前端改进计划.md ./前端改进计划.md
cp -a docs/archive/plans/20261023/测试分片矩阵.md ./计划书/ops/测试分片矩阵.md
```

回滚后需同步还原上述引用改写（`git checkout 664a8489b -- CHANGELOG.md 计划书/下一步改进指南.md
计划书/README.md workflow_status.md 计划书/尼尔森十项可用性原则-界面设计规范与整改指南.md`
会丢掉本轮其它无关改动，建议按 `references_fixed` 逐条手改）。
