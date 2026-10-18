# SOP — 运维手册

> 面向「半夜被叫起来处理问题的人」。目标是让一个**没有上下文的人**在 5 分钟内
> 判断「是不是真的挂了」，在 15 分钟内定位到「是网关、上游、磁盘还是内存」。
>
> 适用范围：生产实例 `freeapi.tingfengai.art`（服务器 `154.37.208.171`，
> 部署栈 `/opt/new-api/`，Caddy v2 反代）。
>
> **本文所有命令与参数都来自本仓库真实文件或既有事故记录，并标注 `file:line`。
> 未经验证的内容一律标「待确认」，不编造。**
>
> 最后核对：2026-10-12（对照 `VERSION` = `v1.3.135`）。

---

## 0. 三条纪律（先读这个）

1. **不要在服务器上直接改生产计费配置做测试。** 计费配置改库即生效（60s 内热更新），
   曾导致 225 次真实请求多扣费、补偿 $20.86。详见 §6。
2. **本机 `curl.exe` 全局无法完成任何 HTTPS 握手**（连百度也是 `000`）。
   判断线上可达性**不要用本机 curl**，用服务器侧自检或 `node -e`。详见 §5.1。
3. **「用户说进不去」不等于站点挂了。** 先取「真实业务流量」这条最硬证据，再下结论。详见 §5.1。

---

## 1. 速查表：常用一条命令

在服务器上执行（`docker` 命令均以容器名 `new-api` 为准，见 `docker-compose.yml:20`）。

| 目的 | 命令 | 依据 |
|---|---|---|
| 查**运行版本**（最可信） | `docker exec new-api /new-api --version` | `Dockerfile:39`（二进制位于 `/`）、`Dockerfile:50`（`ENTRYPOINT ["/new-api"]`）、`common/init.go:20`（`-version` 标志） |
| 查**响应头版本**（外部视角） | `curl -sI https://freeapi.tingfengai.art/api/status \| grep -i x-new-api-version` | `middleware/cors.go:67` 设置 `X-New-Api-Version` |
| 查**容器状态 / 何时起的 / 重启次数** | `docker ps --filter name=new-api` | 容器名 `new-api`（`docker-compose.yml:20`） |
| 查**存活探针**（进程活着即 200） | `curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:3000/healthz` | `router/main.go:29` |
| 查**就绪探针**（主库+日志库可达才 200，否则 503） | `curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:3000/readyz` | `router/main.go:30`、`router/main.go:26-27` |
| 查**应用状态**（Caddy 健康检查用的就是这个） | `curl -s http://127.0.0.1:3000/api/status` | `router/api-router.go:26`、`deploy/Caddyfile.example:59-70` |
| 看**最近日志** | `docker logs --since 10m new-api` | — |
| 看**磁盘** | `df -h /` 和 `docker system df` | `计划书/ops/disk-guard.example.sh:26,51`、`计划书/ops/deploy-zero-downtime.example.sh:111` |
| 看**内存 / 重启** | `docker stats --no-stream new-api` | 部署脚本给容器设 `--memory 900m`（`计划书/ops/deploy-zero-downtime.example.sh:44`） |
| 看**渠道健康分与熔断态** | `GET /api/system-info/channel-health`（需 root 会话） | `router/api-router.go:367` |
| 看**内部指标**（需先开 `METRICS_ENABLED`） | `curl -s http://127.0.0.1:3000/metrics` | `middleware/metrics_auth.go:36-38`；**生产已开启**（2026-10-19 实测 200，含 `training_capture_*`） |

> ⚠️ 容器内监听端口是 **3000**（`Dockerfile:44`），但**宿主端口是 3000/3001 交替的**
> ——蓝绿部署会改 Caddy 上游。命令里出现的宿主端口请以当前 Caddyfile 为准（§3.1）。

### 1.1 磁盘自动守卫（已常驻）

每日 04:17 由 cron 执行 `/opt/new-api/disk-guard.sh`，只清理 + 告警，**不擅自删数据**：

