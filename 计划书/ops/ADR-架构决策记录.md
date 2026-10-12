# ADR — 架构决策记录

> 本文件从**已存在的代码与注释**中逆向提取关键架构决策。
> 每条 ADR 的格式为：背景 / 决策 / 理由 / 后果（含代价） / 证据。
>
> **纪律：只收录能在代码或既有文档里真实找到理由的决策。**
> 找不到理由的、或只有猜测的，不写（见文末 §13）。
>
> 最后核对：2026-10-12（`VERSION` = `v1.3.135`）。

---

## ADR-001：`relaykit/` 是独立 Go module

**背景**
网关需要做 OpenAI Chat / OpenAI Responses / Anthropic Messages / Gemini 四种文本协议
之间的请求、非流式响应、流式事件的互相转换。这些是纯协议层数据建模与语义转换，
与 HTTP 服务、渠道调度、鉴权、计费、数据库无关。

**决策**
把协议转换拆成独立的 Go module `github.com/lza6/new-api-Max/relaykit`（自己的 `go.mod`），
并且**强制**它不依赖根模块。

**理由**
- 可以脱离 new-api 主模块，嵌入其它 Go 网关或代理服务（`relaykit/README.md:3-5`）；
- 不依赖 new-api 主模块、Gin、数据库或全局设置（`relaykit/README.md:14`）；
- 边界清晰：宿主负责渠道选择、模型映射、媒体解析、SSE 读写，relaykit 只做转换
  （`relaykit/README.md:200,217,174`）。

**后果（含代价）**
- ✅ 转换逻辑可被独立测试（矩阵由 golden tests 覆盖，`relaykit/README.md:241-245`）。
- ⚠️ **代价 1：构建验证必须单独做。** 根模块构建通过**不足以**证明 relaykit 仍可独立构建
  —— 必须 `cd relaykit && GOWORK=off go build ./...`（`AGENTS.md:78`、
  `relaykit/README.md:234-239`）。
- ⚠️ **代价 2：JSON 引擎要经注入而非直接 import。** relaykit 自己**不依赖**第三方 JSON 库，
  而是暴露 `Codec` 接口，由宿主在启动时 `SetCodec` 注入（`relaykit/relayconvert/kitutil/json.go:1-5,13-16`）。
  由此**禁止**在 `relaykit/` 内使用宿主 `common` 包，要用 `kitutil.*`（`AGENTS.md:90`）。
- ⚠️ **代价 3：宿主必须补位。** 缺少 `max_tokens` 且未配置 `Claude.DefaultMaxTokens` 时
  转换直接报错；需要媒体解析而未配置回调时也明确报错 —— 转换器本身**不发起网络请求**
  （`relaykit/README.md:199,217`）。

**证据**
`relaykit/go.mod:1`（module 声明）；`relaykit/README.md:3-5,14,33,234-239`；
`AGENTS.md:22,75-78,90`；`relaykit/relayconvert/kitutil/json.go:1-5,13-16`。

---

## ADR-002：根模块所有 JSON 走 `common/json.go` 包装

**背景**
根模块的业务代码需要 marshal/unmarshal JSON。如果各处直接调 `encoding/json`，
将来想换 JSON 引擎（例如换成性能更高的 sonic）就要改遍全仓。

**决策**
根模块内**禁止**直接 import/调用 `encoding/json` 做 marshal/unmarshal；
一律走 `common.Marshal` / `common.Unmarshal` / `common.UnmarshalJsonStr` /
`common.DecodeJson` / `common.GetJsonType`。
（`json.RawMessage`、`json.Number` 等**类型**仍可引用。）

**理由**
- `common/json.go` 是**宿主选择 JSON 引擎的唯一位置**：换实现只需改 `hostJSONCodec`，
  所有 `common.*` 与 `kitutil.*` 助手（含 relaykit 的 DTO (un)marshal）一起跟随
  （`common/json.go:12-17`）。
