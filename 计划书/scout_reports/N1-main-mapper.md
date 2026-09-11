# N1 侦察报告：主项目现状全面评估（已完成）

## 一、架构图谱

单体 Go 网关 + 嵌入式 React SPA，模块按技术分层而非领域分层，但 layer 内聚类良好。

```
main.go              启动编排：初始化→goroutine 后台任务→Gin 路由→优雅停机
├── router/          9 个路由文件：api(250+ 端点)/relay/task/video/plugin/authz/channel/dashboard/web
├── middleware/      38 文件：auth/distributor(渠道选择)/rate-limit/turnstile/task_plugin/performance
├── controller/      118 文件：relay(904行)/channel/setup/subscription/task_plugin/plugin_protocol
├── service/         91 文件：billing/billing_session/task_polling/task_billing/tiered_sette/authz(Casbin)/oauth
├── model/           106 文件，migrateDB AutoMigrate 35 张表 (model/main.go:337-374)
├── relay/           45 个渠道适配器目录(channel/)+ handler + helper(验证/计价/流扫描)
│   └── channel/task/jsplugin  JS插件适配器桥
├── relaykit/        独立 Go 模块：126 文件 33k 行，协议 DTO + relayconvert 转换器，零宿主依赖
├── pkg/             billingexpr(表达式计费)/cachex(混合缓存)/ionet/jsplugin(Sobek 引擎)/perf_metrics
├── plugins/tasks/   10 个内置 JS 任务插件(共 4816 行)：kling/sora/veo/jimeng/hailuo/vidu/alibaba/doubao/google/suno
├── setting/         config.GlobalConfig 统一注册的配置模块(billing/ratio/console/operation/payment/task_pricing...)
├── constant/ dto/ types/ common/ logger/ i18n/ oauth/ electron/
└── web/             React19+Rsbuild+TanStack+Zustand+BaseUI+Tailwind4，26 features/62 路由文件/64 UI 组件
```

## 二、后端优点（带证据）

1. **计费安全不变量防御纵深**：common/quota_math.go:13-17 集中饱和钳制（int32 单请求边界 + 1<<53 钱包边界），QuotaClamp 审计 + *Checked 变体（quota_math.go:43-72），饱和事件写日志 other.admin_info.quota_saturation（log_info_generate.go:21-33）；AGENTS.md:122-132 规范化
2. **BillingSession 生命周期清晰**：service/billing_session.go:29-42 互斥锁管理 settled/refunded/fundingSettled 状态机，两步提交防重复退款
3. **表达式计费系统成熟**：pkg/billingexpr/expr.md（393 行设计文档）+ AST 变量自动检测（tiered_settle.go:26-40）+ 版本化 + 1190 行测试
4. **JS 插件协议边界干净**：宿主拥有连接/轮询/计费/落库，插件只做数据变换；pkg/jsplugin/engine.go 5s 调用超时、并发准入、HookError 消毒；路由冲突原子发布（routing.go:22-56）
5. **多 master 部署意识**：SystemTask DB-lease 跨节点去重（system_task.go:17-28）；Casbin 周期同步（main.go:117）；SystemInstance 节点上报
6. **三数据库兼容系统性执行**：model/main.go:50-65 集中保留字列；locking.go lockForUpdate 跳过 SQLite；迁移自带 dialector 分支
7. **限流分层完整**：全局 API/Critical/用户级/Search/model-rate-limit 多层（rate-limit.go:167-249）

## 三、后端问题（带证据）

1. **encoding/json 直接引用广泛违反自家规范**（AGENTS.md:80-91 明令）：relay/channel/{coze,dify,openrouter,palm,replicate,baidu,ali,aws,codex,cohere,cloudflare} + service/midjourney.go:284-355 + model/channel.go:324 + model/prefill_group.go:49 数十处
2. **Midjourney 双轨并存**：TaskPlatformMidjourney 常量 + 旧轨 1647 行（mjproxy_handler.go 701 + controller 329 + model 249 + service 368）与新 task-plugin 体系功能重叠
3. **RelayInfo God Object**：relay/common/relay_info.go 1184 行，所有请求态聚合
4. **业务逻辑落在中间件层**：middleware/distributor.go 735 行，渠道选择/渠道 pin/身份过滤/模型映射全在中间件
5. **倍率体系双轨制**：legacy model_ratio.go:28（243 行硬编码表）+ builtin_billing.go 表达式并存，GetBillingMode 靠"管理员是否配置过 legacy"推断优先级
6. **TASK_PRICE_PATCH 环境变量补丁**：common/init.go:214-226 注入，3 处消费，定价隐式依赖运维
7. **测试分布不均**：45 个渠道适配器仅 openai(5)/claude(4)/gemini(3)/ali(2) 有测试；e2e/ 仅 1 个文件
8. **依赖偏旧**：gin v1.9.1、go-redis/v8（go.mod:31,38）
9. **自研日志简陋**：logger/logger.go 189 行手写，无 slog/zap 结构化

## 四、前端优点
1. web/AGENTS.md 221 行可执行规范，"先检索再复用最后新增"强制流程
2. 26 个 features 统一 api.ts/components/hooks/lib/types.ts 结构；section-registry 模式复用
3. 118 个测试文件 962 个用例，测试规范禁止快照滥用和 sleep 竞态
4. handleServerError 错误处理体系统一（保留服务端原因+cause 去重）

