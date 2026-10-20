# 计划文档审计 manifest（2026-10-20）

> 本文件是**只读审计**的记录，不是删除操作。审计结论：**删除候选 0 个**。
> 依据：所有非永久类文档要么含未闭环/决策项，要么属于历史验收证据；SOP/ADR/黄金范例/
> 运维手册/排障文档按规则永久保留。

## 审计基线

- `VERSION`：`v1.3.161`
- HEAD：`d64b0deb6`（审计时）
- 权威状态来源：根 `workflow_status.md`、`test_ledger.md`、`policy.md`、`计划书/audit-ledger.md`

## 规则

1. SOP / ADR / 黄金范例 / 运维手册 / 排障文档：永久保留。
2. 审计报告 / E2E 证据 / 变更报告：历史证据，保留。
3. 含未完成复选框、TODO、待决策、待上线、证据不足的文档：不可删除。
4. 只有「全部闭环/过时/不适用且无决策项」才归档删除。

## 删除候选

无。

## 永久保留（14 篇）

- `计划书/ops/ADR-架构决策记录.md`（§13 有 5 项待确认）
- `计划书/ops/黄金范例.md`
- `计划书/ops/SOP-运维手册.md`
- `计划书/ops/SOP-回滚.md`
- `计划书/ops/SOP-新功能交付.md`
- `计划书/ops/SOP-排障-训练数据采集.md`
- `计划书/ops/SKILL-新增功能交付.md`
- `计划书/ops/token-billing-group.md`（价格以运行时 options 为准）
- `计划书/adr/ADR-001` … `ADR-006`（6 篇）

## 历史证据（保留）

- `计划书/audit/*.md`（6 篇）
- `计划书/e2e-evidence/**/*.md`（9 篇）
- `CHANGELOG.md`

## 仍然开放的审计项（不可删除的原因）

- `计划书/audit/scan-billing.md`：P1-1 Stripe 两套单价口径、P1-2 `min_topup × QuotaPerUnit`、
  P2-1 嵌套事务、P2-2 预警档位、P2-3 Creem 补单口径、P3。
- `计划书/audit/scan-auth.md`：登录时序侧信道、用户名枚举、Creem webhook 全量 body 入日志等。
- `计划书/audit/scan-contract.md`：前端 `page_size=1000` 与后端硬截 100 不一致、静默回退。
- `计划书/audit/critic-v1.3.129-132.md`：`SiteDailyVisitor` 未进 conformance、SEO 注入块挂载前可见等。
- `计划书/audit/geo-safe-result.md`：缺真实浏览器 E2E；“服务条款 vs 用户协议”待裁决。
- `计划书/e2e-evidence/spec-008/hotpath-stress.md`：HTTP 层 p95、生产硬件数字、长期内存稳定性。
- `计划书/audit-ledger.md`：消耗排行榜、累计充值金额、Claude 计费待业主拍板。
- `workflow_status.md`：Claude 计费、累计充值、p95、P3 UI、`/readyz`。
- `计划书/ops/ADR-架构决策记录.md` §13：Sobek / 固定窗口 / Caddy 参数 / 三库选型 / 技术栈选型理由。

## 本轮已修正的文档事实

- 根 `workflow_status.md` / `test_ledger.md` / `policy.md` / `计划书/README.md` 的版本与互相矛盾引用。
- `SOP-运维手册.md`：Caddy 主上游健康检查实际用 `/healthz`；METRICS 生产已开启；
  删除对已删除 `计划书/workflow_status.md` 的当前时态引用。
- `黄金范例.md`、`SOP-新功能交付.md`、`ADR-架构决策记录.md`：对已删除文件的引用改为
  “历史证据，用 git 找回”。
- `CHANGELOG.md`：补录 v1.3.150 / v1.3.151。

## 回滚

本 manifest 不涉及删除；如需回滚文档修正，使用 `git revert` 对应提交即可。
