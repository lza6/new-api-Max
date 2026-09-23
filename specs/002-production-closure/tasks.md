# Tasks: Production Closure & Final Audit

> 每个任务 = 独立原子单元，验收 = 真实命令输出 + 证据入库。按依赖顺序执行。

## Phase A: 已提交批次（基线确认）
- [x] A1 T2-2/T2-3 渠道健康路由 + stream_fallover 默认 on（commit 7d431417f）
- [x] A2 T3 状态页鉴权测试（commit db1a31d7d）
- [x] A3 T4 费用明细前端入口（commit c12465831）
- [x] A4 T5 时间线 span 化（commit 2457d7bf9）

## Phase B: 插件安全（T6-2/T6-4）
- [x] B1 插件沙箱审计与加固（宿主对象不可达 + import 编译阻断测试）
- [x] B2 marketplace URL 协议白名单（https only + 1MB + SHA-256 已覆盖 225 测试）
- [ ] B3 webhook/定时同步幂等（若已有基础则补测试，否则标注待用户优先级）

## Phase C: UX 与可达性（T7）
- [x] C1 交互反馈盘点表 `计划书/audit/ux-interaction-ledger.md`
- [x] C2 人话错误映射扩充（504/超时）（429/502/504/额度用尽/渠道无可用）+ 7 语言
- [ ] C3 a11y 关键页面测试（颜色对比/焦点/aria/表单 label）

## Phase D: 安全专项（T8）
- [x] D1 OWASP ASVS 审计报告 `计划书/audit/security-asvs-audit.md`
- [ ] D2 高危缺口修复 + 回归测试
- [ ] D3 审计事件去敏回归测试

## Phase E: 前端工程化（T9）
- [x] E1 包体积基线报告 `计划书/audit/bundle-size.md`
- [ ] E2 入口懒加载瘦身（高权重路由 React.lazy）
- [ ] E3 组件复用审计 + knip 死代码

## Phase F: 数据库（T10）
- [x] F1 三库矩阵真实通过（MySQL 9.6.0 + PG 16.14 本机实例）
- [x] F2 迁移幂等 + logs 索引测试锁

## Phase G: 文档治理（T12/T13/T14）
- [x] G1 术语表补充（计费/限速/任务/Web 防护）
- [ ] G2 `计划书/db_structure.md` + `计划书/project_specs.md`
- [x] G3 合规 checklist（备案/算法备案/实名/日志/内容安全）
- [ ] G4 验证台账更新（防重复跑）
- [ ] G5 `graft build` 刷新

## Phase H: 交付（用户授权后）
- [x] H1 11 commits + push origin main（SHA 核对一致）
- [x] H2 v1.3.13 tag + Release（7 产物）
- [ ] H3 线上部署验收（需用户提供服务器授权）
- [ ] H4 回滚路径记录（SOP 已存在，更新版本）

## 验收矩阵
| 任务 | 验证命令 | 证据 |
|------|---------|------|
| B1/B2 | go test ./pkg/jsplugin/... ./controller/... | 恶意脚本/非法 URL 用例 |
| C1/C2/C3 | bunx vitest run src/features/... src/i18n | 测试输出 |
| D1-D3 | go test ./middleware/... ./controller/... + 审计报告 | security-asvs-audit.md |
| E1-E3 | bun run build + bunx knip | bundle-size.md |
| F1/F2 | powershell -File scripts/db-conformance.ps1（或服务器跑法） | 三库矩阵记录 |
| G1-G5 | bun run i18n:sync + graft build | 文档存在且非空 |
| H1-H4 | git push + gh release | 远端 SHA/Release 链接 |