```bash
# 安装方式（脚本头部注释，计划书/ops/disk-guard.example.sh:11-16）
install -m 0755 disk-guard.example.sh /opt/new-api/disk-guard.sh
cat > /etc/cron.d/new-api-disk-guard <<'EOF'
SHELL=/bin/bash
PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
17 4 * * * root /opt/new-api/disk-guard.sh >> /var/log/new-api-disk-guard.log 2>&1
EOF
```

它做的事：清 72 小时前的构建缓存、保留最近 4 个 `new-api` 镜像（正在运行的永不删）、
清悬空镜像与停止超 7 天的容器、删 `/root` 下超 7 天的构建残留、`journalctl` 收敛到 200MB；
磁盘使用率 ≥85% 时**只打印 WARNING**。
可覆盖参数：`KEEP_IMAGES` / `WARN_PERCENT` / `BUILD_KEEP_DAYS`
（`计划书/ops/disk-guard.example.sh:22-24`）。

> 📌 `计划书/ops/*.example.sh` 是**脱敏模板**；含凭据的真实脚本只存在于服务器
> `/opt/new-api/`，**不入库**（`计划书/ops/deploy-zero-downtime.example.sh:5-7`）。
> 本文只写用法，不写凭据。

---

## 2. 部署（蓝绿零停机）

脚本：`/opt/new-api/deploy-zero-downtime.sh <tag>`（脱敏模板：
`计划书/ops/deploy-zero-downtime.example.sh`）。

### 2.1 流程（每一步的判定条件都来自脚本原文）

0. **先构建镜像（脚本不会自动 build/pull）**：
   ```bash
   cd /opt && git clone --depth 1 --branch <tag> <repo> new-api-src-<tag>
   cd new-api-src-<tag> && docker build -t new-api:<tag> .
   ```
   在 4C4G 上约 8–20 分钟；构建期间旧容器继续服务。
   确认镜像存在：`docker images new-api | grep <tag>`。
1. **探测当前上游端口，选另一个**：读 Caddyfile 里的 `127.0.0.1:300x`，
   在 `3000`/`3001` 之间交替（`计划书/ops/deploy-zero-downtime.example.sh:23-27`）。
2. **起新容器**（旧容器全程不停）：
   - 容器名 `new-api-new`，非 root（uid/gid `10001`）；
   - 先 `chown -R 10001:10001 "${PROD_DIR}/data" "${PROD_DIR}/logs"` —— 否则进程无法落盘
     （日志 / 本地图床 / SQLite）（`计划书/ops/deploy-zero-downtime.example.sh:30-33`，
     对应 `Dockerfile:36-37,43,46-48`）；
   - 资源限制 `--memory 900m --memory-swap 900m`、`GOMEMLIMIT=512MiB`、
     `--cap-drop=ALL --security-opt=no-new-privileges=true`
     （`计划书/ops/deploy-zero-downtime.example.sh:40,44-46`）。
3. **等健康**：轮询 `http://127.0.0.1:<新端口>/api/status`，最多 90s（45 次 × 2s）
   （`计划书/ops/deploy-zero-downtime.example.sh:50-55`）。
4. **切 Caddy 上游**：`sed` 替换端口 → `systemctl reload caddy`（毫秒级）
   （`计划书/ops/deploy-zero-downtime.example.sh:58-59`）。
5. **外网自检**：以真实域名 + 真实 TLS 访问自己的公网入口（见 §2.2）。
6. **停止旧容器并改名**：`docker rm -f new-api` → `docker rename new-api-new new-api`
   （`计划书/ops/deploy-zero-downtime.example.sh:73-74`）。
7. **自动清理**：清构建缓存、保留最近 `KEEP_IMAGES`（默认 4）个镜像、删 `/root` 源码残留
   （`计划书/ops/deploy-zero-downtime.example.sh:91-111`）。

