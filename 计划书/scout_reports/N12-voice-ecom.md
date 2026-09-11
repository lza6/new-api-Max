# N12 侦察报告：语音 TTS + 电商营销 Agent（已完成）

## A 组：语音 / TTS

### CosyVoice [A]
- 定位：阿里 FunAudioLLM 的 LLM-based TTS（Fun-CosyVoice 3.0, 0.5B），9 语种 + 18+ 中文方言，零样本声音克隆。
- 亮点：
  1. **双向流式（Bi-Streaming）**：文本入流 + 音频出流同时支持，延迟低至 150ms（README.md "Bi-Streaming" 段）。
  2. **服务化 runtime 双协议**：`CosyVoice/runtime/python/fastapi/server.py` 用 FastAPI `StreamingResponse` 把模型输出逐 chunk 转成 int16 PCM 字节流（`generate_data()` 生成器），另有 gRPC 实现（`runtime/python/grpc/` 含 `cosyvoice.proto`）。
  3. 生产级加速路线齐全：vLLM 支持、Triton-TRTLLM runtime（`runtime/triton_trtllm/`）。
- 对 new-api 的意义：这是「自部署 TTS 上游」的典型形态——FastAPI 流式 PCM 走 HTTP chunked、gRPC 走 proto 流。网关若要接入这类自建 TTS，可按 openai `/v1/audio/speech` 的流式语义适配（chunk 边界、采样率声明、按秒计费所需的 audio duration 统计）。
- 评分：业务相似度 3/5 · 架构价值 4/5 · 工程成熟度 5/5 · 可迁移性 3/5

### VoxCPM [A]
- 定位：OpenBMB tokenizer-free TTS（VoxCPM2, 2B, 30 语种），语音设计 + 可控克隆 + 48kHz 输出，Apache-2.0。
- 亮点：
  1. **官方 vLLM-Omni 集成，暴露 OpenAI 兼容 `/v1/audio/speech` 端点**（README.md L299：PagedAttention KV cache、continuous batching、"drop-in OpenAI-compatible /v1/audio/speech endpoint"）。
  2. Nano-vLLM 加速 RTF ~0.13（RTX 4090），支持批量并发 + FastAPI HTTP server（README.md L277-295）。
- 对 new-api 的意义：关键一条——**VoxCPM2 的生产形态本身就是 OpenAI 兼容端点**，new-api 只需把它注册为 openai 类渠道（base_url 指向 vLLM-Omni）即可获得 TTS relay 能力，无需新写 adaptor；这是"语音模型走上通用网关"的最短路径实证。计费可按返回音频时长（OpenAI TTS 的 usage 语义）。
- 评分：业务相似度 3/5 · 架构价值 4/5 · 工程成熟度 4/5 · 可迁移性 5/5

### LuxTTS [A]
- 定位：超轻量 zipvoice TTS，150x realtime，48kHz，仅 1GB VRAM，CPU 也能超实时。
- 亮点：ONNX 推理路径（`LuxTTS/zipvoice/onnx_modeling.py`），代码极小，无服务层——纯库形态。
- 对 new-api 的意义：代表"边缘/廉价自部署 TTS"档位。若网关想做「小成本 TTS 渠道」，LuxTTS 说明上游可以是一个 HTTP 包装脚本，网关侧无需特殊协议，但仍需统一 audio usage 计费。价值低于 VoxCPM/CosyVoice，仅作档位参照。
- 评分：业务相似度 2/5 · 架构价值 2/5 · 工程成熟度 3/5 · 可迁移性 3/5

### OpenSuperWhisper [A]
- 定位：macOS 实时语音转写桌面应用（whisper.cpp + Parakeet 双引擎），全局快捷键按住录音、自动粘贴。
- 亮点：双引擎可切换 + 模型应用内直下（Readme.md "Two transcription engines"）；`agent/` 目录含 builder/coder/github 等 agent 脚本用于维护项目本身。
- 对 new-api 的意义：ASR 客户端侧参照。它证明「本地 Whisper + 云端修正」是终端用户真实形态——网关若开 ASR 渠道，客户多为本地 whisper.cpp 兼容客户端，需要 `/v1/audio/transcriptions` 完整 multipart 兼容。当前 new-api 已有 openai audio relay（`relay/channel/openai/audio.go`），差距主要在多引擎语义（如 Parakeet 的流式 partial）。
- 评分：业务相似度 2/5 · 架构价值 2/5 · 工程成熟度 4/5 · 可迁移性 2/5