## 五、前端问题
1. i18n 键即英文源串：zh.json 6218 行、7 语言全量平铺，改文案=改 7 文件
2. system-settings/models 41 个组件文件承载倍率/按次/表达式三套编辑器，逼近可维护上限
3. Zustand store 仅 4 个，跨 feature 状态无统一视图
4. e2e 缺位：web 侧无 Playwright，62 路由 0 条浏览器路径测试

## 六、计费/配额体系成熟度：高（同类第一梯队）
- 三种计费模式统一（ratio/price/tiered_expr）；表达式 15 内置函数+版本化
- 预扣费→结算→退款全链路：BillingSession 两步提交 + FundingSource 抽象
- 溢出防护五层：请求边界→decimal→饱和钳制+审计→倍数白名单→无符号上限
- 5 种支付渠道 + webhook 合规确认
- 薄弱：双轨制、无成本价→利润率视角、订阅预扣费链路复杂度高

## 七、测试与工程化数字
| 维度 | 数字 |
|------|------|
| Go 测试 | 259 个 _test.go，~69,217 行测试 / ~215,000 行总 Go |
| 分布 | controller 44 / model 38 / service 21 / plugins 14 / middleware 11 / relaykit 32 |
| 前端 | 118 个测试文件，962 用例，33 __tests__ 目录 |
| e2e | Go 1 个；前端 0 |
| CI | 6 工作流（ci.yml 无 lint 步骤） |
| 规模 | 提交 6336 次，最近 2026-09-09，活跃维护 |

结论：单元/集成层成熟（尤其计费、模型、插件），lint 未进 CI、e2e 与浏览器验证缺位。

## 八、小白用户体验薄弱点（7 条）
1. **无部署后 onboarding**：setup 向导仅 4 步，完成后无"添加第一个渠道→生成令牌→发第一个请求"引导流
2. **默认 root 密码弱**：createRootAccountIfNeed 固定 root/123456（model/main.go:47-62）
3. **费用透明度分层失衡**：expr_b64/matched_tier 只进 admin_info，普通用户对 tier 命中原因/长上下文加倍/缓存折扣全黑盒；quota_saturation 用户不可见
4. **定价页概念负担重**：11 个筛选维度，无"调用一次约花多少钱"估算器
5. **文档内嵌极薄**：docs/ 仅 4 子项，首页指向外站 docs.newapi.pro，断网/私部署帮助断裂
6. **渠道调试体验分散**：渠道测试与请求模拟分离，失败原因与日志页无跳转关联
7. **无成本告警默认值**：额度通知需手动开启

## 九、扩展性评估：新增"非 LLM 能力"接入成本

**高度可复用机制（最强扩展资产）**：
| 机制 | 复用方式 | 成本 |
|------|---------|------|
| JS 任务插件（Sobek） | 单文件 plugin.js：meta.routes+protocols+fetchMode；kling 插件 477 行完成一个平台 | 低：免 Go 代码、热更新、免前端 |
| Task 表+轮询 | Task{Platform/Action/Status/Data/Properties}，8 类 poll failure 分类 | 已抽象 |
| 计费表达式 | usageSchema 声明 host 单位+OtherRatios+billingexpr+三段结算 | 已抽象 |
| Artifacts 产物链 | listArtifacts/buildContentRequest+task_artifact_access 鉴权 | 已抽象 |

**硬编码障碍**：
1. "视频"概念渗入通用层：model/task.go:12-28 ToVideoStatus 硬映射；TaskAction 枚举全是视频动作；video-router 专用路由
2. 交互模型单一：task 假设"提交→轮询→终态→计费"；webhook 回调/双向状态机/幂等重放无位置（现有 5 个支付 webhook 各自为政，无通用 webhook 注册机制）
3. Midjourney 旧轨是新平台反面教材（但证明 TaskPlatform 字符串开放、平台解析纯查插件 registry）
4. Playground 只有 chat；非对话型能力需新建 feature
5. 计费表达式拒绝 Realtime/task 场景用 fixed()（expr.md:125-127）

**接入成本判断**：异步"提交-轮询"型非 LLM 能力（文档解析/批量转录/爬虫）= 低成本（纯 JS 插件 1-2 天）；交互/webhook 型（电商动作）= 高成本（需新的通用 webhook/事件子系统）

## 十、技术债清单（13 项）
1. JSON 规范大面积违反 | 2. Midjourney 旧轨 1647 行 | 3. RelayInfo 1184 行 God Object | 4. distributor 735 行承载业务 | 5. 双轨定价 | 6. TASK_PRICE_PATCH | 7. USD2RMB=7.3 硬编码（model_ratio.go:19） | 8. 渠道适配器测试断层 | 9. e2e 缺位 | 10. CI 无 lint 门禁 | 11. 依赖滞后 | 12. 全局可变状态 | 13. 巨型文件（controller/relay.go 904、task_polling.go 836、distributor.go 735、relay_info.go 1184）

**总体判断**：工程成熟度显著高于典型开源 AI 网关——计费/插件/多库兼容接近生产级纪律；主要债务集中在"新旧双轨并存"与"巨型文件"，均非结构性死结。向非 LLM 能力扩展的最大机会窗口是 JS 插件体系（已经 10 个内置插件验证），最大缺口是通用 webhook/事件机制与部署后 onboarding。