- **在 `init()` 而非 `main()` 注入**：`common` 被几乎所有根包和测试二进制 import，
  而 `main()` 在 `go test` 下从不执行 —— 放 `init()` 才能保证**测试与生产跑在同一个引擎上**
  （`common/json.go:15-17,36-38`）。

**后果（含代价）**
- ✅ 引擎可替换、行为一致（含测试环境）。
- ⚠️ 多一层间接调用；非业务代码（如 codec 实现自身、`IndentJson` 的缩进美化）
  仍直接用 `encoding/json`，属于被允许的例外（`common/json.go:68-74`）。

**证据**
`common/json.go:12-18,36-38,40-66,68-74`；`AGENTS.md:80-90`。

---

## ADR-003：计费的额度转换必须走 `common/quota_math.go`

**背景**
计费路径要把「价格 × 倍率 × 用户可控乘数」这类浮点/十进制结果转成整数 quota。
历史上散落的裸转换（`int(float64(quota) * ratio)`、`int(math.Round(...))`、
`int(decimal.IntPart())`）在输入无界时可能**整数回绕成负值**——即**把扣费变成给用户送钱**。

**决策**
把**所有** quota 舍入/转换集中到 `common/quota_math.go`，业务代码只准调用这些助手：

| 助手 | 语义 |
|---|---|
| `QuotaFromFloat` | 截断（float 乘积） |
| `QuotaRound` | 四舍五入（半远离零） |
| `QuotaFromDecimal` | 十进制乘积 |
| `QuotaFromFloatChecked` / `QuotaRoundChecked` / `QuotaFromDecimalChecked` | 额外返回 `*QuotaClamp` 供审计 |
| `*Strict` 变体 | 越界时**返回错误**而不是用饱和值继续计费 |
| `WalletQuotaFromDecimalStrict` | 钱包/充值，用 JS 安全边界 |

**理由**
- **一处饱和 + 一处日志策略**，所有计费路径共享（`common/quota_math.go:10-12`）。
- 单请求扣费限制在 **int32** 边界，使**批量累加无法逼近 64 位回绕**；
  钱包/充值另用 JS 可安全表示的 64 位域（`MaxWalletQuota = 1<<53-1`）
  （`common/quota_math.go:10-17`）。
- 每次 clamp/NaN 回退都 `SysError` 记日志 —— 正常运营下单请求**不会**接近这些边界，
  撞上就是 bug 或滥用信号（`common/quota_math.go:74-99`）。
- **可审计**：`*Checked` 变体返回的 `*QuotaClamp` 会被记到消费/任务日志的
  `admin_info.quota_saturation`（admin-only），异常在管理端 UI 与后端日志都留痕
  （`common/quota_math.go:39-72`；`AGENTS.md:127-128`）。

**后果（含代价）**
- ✅ 不可能因算术回绕产生负扣费（理论上的「credit」）。
- ✅ 饱和事件可审计、可告警。
- ⚠️ 每条计费路径必须显式选择「截断 vs 四舍五入 vs 严格报错」——
  选错会造成 ±1 的金额差异；`QuotaRound` 的注释明确要求所有**阶梯计费**路径
  （预扣费、结算、明细校验、日志字段）用它以避免 ±1 不一致（`common/quota_math.go:129-132`）。
- ⚠️ 预扣费与结算**都必须安全**：饱和的超大 quota 必须在预扣费阶段以
  「额度不足」失败，而不是静默回绕（`AGENTS.md:130`）。

**证据**
`common/quota_math.go:10-17,29-72,74-100,109-175`；`AGENTS.md:122-132`。

---

## ADR-004：计费表达式「一个表达式，一个真相」

**背景**
模型计费规则包含阶梯价、缓存/图片/音频差异、时间折扣、请求感知乘数。
早期用零散的 ratio 表 + 隐式规则表达，规则藏在多处、难以审计。

