# N10 侦察报告：视频/多媒体生成（已完成；对照主项目源码基准）

主项目现状基准：model/task.go:50 Task 结构、service/task_polling.go:28 TaskPollingAdaptor（批量轮询、CAS UpdateWithStatus、sweepTimedOutTasks）、service/task_billing.go:257 RecalculateTaskQuota 差额结算、service/task_artifact_store.go 产物持久化。

## 关键项目

### MoneyPrinterTurbo（python|large）★ 5/5/5/5
- 七阶段流水线+stop_at 停止点：每个 stop_at 可把中间产物作为完整任务交付（task.py:1284）
- **付费 API 防重复扣费（最高相关）**：volcengine_seedance.py:251-283——提交接口不自动重试，超时/5xx/响应不可读统一抛 VolcEngineSeedanceUnconfirmedTaskError（"付费任务可能已存在远端"，携带远端 task_id）；只有明确 4xx 才判确定性失败。影响付费规格的配置非法值报错而非静默回退（"无效值不能静默回退…仍可能产生超出预期的费用"）
- preflight 前置检查：消耗额度之前校验 FFmpeg/API Key/配额；并发预占名额修复竞态+队列上限防成本失控
- **对 new-api**：现有 Submit→Poll 两态闭环缺"提交结果不可确认"态——把"远端可能已创建付费任务"作为一等状态记录（带远端 asset_id），轮询器凭远端 id 兜底查询而非盲退；preflight 前移渠道参数校验

### VideoCaptioner（python|large）3/3/4/4
- QThread 嵌套流水线+进度加权转发（子线程按权重折算总进度 0-40%/40-60%/70-100%）
- **对 new-api**：Task.Progress 现存上游字符串；网关侧编排组合任务时加权合成直接可用；"warnings 随成功返回不作为失败"——部分降级不吞整体成功

### HKUDS VideoAgent（python|large）2/4/3/3
- LLM 生成执行图→LLM 审判（参数路由是否成立、有无功能冗余）→反思重生成→顺序执行（multi.py:111,164,271）
- **计划期类型检查**：执行前 LLM 验证 DAG 可行性；Task.Properties json 字段可容纳 graph/chain JSON 结构

### Ai-movie-clip（python|medium）4/3/2/3
- 同步/异步双模端点封装器：一函数双形态发布（同步等结果/异步领 task_id 轮询）——"任务化改造存量接口"通用胶水
- 印证 new-api task API 形态业界通用；其问题面（内存态/无持久队列）new-api 已用 DB+gopool 解决——缺的是任务级进度语义（current_step 人话阶段描述）

### 其他
- FunClip：识别一次剪辑多次（贵上游结果可复用，Task.Data 挂中间产物计费只对增量）。3/2/3/2
- video-analyzer：**帧预算**把不可控视频长度转化为有上限 token 消耗（max-frames 成本阀门）——任务提交应允许声明预算上限提前拒绝。2/2/3/3
- PPLLaVA：视频理解计费口径应基于 token 而非时长。1/1/3/1
- auto-subs：跨边界错误传播规则写成显式契约（效仿把 pollClass* 分类表写进接口注释）。1/2/4/1
- （VideoRAG/Pixelle-Video/Open-Generative-AI/seedance-2.0/OpenMontage/whisper-flow 等余项为摘要级覆盖：均验证"提交-轮询-产物"范式，无超出 MoneyPrinterTurbo 的新机制）

## 媒体生成任务共性工程模式
1. **三态提交语义**：confirmed-failed / unconfirmed-paid（远端可能已创建）/ confirmed-success——new-api task 闭环应升级
2. 进度加权合成：每子阶段声明进度区间，主任务进度=Σ(子进度×权重)
3. preflight 前置校验：消耗上游额度前验参数/配额/依赖
4. 贵结果复用：中间产物挂 Task.Data，重剪辑不重跑 ASR，计费只对增量
5. 预算阀门：max_frames/max-seconds 类上限在提交时声明并提前拒绝
6. 失败保留已达进度（_mark_task_failed 保留 stage 前进度）

## 补发追加（Pixelle/Open-Generative-AI/html-video/OpenMontage/seedance 等）

