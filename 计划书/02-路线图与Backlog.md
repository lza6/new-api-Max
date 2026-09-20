# 02-路线图与 Backlog（候选，未承诺）

> 以下 P1/P2 候选来自旧版规划草案/审计（2026-09-19/20 识别），**尚未逐项立项**。
> 落地前必须按 03-工作流-SOP 走：头脑风暴 → 方案 md（含验收标准）→ TDD → 三库验证 → 证据 → 提交。

## P1 候选（按价值排序）
| 项 | 一句话方案 | 预估 |
|---|---|---|
| 用户画像层（规则版） | 按 user 聚合 logs → 模型偏好/成本习惯/时段/错误率；零 LLM | 后端 1 包 + 前端 1 feature |
| 三库 conformance 契约测试套件 | 把 AGENTS.md 三库人肉验证写成 Go 契约测试进 CI | ~1 周 |
| Redis 队列批量落库 | 日志增量入 Redis 队列，周期批量 flush，按天分表 | ~1 周 |
| 计费现代化（影子价/分段明细） | 微美元影子价 + tiered breakdown（v1.2.28 已做 stats 层，计价改造待定） | 需全量计费回归 |
| 凸组合因子路由 | 以 health/latency/cost/cacheAffinity/quality/consistency 替代简单权重 | ~1.5 周 |
| 无锁配置快照 | 渠道/定价/限流读路径 atomic.Pointer[Snapshot] | ~4 天 |
| Agent 协议 P0 五项核对 | thinking 签名往返/cache_control 计量/beta 头白名单/usage 分块/FC 配对 | 核对 2 天+修复 |
| 存量 JSON 规范收敛 | 逐文件收敛到 common.*（common/json.go 约束） | 穿插 |

## v1.3 战略方向（P2，另行细化）
- 技能市场（SKILL.md 托管 + 安全扫描 + 运行时沙箱 + 网关计费）
- Skill 进化闭环（relay 层拦截→会话摘要→聚合→发布门禁）
- 通用 webhook/事件子系统（5 个支付 webhook 归一 + 幂等重放）
- jsplugin 安全模型升级（ExecutionGate + 分层权限 + Ed25519 签名 + 沙箱降级链）
- CUA/沙箱增值（凭据保险库 + 流量令牌 + AutoPause）
- 垂直任务模板（Deep Research / 数学建模 / 生图可靠性层）
- /v1/pricing 稳定镜像端点（成定价源）

## 明确不做（边界）
- 9router 式 MITM 拦截；nexus 60+ 策略枚举；aisix etcd 配置面；litellm Prisma 数据层。
- AGPL/GPL 传染许可项目；医疗/法律/交易下单垂直（牌照问题）。