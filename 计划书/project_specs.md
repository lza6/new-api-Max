# 项目任务进度权威源（project_specs.md）

> 重建：2026-09-25 · 从 git HEAD（a0d0589f5, v1.3.28）恢复，进度以 `specs/002-production-closure/tasks.md` 为准核对回填。
> 更新纪律：每个主题批次完成后回填；本文件是「当前进度」唯一权威源。未验证项一律标注「待验证」。

## 总览

| 阶段 | 主题 | 状态 | 证据 |
|---|---|---|---|
| P0 | T1 热路径性能冲刺 | ⏳ 台账 0001-0010 已存在；10ms 目标未达成（0004 本地 SQLite overhead p50=22.64ms；待授权重建基准） | perf-verification-ledger.md 0001-0010（0004 基准 + 0010 三库矩阵复验） |
| P0 | T2 渠道健康与容错 | ✅ 路由/前端徽章/参数化/聚合概览全部落地（v1.3.30/31） | commit 7d431417f；channel_health_setting.go；channel-health-overview-card.tsx；perf-ledger 0011；audit/channel-health-status.md |
| P0 | T3 Web 防护 + 状态页 | ✅ 配置化+状态页已落地；❌ UA/地域/白名单缺口 | commit db1a31d7d；TestServerStatsAdminOnly；audit/web-protection-status.md |
| P1 | T4 计费透明度 | ✅ 完成（用户入口） | commit c12465831；CostDetailPanel + 测试 |
| P1 | T5 可观测性 | ✅ 已收尾（request-id 生成/日志/时间线 span/上游透传+回包 id 记录，4bff5ebab） | commit 4bff5ebab；relay/channel/api_request.go:56-58/606-607；relay/channel/api_request_test.go:239-351；middleware/audit_sensitive_test.go |
| P1 | T6 插件成熟 | ✅ 完成（沙箱测试） | commit 5352d441b；sandbox 2 测试 + marketplace 225 测试；B3 待用户优先级 |
| P1 | T7 UX/可达性 | ✅ C1/C2 落地；✅ C3 a11y 冒烟已落地；❌ 移动端断点截图待补 | commit 4907a9ebe；a11y-smoke.test.tsx；ux-interaction-ledger.md |
| P1 | T8 安全 ASVS | ✅ D1 审计报告；D2/D3 已核验（specs 标注） | commit 42c070993；security-asvs-audit.md |
| P2 | T9 前端体积 | ✅ E1 基线；❌ E2/E3 待办（specs Phase E 未勾选） | bundle-size.md 记录 0002/0003（基线 59,352 kB JS） |
| P2 | T10 数据库 | ✅ 三库矩阵真实通过（0002 + 本次 0010 复验） | ledger 0002/0010；MySQL 9.6.0 + PG 16.14 本机实例 |
| P2 | T11 部署 SOP | ✅ SOP v1.3.28 已更新（含 blue-green 衔接） | 计划书/ops/deployment-sop.md |
| P2 | T12 i18n/合规 | ✅ G1 术语表；✅ G3 合规 checklist | ops/ai-compliance-checklist.md |
| P2 | T13 知识沉淀 | ✅ project_specs/db_structure/README 恢复；audit 两篇新增 | 本轮批量 |
| P2 | T14 旧产物清理 | ⛔ 需用户授权 | 见下一步改进指南 T14 |
| P2 | T15 未来方向 | ✅ 模型同步 dry-run 已落地；任务品类扩展已立项（待上游凭证） | ops/t15-task-cards.md |

## P2 批次六任务（v1.3.42，2026-09-27）

| 任务 | 状态 | 证据 |
|---|---|---|
| T9 前端体积 | ✅ 基线刷新 v1.3.41 + knip 复验 + 性能预算测试 | bundle-size.md（记录 0004）+ 预算测试 |
| T10 数据库 | ✅ CI 三库矩阵 job + 幂等脚本 + EXPLAIN 复查 + 日志归档开关 | .github/workflows + db-conformance.ps1 + perf-ledger |
| T11 部署 SOP | ✅ 503 事故教训入库（Caddy 1s/1/1 + banned_ips 封 172.18.0.1 根因）+ Dockerfile 瘦身 + CI 校验 | ops/deployment-sop.md + Dockerfile |
| T12 i18n/合规 | ✅ 术语表补计费/限速/任务插件/Web防护 + 键集测试 + 合规 AI 标识 | docs/translation-glossary.md + locale-consistency.test.ts |
| T13 知识沉淀 | ✅ db_structure 回填 S7 索引 + project_specs 回填 + graft build | db_structure.md + graft/ |
| T14 旧产物清理 | ⛔ 清单已列，等用户逐项批准 | 见 T14 清单 |

> v1.3.41（2026-09-26）已闭环：生产 503 根因=Web 防护封禁 Docker 网关 172.18.0.1（auto:web_rate_limit）→ Caddy 健康检查 429 → 双上游摘流；已删封禁 + 升级 v1.3.41 + Caddyfile 加固（3s/2s/3/2）。线上验收：/api/status 200 + X-New-Api-Version: v1.3.41 + 20/20 健康检查全绿。

## 在办批次（本轮 · 2026-09-25）
- T13 文档治理：project_specs.md / db_structure.md / README.md 恢复；perf-ledger 追加记录 0010；T3/T2 审计两篇。
- T11 SOP 更新 v1.3.28 + rolling-update-newapi-v3.sh 衔接（a0d0589f5）。
- 本轮（2026-09-25~26）：T1 热路径（v1.3.29）+ T2-1 参数化（v1.3.30）+ T2-2 概览卡（v1.3.31）+ 终局闭环审计（P3 前端加固 + P2 latencyFactor NaN/combo min_score）。全部真实验证 + 推送 + Release。

## 待办（下轮）
- T2-1 健康分策略参数化（窗口/权重/阈值进 operation_setting）
- T2-2 前端健康分聚合概览卡（均值/最差渠道/近期可用率）
- T3 策略维度补全（UA/路径/地域/白名单）
- T5 多实例全局 trace（可选增强）
- T14 旧产物清理（用户确认清单后）
- H3 线上部署验收、H4 回滚演练更新（需授权）