### Pixelle-Video（python）★ 迁移价值最高 5/5/4/5
- 三层解耦：TaskManager（内存任务表+TaskStatus 枚举+update_progress 折算+cancel 拒绝终态+cleanup loop，注释"can be replaced with Redis later"）→ BasePipeline（__call__(text, progress_callback)+模板方法生命周期 setup→generate→plan→produce→post→finalize，子类覆写步骤即定制）→ ProgressEvent 结构化事件（event_type/progress 0-1 强校验/frame_current/step/action；嵌套换算 base+per_frame*completed+per_frame*event.progress）
- **new-api 对应**：Task 只有上游透传 Progress 字符串；结构化进度事件在网关侧一样可存（Task.Data json）、可查询、可取消；LinearVideoPipeline 对应"网关侧组合任务"（jimeng 生成+FFmpeg 后处理）生命周期模板

### html-video（node）★ 进度反馈最精 3/5/5/5
- **seq+sinceSeq 断线续传事件流**：任务与 HTTP 请求解耦（"a generation must NOT die when the browser navigates away or the SSE connection drops"）；事件带单调 seq，subscribe(taskId, sinceSeq) 先回放存量再挂订阅者；死订阅者不阻塞任务；终态 TTL 剪枝（task-registry.ts:5-8,116-134,78）
- **new-api 对应**：任务查询目前是 GET 轮询快照；seq 重放协议可直接移植（Task 旁表加单调事件序号，SSE 端点按 Last-Event-ID 语义重放）——同时解决"轮询风暴"和"进度丢失"（~200 行参考实现）

### Open-Generative-AI（node）4/3/3/4
- 状态词汇规范化函数（SUCCESS/FAILURE 同义词集合收敛各平台不一致）
- **失败错误携带 generationResult.cost + "Refunded N credits" 附在错误信息后**——用户同一处看到失败与补偿
- **new-api 对应**：RecalculateTaskQuota 补扣/退还流水已在 DB，但 fail_reason 没有"本次失败已退 N"汇总表述——拼接进任务错误文案是可感知体验项；各 adaptor ParseTaskResult 缺公共状态同义词表

### seedance-2.0（skill 包）4/4/5/4
- 13 步门控循环+Authority Order 九层冲突裁决；**续接血缘协议**："accepted observed state overrides planned state"——已接受镜头实测终态覆盖计划态、被拒素材不得成为续接源（SKILL.md:121-133）
- **付费纪律**：resume 必须用 --prediction-id（"never submits"），轮询端点物理上不收生成 body 防误提交
- **new-api 对应**：续接类任务 Task.Data 记 source_task_id+observed_end_state；提交/轮询接口分离固化成 adaptor 开发规则

### OpenMontage（python）4/5/4/4
- YAML manifest 流水线（stages/produces/checkpoint_required/human_approval_default）；**预算治理三参数**：budget_default_usd 2.00、single_action_approval_usd 0.50（单动作超阈值要人批）、require_approval_for_new_paid_tool（cinematic.yaml:27-30）
- checkpoint jsonschema 校验+阶段→必需 artifact 映射+被覆盖 checkpoint 归档不销毁
- events.jsonl append-only，"Observability must never break production"（观测函数吞自身错误）
- **new-api 对应**：Task.Quota 是单任务预扣，缺"单动作审批阈值"+"新付费工具需授权"策略层；events 埋点编码纪律

### AI-Youtube-Shorts-Generator 3/3/4/4
- LLM 输出物理边界钳制（_sanitize_highlights 把时间段钳制到视频时长内）——网关侧防幻充校验点

### whisper-flow 2/3/3/3
- 流式 partial→final 双级事件是流式任务进度语义标准；流式任务计费应按入流字节/时长而非按"次"

## 共性工程模式（7 条）与 new-api 可吸收点（按落地成本排序）
1. **状态同义词规范化表**（零风险）——各 adaptor ParseTaskResult 共享映射表
2. **"unconfirmed 提交"任务态**（低成本高价值）——提交超时记录远端 task_id 标记未确认，轮询器凭远端 id 兜底而非直接退款，减少"已付费但用户被退款"资损
3. **任务事件流 seq+sinceSeq**（中成本）——Last-Event-ID 语义重放，SSE 地基
4. **结构化 Progress**（中成本）——Task.Data 约定 {event_type,current,total,step}，解析不出退化为字符串（向后兼容）
5. **差额结算用户侧可见性**（低成本）——"Refunded N credits"拼进 fail_reason
6. **预算三参数进任务提交**（按需）——single_action_approval_usd/新付费工具授权/wall-time
7. **续接任务血缘记录**（按需）——source_task_id+observed_end_state
