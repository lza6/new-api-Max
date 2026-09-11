# N9 侦察报告：图谱/知识库基础设施（已完成，20 项）

## 技术路线对比表

| 路线 | 代表 | 强项 | 弱项 | 增量策略 |
|------|------|------|------|---------|
| 真 LSP | serena（缺失） | 编译器级精度 | 进程重、嵌入难 | 文件级 |
| 轻量类型解析 Hybrid LSP | codebase-memory-mcp | 速度精度折中最优 | 实现成本极高 | mtime+hash→CASCADE→重解析 merge |
| AST/tree-sitter 图 | GitNexus、code-review-graph、roam-code、Graft Tier-1 | 确定性、$0、可解释 | 无语义理解 | SHA-256 diff，衍生层按失效规则重建 |
| LLM 摘要节点图 | Graft Pass1/2、graphify、Understand-Anything | "为什么"层 | LLM 成本、stale | body_hash 标 stale |
| 嵌入/向量 | claude-context、txtai、cocoindex | 语义近似 | 相似≠相关、需向量库 | Merkle DAG / memo 双哈希 |
| GraphRAG | LightRAG、graphrag（维护模式） | 全局性/主题性 | 抽取成本高 | chunk 级 upsert merge |
| 树推理 vectorless | PageIndex | 可溯源、零向量栈 | 每次烧 LLM | 树增量 |
| 上下文网关 | lean-ctx | token 经济学 | 不解决理解 | 会话级 |
| 图查询基础设施 | clickgraph | Cypher 生态兼容 | 只做查询编译 | stateless |

## 关键项目

### GitNexus（node，v1.6.11）
- 19 阶段 typed-DAG 流水线入 LadybugDB；三路查询接口（MCP/HTTP/CLI）复用同一后端
- **"删除即失效"规则**：衍生层（Leiden 社区/执行流）删除/新增/修改任一即全量重导，宁保守勿脏读（derived-writeback.ts）
- 跨仓库 Contract Bridge（HTTP 消费者→提供者 ContractLink）；路由提取"精度优先于覆盖率"（"an invented route is a lie"）
- 评分：4/5/5/3

### Graft（已在主项目使用，0.16.0）
- Tier-1 tree-sitter 确定性层（wiring.json，$0 无 LLM）+ Pass1/2 LLM 摘要层分离
- **"查询路径新鲜度门"**：每次查询前 ~3ms fingerprint 探测漂移，编辑器/分支切换也覆盖；失败降级读旧图+锁防重建风暴（refresh.ts）
- ask = 词法排序种子→personalized PageRank 重排序（无 embedding）；blast = git diff 行区间→最内符号种子→传入边 BFS
- SWE-bench Verified 验证：54%→66%、-25% tool calls
- 评分：4/5/5/4

### code-review-graph（python）
- 最易被 Go 吸收：SQLite 单文件、纯增量（git diff+SHA-256，2900 文件 <2s）、MCP stdio 线程池防管道死锁
- **边三级置信度**（EXTRACTED/INFERRED/AMBIGUOUS）贯穿全图；README 语义搜索端点点名支持 new-api 作 OpenAI 兼容后端
- 评分：4/4/4/5

### lean-ctx（rust）— 上下文网关
- ctx_read 三档（signatures/map/full）；search_delta（30 分钟内重复搜索只返回增量）；prompt-cache-safe 压缩代理；Shadow Mode 实测节省
- **"上下文消耗记账"一等公民（savings ledger）**——与 new-api 计费体系映射为"为租户展示 AI 代理省了多少 token"
- 评分：3/5/4/3

### VectifyAI PageIndex — vectorless RAG
- LLM 生成文档层级树索引，检索=树上推理，可溯源到章节；"similarity≠relevance，relevance 需要推理"
- **对 new-api 商业契合度最高**：每次检索是真实 LLM 推理调用（走网关计费），无向量库运维、无 embedding 绑定
- 评分：4/4/4/4

### LightRAG（HKUDS）— GraphRAG 首选
- **PostgreSQL 一体化存储**（KG+向量+文档状态全在 PG）——复用 new-api 现有 PG 依赖零新增运维面
- 增量合并（同名实体属性/边累积）+文档删除自动重建受影响 KG；存储 20+ 后端插拔
- 评分：4/4/4/4

### 其他
- roam-code：git 历史一等信号（co-change 耦合 AST 算不出）；preflight 多信号风险合成（结构半径×git 耦合×复杂度）。3/4/4/3
- codebase-memory-mcp（C）：Hybrid LSP 内嵌轻量类型解析替代外挂真 LSP——Go 项目内嵌 gopls 成本极低。3/4/4/2
- claude-context：Merkle DAG 目录脏检测；**new-api 天然是它的上游供应商**（要求 OpenAI 兼容 embedding 端点）。3/2/4/4
- Understand-Anything：10 subagent"确定性优先"职责分界（LLM 只贡献叙事，明文禁止自己数文件）。3/2/3/5
- clickgraph：Cypher→SQL 编译器——new-api 日志库已支持 ClickHouse，调用链分析可不加 Neo4j 运维。2/4/4/3
- cocoindex：声明式增量索引+memo 双哈希（hash(input)+hash(code)）+数据血缘。2/4/5/2
- graphrag：维护模式不建议依赖；增量 merge 语义可参考。2/3/4/2
- txtai：稀疏+稠密+图+SQL 同库融合；pgvector+PG 落在 new-api 已支持的 PG 上。2/3/5/2
- deer-flow：checkpointer+subagent_batches 可恢复多代理编排；任务计费按 plan/research/coding 阶段归因。2/4/4/3
- ai-knowledge-graph/knowledge_graph：教学级 MVP 参考。1-2 分

## 三条收敛共识
1. **双层架构主流化**：确定性 tree-sitter 结构层打底+LLM 语义层按需叠加；纯向量或纯 LLM 单层路线被边缘化
2. **增量通用配方**：内容哈希脏检测→只重解析变更→衍生层按明确失效规则重建；两个最佳细节：GitNexus"删除即失效"、Graft"查询路径新鲜度门"
3. **置信度标记机制化**：EXTRACTED/INFERRED/AMBIGUOUS 三级边标记对抗幻觉

## 对 new-api 产品化分层建议
1. **文档知识库（短期，商业契合度最高）**：LightRAG（PG 一体化）或 PageIndex（树推理、LLM 调用全走自家网关计费）——"卖铲子"到"卖挖矿服务"的延伸
2. **上下文网关中间件（中期，架构同构）**：lean-ctx 式 relay 链路可选 context 优化层（缓存安全去重/增量返回/租户级节省仪表盘）
3. **代码理解 MCP（长期）**：MCP 服务形态提供"仓库图+影响半径审查"，结构层 tree-sitter+语义层走自有 embedding/LLM