> ⚠️ **`NODE_TYPE` 现状（务必理解，不要照抄旧结论）**：
> `common.IsMasterNode` 的判据是 `NODE_TYPE != "slave"`，即**不设置就是 master**。
> 当前生产脚本**没有**设置 `NODE_TYPE` → 蓝绿重叠期间新旧两个容器都是 master，
> 后台任务（订阅重置 / 认证清理 / `authz.Init`）会**重复执行**。
> 旧文档要求“新容器必须 slave”，但那样在切流并删除旧容器后**没有任何 master**。
> 两种方案二选一，变更前需评估：
> (a) 接受重叠期短暂双 master（当前生产做法）；
> (b) 新容器 `NODE_TYPE=slave`，切流删旧容器后再重启新容器为 master。
> **不要只做一半**（只加 slave 不补 master 会让后台任务永久停摆）。

### 2.2 部署后自检（脚本已内建，人工再复核一次）

脚本内建的自检是：以 `--resolve` 把真实域名指向 `127.0.0.1`，走真实 TLS 请求
`https://<domain>/api/status`，**非 200 立即回滚**（`计划书/ops/deploy-zero-downtime.example.sh:62-71`）。

人工复核清单：

```bash
# 1) 上游端口已切换（应为新端口）
grep -oE "127.0.0.1:300[0-9]" /etc/caddy/Caddyfile

# 2) 容器 healthy 且重启次数为 0
docker ps --filter name=new-api --format '{{.Status}} {{.Ports}}'

# 3) 探针
curl -s -o /dev/null -w 'healthz=%{http_code}\n' http://127.0.0.1:3000/healthz
curl -s -o /dev/null -w 'readyz=%{http_code}\n'  http://127.0.0.1:3000/readyz

# 4) 版本一致（服务器侧自检 = 外部用户视角）
curl -sI https://freeapi.tingfengai.art/api/status | grep -i x-new-api-version

# 5) 未认证的新接口应 401（证明鉴权还挂着）
curl -s -o /dev/null -w '%{http_code}\n' https://freeapi.tingfengai.art/api/option/feature-switches
```

### 2.3 回滚

**回滚有两条路径，按故障阶段选：**

| 阶段 | 回滚动作 | 依据 |
|---|---|---|
| 新容器已起、**流量还没切** | 什么都不会发生 —— 旧容器仍在上游。直接 `docker rm -f new-api-new` 即可 | `计划书/ops/deploy-zero-downtime.example.sh:29-71`（切流是独立的一步） |
| 已切流、**外网自检失败** | 脚本**自动**把 Caddy 上游改回旧端口 + reload + 删新容器 | `计划书/ops/deploy-zero-downtime.example.sh:65-71` |
| 已切流且已停旧容器，**上线后发现业务问题** | 手工把 Caddyfile 上游指回**上一个仍在运行的容器**；若该容器已删，则用上一版本镜像 `new-api:<上一个 tag>` 重新起容器并切流 | 镜像保留 4 个即为此（`计划书/ops/deploy-zero-downtime.example.sh:87,91`） |

> 📌 **回滚窗口 = 保留的镜像数**。默认 `KEEP_IMAGES=4`（当前 + 3 个回滚窗口）。
> 把 `KEEP_IMAGES` 调小会缩短回滚能力 —— 磁盘紧张时优先清**构建缓存**而不是镜像
> （构建缓存才是磁盘真凶，见 §5.7）。
>
> **待确认**：仓库内存在两份历史脚本 `scripts/rolling-update-newapi-v3.sh`（双端口
> `:3000/:3002` 方案）与 `/opt/new-api/rollback.sh`（早期 `docker compose` 方案）。
> 生产**当前使用**的是 `deploy-zero-downtime.sh` 蓝绿方案；上述两份是否仍在用需现场确认。

### 2.4 前端改动必须重建镜像

前端产物经 `go:embed web/dist` 打进二进制（`main.go:43`、`main.go:46`），
构建时由 Docker 把构建产物拷进去（`Dockerfile:27,39`），**不是运行时热更新**。
改了 `web/` 就必须重新构建镜像并走一次部署，改配置类 env 开关也需要新二进制。

---

## 3. Caddy 上游与安全头

