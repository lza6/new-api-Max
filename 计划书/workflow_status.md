# Workflow Status — 参考项目全量对标（任务闭环登记）

> 日期：2026-09-20 ｜ 主项目：new-api v1.2.38（Agent 网关）
> 目标：1191 个参考项目全量识别 → 优点提炼 → 差距分析 → 生成《参考的结果计划指南.md》

## 一、节点状态（扫描工作流）

| 节点 | 任务 | 状态 | 证据/产物 |
|---|---|---|---|
| N0 识别 | 主项目定位（Agent 网关：渠道聚合/计费/任务/日志透明/skills 沉淀/横向扩展） | ✅ DONE | AGENTS.md + 本指南 §1 |
| N1 全量盘点 | 参考根 1191 顶层目录；1177 项 README 首段索引 | ✅ DONE | `.deploy/refscan/all_projects_readme.tsv` |
| N2-N18 并行扫描 | 子代理 6 批（agents-a/b、api_gateway、skills、docs_rag、media）+ 主代理 11 批（image/ecommerce/mcp/automation/dev_tools/security/web_frontend/unclassified×4） | ✅ DONE | `.deploy/refscan/reports/report_<batch>.md`（18 份，263KB，含 missing-13 补扫） |
| N18 优点提炼 | 每批 Top5 + 共性亮点 + 扩展洞察 | ✅ DONE | 各 report_*.md §3/§4 |
| N19 差距分析 | 主项目 vs 参考最佳（10 项差距矩阵） | ✅ DONE | 本指南 §4 |
| N20 方案设计 | 六大可迁移方向 + 优先级 + 依赖关系 | ✅ DONE | 本指南 §3/§5 |
| N21 汇总 | 《参考的结果计划指南.md》生成 | ✅ DONE | `计划书/参考的结果计划指南.md` |
| N22 落地跟踪 | 后续批次登记（下一步改进指南已立项） | ⏳ 待实施：P0-1 已完成（见下） | `计划书/下一步改进指南.md` P0-P2 台账 |

## 二、关键结论（真实证据）
1. 参考集 = AI 需求侧全景：agent 平台/记忆/技能生态/媒体/电商/办公/PPT——印证主项目横向扩展路线。
2. 最高价值：MCP 通道、成本可视化、日志证据闭环、技能市场、电商视频/办公横扩展。
3. 边界：渗透/逆向/验证码对抗类仅防御研究，不进产品。

## 三、已完成 vs 待办
- ✅ 已完成：全量扫描（1191 目录）、17 份批次报告、《参考的结果计划指南.md》、《下一步改进指南.md》。
- ⏳ 待用户确认后：按《下一步改进指南.md》P0→P2 逐批实施（每个批次真实跑通、三库验证、留证据后登记到本文件）。

## 四、批次闭环状态
| 批次 | 状态 | 提交 | 证据 |
|---|---|---|---|
| P0-1 三库 conformance 契约测试套件 | ✅ DONE (v1.2.39) | 见 git log | `计划书/audit-ledger.md` + `model/db_conformance_test.go` + CI `db-conformance` job |
| P0-2 计费安全收口 | ✅ DONE (v1.2.40) | 见 git log | `计划书/audit-ledger.md` + `service/token_counter.go` + `quota_saturation_test.go` |
| P0-3 认证安全审计 | ✅ DONE (v1.2.40，审计结论：既有实现已满足关键 ASVS) | 见 git log | `计划书/audit-ledger.md` |
| P0-4 日志透明化（错误归因落日志） | ✅ DONE (v1.2.40) | 见 git log | `计划书/audit-ledger.md` + `controller/relay.go` + `relay_error_log_test.go` |
| P1-1 用户画像层 | ✅ DONE (v1.2.48：EXPLAIN 索引命中+占比一致性验收) | 见 audit-ledger | service/user_profile/ |`r`n| P1-2 Redis 批量落库 | ✅ DONE (v1.2.47/50：重试+回退+指标+并发/故障注入验收) | 见 audit-ledger | model/consume_log_flusher.go + test |`r`n| P1-3 SSE 断线续传 | ✅ DONE (v1.2.49/51：done 截断修复+500 条压测+权限+Last-Event-ID) | 见 audit-ledger | controller/task_event.go + 前端 task-event-stream |`r`n| P1-4 渠道健康+组合路由 | ✅ 已落地 | 见 audit-ledger | channel_health_score.go + channel_combo_route.go |`r`n| P2-2 事件子系统（最小核心） | ✅ DONE (v1.2.42) | 见 audit-ledger | service/event_bus.go + event_bus_test.go |`r`n| P2-1 无锁快照 / P2-3 jsplugin 沙箱 / P2-4 平台生态 | ⏳ 待办（大项独立批次） | - | - |

## 五、下一步（最小可行）
1. 推荐下一批：P0-2 计费安全收口 或 P0-4 日志透明化。
2. 从《下一步改进指南.md》取批次定义，按 03-工作流-SOP 推进，完成后在本文件登记提交 SHA 与证据路径。