**决策**
**一个表达式字符串完整定义一个模型的计费逻辑**，不引入外部 ratio 表、隐式补全倍率、
隐藏换算系数。

**理由**（来自 `pkg/billingexpr/expr.md` 的 Design Philosophy 原文）
- 表达式是**管理员与系统之间的计费契约**：写什么就执行什么，系统只忠实求值、不做解释
  （`pkg/billingexpr/expr.md:3,5`）；
- **自包含**：同样的 token 数与请求上下文，同一表达式永远产出同样成本（`pkg/billingexpr/expr.md:11`）；
- **价格是真价**：token 系数就是供应商公布的 $/1M tokens，`p*2.5` 即 $2.50/1M，
  不需要 ratio 换算或 `/2` 约定（`pkg/billingexpr/expr.md:15`）；
- **上游无关**：系统按上游响应格式归一化 token 数，表达式不必知道上游是
  OpenAI 格式（`prompt_tokens` 含缓存）还是 Claude 格式（`input_tokens` 不含缓存）（`pkg/billingexpr/expr.md:17`）；
- **版本感知**：表达式带版本标签（`v1:`，省略时为默认），版本控制编译环境、
  token 归一化与 quota 换算公式，使演进不破坏既有表达式（`pkg/billingexpr/expr.md:19`）。

**后果（含代价）**
- ✅ 计费可审计、可 diff、可版本化。
- ⚠️ **改动必须先读 `pkg/billingexpr/expr.md`**（`AGENTS.md:118`）。
- ⚠️ 表达式里用 `group` 变量做分组分支时，**else 分支必须精确等于该模型当前的按次价**，
  否则会误伤其他分组；这条由 `TestElseBranchEqualsLegacyPerRequestPrice` 自动锁死
  （`计划书/ops/token-billing-group.md:41-42,56-68`；
  `pkg/billingexpr/token_billing_group_test.go:86`）。
- ⚠️ **改生产计费表达式是最危险的操作之一**：改库即生效，没有「部署」这一步（见 ADR 之外的
  §7 事故响应）。

**证据**
`pkg/billingexpr/expr.md:1-17`；`AGENTS.md:118`；
`计划书/ops/token-billing-group.md:41-42`；`pkg/billingexpr/token_billing_group_test.go:86`。

---

## ADR-005：响应缓存默认关 + 默认按用户隔离 + 宁可不命中不可错命中

**背景**
网关可以做「精确响应缓存」：同一请求直接回上游字节，省一次上游调用。

**决策**
三条**硬约束**，违反任意一条都不如不做：

1. **默认关**（走能力开关注册表 `RESPONSE_CACHE_ENABLED`）；
2. **默认按 user 隔离**（缓存键含 `user_id`；`AllowCrossUser` 默认 `false`）；
3. **宁可不命中，不可错命中** —— 缓存键的归一化必须保守。

另外只缓存：① 非流式；② HTTP 200；③ 完整读到 EOF 且 `usage` 存在。
流式、上游错误、空壳响应一律不缓存。`AllowModels` **为空表示全不缓存**，
必须由管理员显式加白名单。

**理由**
- 跨用户泄漏是这项功能**最严重的风险**（`setting/response_cache_setting/response_cache_setting.go:21`）；
- 命中时把上游字节**原样**回给客户端，因此 `usage` 天然如实回填，
  客户端看到的 token 数与他本该看到的一致（`service/response_cache.go:25-27`）；
- 不做 embedding/语义缓存：那会引入向量库与误命中风险，成本高于收益
  （`service/response_cache.go:25-26`）；
- 键归一化保守：顶层解成 `map[string]json.RawMessage` 而不是 `map[string]any`，
  否则 `seed: 12345678901234567890` 这类大整数会被 float64 舍入，
  两个**不同**请求可能算出同一个键 → **错命中**。刻意不递归排序嵌套键，
  代价是嵌套键序不同的等价请求不命中 —— **方向正确**（`计划书/workflow_status.md:307-310`）。