生产 Caddyfile 在**宿主机** `/etc/caddy/Caddyfile`（`deploy/Caddyfile.example:3`），
仓库内只是示例副本。

### 3.1 上游必须是单端口

```caddyfile
reverse_proxy 127.0.0.1:3000 { health_uri /healthz ... }
```

- 健康检查用轻量 `/healthz`（比 `/api/status` 少读配置/DB）（`deploy/Caddyfile.example:69-70`）；
- `flush_interval -1` 保证 SSE 流式不被缓冲（`deploy/Caddyfile.example:75`）。

### 3.2 静态资源长缓存

`/assets/*` 走 `Cache-Control: public, max-age=31536000, immutable`
（哈希文件名，`deploy/Caddyfile.example:57-58`）。

### 3.3 安全头（CSP 有坑，改前必读）

- HSTS / `X-Content-Type-Options` / `X-Frame-Options` / `Referrer-Policy` /
  `Permissions-Policy` / `-Server` 见 `deploy/Caddyfile.example:22-34`。
- **CSP 必须同时放行 `blob:` 于 `img-src` 与 `connect-src`**
  （`deploy/Caddyfile.example:47-52`）：
  - `img-src` 缺 → 上传附件缩略图不显示（`URL.createObjectURL` 被拦）；
  - `connect-src` 缺 → 游乐场把 `blob:` 附件 fetch 成 `data:` 时被拒，
    表现为**「选完图点发送毫无反应」**且不报错。

> 排障口诀：页面「点了没反应」先看浏览器 console 的 CSP 违规，别猜业务逻辑。
> （来源：生产记忆 `new-api-csp-blob-and-latency-rootcause.md`，v1.3.109 已把附件转换的
> 静默 `.catch(() => {})` 改为上报 `onError`。）

---

## 4. 排障清单（症状 → 根因 → 查证）

### 4.1 网站打不开 / 连接被重置

**先分清「站点挂了」还是「你这条路不通」。**

| 视角 | 命令 | 说明 |
|---|---|---|
| ① 真实业务流量（**最硬证据**） | 查 `logs` 近 24h 逐时请求数 | 有连续流量 = 站点在服务 |
| ② 服务器自检 | `docker ps`；`curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:3000/healthz` | 进程活否 |
| ③ 服务器走公网访问自己 | `curl -s https://freeapi.tingfengai.art/api/status`（**在服务器上执行**） | 等价外部用户视角 |
| ④ 换出口 | 经本机代理访问 | 通 → 说明是本地链路问题 |

**已知根因**（2026-10-10 实测定论）：主站完全正常，是**本机到该域名的直连 TLS 被
路径上的中间设备注入 RST**。判定手法：固定目标 IP 只换 SNI —— 别的 SNI 能握手、
只有该域名在约 1 RTT 内被 RST；服务器侧 `tcpdump` 显示服务器已正常回 `ServerHello`，
RST 是伪造源 IP 注入的。
本机浏览器同样受影响，原因是系统代理的 `ProxyOverride` 里有 `*.*.art`
（`freeapi.tingfengai.art` 命中该通配而被排除出代理、走直连）。
（来源：生产记忆 `tingfengai-domain-rst-20261010.md`。）

> ⚠️ **不要用本机 curl 判断**：本机 `curl.exe`（schannel）已全局失效，
> 连 `https://www.baidu.com` 都返回 `000`。要探 HTTPS 请用 `node -e` 或
> `openssl s_client`（来源：生产记忆 `local-curl-schannel-broken.md`）。

### 4.2 全站 503 —— 先看内存和 Caddy 日志

**根因链（2026-09-28 事故）**：小机内存被打爆 → swap 打满 → 疯狂换页 → `/api/status`
响应 >2s → Caddy `health_timeout 2s / health_fails 3` 把上游判死 →
`no upstreams available` → 全站 503。

```bash
free -m                                   # 物理内存 + swap
docker stats --no-stream                  # 各容器内存
journalctl -u caddy --since '-30m' | grep -i 'no upstreams'
```

