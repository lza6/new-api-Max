# N8 侦察报告：Agent/LLM 可观测性（已完成）

17 项目报告。参考目录中无独立 langfuse/phoenix（仅 hermes-trace、trace-mcp 命中）。Agent-Reach 两份/ headroom 两份均为同一项目。

## 关键项目

### Claude-Code-Agent-Monitor（node|large）★ 本组最重要
- 实时监控平台（Node/Express/React/SQLite + WebSocket）：session 状态机、hook 事件流、tool call、subagent 树、token/成本；四列看板 hover Waiting 徽标显示"为什么等"
- Timeline 按 tool_use_id 配对 Pre/PostToolUse 事件，per-tool payload 渲染器（server/db.js:333）
- **四维定价 bucket**：(model, speed, inference_geo, service_tier)（server/lib/token-usage.js:1-90）
- hook 事件入口"绝不阻塞 agent"：静默失败+超时（ARCHITECTURE.md）
- 评分：业务 5 / 架构 4 / 工程 4 / 迁移 4

### agent-inspect（node|large）★
- 本地证据层：一次 JSONL trace 三用途（调试执行树/CI 确定性轨迹检查/Evidence v2 分享）
- explain.ts 拆分 ExplainFact（confidence:"observed"）与 ExplainInference（"deterministic"）——事实与推理分离的解释性 UI 原语
- causal-failure.ts 保守首因故障分析：四类（explicit_error/failed_outcome/contract_failure/failed_ancestor），绝不猜
- check 确定性 trajectory gate 可进 CI；Evidence v2 写时脱敏+哈希清单+离线 verify
- 评分：业务 3 / 架构 5 / 工程 5 / 迁移 4

### agent-deck-main（go|large）★ 成本子系统最完整（internal/costs/ 22 文件）
- 四层管道：Parser（每上游一个）→ Collector → Pricer（override>本地缓存>硬编码三级 fallback，LiteLLM 数据+过期）→ Store；**微美元 int64** 计价
- BudgetChecker 五级预算（日/周/月/组/会话）**与成本 INSERT 同事务**评估，Warn/Stop 动作带 Reason（budget.go:33-90）
- costs sync 从既有 transcript 回填历史
- 评分：业务 4 / 架构 5 / 工程 5 / 迁移 4

### hermes-trace（py|medium）★ 小而美
- 单文件 900 行：Session→Turns→Spans（llm_call/tool_call/subagent）；18 个 lifecycle hooks 自动埋点
- 五格式输出：JSON/Mermaid/文本树/ASCII Gantt/零依赖 D3 HTML 甘特（tracer.py:650-690）
- 子代理 trace parent_trace 双向链接；LLM 调用编号跨压缩单调递增；hooks 全 try/except
- 评分：业务 3 / 架构 4 / 工程 4 / 迁移 5

### 其他
- hyperdx：ClickHouse 观测平台（"Kibana for CH"），属性搜索语法 level:err、live tail、OTel；抄交互范式不必引入本体。2/5/5/3
- claude-hud-main：statusline 常显；cost.ts 定价正则+cache 1.25x/读 0.1x 倍率；native/estimate 双来源标注。2/3/4/3
- agenttrail：Declared vs Observed 双信号对照（PLAN.md 声明 vs fs watcher 观察），不一致即告警。2/4/3/4
- llmquota：多 CLI 配额聚合 TUI；providers/ 每家一个采集器；ring bus 无 daemon 跨 CLI 通信。3/3/4/3
- ai-maestro：35→80+ agent 的 peer mesh OS；terminal 与 streaming 是同一持久实体两种执行模式（--resume 验证）。2/4/4/3
- omnara（go）★：durable agents（Postgres 原子提交+崩溃恢复）；Interactions 统一表单 {title, context(label/value), questions}——审批证据 UI 契约；blackbox 黑盒测试套件。3/5/5/4
- mission-control：自托管 agent 控制平面；"Logs show what ran. A completion receipt shows what finished"。3/4/3/3
- headroom（rust）：上下文压缩层；节省美元化（Proxy $ Saved）；CCR 可逆压缩（原文本地缓存）。2/4/4/3
- Claude-Code-Everything-You-Need-to-Know：按读者路径分层+每概念 When to use/Skip 对照。1/1/3/4
- trace-mcp：名不符实（是内容索引 MCP）；文档纪律（设计值引用代码文件）可借鉴。1/2/4/2
- Agent-Reach：doctor/probe 接入体检模式（渠道添加后自动测试）。1/2/3/2

## 汇总：LLM 网关日志透明化 Top 设计清单（按小白友好度排序）

1. **解释性日志卡片：Facts + Inferences 分离**（agent-inspect）——观察事实（token/候选渠道/健康度）与确定性推理（"未选 B：其上一分钟 429"）分离展示，置信度显式标注；失败给首因而非堆栈
2. **成本归因四维 bucket + 微美元整数账本**（Agent-Monitor + agent-deck）——(模型×速度×地域×层级) 分桶，价格三级 fallback，五级预算与计费同事务、Warn/Stop
3. **请求 Timeline 按请求 ID 配对**（Agent-Monitor）——路由决策→上游调用→重试→计费→响应垂直时间线，每步可点 payload
4. **单文件甘特/时间线导出**（hermes-trace）——零依赖 HTML 甘特/Mermaid，小白贴工单
5. **Declared vs Observed 对照高亮**（agenttrail）——声明路由 vs 实际命中分叉即亮
6. **状态看板 hover-why**（Agent-Monitor + agent-deck）——状态带原因（"连续 3 次超时，自动禁用 5 分钟"）
7. **审批/交互统一证据表单**（omnara）——{title, context label/value 证据, options}，Deny 带理由回灌
8. **节省美元化**（headroom + claude-hud）——"本次帮你省了 $X"；估算值与官方账单值区分标注
9. **脱敏证据包**（agent-inspect Evidence v2）——一键导出脱敏+哈希 bundle 可离线 verify
10. **帮助文档按读者路径分层**（CC-Everything）——每功能页配 When to use/Skip；术语表解释"缓存写为何比读贵 12.5 倍"
11. **OTel 兼容导出逃生舱**（hyperdx）——不锁死自带 UI
12. **活性看门狗**（Agent-Monitor 15s reap + Agent-Reach doctor）——渠道健康自动探测+添加后体检

关键证据：
- Claude-Code-Agent-Monitor\server\lib\token-usage.js
- agent-inspect\packages\core\src\explain.ts、causal-failure.ts
- agent-deck-main\internal\costs\{pricing.go,budget.go,collector.go}
- hermes-trace\hermes_trace\tracer.py
- omnara\docs\events\interactions.mdx
- claude-hud-main\src\cost.ts