**后果（含代价）**
- ✅ 默认关闭时生产行为与部署前**完全一致**，只是一个可打开的开关。
- ⚠️ 命中率偏低是**刻意选择**：`AllowCrossUser=false`、空白名单、不排序嵌套键都会降低命中率。
- ⚠️ **键格式版本必须 bump**：归一化规则一变就要改 `responseCacheKeyVersion`，
  否则新旧规则的键会在同一次部署的滚动窗口里混用，可能错命中
  （`service/response_cache.go:37-39`）。
- ⚠️ v1 **不缓存流式**；写入侧只接在 OpenAI 适配器路径，原生 Claude/Gemini 适配器的
  非流式路径**只读不写**（`计划书/workflow_status.md:341-343`）。
- ⚠️ **「绕过 relay handler 自己计费会 panic」**：钩子必须包在 `adaptor.DoRequest` 外层、
  伪造 200 响应喂给既有链路。曾在 `controller/relay.go` 里命中即自己回写 + 自己调
  `PostTextConsumeQuota`，结果**单测全绿、真实 E2E 抓到每次命中都 panic**
  （依赖 relay handler 逐层填充的 relayInfo 状态），且那条路径**不写消费日志**
  （`计划书/workflow_status.md:312-318`）。

**证据**
`service/response_cache.go:23-39,40-49`；
`setting/response_cache_setting/response_cache_setting.go:19-22,52-58`；
`计划书/workflow_status.md:307-318,341-343`。

---

## ADR-006：响应缓存默认走磁盘后端，但进程内 LRU 有硬容量上界

**背景**
缓存条目是**整个上游响应体**（可达数百 KB）。生产是 4 核 4G / 容器 900MB 上限、
磁盘 78G 富余的机器形态。

**决策**
响应缓存后端可选 `memory | disk | redis`，**默认 `disk`**；
`memory` 后端是带 TTL 的**进程内 LRU，容量有硬上界**。

**理由**
- 放进程内是与内存抢资源，放磁盘才是合理利用现有资源
  （`setting/response_cache_setting/response_cache_setting.go:33-35`）；
- 进程内 LRU 必须有硬上界（用户关切的无界增长）——每次写入淘汰最旧一条，
  读取时顺带做过期判定；**没有后台 goroutine**，清扫在写入路径上分批做，
  避免为一个小功能常驻一个 ticker（`service/response_cache.go:69-74,88-89`）。

**后果（含代价）**
- ✅ 「不允许无限增长」是硬保证（`disk_max_bytes` 超限按最旧优先淘汰）。
- ⚠️ 磁盘后端有四条边界，**缺一条就是把内存问题换成磁盘问题**
  （`计划书/workflow_status.md:462-471`）：
  1. 单条上限 `disk_max_entry_bytes`（默认 1MB）；
  2. 总量上限 `disk_max_bytes`（默认 512MB，硬顶 8GB）；
  3. 宿主剩余空间下限 `min_free_disk_bytes`（默认 2GB，低于即停写并紧急回收一半配额）；
  4. TTL + 5 分钟清理 loop。
- ⚠️ **绝大多数进程内缓存不该迁磁盘**：连接池计数、hot 缓存、排名/统计缓存的 value
  只有几十到几百字节，落盘要多一次 open/read + 反序列化，**比 map 命中慢一个数量级**，
  还新增磁盘故障面。对这些正确的动作是**降容量/缩短 flush 窗口**，不是换介质
  （`计划书/workflow_status.md:473-477`）。

**证据**
`setting/response_cache_setting/response_cache_setting.go:32-44,61,65-74,100-109`；
`service/response_cache.go:69-74,88-89`；`计划书/workflow_status.md:455-477`。

---

## ADR-007：任务/媒体产物默认走「上游代理」，站内图床是可选 local 模式

**背景**
任务（视频/图片生成）的产物字节需要被客户端取回。可选方案：
(a) 在网关回调上游 URL 让客户端直连；(b) 网关把产物字节抓到站内存储（图床）再下发。

