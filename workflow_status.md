# workflow_status.md — 终局闭环审计（2026-10-02）

## 任务契约
对 v1.3.68–v1.3.71（首字延迟治理 + 实时可观测性 + 延迟拆解）做终局闭环审计：
需求完整性 / 逻辑正确性 / 边界 / 代码质量 / 测试覆盖 / 真实运行结果。

## 本批改动
| 版本 | 改动 | 生产 |
|---|---|---|
| v1.3.68 | 出站请求体 gzip 压缩（渠道 opt-in） | ✅ 已部署 |
| v1.3.69 | 实时请求详情面板 + 压缩阈值 256KB | ✅ 已部署 |
| v1.3.70 | 面板延迟拆解（httptrace） | ✅ 已部署 |
| v1.3.71 | 消费日志记录延迟拆解 | ✅ 已部署 |

## 已确认结论
- **400 `input[N].call_id` = 上游问题**（非本网关）。直连上游空 tool_call id 复现完全相同的 400。
- **首字延迟 80%+ 在上游 prefill**，非上传带宽（上游 800KB→21.8s；2MB 上传仅 0.36s）。

## 自我审查修复（已提交）
| # | 缺陷 | 级别 | 状态 |
|---|---|---|---|
| 1 | httptrace emit 锁外读写（HTTP/2 data race） | P1 | ✅ 修复 |
| 2 | bodySize 对非可回放 body 返回 0（面板 0B/Infinity） | P1 | ✅ 修复 |
| 3 | started_at 秒级截断（elapsed 误差 999ms） | P2 | ✅ 修复 |
| 4 | LivePhaseWaiting 半实现（永不驱动） | P2 | ✅ 移除 |
| 5 | 聚合计时无数据返回 0（误显示 0ms） | P1 | ✅ 修复 |
| 6 | 失败行无 error_msg 展示 | P2 | ✅ 修复 |
| 7 | 压缩阈值 env=0/负 短路（footgun） | P2 | ✅ 修复 |
| 8 | withUpstreamTimingTrace 未用参数 | P3 | ✅ 修复 |
| 9 | ⚡ 无 aria-label（a11y） | P3 | ✅ 修复 |

## 验证日志
- [x] go build ./... + go vet（relay/service/controller）
- [x] relay/common 全量 + service TestLiveRequestTracker（含 race -count=3）
- [x] 前端 typecheck + lint + build + system-info 测试 3/3 + timeline 15/15
- [x] 生产 v1.3.71 healthy，外网 15/15=200，500=0
- [x] 生产延迟拆解日志 11/11 覆盖

## 交付物
- [x] HTML 变更报告（含测验）：`计划书/reports/change-report-latency-observability.html`
- [x] skills：`.claude/skills/new-api-add-feature/SKILL.md`（已有，覆盖完善）
- [x] 记忆台账更新（验证 ledger + 生产部署）
- [ ] 部署本轮修复（v1.3.72）

## 下一步
1. 部署本轮审计修复
2. 完成剩余交付物
