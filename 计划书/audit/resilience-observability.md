# A4 稳定 / 安全 / 可观测性审计

> 审计时间：2026-09-20 | 方式：主代理直审 + 实证（文件:行）| 范围：限流/并发/超时熔断/SSRF/防护/健康检查/可观测性/高可用建议

## 1. 限流与并发防护（✅ 到位）
- 全局真实并发桶：middleware/global-concurrency-limit.go（T6）——计数信号量 + 有界 FIFO 队列；满时排队而非直接 429；队列满/超时 → 429（GlobalConcurrencyWaitTimeout）。统计 active/waiting/limit 可读（GetGlobalConcurrencyStats），供前端水位展示。**注意**：单实例进程内有效，多实例需接 Redis（文档已注明本期不做）。
- 限流：middleware/rate-limit.go——Redis 固定窗口 Lua 原子脚本（INCR+EXPIRE+判定），按 IP/用户维度（rateLimit:v2:ip|user:mark），内存限流兜底；固定窗口边界可突发 2 倍（注释明确，勿改）。
- 模型/Token 限流：middleware/model-rate-limit.go、token-rate-limit.go 已存在。
- 依赖：router/relay-router.go 对 /v1 中继链挂 GlobalConcurrencyLimit → ModelRequestRateLimit → TokenRateLimit → Distribute。

## 2. 超时与重试（✅ 部分到位，熔断缺失）
- RELAY_TIMEOUT / RELAY_IDLE_CONN_TIMEOUT / RELAY_RESPONSE_HEADER_TIMEOUT：common/constants.go:169-181 + common/init.go 读 env；生产 compose 已设 RELAY_TIMEOUT=300（v1.2.34 起）。
- 429 自动退避：controller/relay.go:256-259——ShouldBackoff429(status, used429Retries, Relay429MaxRetries)，退避 RELAY_429_RETRY_DELAY=1000ms，最多 RELAY_429_MAX_RETRIES=2；生产已启用。
- **缺口**：无真正熔断器（Circuit Breaker）——渠道连续失败只靠冷却（service/channel_cooldown.go DecideCooldown，按 ErrClass 判冷却）与重试上限；建议后续加"连续 N 次 5xx/超时 → 渠道降级窗口"（P2）。

## 3. SSRF / Web 防护（✅ 到位）
- 渠道 base_url SSRF 二次解析：service/url_guard.go ValidateChannelURL——doRequest 发起前校验，私网/环回/链路本地/云元数据 CIDR 硬编码不可配置（B1-2）；URL_GUARD_ALLOWLIST 仅公网域名且跳过 DNS 二次校验（实现上 allowlist 直接放行，建议复核 allowlist+私网解析组合，P2）。
- Web 防护：middleware/web-protection.go——非 /v1 前缀的 Web 层防刷/限流/封禁；/v1 中继仅计量不计费（零开销），并顺带记录网络字节（流量统计）。
- 信任代理：middleware/trusted_proxies.go——TRUSTED_PROXIES 默认信任 loopback/RFC1918/ULA（兼容性默认，生产建议显式配置或 none，P2）。
- 恢复：middleware/recover.go RelayPanicRecover（返回 500 结构化错误）；request-id：middleware/request-id.go（X-Request-Id 贯穿）。

## 4. 健康检查（✅ 基本到位）
- /api/status（进程+依赖状态）、/api/status/test（DB ping + http_stats 并发/完成指标）、docker-compose healthcheck（wget /api/status 校验 success）。缺依赖级探活（Redis/Postgres 单独探活）——建议 P2。

## 5. 可观测性（✅ 部分，缺口明确）
- TTFT/性能指标：pkg/perf_metrics + controller/perf_metrics.go；端点 /api/perf-metrics/summary 与 /api/perf-metrics（pricing 模块，公开或登录）；/api/performance/stats（root）含运行统计、GC、日志查看。
- 日志：结构化日志 logger.*；按天日志文件；consume log 异步批量（LOG_FLUSH*）。
- 请求追踪：request-id 中间件已挂（X-Request-Id）。
- **缺口**：无 Prometheus 兼容 /metrics 导出端点；无 trace（OTel）；前端 perf 页面依赖 DB 聚合。建议：加一个轻量 /api/metrics（process/http/relay 计数）供大盘抓取（P2）。

## 6. 缓存与连接（✅ 生产已配）
- Redis（REDIS_CONN_STRING）+ 内存缓存（MEMORY_CACHE_ENABLED=true）+ 连接池收敛（SQL_MAX_OPEN_CONNS=64/IDLE=16/LIFETIME=300）+ 异步日志（LOG_FLUSH_ENABLED/INTERVAL/BATCH=500）。证据：/opt/new-api/docker-compose.yml（v1.2.35 部署实录）。

## 7. 高可用建议（单机 → 多实例演进，仅建议）
- Load Balancer：多实例 + 反向代理（Caddy/Nginx）做 TLS 终止与健康分流；注意全局并发桶需接 Redis 化。
- Caching：静态资源上 CDN；模型列表/定价/画像读走 Redis（已有基础）。
- DB：主从复制（PG streaming）+ 连接池按角色拆分；logs 大表按时间归档/分区。
- MQ：目前无强需；429 退避与异步日志已用进程内队列。若引入任务平台事件流再评估 Kafka/PG notify。
- Rate Limiting / Circuit Breaker / Health Checks / Observability：见上，Breaker 与 Metrics 为最优先补位。

## 结论
- P0：无（核心防护链路在位）。
- P1：无阻断；建议把「契约/慢查询/错误人话」三条优先处理（见 A1/A2/A3）。
- P2：熔断器、allowlist+私网组合复核、TRUSTED_PROXIES 显式化、/metrics 导出、依赖级健康检查。
