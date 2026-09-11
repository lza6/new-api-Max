# N20 侦察报告：多 Agent 协作编排（已完成，20 项）

## 编排模式对比表

| 模式 | 代表 | 适用场景 | 协调成本 | 失败语义 |
|---|---|---|---|---|
| 主从星型 | claude-vibe-squad、agent-swarm、hive、AO | 可分解并行任务、单一决策口 | 中（协调者上下文瓶颈） | 失败→重派/升级模型；协调者单点 |
| 流水线 | OpenSwarm pairPipeline、three-man-team、plan-cascade、fable、BAD | 质量敏感顺序加工 | 低-中（静态文件/合同接口） | REVISE 循环+stuck 阈值熔断；每步可独立重放 |
| 依赖 DAG 任务图 | dsh-agent-teams、plan-cascade、BAD | 任务有依赖、要并行与审计 | 中（状态机+锁+信箱） | 失败阻断下游、自动 repair-N/review-N+1、冷恢复、attempt 撤销 |
| 对等雇佣 | ccteam、agent-teams-ai | 跨厂商混编、人随时介入 | 低（协议极薄 6 工具） | 幂等键重放、guard never kill、按 id 恢复 |
| 治理账本 | vnx-orchestration、ccteam flows | 合规/成本敏感长期运行 | 高（回执 schema+验证链） | 回执拒判（有 PR 才算 done）、gate GO/HOLD |
| 纯方法论/文件态 | three-man-team、APM、planning-with-files | 小团队低成本、抗上下文丢失 | 极低（零基建） | 依赖守约；hash 认证防篡改 |

## 关键项目亮点

### dsh-agent-teams ★ 质量门禁机器可判定
- 任务带合同（acceptance/verify/changed_paths）；review 失败自动生成 repair-N/review-N+1；"共识"=六条布尔条件（所有门禁过+acceptance 过+无 blocker/high+独立 Reviewer 审过+验证命令通过+范围合法）
- attempt_id 租约解决迟到写入/僵尸认领；先出 DAG 草案 Approve & Run 才落地
- **Builder/Critic/Evaluator 闭环的机器可判定版本**

### agent-teams-ai ★ 计费数据模型最强
- TokenUsageSourceKind = sdk_exact|gateway_exact|log_parsed|tokenizer_estimated|cost_estimated（来源精度分级）
- billingMode = api|subscription|free；三路成本拆分 billableUsd/apiEquivalentUsd/estimatedUsd
- 预算 monthly token/美元上限+80%/100% 双阈值告警；rate-limit 后自动 nudge 续跑
- **new-api 现有 quota 体系正好是 gateway_exact 源，订阅型渠道补 apiEquivalentUsd 影子价**

### ccteam ★ 与 new-api 气质最接近
- 仅 6 个 MCP 工具（菜单不是手册——每字节收上下文费，失败语义放错误消息）
- policy hook：每次委派重读，stdin 收调用者+请求+配额表，exit 0/2——限流从网关黑盒变成用户可编程守门员
- --max-cost per-flow 预算+per-vendor 日上限（"guard never kill"——只挡新委派不杀在跑）；idempotency_key 重放防双扣；诚实信号（未上报读作 unknown 不读作 0）
- 评分：4/5/4/5

### 其他
- agent-orchestrator：durable 最小事实+读时派生状态（显示状态永不落库）；计价目录 sha256 锚定同步 litellm 价格表——与 billingexpr versioning 互补。4/5/5/4
- OpenSwarm：StuckDetector 量化阈值（同输出重复 3/同错误 2/REVISE 循环 4/独白 6）；角色级模型升降级链（haiku→sonnet，iteration>3 升级）。3/4/4/4
- claude-vibe-squad：TSV 路由表（角色→模型档位→评审反亲和"不让模型给自己盖章"→升级策略）——编排路由即计费路由的数据模型。3/4/3/4
- vnx-orchestration：NDJSON 回执+哈希链 append-only（审计即计费）；token_usage 从 transcript 实测收割。2/4/3/3
- plan-cascade："AI Verify 门"专查 stub/TODO——反伪实现机器化；批内 gate 缓存+增量门禁。2/4/4/3
- planning-with-files：三文件+hook 注入+SHA-256 计划认证（篡改即 [PLAN TAMPERED] 拒注入）+stop gate block cap 防死锁。2/4/5/4
- BAD：协调者零文件接触硬纪律；5h/7d 订阅限流阈值自动暂停-续跑。2/3/3/4
- three-man-team/fable/hive/APM/solo-orchestrator/awesome-agent-orchestrators：方法论/极简形态参考

## 最适合 new-api 借鉴的 2 种模式
1. **对等雇佣+账本护栏（ccteam）**：daemon 薄协议+每调用者独立身份→独立账本行+幂等键+policy hook 可编程配额脚本+预算 guard never kill
2. **依赖 DAG+机器可判定质量门禁（dsh-agent-teams）**：任务合同 schema+repair 链+共识六条件+attempt 租约；vibe-squad TSV 路由表给"编排路由即计费路由"数据模型

## 计费与限流专项 7 条
1. 来源精度五级分级（gateway_exact 是 new-api 天然强项）
2. 计费模式三分 api/subscription/free；订阅按 apiEquivalentUsd 影子价折算
3. 三路成本拆分 billableUsd/apiEquivalentUsd/estimatedUsd
4. 预算硬顶+双阈值告警；"guard never kill"语义
5. 每回执哈希链 append-only；token 实测收割优于估算
6. 计价目录 sha256 锚定同步（与 billingexpr versioning 互补）
7. 限流自动化：5h/7d 阈值暂停-续跑、滚动窗口+指数退避、rate-limit 冷却后 nudge