### VoiceMem [A]
- 定位：「流式双脑」语音长期记忆系统（python）——左脑管理事实记忆（Mem0 满载兼容），右脑管理长短期情绪归因，为语音模型加人格与记忆。
- 亮点：
  1. **EOU 0–300ms 投机预取**：`voicemem/stream.py` 流式输入会话——边喂 PCM/外部 ASR partial 文本边投机预取记忆，turn 一结束记忆已就绪，消除"说完才搜记忆"的串行延迟。
  2. **TTS 是第九个可替换位**：`voicemem/tts.py` 契约仅一个方法 `async def stream(text) -> 24kHz PCM16 bytes`，内置 openai/local-piper/voxcpm/breeze 四后端；`speak_stream()` 边生成边合成（吐满一句即送合成，不等全文）。
  3. 组件全解耦：`voicemem/utils/defaults.py` 定义九个可替换位。
- 对 new-api 的意义：对网关直接意义在**计费与协议参照**：(a) 实时语音场景的 usage 是「音频秒数 + token 混合」——new-api 的 RealtimeUsage 已有雏形（`relay/channel/openai/relay_realtime.go` 的 `dto.RealtimeUsage`）；(b) 上游语音渠道的统一抽象应该是"流式文本进、流式 PCM16 出"这一条契约，而非各平台私有 WS 协议堆叠。
- 评分：业务相似度 2/5 · 架构价值 4/5 · 工程成熟度 4/5 · 可迁移性 4/5

### sokuji [A]
- 定位：跨平台实时双向语音翻译（Electron + 浏览器扩展），9 个云端 provider + 100% 本地推理（WASM/WebGPU，44 ASR/75 翻译/137 TTS 模型）。
- 亮点（本项目是 A 组对网关最有价值的架构参照）：
  1. **Provider 统一抽象层**：`sokuji/src/services/clients/` 下 40+ 文件实现 `IClient` 统一接口，`ProviderConfigFactory.getDescriptor(provider).createClient(...)` 按 descriptor 创建（`ClientFactory.ts`）——每家 provider 一个 client 文件 + descriptor 声明传输方式，与 new-api 的 channel adaptor 模式同构但粒度更细。
  2. **双传输层**：WebSocket 与 WebRTC 并存，`OpenAIWebRTCClient.ts` 直接 `createDataChannel('oai-events')` 复用 OpenAI Realtime 的 event 协议但跑在 WebRTC 上；`supportsWebRTC`/`usesNativeAudioCapture` 在工厂层声明能力。
  3. **成本计量客户端化**：`SonioxCostMeter.ts` 等按 provider 粒度在客户端计量费用——网关做语音计费可视化可参照。
  4. Sidecar 架构：本地推理跑在独立 sidecar 进程（`sidecar/sokuji_sidecar/`）。
- 对 new-api 的意义：**实时语音网关必须支持的传输矩阵 = WebSocket（OpenAI Realtime / 豆包 / xunfei 已有）+ WebRTC data channel（OpenAI GA 模式）+ HTTP 流式（TTS）**。sokuji 证明同一上游（OpenAI Realtime）会以两种传输出现，new-api 的 `OpenaiRealtimeHandler` 目前只走 WS，WebRTC 信令代理（SDP offer/answer 中转）是明确缺口。另：provider descriptor（支持传输/认证方式/能力声明）的声明式做法值得抄进渠道元数据。
- 评分：业务相似度 3/5 · 架构价值 5/5 · 工程成熟度 5/5 · 可迁移性 4/5

### Eirias__omnivoice-studio [A]
- 定位：开源 ElevenLabs 替代——桌面级语音克隆/语音设计/视频配音/全局听写，646 语种，多 TTS 引擎矩阵。
- 亮点：
  1. **引擎矩阵声明**（README.md L353 表）：每个引擎标注 CUDA/MPS/CPU 支持与实时性——「能力 × 硬件」声明式矩阵。
  2. 后端服务层完整（`backend/services/`）：tts_backend/asr_backend/speaker_clone/dub_pipeline/gpu_sandbox/incremental 独立模块。
  3. WebSocket 事件总线做 UI 即时刷新 + 指数退避重连（README.md L390）。
- 对 new-api 的意义：多引擎矩阵声明可启发渠道元数据扩展；GPU 沙箱思路对应网关自部署算力隔离场景。产品型项目，架构参照价值中等。
- 评分：业务相似度 2/5 · 架构价值 3/5 · 工程成熟度 4/5 · 可迁移性 3/5