Caddy 健康检查参数对内存紧张**很敏感，是 503 放大器**（`deploy/Caddyfile.example:70-74`：
`health_interval 5s / health_timeout 3s / health_fails 3`）。

**根因链（2026-09-29 事故）**：Redis `maxmemory` 仅 48MB，而 `GET /api/log/traffic?days=30`
把窗口内全量 44 万行（`other` JSON 290MB+）整体塞进 Redis → `OOM command not allowed`
→ Redis 阻塞 → 限流 Lua `EVAL` 全部失败 → 当时**限流器 fail-closed 直接 500**
（246 次）→ 全站 503。

> ✅ **两处都已在代码中修掉，不再是当前行为**：
> - 限流器 Redis 不可用时**降级为进程内内存限流**，不再 500（`middleware/rate-limit.go:112-131`）；
> - `GetLogsTraffic` 已改为 **SQL 侧按日聚合**，且 Redis 缓存值**只允许按日行（≤days 组）**，
>   绝不写入全量原始行（`controller/log.go:170-200`）。

### 4.3 首字慢 / 空响应 —— 用日志字段拆「网关 vs 上游」，不要猜

消费日志的 `other` 里有决定性字段（查询模板见下）：

- `upstream_ttfb_ms`：真实上游首字节耗时；
- `frt`：到首字总耗时；**`frt − upstream_ttfb_ms` = 网关自耗**；
- `timeline_stages`：inbound / upstream_first_byte / total_to_first_response；
- `stream_status.end_reason`。

```sql
-- PostgreSQL（生产库）
SELECT channel_id, count(*),
  round(avg((other::json->>'upstream_ttfb_ms')::numeric)) AS up_ttfb,
  round(avg((other::json->>'frt')::numeric
      - COALESCE((other::json->>'upstream_ttfb_ms')::numeric,0))) AS gw_overhead
FROM logs WHERE type=2 AND is_stream=true AND created_at > <epoch>
  AND other::json->>'upstream_ttfb_ms' IS NOT NULL GROUP BY 1;
```

**2026-10-08 定论**：全站近 1h 网关自耗约 3s（≈24%），**上游是瓶颈**
（直连上游实测：小请求 62ms，3.4MB→8.8s，9.18MB→上游自己 502）。
「3m/6m 的等待」不是网关重试叠加，而是上游自己拖 ＋ 渠道配置了
`disable_stream_first_token_timeout=true`（缓冲可持有到 `STREAMING_TIMEOUT` 默认 300s）。
（来源：生产记忆 `new-api-csp-blob-and-latency-rootcause.md`。）

### 4.4 计费看起来不对

**先看消费日志 `other` 里的计费决策字段**（`计划书/ops/token-billing-group.md:80-89`）：

| 字段 | 含义 |
|---|---|
| `billing_mode` | `tiered_expr` = 走表达式计费 |
| `matched_tier` | 命中的档位（`tok_lt200k` / `tok_200k_500k` / `tok_ge500k` / `per_request`） |
| `billing_unit` | `request`（按次）或 `token` |
| `fixed_price` | **本次实际单价（USD）** —— 前端价格列优先显示它 |
| `group_ratio` | 分组倍率 |

常见症状对照（`计划书/ops/token-billing-group.md:71-78`）：

| 现象 | 原因 | 处理 |
|---|---|---|
| `分组 X 下模型 Y 无可用渠道` | 渠道 `group` 未加该分组，或 `abilities` 缺行 | 补渠道分组 + abilities（同文件 §3 第 3/4 步） |
| 扣费仍按次价 | token 分组没设成目标分组，或不在该用户可用分组白名单 | 查 `tokens."group"` 与 `options.UserUsableGroups` |
| 日志显示固定价 | 该请求走的是表达式 else 分支（分组不匹配） | 属正常；看 `other.matched_tier` 确认 |
| 改价后**其他分组**扣费也变了 | else 分支没写成当前按次价 | **立即回滚表达式** |

排查时看 `other` 有一个前提：**非管理员看不到 `admin_info`**（该字段在日志视图里被剥离）。

