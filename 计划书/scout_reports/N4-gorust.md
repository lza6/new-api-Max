# N4 侦察报告：Go/Rust 同语言工程模式（已完成，24 条 Top 清单）

## A. 错误处理（Go 直迁，5 项）
1. **共享哨兵错误"一值多门牌"**（beads beadserrors/errors.go）：错误词汇表独立成只依赖标准库的叶子包，各层 type alias re-export，errors.Is 全链路命中同一值——杜绝每层 errors.New 孪生
2. **错误信封按 API 面分类**（aisix error.rs）：proxy 面与 admin 面各一个公开信封，框架转换留消费层
3. **超时归因保留 cause chain**（aisix dispatch.rs）：errors.Is 判超时可能来自三种无关条件，包装必须携带原始链——对应渠道错误归因
4. **SDK 门面包**（casbin-gateway auth/auth.go）：第三方 SDK 调用收进薄包装包
5. **冷却/降级决策单点化**（aisix cooldown.rs）：决策放所有 dispatch 路径公共必经点，防新 relay 格式静默漏冷却（真实 H-1 类审计缺陷）

## B. 并发与状态（4 项）
6. **泛型并行管道**（trivy pkg/parallel/pipeline.go）：Pipeline[T,U]{numWorkers,onItem,onResult}，errgroup+worker 池——批量后台任务直接抽同款
7. RWMutex+快照迭代（agent-framework-go internal/concurrent/）
8. 资源池幂等 Start+context 取消（agent-deck mcppool）
9. **来源枚举防回环**（fastclaw internal/bus/）：消息带 Source（user/cron/heartbeat），合成消息不触发续跑探测

## C. 测试组织（5 项）
10. **conformance 契约套件**（beads backend/conformance/）：多后端兼容机器可验证——不支持的操作返回类型化 *ErrUnsupported 并用 RunUnsupportedContract 证明，覆盖率写进包文档。**对 new-api 三数据库验证矩阵最直接可操作**
11. **fixture 从引擎生成**（netwatch fixture.rs）：测试数据喂真引擎+FixedClock 产生，禁止手拼（手拼必然出现引擎算不出的矛盾数字）——对应计费/定价回归 golden 数据
12. 测试基建包化（trivy internal/{dbtest,gittest,cachetest}）
13. 接口驱动 fake+脚本化流式（autopentest-go fakeCaller）；文件即回归（NewAPI-Gateway 测试名=回归场景名）
14. feature 门控 mock（agtx → Go build tag）

## D. 模块边界（5 项）
15. aisix Hub-and-Bridge：provider 无关原语居中、翻译留 provider 侧——与 relaykit 同思路 Rust 验证，互为参照
16. 共享循环机制抽包（autopentest-go internal/agentloop/）动机写进包文档
17. core 与传输分离（agtx/aisix core 零框架依赖）
18. **优先级分层+同层加权随机不放回的路由计划**（NewAPI-Gateway model_route.go:238）：重试语义全在数据层预构建 plan[retry]，relay 层只消费
19. 权限策略编译化（casbin-gateway）：存储 UI 开关，执行编译出的 policy 文本

## E. 性能架构（aisix，Rust→Go 思想迁移）
20. **读路径无锁配置快照**：ArcSwap<Arc<Snapshot>>+双索引+generation 代际戳 → Go 对应 atomic.Pointer[Config]+不可变快照+版本号比较，适合渠道/定价/限流配置热更新
21. 预算检查 LRU+sticky fail-mode（控制面不可达时沿用最后决策 600s）——对应分布式计费预扣降级
22. 基线按环境指纹作用域化（netwatch baseline.rs）——渠道健康基线设计

## F. 工程治理
23. 治理文档分工+drift 标记（beads）；测试覆盖率写进包文档
24. CI 细节：多版本矩阵+-race -shuffle=on+AI API 一致性审查（agent-framework-go）；clippy -D warnings+schema drift+feature 组合可编译性（aisix）；11 个分域 workflow（dagu）

## 综合价值排序
aisix（性能架构+错误/冷却/预算设计）> beads（conformance+错误词汇表）> NewAPI-Gateway（同栈同域路由算法快照）> agent-framework-go（流式契约+测试纪律）> trivy/dagu（并行管道+测试基建）> netwatch（fixture 纪律）

## 勘误
- nezha 是 Tauri AI-IDE 而非 Go 监控面板
- clearwing 是 Python 项目（Rust 仅在 genai-pyo3 内核）
