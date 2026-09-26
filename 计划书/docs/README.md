# 计划书/docs/ 子目录索引（专项分析/头脑风暴文档）

> 用途：存放「技术点/技术栈专项分析、方案选型、根因分析、头脑风暴、深挖文档」等**扩展文档**。
> 主执行蓝本始终是上一级《下一步改进指南.md》；本目录文档按主题引用，不替代任务卡与验证台账。
> 纪律：先读主指南 → 需要深挖某主题时按需打开本目录对应文档 → 落地后回填证据，不遗留孤本方案。

## 现有文档（按主题）
- **基线快照/待办**
  - `2026-09-25-head-v1.3.29-待办.md` — 当前 HEAD（aced9a37d，VERSION 文件仍为 v1.3.28）基线快照、T1–T15 状态核对、下一批建议。
- **T1 性能**：`t1-perf-deepdive.md` — 热路径根因、基准策略（BLOCKED 诚实记录）、优化候选。
- **T2/T3 健康分与 Web 防护**：`t2-t3-gap-options.md` — 参数化/策略维度最小方案选型。
- **T4 计费/配额安全**：`t4-billing-quota-safety.md` — billingexpr 导读 + 安全不变量 + 落地锚点。
- **T5 可观测性**：`t5-observability-deepdive.md` — request-id 全链路现状/剩余缺口（透传已落地）。
- **T6 插件/沙箱/市场**：`t6-plugin-market-safety.md` — 缺口与 webhook/定时同步评估。
- **T7 UX/a11y**：`t7-ux-a11y-path.md` — 盘点表、axe 用例、移动端截图、错误映射路径。
- **T8 安全 ASVS**：`t8-security-asvs-deepdive.md` — 硬规则、D3 去敏现状、D2 缺口路径。
- **T9 前端体积**：`t9-bundle-engineering.md` — 基线、懒加载、复用审计、性能预算。
- **T10 三库矩阵**：`t10-db-matrix-deepdive.md` — 硬规则、矩阵方法、常见坑。
- **T11 部署/运维**：`t11-deploy-ops-deepdive.md` — SOP、blue-green、HEALTHCHECK、备份演练。
- **T12 i18n/合规**：`t12-i18n-compliance.md` — 术语表、一致性扫描、合规提示路径。
- **T14 旧产物清理**：`t14-cleanup-ledger.md` — 清理候选清单（git status 实测）+ 纪律 + 确认流程。
- **T15 未来方向**：`t15-roadmap-brainstorm.md` — 订阅统计/webhook/公开定价方案纵深。

## 命名规范
- 主题文档：`t<主题号>-<简短英文名>.md`；临时快照：`YYYY-MM-DD-<说明>.md`。
- 只读整理一律在头部标注「生成日期 · 未改业务代码」；落地后回填证据并更新本索引。