### 4.5 内存持续上涨

先确认是否有观察手段：

```bash
curl -s http://127.0.0.1:3000/metrics | grep -E '^process_(goroutines|memory_heap_objects)'
```

判读：`process_goroutines` 持续上升不回落 → 大概率 goroutine 泄漏（如未 stop 的 ticker）；
`process_memory_heap_objects` 与 alloc 同步上涨 → 大概率有「只加不减」的 map/slice。
（指标清单来源：`计划书/workflow_status.md` Batch-10 章节。）

> ✅ **生产已开启 `METRICS_ENABLED`**（2026-10-19 实测 `/metrics` 返回 200，
> 含 `training_capture_*` 系列）。若某实例返回 SPA HTML，按 §5.6 检查该实例的 env。

已修复的 4 处**真实**无界点（曾经的内存增长源）：`web_protection_tracker` 的 `banCache`
（8192 硬上界）、`url_guard` 的 `ssrfCache`（4096）、`tokenizer` 的 `tokenEncoderMap`
（512）、`token-rate-limit` 的 `tokenQBSBuckets`（空闲淘汰）
（`计划书/workflow_status.md` Batch-10 第二章）。

容器内存上限 `900m`（`计划书/ops/deploy-zero-downtime.example.sh:44`）。

### 4.6 磁盘持续上涨

**真凶通常不是镜像，是 Docker 构建缓存**（2026-10-11 实测：54G/78G→8.3G，
其中构建缓存独占 44.31G）：

```bash
df -h /
docker system df
docker system df -v 2>/dev/null | head -40
```

处置：跑一次部署（尾部自动清理，`计划书/ops/deploy-zero-downtime.example.sh:91-111`），
或手工执行 `disk-guard.sh`（§1.1）。**保留镜像数不要低于 4**（回滚窗口）。

### 4.7 某渠道大量失败

三层机制，按「谁先摘除」排序：

1. **冷却**：按错误类定时冷却（`CHANNEL_COOLDOWN_{AUTH,RATE_LIMIT,SERVER_ERROR,TIMEOUT}_SECONDS`
   可配，`service/channel_cooldown.go:51-53,84-98`）；到期由 `cooldownRecoverySweep` 自动恢复
   （`service/channel_cooldown.go:184,201`）。
2. **熔断**：按「连续失败次数」摘除；默认 `CHANNEL_CIRCUIT_FAILURE_THRESHOLD=5`、
   `CHANNEL_CIRCUIT_OPEN_SECONDS=120`（`service/channel_circuit_breaker.go:47-48`）。
   **行为开关 `CHANNEL_CIRCUIT_BREAKER` 默认 off**（`service/channel_circuit_breaker.go:29`）。
3. **健康分加权**：`CHANNEL_HEALTH_WEIGHTED_LB`（默认关）。

查证：

```bash
# 渠道健康分 + 熔断态（需 root 会话）
curl -s -b cookie.txt https://freeapi.tingfengai.art/api/system-info/channel-health
docker logs --since 30m new-api | grep -iE 'channel|cooldown|circuit'
```

**已知坑（真实缺陷，已修）**：熔断探针曾由**过滤谓词**消费 —— 谓词每候选/每轮多次调用，
副作用会把探针令牌烧掉，导致渠道**永不恢复**。修法：`ChannelCircuitAllows` 改**纯判定**
（不消费探针），探针改在 `middleware/distributor.go` 选定后**单飞消费**
（来源：生产记忆 `new-api-prod-safety-batch-007.md` C1 条）。

### 4.8 会话随机过期 / 用户被反复登出

**根因（真实代码）**：未设置 `SESSION_SECRET` 时，**每个进程启动时各自生成随机密钥**，
于是 A 实例签发的会话 JWT / refresh HMAC 在 B 实例校验失败 —— 多实例 / 滚动发布下
用户会话被随机打失效，表现为「会话过期」循环。
（`common/init.go:59-66`，中文警告原文：「未设置 SESSION_SECRET，当前进程使用随机密钥。
多实例/滚动发布必须在所有实例配置一致的 SESSION_SECRET，否则会话会随机过期。」）