**决策**
**默认是 (a) 上游代理**；站内磁盘图床只在 `TASK_ARTIFACT_STORE_MODE=local` 时启用。
S3 模式的配置契约已存在，但**实现未交付前被刻意禁用** —— 配置为 S3 时
**回退到 upstream 模式**并记 `SysError`。

**理由**
- 默认行为是**既有 upstream 代理行为，零变化**（`service/task_artifact_store.go:71-73`）；
- 默认（mode != local）为 `disabledArtifactStore`，`Persist` 返回
  `ErrTaskArtifactStoreDisabled`（`service/task_artifact_store.go:33-51`）；
- S3 配置只在**语法层**校验，「从不解析主机、不联系端点、不验证凭据」——
  避免用未实现的后端接收用户配置后误以为已生效
  （`setting/system_setting/task_artifact_store.go:77-78`）。

**后果（含代价）**
- ✅ 零配置即有正确行为；不引入新的存储故障面与凭据管理。
- ✅ 配置损坏时不崩：非法配置**回退 upstream** 并 `SysError`
  （`setting/system_setting/task_artifact_store.go:65-74`）。
- ⚠️ **站内图床与清理 loop 存在竞态**：`Resolve` 成功、`Serve` 时文件已被清理 loop
  （每 5 分钟一轮）删掉 → `Serve` 返回 `os.ErrNotExist` **且不写任何响应**。
  正确处理是**继续往下走、回退上游代理**；把错误丢掉直接 return 会让客户端拿到
  「200 + 空响应体」且**不会**回退（`controller/task.go:331-347`）。
- ⚠️ **`Resolve`/`Serve` 这条分支很难测**：`taskArtifactStore` 在包初始化时按 env 构建一次，
  所以专门开了测试注入点 `SetTaskArtifactStoreForTest`
  （`service/task_artifact_store.go:55-65`）——否则那条竞态分支永远测不到，
  「这也正是它曾经带着 200 空体缺陷上线的原因」。

**证据**
`setting/system_setting/task_artifact_store.go:15-21,39-40,52-75,77-78`；
`service/task_artifact_store.go:24-31,33-51,55-65,71-76`；`controller/task.go:331-347`。

---

## ADR-008：灰度开关用「原子快照」而不是可变全局 bool

**背景**
项目有 13 个「能力已实现、默认关、管理端无入口」的开关，值只在进程启动时从 env 读一次。
要让它们可灰度，必须让**管理端能改**且**改完立即生效**。

**决策**
**不引入「共享可变包级 bool 变量」**。改为用 `atomic.Pointer[map[string]string]` **发布不可变快照**：
读侧 `Load()` 后只读，写侧构造新 map 再 `Store()`。

**理由**
- 共享可变包级 bool 的写法（管理端写 while 热路径读）在 Go 内存模型下是
  **无同步并发读写，属未定义行为**（可触发不可 recover 的 fatal）
  （`common/feature_flag.go:15-19`）；
- 快照发布后**无锁、无竞争、无 race**（`common/feature_flag.go:18-19`）；
- **分层**：`common` 是最底层包，不能反向依赖 `setting/feature_switch`。故
  key 常量 + 覆盖存储放 `common`，元数据（标题/风险/依赖/度量）+ 持久化 + 管理端语义放
  `setting/feature_switch`（`common/feature_flag.go:21-23`）；
- **存 string 而非 bool**：`POLICY_ENGINE_MODE` 是枚举（`off|shadow|enforce`），
  与布尔开关共用同一张表可以避免维护两套优先级规则（`common/feature_flag.go:66-69`）。

**后果（含代价）**
- ✅ 优先级清晰：**运行时覆盖（管理端热更新/测试） > 调用方传入的 fallback（env 默认）**；
  覆盖值无法解析为布尔时**回退 fallback**，配置损坏不得让热路径 panic
  （`common/feature_flag.go:25,82-95`）。
