# N2 侦察报告：网关竞品（已完成，12 项目）

## 逐项要点

### litellm（python|large）— 最成熟网关/SDK 双体
- 核心抽象：deployment 挂 model_group，路由策略可插拔 Strategy 类族
- **Adaptive Router**：Thompson 采样多臂老虎机——请求分 7 桶（代码/写作/推理…），每 (request_type, model) 维护 Beta(α,β) 后验，质量×成本加权 argmax；post-call hook 正则+工具调用检测做每轮 credit/blame，10s 批量刷 PG（router_strategy/adaptive_router/）
- 预算限流器=过滤器模式：先按 provider 预算过滤健康 deployments 再交给任意选路策略（budget_limiter.py），DualCache 内存+Redis
- **Spend 批量落库**：花费增量入 Redis 队列，后台 60s 批量 flush Postgres，按天分表 bulk upsert；成本回写 x-litellm-response-cost 头
- 缓存栈完整（redis/qdrant 双语义缓存、S3/GCS/Disk、DualCache）
- 评分：5/5/5/3

### 9router（node|large）
- 订阅 OAuth 账号一等公民，多账号轮转；MITM 代理（合规风险，不适合照搬）
- **错误规则驱动指数退避账号切换**：ERROR_RULES 配置驱动（文本规则先于状态码），429 按 backoffLevel 指数退避封顶 4min，getEarliestRateLimitedUntil 汇总账号池最早恢复时间返回客户端（accountFallback.js）——粒度是 key 而非渠道
- **RTK token 压缩管线**：转换前原地压缩 tool_result，覆盖 OpenAI/Claude/Kiro/Responses 四形态；按工具名分发过滤器，is_error 永不压缩
- Usage API 适配器矩阵：18+ 供应商主动拉真实余量供路由决策；OAuth 凭据主动刷新（lead time+per-credential 锁）
- 评分：4/4/3/4

### OmniRoute（node|large）
- **19 种可组合路由策略**：fill-first（榨干配额再切）、headroom（选剩余最多）、reset-window、**cache-optimized（可复用 prompt 前缀钉同一账号最大化 prompt-cache 命中）**、lkgp 粘滞、context-relay
- **Auto-Combo 16 因子评分**：[0,1] 归一化凸组合（权重和=1：health 0.16/quality 0.03…）+5% bandit 探索+BudgetExceededError strict 模式；quality 与 reliability 语义分离（无观测 quality=0.5 中性、reliability=1.0）
- **三层独立自愈**：L1 provider 级熔断（仅 408/5xx，OAuth 8次/API-key 12次阈值）→L2 单 key 冷却（防雷群指数×2，429 遵守 Retry-After）→L3 单模型锁定（只锁模型不动连接）
- 压缩引擎 60+ 文件（caveman/RTK/prefixFreeze/fidelityGate）
- 评分：5/5/4/3

### aisix（rust|large）— Rust 企业网关
- **cooldown 与 retryability 解耦**：401 不可重试但必须冷却（否则后续都撞）；fallback_on_statuses 白名单（针对用 4xx 表示过载的供应商）；UnsupportedCapability 显式不可重试（注释记录曾因可重试烧光预算的 bug）；**所有 dispatch 路径共用一个 decide_cooldown 防 H-1 漏冷却**
- EWMA 延迟选路/least_busy(in-flight/weight)/ketama 一致性哈希 session 亲和/priority tier
- Ensemble 面板+评审（min_responses 门槛，judge 0.2 温度，8KB 截断）；语义路由（embed+示例向量 max 相似，复用全部 failover 机制）
- 评分：3/5/5/3

### freellmapi（node|large）★ 业务最对口
- **冷却探针早恢复**：定期对启发式冷却的 key 发廉价 validate 调用，通过即提前解冻；只探过半程的冷却（MIN_ELAPSED_FRACTION 0.5）、失败 2/4/8→15min 退避、每轮限 3 个、绝不延长（cooldown-probe.ts）
- **配额预测**：聚合 provider 自报余量输出"今天还能不能调"+低余额告警（绝对阈值只对 ≥200 大窗口生效防小池误报）
- **in-flight lease**：key 选择到落库间不可见窗口用内存租约封 check-then-act 竞态；四维滑窗（RPM/RPD/TPM/TPD）学习 provider 真实上限
- **耗尽诊断**：RouteError 携带每模型被跳过原因，聚合成 client-safe 摘要（"no usable key×3, prompt too large×2. Soonest reset ~2m"）
- 凸组合+乘法护栏评分：归一化凸组合（权重和=1）× headroom × rateLimit 护栏；Beta 后验 Thompson 采样可靠性
- 评分：5/4/4/4

### 其他
- nexus-llm-router：60+ 策略枚举（反面教材 18k 行单文件）；trigram 语义模糊缓存（Jaccard 0.92 无 embedding）；JSONL 路由审计+rationale 可追责。3/3/4/3
- The-NeXT-AI ai-gateway：Redis Lua 原子熔断（三脚本，多实例共享）；**tiered 分段计费 BillingChargeBreakdown[]（start/end_token/cost 逐段，10 位小数定标整数运算）**；幂等键 pending 态 promise 去重。4/3/3/4
- free-router（~1200 行零依赖）：**流式首包缓冲 fallover**——缓冲 SSE 直到出现 content/tool_call 才 writeHead，空响应供应商无感替换（server.mjs:764-875）；请求能力过滤；每周免费模型 6 题 benchmark 自动发现+评分；出站密钥脱敏 redact.mjs。4/4/3/4
- Sparrowgate：短 prompt 降档路由+缓存 savings_pct 展示；错误消息密钥脱敏 helper。2/1/1/3
- casbin-gateway：**供应商 Authenticity 探测**（Go 同栈零迁移）——模型身份 25 分/自报训练厂商/重复请求一致性/logprobs 参数是否被静默丢弃/两级 tool schema/流事件完整性/缓存是否真入账/知识截止确定性问答，权重表+A-F 评分，历史对比"两次分歧=后端被换"。转写读取用量。3/4/4/5
- FreeToken：边缘 MoE 推理引擎，Anthropic 路由用类型化事件流组装；15s 流静默 ping keepalive。1/2/4/1
- llmquota：各 CLI 配额端点逆向字段结构（Claude utilization/resets_at、Codex 5h+weekly 双窗）。2/2/3/3

## Top 5 最值得 new-api 汲取（按性价比）
1. **freellmapi 免费池配额生命周期**（冷却探针早恢复+配额预测+in-flight lease）——公益网关直接可落三件套
2. **casbin-gateway Authenticity 探测**（Go 同栈零迁移）——渠道验真"挂羊头卖狗肉"，用户信任根基，new-api 完全缺失
3. **aisix cooldown/retryable 解耦+错误分类**——替换渠道 auto-disable，粒度从渠道细化到 key/模型
4. **free-router 流式首包缓冲 fallover**——new-api 流式请求无法 fallover 的最小代价解法（~100 行 Go，注意超时上限）
5. **OmniRoute/litellm cache 亲和粘滞路由+16 因子凸组合评分**——替代简单渠道权重

## 先进但不适合
litellm Prisma+Postgres 层与 rust 子项目；9router MITM（合规风险）；nexus 60+ 策略爆炸（验证"策略必须收敛"）；OmniRoute 42 语言 i18n 全端投入；FreeToken 推理层；aisix etcd 配置面
