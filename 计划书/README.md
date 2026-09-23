# 计划书（项目规划文档库）

> 本目录是 new-api-Max 项目**规划/审计/证据**的唯一存放点。代码改动前先在这里建 Spec/任务卡，改完回填证据。

## 目录结构（约定）
```
计划书/
  README.md                    # 本文件（目录规范）
  project_specs.md             # 任务/进度权威源（T13-3 待建）
  db_structure.md              # 表/索引/约束权威源（T13-2 待建）
  下一步改进指南.md            # 主迭代指南（T1-T15 路线）
  ops/deployment-sop.md        # 部署/回滚 SOP（T11-1 已建）
  audit/                       # 只读审计报告（security-asvs / ux-interaction / bundle-size / perf-ledger）
  e2e-evidence/                # 真实运行证据（JSON/截图/日志片段）
  archive/                     # 过期文档归档
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