查证：

```bash
docker exec new-api env | grep -c SESSION_SECRET      # 1 = 已配置
docker logs new-api 2>&1 | grep -i 'SESSION_SECRET'
```

处置：在所有实例注入**同一个**随机的 `SESSION_SECRET`。
注意 `SESSION_SECRET=random_string`（默认占位值）会导致进程**直接启动失败**
（`common/init.go:52-55`）。`CRYPTO_SECRET` 未设置时**回退取 `SESSION_SECRET`**
（`common/init.go:67-71`）—— 所以改 `SESSION_SECRET` 会**同时**影响 crypto 相关数据，
改前务必确认。

### 4.9 登录 / 验证码 / OAuth 相关异常

认证相关改动受 `AGENTS.md` 的 OWASP ASVS 强制条款约束（`AGENTS.md:49-55`）：
**开工前先读适用的 OWASP Cheat Sheet**，并按要求补失败 / 过期 / 重放 / 绕过的回归测试。
排障时先分清是「认证失败」还是「上游 OAuth 提供方故障」。

---

## 5. 常见误判与坑（省时间用）

### 5.1 本机 curl 说「挂了」

见 §4.1。**本机 curl 任何 https 都是 `000`**，不代表线上有问题。

### 5.2 「页面还是旧版本」

常见原因不是部署失败，而是：
- 浏览器缓存（硬刷新即可）；
- 前端改动未重建镜像（前端是 `go:embed`，见 §2.4）。
历史真因是 compose 镜像写死固定 tag，watchtower 对固定 tag 不更新
（来源：生产记忆 `freeapi-production-deploy.md`）。**当前已改本地构建 + 蓝绿，不再用 watchtower。**

### 5.3 「点了没反应」先查 CSP

见 §3.3。

### 5.4 `/v1/pricing` 出现 429

这是**既有限流器（60/min/IP）在单 IP 高频下的正常工作**，不是故障
（来源：`计划书/workflow_status.md` Batch-8 §2.7）。

### 5.5 开关改了没生效？

能力开关的优先级是 **运行时覆盖（管理端） > env 默认**，覆盖值无法解析为布尔时
**回退 fallback 而不 panic**（`common/feature_flag.go:82-95`）。
有 4 个开关（渠道密钥加密 / 密码登录加密 / 错误日志 / 媒体 token 非流式）的值由
`common`/`constant` 包变量持有，管理端修改后对**已读过旧值的代码路径**是
「下一次读取即生效」（`common/feature_flag.go:176-185`）。

### 5.6 `/metrics` 返回一坨 HTML

`METRICS_ENABLED` 未设为 `true`（大小写不敏感，只有 `true` 才开）
→ 路由根本没注册 → 落到 SPA fallback 返回前端 HTML
（`middleware/metrics_auth.go:36-38`）。
排除：`curl -s http://127.0.0.1:<端口>/metrics | head -1` 应以 `# TYPE` 开头；
若返回 `<!doctype html>`，说明**当前实例**没开（注意蓝绿两个端口可能配置不同）。
端点本身受保护：可信来源 IP 放行，否则要求 root 会话（`middleware/metrics_auth.go`）。
**是否在生产开启需运维/产品决策**（开启后即可观察内存/goroutine 趋势与 loop 心跳）。

### 5.7 磁盘涨了先别删镜像

见 §4.6。镜像只占几 GB，构建缓存才是几十 GB 的那部分。

---

## 6. 事故响应：生产数据被错误修改

> 以下流程来自 2026-10-09 生产的**真实计费事故**（225 次请求多扣费、11 个账号、
> 补偿 10,428,964 quota / $20.86）。来源：生产记忆
> `never-change-production-billing-directly.md`、`new-api-prod-incident-20261009.md`。

### 第 1 步：先停手 —— **立即恢复配置，别再改任何东西**

计费配置与代码不同：**没有「部署」这一步，改库即生效（60s 内热更新）**。
每一秒都在影响正在跑的真实请求与用户钱包。

