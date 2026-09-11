# N3 侦察报告：new-api 衍生品与配额生态（已完成，9 项目）

## 逐项要点

### NewAPI-Gateway（go|medium）— 上游聚合层
- 把多个 new-api 实例当"上游供应商"聚合——new-api 生态长出的"第二层网关"，消费 /api/pricing、/api/token、/api/user/self 三接口做 5 分钟级同步
- **value_score 性价比路由**：成本分 1/(1+unit_cost)×预算分 (balance+1)/(balance+recent_usage+1)，与人工权重融合（model_route.go:653,820）
- **分层重试计划**：优先级降序分层，层内按贡献值"加权随机不放回"生成完整重试序，层全败才降级（model_route.go:239）
- **健康调节乘数**：exp(-α·failRate)×(1+β·healthScore·confidence)，限幅 [0.05,1.12]，样本<5 不生效，参数走系统选项实时调
- **无效响应抑制（new-api 没有）**：200 但内容无动作性的上游，3 次/10min→冷却 15min（classifySSEDataLine）
- 签到编排（互斥锁+时区感知日界+自动禁用）；FailureCategory 五类枚举
- 评分：4/4/4.5/4.5

### Elysia-Api-for-Koishi — 平行竞品（自称受 New-API 启发）
- **Maheshvara 中间表示（IR）**：OpenAI Chat/Responses/Claude Messages/Gemini 四协议统一 IR 互转（10824 行+fidelity 回归测试）；Responses 事件集是 IR 超集，任意协议可渲染成 Responses SSE——对照 new-api"每对协议直写 adapter"
- **Usage 账单保真**：Claude 缓存写入双 TTL 桶（cache_creation_5m/1h_tokens）、tool_prompt_tokens、reasoning tokens、文/图/音/视分列、估算标记位——"账单保真，跨线不糊"
- **渠道亲和 sticky routing**：同 key+同模型组短窗内优先复用上次成功上游，提升 prompt cache 命中率；只重排候选不改变候选集
- 媒体外置去重（SHA-256 提取 body 留占位符）；日志留存三重上限治理；SSRF 出站守卫+ConstantTimeCompare
- 评分：3.5/4.5/4.5/3.5

### hermes-quota-plugin — 终端 UX
- **fail-open fetcher 契约**："永不抛错，失败返回 unavailable_reason，绝不返回假 0"
- worst-chip 模式（只亮最紧张资源）+多窗口配额（Session/5h/Weekly 各带 used_percent+reset_at）+结构化失败原因
- **new-api 余额只有单值快照，无窗口/reset 语义——此 UX 蓝本直接可抄**
- 评分：2/3/4/4.5

### tokentest.io — 买家验收层
- AI 路由黑盒"生产准入评测"：六维加权评分（Identity 30%/Deterministic 30%/Token Usage 15%/Safety/Stability/Channel）+P0/P1 一票否决
- **D4 usage 完整性审计**：usage 存在性/total 一致性/input 随上下文单调递增/流式 usage/cache 证据——"上游 usage 虚报检测"，nonce 重放验证身份
- CI 门禁 junit 输出；报告前脱敏；自带 SSRF 测试
- 评分：3/4/4/4

### 配额/token 管理生态（agent 侧）
- token-savior：工具 profile 分层（15 热工具 vs 60 藏进语义检索）；34 输出压缩器；预算诊断"实返 12KB vs 朴素 47KB"对照口径；诚实 benchmark 文化（公开撤回错误测量）。1/3/4.5/2
- token-optimizer-mcp：**计量契约四量分账**（observed/gross reduction/expansion debit/net avoided 可为负）+双侧 SHA-256 指纹+混合口径隔离区——最严格"节省量"账本。1/4/5/3
- claude-token-optimizer/efficient：无直接价值（负结果披露文化值得注意）

### 汇总：衍生品生态分层
上游聚合（NewAPI-Gateway）/平行竞品（Elysia）/质量验收（tokentest）/终端 UX（hermes-quota）。演进信号：运营侧向"成本最优+防虚报+自动化"深化；协议侧向 Responses API 统一（Elysia IR、NewAPI-Gateway /v1/responses 透传佐证）。计费保真（缓存双 TTL 桶、推理 token、多模态细分）是新一代竞争点。

**吸收价值最高三项**：value_score 路由、无效响应抑制、usage 水分审计——覆盖衍生品约 80% 独有增量。