- ✅ 显式发布 `nil` 表示「无覆盖」态，避免空 map 与 nil 语义分叉
  （`common/feature_flag.go:123-127`）。
- ⚠️ **key 字面量即 env 变量名，属稳定契约，不得随版本改名** ——
  改名会让已持久化的管理端配置失效（`common/feature_flag.go:27-28`）。
- ⚠️ 有 4 个开关的值由 `common`/`constant` 包变量持有（渠道密钥加密、密码登录加密、
  错误日志、媒体 token 非流式）。处理方式是**保留导出变量不动**（测试写法零改动），
  生产读点改调 getter；代价是管理端修改后对**已读过旧值的代码路径**要等下一次读取才生效
  （`common/feature_flag.go:176-185`）。
- ⚠️ 开关表本身**不能成为新的无界增长点**：key ≤13，写入前经 `Meta` + `normalize` 校验，
  未知键丢弃（`计划书/workflow_status.md:249`）。

**证据**
`common/feature_flag.go:10-28,66-70,82-95,112-151,176-185`；
`计划书/workflow_status.md:249`。

---

## ADR-009：公开统计端点用 SWR + 单飞，请求路径上不做同步聚合

**背景**
主页要显示「今日访客 / 总访问量 / 近 30 天访客」等聚合量。
朴素写法是「读缓存 → 未命中就同步查库」。生产压测（12 并发 × 20 请求）实测：
`/api/site/visitors` 出现 **11 次 30s 超时**，而同日 `/pricing` 与 `/` 全部正常。

**决策**
这个公开端点同时做两件事：
- **后台刷新（stale-while-revalidate）**：只要手里有值就**立即返回旧值**，
  并让一个后台 goroutine 去刷新；请求路径上**永远不会**阻塞在聚合上。
  只有冷启动（进程内还没有值）才同步算一次。
- **单飞（singleflight）**：用 `inflight chan struct{}` 保证同一时刻只有一个刷新在跑。

外加 **TTL 分层**：小表 60s，唯一昂贵的 DAU/MAU **30 分钟**；并在启动时预热。

**理由**
- 超时的两个原因**缺一不可**：
  ① 整个聚合一次要 **6.3 秒**（DAU/MAU 是对大表 `logs` 做 `COUNT(DISTINCT user_id)`）；
  ② 缓存一过期，并发的 N 个请求会**同时**未命中并各自跑一遍这 6.3 秒的聚合
  （缓存击穿），互相争抢 DB（`service/site_visitor_tracker.go:517-524`）；
- DAU/MAU 是聚合里**唯一**昂贵（秒级）的部分，且这两个数字本身变化极慢
  （日活/月活不会以秒为单位变动），所以 30 分钟一次的后台计算换请求路径彻底无阻塞
  是最合适的取舍（`service/site_visitor_tracker.go:632-637`）；
- 刷新失败时**保留旧值**，并把 `computedAt` 推到当前时刻，
  避免「过期 + 持续失败」时每个请求都立刻再起一个后台刷新形成**忙循环**
  （`service/site_visitor_tracker.go:603-607`）。

**后果（含代价）**
- ✅ p95 从 **30146ms → 368ms（约 82×）**，超时归零（`计划书/workflow_status.md:701-704`）。
- ⚠️ **代价：数据可能比 TTL 旧一轮** —— 对「站点访客有多少」这类展示量完全可接受
  （`service/site_visitor_tracker.go:532-533`）。
- ⚠️ **实现单飞的教训**：第一版只用 `refreshing` 标志，测试当场抓到
  **冷启动 16 并发仍各算一遍**（16 次计算）。必须加「等待在飞计算」才是真单飞 ——
  **这就是并发测试不可省的原因**（`计划书/workflow_status.md:697-700`）。
- ⚠️ 单飞不能替代 TTL 分层：没有 TTL 分层，缓存的每次过期仍要重跑 6.3 秒。