### 第 2 步：取证 —— 圈定影响范围

用日志字段精确圈定，不要凭时间猜：

```sql
-- 受影响的请求集合
SELECT user_id, count(*), sum(quota)
FROM logs
WHERE type=2
  AND created_at BETWEEN <开始epoch> AND <结束epoch>
  AND other::json->>'billing_mode' = 'tiered_expr'
GROUP BY 1 ORDER BY 3 DESC;
```

### 第 3 步：算差额

口径 = **实收 − 应收**。本次事故的应收 = `$0.002 × QuotaPerUnit(500000) × group_ratio`。

### 第 4 步：补偿 —— 走管理员 API（有审计）

```bash
# POST /api/user/manage
# body: {"id":<uid>,"action":"add_quota","mode":"add","value":<quota>}
# header: Authorization: Bearer <access_token>   ← 来自 /api/user/login 的 data.access_token
#                                                  （不是 cookie）
# 审计动作名：user.quota_add
```

### 第 5 步：公告 + 复盘

站内公告写在 `options.console_setting.announcements`（本次公告 id=2），
内容包含事故时间、**逐账号补偿表**、改进措施。
改进必须落地为**自动化防线**：本次新增了
`pkg/billingexpr` 的 `TestElseBranchEqualsLegacyPerRequestPrice`
（`pkg/billingexpr/token_billing_group_test.go:86`），断言表达式 else 分支与现状按次价
**逐分组等值**，防止再次误伤其他分组。

---

## 7. 明确禁止的操作

| 禁止 | 理由 |
|---|---|
| **在生产直接改计费配置（`options.billing_setting.billing_expr`）做测试** | 改库即生效，曾致 225 次请求多扣费、$20.86 补偿。改计费必须走 `计划书/ops/token-billing-group.md` §4 的强制流程 |
| **在生产用 curl 判断可达性后据此回滚** | 本机 curl 全局失效，会把「本机问题」当成「线上故障」触发无谓回滚 |
| **把含凭据的真实脚本提交进仓库** | 服务器脚本含 DB/Redis 凭据；仓库只放 `.example` 脱敏模板 |
| **删除正在运行的镜像 / 把 `KEEP_IMAGES` 调到 < 4** | 会毁掉回滚窗口 |
| **直接删生产数据或批量清理镜像 / 日志（未被授权时）** | 破坏性且不可逆；磁盘清理用 `disk-guard.sh`（只清缓存与旧镜像，正在运行的永不删） |
| **用 `docker compose up` 重建容器做发布** | 会先停后起，造成 5-15s 全站不可用；发布一律用 `deploy-zero-downtime.sh` |
| **给非生产实例开 `SessionSecret` 之外的随机密钥后当多实例用** | 会话会随机失效（§4.8） |
| **在 `docs/` 下新增文档** | `AGENTS.md:148` 明确禁止，除非用户显式要求 |

---

## 8. 本文刻意没写的内容（原因）

- **SLA / 可用性百分比 / 平均恢复时间**：仓库内**没有**真实的历史可用性度量数据
  （`METRICS_ENABLED` 虽已开启，但未接入外部时序存储，无长期历史），编造数字只会误导。
- **生产 `.env` 的具体内容与凭据**：安全边界，本文只给用法。
- **Caddyfile 的完整生产原文**：仓库内只有示例副本（`deploy/Caddyfile.example`），
  生产文件在宿主机；引用示例会与线上不一致。
- **`/opt/new-api/rollback.sh` 的用法**：该脚本在生产记忆里出现于 2026-09-28 的旧方案
  （`docker compose` 时代），当前蓝绿流程用的是 `deploy-zero-downtime.sh` 内建回滚。
  **两份脚本的当前关系待现场确认**，故不写步骤。
- **多实例部署的完整配置**：当前生产是**单实例**（`deploy-zero-downtime.example.sh`
  起的是单个 `new-api` 容器），多实例相关的 `SESSION_SECRET` 一致性要求只在 §4.8 提示。