### sanoTTS [A]
- 定位：294k–2.3M 参数微型 TTS 家族，跑在 $3 ESP32-S3 和浏览器 WASM 上，16 语种 30 音色，GPL-3.0。
- 亮点：337KB int8 完整 TTS 栈（`BOARDS.md` 实测数据）；espeak-ng phonemizer 打进 WASM。
- 对 new-api 的意义：直接意义弱，代表 TTS 极端下限档位（端侧完全离线）。若网关未来做「端侧路由」可作参照。略过深挖。
- 评分：业务相似度 1/5 · 架构价值 2/5 · 工程成熟度 4/5 · 可迁移性 2/5

## B 组：电商 / 营销 / 增长

### EcomAgent [B]
- 定位：面向电商运营的通用 Agent Runtime（python/FastAPI/React）——把「任务输入→上下文装配→模型决策→工具调用→权限确认→执行策略→结果返回」抽象为统一执行闭环。
- 亮点（本项目是 B 组最完整的机制级范本）：
  1. **PreToolUse 三段管线**（`EcomAgent/permission/pre_tool_use.py`）：RBAC 身份门（店长/运营/客服/财务四角色工具级 ACL）→ 业务规则（成本保护：调价不得低于成本价）→ 模式门（READONLY/ASK/EXECUTE）。ASK 时挂起整个 turn（异步 resolver），批准后从挂起点恢复，拒绝原因回传给模型重新规划。
  2. **执行策略层**（`EcomAgent/execution/`）：错误四分类（CLIENT/BUSINESS 不重试，TRANSIENT 指数退避，FATAL 按策略）+ **幂等键** `idem_{session}_{turn}_{tool}_{params_hash}`（重试复用同键，Adapter 经 `X-Idempotency-Key` 透传）+ 读写分级超时 + 双层结果校验。
  3. **Mock Commerce API 两阶段确定性故障注入**（`EcomAgent/mock_commerce/`）：故障在「副作用落库之后」才让客户端超时，精确复现「服务端已生效、客户端超时」——端到端验证幂等重放不产生第二次副作用。
  4. **行为验证 Harness**（`EcomAgent/harness/`）：21 条场景是数据表（输入→期望轨迹），断言到事件序列 + 工具参数 + 副作用审计粒度，与 baseline 场景指纹对比。
  5. 一份 AgentEvent 八类事件流同时驱动 UI/评测/Trace 三个消费方。
- 对 new-api 的意义：**这是"淘宝电商平台用户"场景的活教材**。对 new-api「非程序员用户」扩展的直接启发：(a) 写操作必须过 PreToolUse 管线且模型无法绕过；(b) 幂等键是 Agent 操作电商后台的生死线；(c) 权限挂起/恢复机制（对话内权限卡 + 跨会话审批中心双入口）是运营人员信任 AI 的关键交互形态；(d) 行为回归指纹基线让"AI 行为验收"可进 CI。
- 评分：业务相似度 5/5 · 架构价值 5/5 · 工程成熟度 5/5 · 可迁移性 5/5

### commerce-agents（Anthropic 官方）[B]
- 定位：Anthropic 官方双 agent 参考实现——shopping agent + merchant agent，同一套库跑 Messages API / Agent SDK / Managed Agents 三路径，4 个 vertical。
- 亮点（安全机制密度全 B 组最高，见 `commerce-agents/docs/safety.md` 逐条带代码路径的表格）：
  1. **Fencing 防注入**：第三方文本（商品描述/页面内容）净化后包进固定标签 fence、截断上限，删除不可见字符、伪造 turn 标记（`commerce_common/fencing.py`）。
  2. **写操作 staging**：merchant 所有写入都是 staged change，`apply_change` 仅对 host 在 portal 明确 approve 的 change id 生效——「对话里打字说同意」无效（`merchant_agent/gates.py`）。
  3. **来源追踪 provenance**：cart 写入只接受本会话工具返回过的 product id——模型凭空编 id 直接被拦。
  4. **无支付原则**：`StorefrontBackend` 没有支付方法，checkout 只渲染购物车交给 host 完成。
  5. **Grounding 强制**：条款问题/售后问题/未见过的商品 id 必须先调读工具再回答（`tool_choice` 强制）。
  6. Guardrails 双检（staging 时 + apply 时）：单变更条数/价格变动幅度/促销深度/补货量/预算上限。
  7. **Claude Code 插件产品化**：`plugins/commerce-builder/` 的 `/scaffold-commerce-agent` 等命令让非程序员按问答式脚手架构建自己的 agent。
