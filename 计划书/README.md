# 计划书（项目规划文档库）

> 本目录是 new-api-Max 项目**规划/审计/证据**的唯一存放点。代码改动前先在这里建 Spec/任务卡，改完回填证据。
> 重建：2026-09-25（T13 文档治理）· 目录索引自 git HEAD 恢复并补充本轮新增审计报告。

## 目录结构（当前实际）
```
计划书/
  README.md                    # 本文件（目录规范）
  project_specs.md             # 任务/进度权威源（T13，以 specs/002-production-closure/tasks.md 为准）
  db_structure.md              # 表/索引/约束权威源（T13；schema 变更随改随更 + 三库矩阵）
  下一步改进指南.md            # 主迭代指南（T1-T15 路线；含完成记录）
  ops/deployment-sop.md        # 部署/回滚 SOP（T11-1，v1.3.28 + blue-green 衔接）
  ops/ai-compliance-checklist.md    # 合规 checklist（G3）
  ops/t15-task-cards.md        # T15 立项任务卡
  ops/t15-decision-record.md   # T15 决策记录
  docs/                         # 专项分析/头脑风暴/方案深挖文档（T1-T15 主题，见 docs/README.md）
  audit/                        # 只读审计报告
    perf-verification-ledger.md    # 性能/三库验证台账（记录 0001-0010+）
    security-asvs-recheck-raw.md   # OWASP ASVS 复核原始记录（D1；取代旧 security-asvs-audit.md）
    ux-interaction-ledger.md       # UX 交互反馈盘点（C1）
    bundle-size.md                 # 前端包体积基线（E1）
    web-protection-status.md       # T3 Web 防护状态审计
    channel-health-status.md       # T2 健康分概览审计
  e2e-evidence/                # 真实运行证据（JSON/pprof/日志片段）
  archive/                     # 过期文档归档（授权后移动，不删除）
  task-cards/                  # 单批任务卡（也可用 .specify/specs/NNN-* 承载）
```

## 命名规范
- 文档：`NNN-主题简短名.md`（NNN 递增，短横线分隔）。
- 审计：`计划书/audit/<域>-<主题>.md`；证据带 `文件:行号`。
- 证据：`计划书/e2e-evidence/<类型>-<版本/日期>.{json,png,md}`。

## 更新纪律
1. 每批改动：先建/更新 Spec（推荐 `.specify/specs/NNN-*`）+ 任务卡 → 实现 → 回填证据。
2. 完成后回填：`workflow_status.md`（根）追加本批记录；更新 `audit/perf-verification-ledger.md`（已验证项，避免重复跑）。
3. schema 变化必回填 `db_structure.md`；大代码批次后跑 `graft build` 刷新知识图。
4. 不把临时草稿/大文件（exe/db/日志）放本目录。
5. git 恢复纪律：HEAD 中已删除的规划文档属于历史交付资产，恢复采用「保留证据 + 标注基线」方式；被工作树本轮改动覆盖的
   文件（如 下一步改进指南.md）直接就地更新，不强制回退用户内容。