**证据**
`service/site_visitor_tracker.go:515-546,552-611,628-644`;
`计划书/workflow_status.md:681-709`。

---

## ADR-010：服务端 SEO 注入块不得包含页面上不存在的内容

**背景**
前端是**纯客户端渲染**：`web/dist/index.html` 是一个壳，所有路由共用同一份字节。
实测（2026-10-11）6 个公开页返回的都是**完全相同的 3982 字节**，
其中 `<h1>` 数量为 **0**、`<link rel="canonical" href="/">` **写死**。
即：不是「内容不够好」，而是**爬虫根本看不到内容**。

**决策**
在服务端（SPA fallback 处）按**实际请求路径**注入：
- **meta**（title/description/canonical/OG）——以原始 HTML 为准；
- 一段**准确**的内容块（`<h1>` + 摘要 + 边界说明小节 + 站内导航 + 上游官方文档外链），
  位于 `#root` **之前**，由 `MutationObserver` 在 React 真正渲染后移除。

**理由**
- 内容块在 React 挂载后被移除，**用户看不到它** —— 因此它**必须与页面真实内容一致**，
  否则构成 cloaking。注入块只写「这个页面是什么」，**不写任何未经证实的数字**
  （`controller/spa_seo.go:33-36`）；
- `<meta name="description">` 与内容块的 Summary 都要求「准确、无夸大」
  （`controller/spa_seo.go:46-52`）；
- canonical 由**实际请求路径**决定而不是写死 `/`，这样未知路径也不会错误地把权重指向首页
  （`controller/spa_seo.go:64-66`）；
- **刻意不做**编造作者/资历/评价/评分、无真实内容的 `FAQPage`/`AggregateRating`/`Review`、
  堆关键词、批量泛化内链、购买外链（`计划书/workflow_status.md:519-523`）。

**后果（含代价）**
- ✅ 修复后 6 页字节数各不相同、每页 1 个 `<h1>`、canonical 各自绝对地址
  （`计划书/workflow_status.md:499-507`）。
- ✅ 「用户看不到」有真实浏览器验证作为安全前提：3 个路由的 `#spa-content`
  均已从 DOM 移除、`#root` 正常渲染（`计划书/workflow_status.md:513-515`）。
- ⚠️ **代价 1：内容块在源码里是 HTML 字符串，写坏不会让任何「存在性」断言失败。**
  og:url 替换串曾漏写开头的 `<meta property="og:url"`，整条标签被替换成非法文本
  `X content="https://…" />`；HTML 解析规则下 `<head>` 内非空白文本会**提前结束 head**，
  那段字符被渲染到页面顶部。**为什么 CI 全绿**：旧断言只检查 `og:locale` 是否存在，
  而 og:locale 是**拼接追加**的 —— 「断言了某标签存在 ≠ 断言了标签仍合法」
  （`计划书/workflow_status.md:543-568`）。
- ⚠️ **代价 2：可见附加内容必须放进容器内部**（`#spa-seo-content` 之内），
  否则清理脚本只删根容器、表格永远不被移除（v1.3.131 的真实缺陷）。
  JSON-LD 例外：它是 `<script>`，放进容器会在 React 挂载后被一并删掉，所以留在容器外
  （`计划书/workflow_status.md:627-643`）。
- ⚠️ 多语言**不做 hreflang**：本站 7 种语言是**同一 URL 的运行时切换**，
  没有独立的语言化 URL；给同一 URL 输出多语言内容会与 canonical 语义冲突
  （`controller/spa_seo.go:84-94`）。

**证据**
`controller/spa_seo.go:16-41,43-61,64-73,84-94`；
`计划书/workflow_status.md:495-523,543-568,627-643`。

---

## ADR-011：Redis 不可用时限流降级为内存，而不是 fail-closed 500

