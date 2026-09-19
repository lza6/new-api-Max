# 首字延迟对标 sub2api 与落地（2026-09-20）

## 1. 问题
同一上游 API：sub2api 首字约 3s，本站最少 10s（高峰更差）。用户要求对标并超越。

## 2. 已有实证（9-19 诊断，非本日新增）
- 上游 vendor 自身慢且波动（直连 2-22s）；网关中位数放大 ≈1.7x（约 1.5-2s），放大集中在**首包之前**（鉴权/渠道选择/计费/连接/CPU 调度）。
- 400 "inference request is invalid" = 上游真实拒绝，网关透传正确。
- 流式透传阶段（首包→done）正常，非瓶颈。

## 3. sub2api 架构可借鉴点（证据：D:\参考项目\sub2api\backend）
| sub2api 做法 | 文件 | 我们现状 | 结论 |
|---|---|---|---|
| HTTP 客户端池化：同配置复用单 Transport/Client，避免每次请求新建 | internal/pkg/httpclient/pool.go（sync.Map 缓存，注释明确"每次请求创建新客户端→统一池"） | 主 relay 已用 service.GetHttpClientWithProxySettings 缓存共享（api_request.go:500-509），仅 ali/ollama 等边缘适配器本地建 client | ✅ 已具备，无需改 |
| 连接池参数可配（MaxIdleConns/PerHost、IdleConnTimeout、Dial 5s、TLS 5s、ResponseHeaderTimeout） | pool.go 常量 + config.go 校验 | RelayMaxIdleConns=500/PerHost=100/Idle 90s/TLS 10s/ResponseHeaderTimeout=1800（init.go + http_client.go） | ✅ 已具备（参数略宽裕，方向一致） |
| 用量/配额**异步 flusher**（UserPlatformQuotaFlushIntervalMs，批量刷写） | config.go:1530 注释 | 预扣费 PreConsumeBilling 同步写库（service/billing.go:20 → quota_reserve.go:113 行锁 UPDATE） | ⚠️ **主要差距之一**：每次请求首包前同步 DB 写 + 行锁；改造涉及计费不变量（L3），需方案审批 |
| 渠道/路由选择轻量 | （ent 查询 + 内存态） | `GetRandomSatisfiedChannel` 在 `MEMORY_CACHE_ENABLED!=true` 时**每请求回源 DB**（channel_cache.go:124 → GetChannel） | 🔴 **本轮落地修复点** |
| 请求热路径精简 | - | 我们 TokenAuth 走 Redis 缓存（token.go:307），gin ReleaseMode（main.go:67），日志门控 | ✅ 已具备 |

## 4. 根因排序（为什么我们慢）
1. **渠道/模型选择每请求回源 DB**（MEMORY_CACHE_ENABLED 默认 false）：2C 小机高并发下每次请求一次 DB SELECT + 锁竞争，叠加 GC/CPU 争抢 → 首包前放大主力之一。
2. **预扣费同步写库**：每请求一行锁 UPDATE（quota_reserve），并发下排队。
3. **上游自身慢/波动** + 单渠道无备用（渠道 #20 yunshuzhilian.asia）。
4. 高负载 CPU 争抢（2 核）。

## 5. 本轮落地（仓库内，安全、低风险）
- docker-compose.yml：new-api 服务新增
  - `MEMORY_CACHE_ENABLED=true`（内存缓存渠道/模型选择，热路径零 DB 回源；启动已挂 InitChannelCache + SyncChannelCache 调度，main.go:102,105）
  - `SYNC_FREQUENCY=60`（缓存同步周期，默认 60s，与上项配套；原注释行移除）
- 代码零改动；Go/前端无需重编译（compose 环境变量即可）。

## 6. 服务器侧待执行（生产变更，需用户确认后我出命令清单）
1. 应用 compose 变更并重启 new-api 容器（保留数据卷，无数据迁移）。
2. 验证：同上游同参数交错直连/网关 curl，计时首字（复用 9-19 §2.2 方法），留档对比（目标：中位数放大 ≤1.1x）。
3. 监控渠道健康分与 frt 分布（B3-3 已有）。

## 7. 后续建议（需设计/审批，按契约不擅动）
- **预扣费异步化/信任路径**（对标 sub2api flusher）：对常规用户预扣改为"额度缓存内预扣 + 异步落库 + 失败补救"，或扩大 billing_session 已存在的"信任且不需要预扣费"路径；须守住计费不变量（负数/溢出/幂等），走 TDD + 全量计费回归。
- **备用上游渠道 + 开启 relay.stream_fallover**（首包超时按上游实际分布调大），缓解单渠道慢/波动与"do request failed"。
- 若首字 SLA 苛刻：实例升 4 核或换首字更稳的供应商。

## 8. 诚实边界
- 本轮未在生产服务器实测（部署属外部写入需审批）；本地无 docker，未跑 `docker compose config`（如环境有 docker 可补验）。
- "超越 sub2api" 的判定以 §6 实测数据为准，不做静态断言。