- 对 new-api 的意义：官方背书的电商 agent 安全基线。staging + provenance + host approval 三件套就是"让运营人员敢让 AI 动后台"的完整答案；其 skill 目录展示了"业务流程 = 技能文件"的产品化形态。new-api 若做 agent 托管/工作流市场，这套门禁清单可直接作为白名单工具的安全规范模板。
- 评分：业务相似度 4/5 · 架构价值 5/5 · 工程成熟度 5/5 · 可迁移性 5/5

### CommerceAgentBench [B]
- 定位：阿里国际 Accio 团队的电商 agent 基准——107 个长程业务任务（53 CLI / 28 browser / 16 file / 10 API-MCP），有状态本地 mock 复刻真实商业软件。
- 亮点：
  1. **Stateful evaluation**：本地 mock services 建模 SaaS/电商/消息/文档/运维系统，agent 必须真实改变状态才算通过（`bench_core/mock_services/`）；verifier 直接检查最终状态。
  2. 三 harness 对齐（`bench_core/harnesses/registry.py`），同 task_id 跨 harness 直接可比；每次运行保全完整可审计工件。
  3. 真实任务面：Alibaba 商品发布表单、Freightos 多步物流预订、Shopify 主题可视化编辑。
- 对 new-api 的意义：若 new-api 要对"非程序员用户的电商 agent 工作流"做质量承诺，此项目给出评测形态——**mock 有状态服务 + 状态终检 + 可复现容器**。
- 评分：业务相似度 3/5 · 架构价值 4/5 · 工程成熟度 5/5 · 可迁移性 3/5

### open-mercato [B]
- 定位："AI-Engineering Foundation Framework"——多租户 CRM/ERP/电商基础框架（Next.js/TS/zod/MikroORM），内置架构感知 AI harness。
- 亮点：模块自动发现 + overlay 覆盖机制；动态实体/表单；feature-based RBAC（角色×feature flag×组织 scoping 三维门禁）；`.ai/` 目录承载 AI 运行记录与 skill 治理。
- 对 new-api 的意义：定位偏差（给程序员的框架）。其动态实体 + RBAC 三维门禁可借鉴到网关多租户计费组管理。
- 评分：业务相似度 2/5 · 架构价值 3/5 · 工程成熟度 4/5 · 可迁移性 2/5

### claude-ads [B]
- 定位：Claude-first 的 12 广告平台付费媒体运营技能包——审计/计划/创意/监控/实验/报告，默认只读。
- 亮点：
  1. **capability manifest 作为读写能力的权威登记表**（`control-plane/manifests/capability-manifest.json`）：每个平台×能力声明 mode（export-read/live-write）、status、实现路径、测试路径、证据源 id；`account-mutation` 全部 `status: disabled` 且写明原因——**"live-write 必须过审批、幂等、验证、审计、回滚五道门才启用"**，manifest 里没解锁就真的不存在写路径。
  2. `/ads launch --draft`、`/ads optimize --draft`：所有变更默认 draft 不落账户。
  3. 33 个细分 skill 按平台×职能切分，`/ads` 单命令入口。
- 对 new-api 的意义：**能力白名单的工程化样板**——"危险能力默认不存在、启用需 manifest 登记 + 证据链"，适合 new-api 做敏感工具的灰度发布规范。技能包+控制面+schema 验证可平移到"技能/工作流市场"审核机制。
- 评分：业务相似度 4/5 · 架构价值 4/5 · 工程成熟度 5/5 · 可迁移性 4/5

### marketing-dashboard [B]
- 定位：本地优先营销运营控制台（Next.js+SQLite）——CRM/外联/内容/分析/审批/自动化与 agent 活动收进一个自托管界面。
- 亮点：**agent workspace 白名单写入**（`src/lib/agent-workspace.ts`）：`isAllowedWorkspaceWritePath()` 用扩展名白名单 + 拒绝隐藏目录/node_modules/state/credentials/logs + posix normalize 防穿越 + root 前缀强制（约 40 行完备代码）。
- 对 new-api 的意义：展示"运营人员管理 agent 舰队"的控制台形态——审批、暂停、回写、host 访问控制全部显式可见。new-api 做用户侧 agent 面板时，文件写入白名单函数可直接抄。
- 评分：业务相似度 3/5 · 架构价值 3/5 · 工程成熟度 4/5 · 可迁移性 4/5

