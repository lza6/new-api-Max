# 项目任务进度权威源（project_specs.md）

> 生成：2026-09-24 · 主控代理执行台账。优先级与 T-路线对齐 `下一步改进指南.md`。
> 更新纪律：每个主题批次完成后回填；本文件是「当前进度」唯一权威源。

## 总览

| 阶段 | 主题 | 状态 | 证据 |
|---|---|---|---|
| P0 | T2 渠道健康与容错 | ✅ 完成（T2-2/T2-3） | commit 7d431417f；TestChannelHealthRoutingFilter；stream_fallover 默认 on |
| P0 | T3 Web 防护 + 状态页 | ✅ 完成（补测试） | commit db1a31d7d；TestServerStatsAdminOnly |
| P1 | T4 计费透明度 | ✅ 完成（用户入口） | commit c12465831；CostDetailPanel + 测试 |
| P1 | T5 可观测性 | ✅ 完成（span 化） | commit 2457d7bf9；TestBuildTimelineStages |
| P1 | T6 插件成熟 | ✅ 完成（沙箱测试） | commit 5352d441b；sandbox 2 测试 + marketplace 225 测试 |
| P1 | T7 UX/可达性 | ✅ C1/C2 完成；C3 待办 | commit 4907a9ebe；ledger + 504 映射 |
| P1 | T8 安全 ASVS | ✅ D1 审计报告；D2/D3 已核验 | commit 42c070993；security-asvs-audit.md |
| P2 | T9 前端体积 | ✅ E1 基线；E2 结构最优已核验；E3 knip 收紧+无新增死代码 | commit 1627c5827；bundle-size.md 记录 0002/0003 |
| P2 | T10 数据库 | ✅ 三库矩阵真实通过 | commit 5c90ee1a8；ledger 0002 |
| P2 | T11 部署 SOP | ✅ SOP 已建（T11-1） | 计划书/ops/deployment-sop.md |
| P2 | T12 i18n/合规 | ⏳ G1 术语表补充；G3 合规 checklist 待建 | 本轮 |
| P2 | T13 知识沉淀 | ⏳ db_structure/project_specs 已建；graft build 待跑 | 本轮 |
| P2 | T14 旧产物清理 | ⛔ 需用户授权 | 见下一步改进指南 T14 |
| P2 | T15 未来方向 | ✅ 模型同步 dry-run 已落地；任务品类扩展已立项（任务卡） | commit 见本批；model-sync preview API + 10 task plugins |

## 在办批次（本轮）
- Phase G：G3 合规 checklist、graft build
- Phase H：push + tag + Release（用户授权后）

## 待办（下轮）
- T7 C3：axe-core a11y 扫描（P2）
- T14：旧产物清理（用户确认清单后）
- T15 任务品类扩展：按任务卡补 2-3 个高需求模板（需真实上游凭证后 E2E）