# N5 侦察报告：Agent 平台与 harness（已完成，15 项目）

## 关键项目

### agent-framework-go（MS Agent Framework Go 版）— 语言同构最佳参考
- RunFunc 全链路 iter.Seq2[*ResponseUpdate, error] 统一流式协议；中间件编译进调用链（compileRunChain 用 slices.Backward）
- 类型化 Option（独立私有类型+MAFValue 接口）防同名冲突
- continuation.go 流式 continuation token（中断续流携带已累积 ResponseUpdate）
- compaction 包：slidingwindow/truncation/summarization/toolresult 驱逐+Trigger 谓词函数组合
- harness 中间件族：loop（evaluator 决定续跑）、toolautocall、toolapproval、todo
- 136 测试文件；CI 多版本矩阵+-race -shuffle=on；workflow/checkpoint 可重启
- 评分：业务 4 / 架构 5 / 工程 4 / 迁移 5

### OpenHands
- litellm 统一接入（llm.py 874 行）；RetryMixin 指数退避+NoResponse 时 temperature 0→1 重试
- fn_call_converter.py 把 FC 模拟成文本协议支持无 FC 模型
- Condenser 家族 10 种上下文压缩策略管道式组合；StuckDetector 卡死检测；MCP 全协议
- 评分：3/5/5/3

### SWE-agent
- HistoryProcessor 管道 399 行：CacheControlHistoryProcessor 给最近 2 条打 cache_control ephemeral——显式依赖网关透传
- CostLimitExceededError/ContextWindowExceededError 语义化异常；bash -n 预检；观察截断 10 万字符
- 评分：2/5/5/4

### google__adk-python（ADK 2.0）
- BaseLlmFlow 30+ processor 管道；anthropic_llm.py:668-716 完整实现 cache_control 标记+cache_read/cache_creation token 拆分统计
- _prompt_cache.py 抽象"Gemini server-side vs Claude prefix cache"双模型共享配置，TTL<1h 不值得写缓存的成本判断
- 评分：3/5/5/3

### 其他要点
- agent-framework-main：ADR 驱动（20+ 份）；长运行操作 ADR 0009 系统对比 OpenAI background/Foundry Run/A2A Task 三种长任务模型——网关任务型 API 命中演化线。3/5/5/3
- crewAI：hierarchical manager LLM 动态分派；unified_memory 三层；组件序列化可 dump/restore。3/4/4/3
- autogen-main：SelectorGroupChat LLM 路由；replay client 录制回放测试范式（无 mock）。2/3/4/3
- agno：cancellation 端到端传播独立模块；多租户 JWT RBAC+审计+OTel；60+ 供应商目录。4/4/4/4
- PraisonAI：failover+错误分类器链（先分类再决定重试/切模型）；CLI backend 把 claude-code/codex-cli 当执行后端——"调用网关"与"调用 coding CLI"抽象为同一接口的趋势信号。3/3/3/3
- swarms：30+ 编排结构即类库；drift detection 语义漂移重跑；model_router。2/3/3/3
- GenericAgent：3K 行自进化最小 agent；**beta 头清单是上游 Anthropic 特性面活体情报**（context-1m-2025-08-07、interleaved-thinking-2025-05-14、context-management-2025-06-27、prompt-caching-scope-2026-01-05、effort-2025-11-24）；_drop_unsigned_thinking：无签名思考块必须丢弃否则 API 拒绝；do_no_tool 引擎侧反伪实现兜底。4/4/3/4
- all-agentic-architectures：35 种模式库；deterministic-picker（LLM 只出特征，Python 组合决策）规避评分平带病；provider 能力矩阵可抄进模型能力标注。2/4/4/4
- langflow：provider_ssrf.py——租户可编辑 base_url 是"携带运营方凭据的 SSRF/凭据外泄原语"，默认禁 loopback/RFC1918/云元数据；图级 checkpoint resume；flow 即 MCP 工具。3/4/4/4
- astron-agent：usage 分块累计而非覆盖（engine/nodes/base.py:94-170）；reasoning_content 独立事件通道；SSE 下 OTel context 生命周期管理；Go 租户服务专职多租户/配额。4/4/4/4
- ai-agents-from-scratch：最小内核证明；本地 GGUF 模型流量也是 agent 流量来源。1/2/2/2

## 共性工程优点（10 条）
1. 重试即标配且分类精细（429/5xx/格式错误 requery/空响应换 temperature/漂移重跑/failover 链）
2. 上下文管理是可组合策略管道（截断/滑动窗/摘要/工具结果驱逐声明式配置）
3. prompt caching 一等公民（4 家独立实现 cache_control+拆分计量）
4. 结构化流事件协议（文本 delta/工具调用/思考/usage 分离，三家同构）
5. 工具调用双轨制（native FC+文本协议模拟）
6. MCP 成标准工具接入面（8/15 内建）
7. checkpoint/resume 常态化
8. 观测内建而非外挂；usage 按 chunk 累计而非覆盖
9. 能力声明矩阵驱动降级
10. 人在环/审批中间件化（请求-响应协议而非开关）

## Agent 生态对网关的能力需求清单

**P0（缺失即坏）**：
1. 双协议一等兼容（OpenAI/Anthropic，SSE 逐 chunk 透传不重排不改写）
2. function calling/tool use 透传（含 parallel tool calls、tool_choice；tool_use/tool_result 配对完整性——GenericAgent _fix_messages 证明上游常有配对损坏）
3. thinking/reasoning 块往返（Claude thinking 签名原样回传否则拒签；DeepSeek reasoning_content 透传）
4. prompt cache 全链路（cache_control 透传+cache_read/cache_creation 拆分计量计费）
5. usage 全字段透传聚合（reasoning/cache tokens；分块累计）

**P1（高频痛点）**：
6. anthropic-beta header 透传白名单（GenericAgent 头清单现成）
7. 上下文窗口感知+语义化错误（ContextWindowExceededError 可识别）
8. 可重试错误分类+Retry-After 透传
9. per-request 成本计量
10. 并发/配额/取消传播

**P2（前瞻）**：
11. Responses API/background 长任务模式
12. SSRF 防护于自定义 base_url（langflow 模式）
13. 模型能力矩阵 API（supports_fc/structured_output/vision/cache）
14. 多模态透传

**对 jsplugin 可迁移**：MAF-Go Middleware 链+ResponseUpdate+continuation token；HistoryProcessor 管道；compaction Trigger 谓词；langflow 分层并行图调度+checkpoint resume；deterministic-picker；astron usage 累计与 SSE/OTel 生命周期