### PriceGhost [B]
- 定位：自托管全网价格追踪（监控任意网站降价/到价/补货通知）。
- 亮点：**四路提取并行 + 价格投票裁决**：JSON-LD/专用爬虫/通用 CSS/AI 分析各报候选价+置信度，分歧时弹出 Price Voting Modal 让用户看全部候选与出处人工定夺——「AI 输出不确定时把选择权交还用户」的 UX 范本。
- 对 new-api 的意义：AI 提取/判断不可靠时，不做静默兜底，而是多路证据并列 + 人工一键裁决。可复用于网关的模型选择、路由异常处置场景。
- 评分：业务相似度 3/5 · 架构价值 2/5 · 工程成熟度 3/5 · 可迁移性 3/5

### Agent-Reach [B]
- 定位：给 AI agent 一键安装 15 平台读取能力——自动装依赖、路由后端、health-check，零 API 费。
- 亮点：`agent_reach/doctor.py` 自检机制（`agent-reach doctor --json` 报告可用后端与缺失项）。
- 对 new-api 的意义：代表"工具自动体检与安装"的产品化路径。网关的渠道健康检查/用户环境自检可参照 doctor 的 JSON 报告设计。
- 评分：业务相似度 3/5 · 架构价值 3/5 · 工程成熟度 4/5 · 可迁移性 3/5

## 汇总一：实时语音对网关协议栈的要求清单

结合 A 组证据与 new-api 现状（已有 `relay/channel/openai/relay_realtime.go` WS 双向泵 + `dto.RealtimeUsage`、`relay/channel/volcengine/tts.go` WS TTS、`relay/channel/openai/audio.go`）：

1. **传输矩阵三件套**（sokuji 实证）：WebSocket（已有）+ WebRTC data channel（OpenAI GA 模式：SDP 信令代理，new-api 缺口）+ HTTP chunked 流式（TTS，已有基础）。
2. **统一 usage 语义**：Realtime = 输入/输出音频秒数 + 文本 token 混合计费；TTS = 输出音频时长；ASR = 音频时长。RealtimeUsage 需扩展 audio duration 字段供"按秒计费"。
3. **TTS 渠道适配最短路径**：VoxCPM2 证明自部署 TTS 可直接暴露 OpenAI 兼容 `/v1/audio/speech`——新增"openai-compatible TTS"渠道类型即可覆盖 vLLM-Omni 系自部署语音。
4. **流式契约标准化**：上游抽象 = "流式文本进 / 流式 PCM16 出"一条方法；chunk 需声明采样率/位深/声道。
5. **声明式渠道能力元数据**（sokuji ProviderDescriptor / omnivoice 引擎矩阵）：支持传输类型、是否 WebRTC、实时性档位、硬件要求——前端据此做模型选择与降级建议。
6. **成本计量**：语音渠道费用波动大，参照 SonioxCostMeter 在流内持续累计并在 UI 实时展示。
7. **ASR 客户端兼容**：本地 whisper.cpp 类客户端是主流，`/v1/audio/transcriptions` multipart 全兼容 + 流式 partial 输出是增值方向。

## 汇总二：电商 agent 产品化对 new-api「非程序员用户」扩展的路线启发

1. **安全四件套是信任地基**（commerce-agents + EcomAgent 双重实证）：PreToolUse 规则引擎管线 → 写操作 staging → provenance → 幂等键。缺任何一件，运营用户就不敢放手。
2. **权限交互形态**：对话内权限卡 + 跨会话审批中心双入口响应同一挂起请求；写操作挂起 turn、异步恢复。
3. **能力白名单的登记制**（claude-ads capability manifest）：危险能力默认 status=disabled 且无实现路径，启用需 manifest 登记 + 测试路径 + 证据源——比 feature flag 严格一级。
4. **模板化工作流 = 技能文件**：业务流程（比价/上架/调价/客服）做成 SKILL.md 数据而非代码，非程序员用户可分享/安装。
5. **结果交付形态**：AI 不确定时四路证据 + 投票裁决交还用户（PriceGhost）；写操作永远 draft 优先。
6. **行为可验收**（EcomAgent harness / CommerceAgentBench）：场景即数据 + 基线指纹回归 + 有状态 mock 终检。
7. **文件/环境写入白名单**（marketing-dashboard agent-workspace.ts）：约 40 行可平移代码。
8. **插件式脚手架**（commerce-builder 插件）：问答式构建自有 agent 是"非程序员上手"的产品入口。

**迁移优先级**：A 组 Top3：sokuji > VoxCPM > VoiceMem。B 组 Top3：EcomAgent > commerce-agents > claude-ads。
