# N7 侦察报告：AI 记忆系统（已完成，15 项目）

## 关键项目

### TencentDB-Agent-Memory — 业务相似度最高（4/5）
- 腾讯 Agent 记忆服务器：L0 原始对话→L1 原子记忆（type/priority/scene/version 单调递增）→L2 场景知识块（heat）→L3 用户画像
- **MemoryProxy（:8096）= LLM 反代+InjectionPipeline 按请求注入画像**，claude-code/codex 等 6+ 框架 adapter，Redis Lua 限流、Credit 计费、健康降级 503 摘除——与 new-api relay 链路同构
- 批量去重：无 LLM 候选召回+一次批量 LLM 判 store/update/merge/skip——LLM 判定 O(1) 化（l1-dedup.ts）
- 三级降级检索：TCVDB hybrid → FTS5+RRF(k=60) → 纯 BM25，按预算/超时截断
- 三维强租户断言（IsolationContext，缺失即抛）+ L0/L1 按用户、L2/L3 按 team 聚合跨用户复利
- PersonaTrigger 5 级优先触发画像自愈重建
- 评分：业务 4 / 架构 5 / 工程 4.5 / 迁移 4

### mem0 — 最流行
- 写入 8 阶段管线：先检索 top-10 注入→单次 LLM ADD-only 增量抽取→MD5 双重去重→批量落库+审计（memory/main.py:879-1206）
- 检索三信号：语义过采样 top_k*4 + BM25 + 实体 boost，自适应归一化
- 多租户强制隔离：filters 必含租户 ID 否则 ValueError；_strip_identity_keys 阻止 metadata 伪造归属
- 评分：3/4.5/4.5/3.5

### letta（MemGPT）
- Core memory（Block 字符配额 5000，Jinja2 注入）+ Recall + Archival（pgvector）
- BlockHistory 版本历史可回滚；后台 SleeptimeMultiAgentV2 定期 memory_rethink 整理（读写分离）
- 评分：2/5/4/3

### 其他要点
- supermemory：时间性向量-图引擎；画像 API 三段分层（static 长期/dynamic 近期/buckets 主题轴）~50ms；per-turn LRU 缓存+tokensSaved/costSavedUSD 逐请求计量。3.5/4/4.5/3
- OpenLore：确定性本地优先（无 LLM 检索）；SQLite 单文件 schema（nodes/edges/decisions/provenance/stale）；原子写+CAS+quarantine；"证据型记忆"——画像字段带 provenance/锚点/显式过期。2/5/5/4
- agentmemory-main：hooks 捕获→LLM 压缩观察→四层整合；stripPrivateData 12+ secret 正则脱敏先行；三流 RRF k=60；艾宾浩斯衰减分层；Jaccard>0.7 自动 supersede。3/4/4/3
- graphiti：bi-temporal（valid_at 内容失效/invalid_at 被取代，可查"任意时刻为真"）；冲突消解是确定性时序规则（新边生效→旧边 invalid_at，失效不删除）；MinHash LSH 消歧省 LLM。2/4.5/4.5/3
- memorizz：防御性检索边界——provider 返回后二次强制作用域过滤（"语义召回是 prompt-injection 边界"）；PolyForm NC 许可。2/4/4/3.5
- mnemosyne：**类型化 Weibull 衰减**（profile k=0.3 约 1 年 vs request k=1.5 约 3 天——画像长命行为短命）；写入门禁两阶段（噪声 regex+detect_secrets）三模式；50%向量+30%FTS+20%importance；来源信任分层。3/5/5/3
- Compartment：**per-record key + crypto-shred**（销毁 wrapped key 即密码学抹除，GDPR 级）；向量也加密（embedding 可部分逆向）；离线三层强制（patch socket+审计 hook）；哈希链审计锚定。3/5/5/4
- hhhuang__CAG：预计算 KV cache 替代运行时检索（WWW 2025 论文）；KV 三段生命周期（生成→落盘→原位截断复用）对应 prompt caching 计费。2/3/2/3
- Agent_Memory_Techniques：30 notebook 教科书；TieredMemorySystem（Hot LRU+Warm+Cold+每用户配额 MemoryBudgetEnforcer）；"稳定事实豁免衰减"；MemoryEvalHarness 评测。3/4/3/4
- okf-agent-memory：**Go 零依赖（go.mod 2 行）**，Markdown+YAML 持久化+纯标准库 BM25+stdio MCP；Validate stale_after/deprecated 可入 CI；路径隔离+YAML 注入防御 655 行专项测试。2.5/4/4.5/4.5
- tigerless-labs__agent-memory：markdown 文件唯一真相+SQLite 索引可重建；四条不变量（单写路径 flock/读不写真相/Manage 权限分级）；Manage.sleep() 两半制——确定性半直接执行、判断半只产 Proposal 由外部裁决，物理删除仅人类可执行；redteam 投毒测试。3/4.5/4/4
- memvid：QR 视频路线已废弃（README L504-508），v2 为单文件 Smart Frames+WAL；仅画像快照导出格式可参考。2/3.5/4.5/2.5

## 记忆系统设计模式共性

**写路径收敛**：原始事件→门禁（脱敏/噪声/秘密检测）→无 LLM 候选召回→批量 LLM 四选一判定（store/update/merge/skip）→去重（MD5/MinHash）→append-only 落库+审计。共识：LLM 调用 O(1) 化、ADD-only 累积、先审计后数据。

**读路径收敛**：多路召回（向量+BM25+图）→ RRF(k=60) 融合（agentmemory/TencentDB/mnemosyne/graphiti 四家共用，事实标准）→重排→预算截断→渐进披露。共识：作用域过滤召回前+后双重执行、检索零 LLM 化。

**遗忘收敛**：三层梯队 hot/warm/cold + 类型化衰减（画像长半衰期/行为短半衰期）+ 访问强化 + 失效不删除（invalid_at/superseded_by）+ 删除分级（确定性合并自动做、物理删除需人工）。

**隐私收敛**：作用域强断言（缺失即抛）+伪造防护+脱敏先行+可选 per-record key 端到端加密。

## 最适合 new-api 的推荐

1. **TencentDB-Agent-Memory（整体蓝图）**——MemoryProxy 与 new-api relay 链路一一对应；L0-L3 映射用户画像层；logs 表天然是 L0 事件流
2. **OpenLore（工程底座）+ mnemosyne（衰减公式）**——SQLite 画像 schema（provenance/stale/审计）+ Weibull 类型化衰减
3. **落地最小路径**：先做"规则画像层"（无 LLM：按 user 聚合 logs → 模型偏好/成本习惯/时段分布/错误率，OpenLore 证据字段+mnemosyne 衰减），验证价值后再加 LLM 异步抽取层（TencentDB 批量判定）