**背景**
限流器用 Redis 的固定窗口 Lua 脚本。2026-09-29 生产事故中，Redis 因大对象写入 OOM
而阻塞，限流 Lua `EVAL` **全部失败**，当时**直接返回 500**，246 次请求被转成 500；
`/api/status` 变慢后上游被 Caddy 摘除，最终**全站 503**。

**决策**
Redis 不可用时**降级为本进程内存限流**，而不是返回 500。内存桶与 Redis 桶**参数一致**。

**理由**
- **fail-closed 会把一次缓存故障放大成全站 5xx**（`middleware/rate-limit.go:112-116`）——
  这是把「限流器要保护系统」的初衷反过来伤到自己；
- 降级**保留限流效果**，只在跨实例部署时精度下降（`middleware/rate-limit.go:117-119`）；
- 降级发生时记 `LogError`，故障可见但不扩大
  （`middleware/rate-limit.go:126-128`）。

**后果（含代价）**
- ✅ 单实例部署下语义几乎等价（生产是单实例，`计划书/ops/deploy-zero-downtime.example.sh:34-47`
  起单个 `new-api` 容器）。
- ⚠️ **代价：多实例下每个实例各有一份内存桶，实际限流阈值被放大到实例数倍。**
  这是用「可用性优先」换「限流精度」的自觉取舍。
- ⚠️ 同类教训的第二半：**大对象不得进 Redis**。`GetLogsTraffic` 的 Redis 缓存值
  被限制为「只允许按日聚合行（≤days 组）」，绝不写入窗口内全量原始行
  （`controller/log.go:183-187`）。

**证据**
`middleware/rate-limit.go:112-131`；`controller/log.go:170-200`；
生产记忆 `freeapi-production-deploy.md`（2026-09-29 事故章节）。

---

## 12. 决策之间的依赖关系

```
ADR-001 relaykit 独立 module
   └── ADR-002 JSON 走 common 包装（relaykit 侧对应 kitutil，引擎经 Codec 注入）

ADR-003 额度转换集中           ← 所有计费路径的前提
ADR-004 表达式即计费契约        ← 依赖 ADR-003 的换算助手
ADR-005 缓存三硬约束            ← 复用响应缓存的开关/设置
ADR-006 缓存默认磁盘 + 进程内 LRU 硬上界   ← 是 ADR-005 的存储层
ADR-008 开关原子快照            ← ADR-005 的总开关、ADR-007 的模式选择都挂在这套机制上
ADR-009 SWR + 单飞              ← 依赖 ADR-006「缓存不该无界」的同一纪律
ADR-011 Redis 降级              ← 与 ADR-006「磁盘/Redis 后端」共享同一可用性取舍
```

---

## 13. 为什么只有这些 ADR（没写的决策及原因）

- **数据库三引擎兼容（SQLite / MySQL / PostgreSQL 同时支持）**：这确实是硬约束
  （`AGENTS.md:92`），但它是**约束**而非一次「决策 + 权衡」，且仓库里没有记录
  「当初为什么选三引擎」的理由。**只有约束条文、没有决策理由，故不写成 ADR。**
- **为什么用 Gin / GORM / React / Rsbuild**：`AGENTS.md:11-17` 只列了技术栈，
  没有记录选型对比或理由。**不编造理由。**
- **为什么任务插件用 Sobek 执行 JS**：`AGENTS.md:16,23` 只说明「用 Sobek 执行」，
  未找到设计权衡的书面理由。**待确认。**
- **限流器为什么用固定窗口而不是令牌桶**：代码里有固定窗口实现与其 Redis 键，
  但未找到「为什么不是令牌桶」的理由记录。**待确认。**
- **Caddy 健康检查参数（interval/timeout/fails）为什么取这些值**：
  只找到「这些参数对内存紧张很敏感、是 503 放大器」的事实记录
  （生产记忆 `freeapi-production-deploy.md`），没有取值的决策依据。**待确认。**

> 若后续有人补上上述理由，请以同样的「背景/决策/理由/后果/证据」格式追加为 ADR-012 起。
