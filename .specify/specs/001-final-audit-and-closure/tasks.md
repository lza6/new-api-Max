# Tasks：终局审计闭环

## Phase A：盘点审计（只读，并行）
- [ ] A1 [P] 功能闭环矩阵（backlog → 代码/证据 → 状态）→ 计划书/audit/closure-matrix.md
- [ ] A2 [P] SQL/DB 审计（慢查询模式、缺索引、三库兼容抽查、注入/锁风险）→ 计划书/audit/sql-db.md
- [ ] A3 [P] 前后端契约与错误人话审计（关键端点抽查、映射覆盖、契约防坑）→ 计划书/audit/contract-errors.md
- [ ] A4 [P] 稳定/安全/可观测性审计（限流、并发上限、超时熔断、SSRF/web 防护、429 退避、健康检查、指标）→ 计划书/audit/resilience-observability.md
- [ ] A5 汇总状态矩阵 → 更新 workflow_status.md；产出 P0/P1/P2 分级缺陷清单

## Phase B：P0/P1 修复
- [ ] B1 登录会话数量上限设置（admin 可开关，默认关；每用户设备数策略）
- [ ] B2 模型测试 tab 入口可达（导航/首页入口）
- [ ] B3 日志列表一键封禁 IP UI（root 权限、封禁/解封、审计留痕）
- [ ] B4 签到奖励额度设置 "Invalid input" 修复
- [ ] B5 审计发现的其他 P0/P1 缺陷修复

## Phase C：P1/P2 补位
- [ ] C1 错误人话映射全覆盖（冷却/额度/模型不可用/上游限流/IP 封禁）+ 契约防坑测试
- [ ] C2 慢查询猎杀 + 索引优化（logs/tasks/usage 聚合路径；三库验证）
- [ ] C3 计费安全不变量回归（quota 测试全绿；影子价不改计费；预扣/结算链路抽查）
- [ ] C4 前端质量门（tsgo/build/vitest/i18n；存量失败基线对照 + 排期）

## Phase D：文档/运维
- [ ] D1 SOP：部署/升级/回滚/排障/备份（写入 计划书/SOP-*）
- [ ] D2 验证账本 计划书/audit-ledger.md（记录已跑测试/基准/慢查询范围，防重复）
- [ ] D3 可观测性说明：现有指标端点/日志/健康检查盘点与缺口建议

## Phase E：总验收与交付
- [ ] E1 全部门禁通过 + 证据齐全（audit/ 文档 + e2e-evidence/ JSON）
- [ ] E2 主题 commit → push → tag v1.2.36 → GitHub Release
- [ ] E3 生产部署（备份 compose + 回滚命令）+ 线上验收证据
