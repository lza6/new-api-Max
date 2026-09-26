# 部署 / 回滚 SOP（new-api-Max · v1.3.28）

> 更新：2026-09-25（T11-1）· 版本基线 v1.3.28（HEAD a0d0589f5）。恢复自 git HEAD 并补齐
> blue-green 零停机滚动更新脚本（scripts/rolling-update-newapi-v3.sh，commit a0d0589f5）的衔接。
> 适用：fork 仓库 lza6/new-api-Max（origin main），服务器 /opt/new-api（docker compose，端口 3000，
> 主库 PG + Redis + 独立日志库；Caddy 反代 freeapi.tingfengai.art）。fork 的 push 触发不建 → 构建/发布用 `workflow_dispatch`。

## 0. 发布链路（build → tag → workflow_dispatch → 服务器 compose → healthcheck → 回滚）
1. 代码：main 分支推 **主题 commit**（禁 `-f`）；本地与远端 SHA 核对一致。
2. 版本：更新 `VERSION`（当前 v1.3.28）→ 主题 commit。
3. 镜像（fork 环境二选一）：
   - CI：`gh workflow run docker-build.yml --repo lza6/new-api-Max -f tag=<TAG>`（手动 dispatch，勿依赖 push 触发）。
   - 服务器直连：`cd /opt/new-api-src && git checkout <TAG> && docker build -t new-api:local-<TAG> .`
4. 备份当前 compose（回滚用）：`cp /opt/new-api/docker-compose.yml /opt/new-api/docker-compose.yml.bak-pre-v<OLD>`；
   旧镜像打备份标签：`docker tag new-api:local-<OLD> new-api:latest-pre-v<OLD>-backup`。
5. 换镜像：编辑 compose 镜像为 `new-api:local-<TAG>`（或 `ghcr.io/lza6/new-api-max:<TAG>`），`docker compose up -d new-api`。
6. 健康检查：`docker compose ps`（Up healthy）；`docker exec new-api env | grep -E 'VERSION|RELAY_|MEMORY_|LOG_'` 核对环境。
7. 线上验收：`curl -s https://freeapi.tingfengai.art/api/status | grep version` 与响应头 `X-New-Api-Version: v1.3.x`；抽样真实请求（流式 200+DONE）。
8. 证据存档：`计划书/e2e-evidence/prod-v<TAG>-acceptance.json` + 本 SOP 记录。

## 1. 零停机滚动更新（blue-green，a0d0589f5 实机验证）
- 脚本：`scripts/rolling-update-newapi-v3.sh <IMAGE_TAG>`（`IMAGE=ghcr.io/lza6/new-api-max:<TAG>`）
- 原理：Caddy 反代在 :3000（master）与 :3002（standby）之间原子切换，**绝不在 Caddy 仍路由到该上游时杀掉它**：
  - A) 若存在 standby 且被路由：先把 Caddy 切到 :3000-only，再替换 standby（新版本）
  - B) standby 健康后原子切换到 :3002-only，再经 compose 替换 master（新镜像）
  - C) 切回 dual（:3000 :3002）；清理旧 rolling 容器（:3001）
- 健康门：standby 每 1s curl `127.0.0.1:3002/api/status`（最多 40s）；master 同理（最多 60s）。
- 回滚机制：ERR trap 恢复已知健康的 Caddy 配置（master-only 或 standby-only）；切换前 `caddy validate` 校验配置。
- 生产实证（commit 消息）：v1.3.27↔v1.3.28 两轮真实版本切换，持续负载下 **0 HTTP 5xx**。
- 环境注入：standby 容器注入 `NODE_NAME=new-api-next`、`NODE_TYPE=slave`、复用 `/opt/new-api/.env` + PG/Redis DSN
  （SQL_DSN / REDIS_CONN_STRING 从 docker-compose.yml 提取）。
- 回滚（针对该脚本）：重跑脚本切回旧 TAG，或手工 `docker compose up -d new-api` 指回 `.bak-pre-vXXX` 镜像标签。

## 2. 回滚（故障时 1 分钟级）
```bash
cd /opt/new-api
cp docker-compose.yml.bak-pre-v<OLD> docker-compose.yml   # 还原 compose（含镜像标签）
docker compose up -d new-api                                # 用旧镜像重建
curl -s https://freeapi.tingfengai.art/api/status | grep version   # 确认回退
```
- 镜像回退：compose 指向 `new-api:latest-pre-v<OLD>-backup`（步骤 4 已打标签）或 reborn 旧 tag。
- 数据库无需回滚（本 SOP 不破坏 schema；若含迁移，先在 scratch 库跑三库幂等再上线）。

## 3. Watchtower 自更新（如用）
- 节点 label：compose 给 new-api 加 `labels: com.centurylinklabs.watchtower.enable=true`，仅更新 new-api，不碰 redis/postgres。
- 参数：`--restart always --poll-interval 300 --label-enable`（避免 24h 轮询与不扫）。
- 注意：fork push 触发不建，watchtower 只负责「镜像已更新」后的自动拉取；镜像构建仍用 dispatch。
- 与 blue-green 的关系：watchtower 只适合单实例常规升级；多实例零停机走 §1 脚本（其自己拉镜像/切流）。

## 4. 运维/排障
- 磁盘防满：服务器 systemd/cron `docker system prune -af --filter "until=48h"`（保留最近 1-2 个备份标签手动清）。
- 慢首字：确认 compose 已注入 `MEMORY_CACHE_ENABLED=true`、`LOG_FLUSH_ENABLED=true`、连接池收敛、429 退避 env。
- 5xx=上游超时：`RELAY_TIMEOUT`（>max prefill）与 `relay.non_stream_first_byte_timeout`；超时语义 504（非 500）。
- 渠道 429/502：单渠道超卖 → 增加健康渠道 + `CHANNEL_HEALTH_ROUTING`（灰度后可开，v1.3.28 默认 on）。
- LB 健康探测：`/api/status` 已从全局 API 限流豁免（commit 866f23e24），Caddy health_uri 可用 1s 间隔轮询。

## 4.1 生产 503 事故复盘（2026-09-26，v1.3.41 修复闭环）

### 现象
- 公网 `https://freeapi.tingfengai.art/api/status` 持续 503（Server: Caddy，Content-Length: 0）。
- 宿主机 `curl http://127.0.0.1:3000/api/status` 与 :3002 均 429；容器内 `wget` 却 200。

### 根因链（三层，逐层定位）
1. **Caddy 健康检查误判**：Caddyfile 是旧激进参数 `health_timeout 1s / health_fails 1 / health_passes 1`
   （v1.3.39 加固未部署到线上）→ 一次非 200 即摘流。
2. **健康检查来源被 Web 防护封禁**：Caddy 从宿主机发请求，经 Docker NAT 后容器看到 ClientIP=
   **Docker 网关 `172.18.0.1`**；Web 防护按 IP 速率自动封禁该地址（`banned_ips` 记录
   reason=`auto:web_rate_limit`，24h）。此后 /api/status 返回 `ip_banned` 429。
3. **双上游全被摘 → 公网 503**：`no upstreams available`。

### 定位命令（按序）
```bash
# 1) 看 Caddy 日志确认 429/摘流
journalctl -u caddy --since '10 min ago' | grep -E 'health|unhealthy|no upstreams'
# 2) 直连后端区分层：宿主机 curl vs 容器内 wget
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:3000/api/status   # 429 → 应用/防护层
docker exec new-api wget -qO- http://127.0.0.1:3000/api/status | head -c 200  # 200 → 是封禁非宕机
# 3) 查 banned_ips（关键：Docker 网关 IP 是否被封）
docker exec postgres psql -U newapi -d new-api -c "SELECT id, ip, reason, expires_at FROM banned_ips;"
# 4) 确认 ClientIP 来源（NAT 后是 172.18.0.1）
```

### 修复（v1.3.41 已含代码侧，运维侧按此执行）
1. **清封禁**：`DELETE FROM banned_ips WHERE ip IN ('172.18.0.1','127.0.0.1','::1');`
2. **Caddyfile 加固**（v1.3.39 参数）：`health_interval 2s / health_timeout 2s / health_fails 3 / health_passes 2`。
3. **升级到含豁免的镜像**：v1.3.41 起 `middleware/rate-limit.go` 的 `isHealthProbePath` 对
   GET /api/status 豁免 GA 全局限流；Web 防护侧不应封禁健康检查来源（后续版本持续加固）。
4. 验证：`curl -s https://freeapi.tingfengai.art/api/status | grep version` + 连续 20 次健康检查 0 失败。

### 防复发清单
- [ ] Caddyfile 恒为 `3/2/2s/2s`（reload 后 `systemctl is-active caddy`）。
- [ ] banned_ips 监控：定期查 `reason='auto:web_rate_limit'` 且 ip ∈ Docker 网段。
- [ ] 升级镜像后必须核对 `X-New-Api-Version` 头（本机 `curl -sI`）。
- [ ] 部署任何版本前先验证 `docker exec new-api curl 127.0.0.1:3000/api/status` = 200（非 429）。

## 5. 演练与备份
- 干跑：STAGING 同版本模拟「build→up→curl→回滚」；恢复演练记录留 `计划书/ops/`。
- 备份：compose 快照 + DB dump（pg_dump）+ 计划书证据目录；保留策略：最近 1-2 个备份。
- blue-green 预演：先在 STAGING 跑 `rolling-update-newapi-v3.sh` 干跑两轮版本切换，确认 Caddy validate/reload 链无回归。

## 6. 生产加固基线（2026-09-25 v1.3.26 实机验证；v1.3.28 沿用）

> 下列加固已在 freeapi.tingfengai.art 生效，新环境按此基线部署。

### 容器资源与日志
- **日志轮转**（docker json-file）：`logging: {driver: json-file, options: {max-size: "50m", max-file: "5"}}`
  → 容器 stdout 日志有界，防磁盘撑爆。
- **资源限制**：`mem_limit: 2g` + `cpus: "2.0"` + `stop_grace_period: 30s`
  → 单容器不耗尽宿主机（生产实测峰值 ~150MB / 3% CPU）。
- **应用日志**：`command: --log-dir /app/logs` 内部按时间段自动切文件（有界）。

### 自更新
- **Watchtower**（仅更新 new-api）：compose 给 new-api 加
  `labels: com.centurylinklabs.watchtower.enable=true`；watchtower 容器
  `--restart always --poll-interval 300 --label-enable`。

### 性能 env（生产验证）
```yaml
MEMORY_CACHE_ENABLED=true  SYNC_FREQUENCY=60
LOG_FLUSH_ENABLED=true     LOG_FLUSH_INTERVAL=1000  LOG_FLUSH_BATCH=500
RELAY_TIMEOUT=900          RELAY_429_RETRY_DELAY=1000  RELAY_429_MAX_RETRIES=2
SQL_MAX_OPEN_CONNS=64      SQL_MAX_IDLE_CONNS=16   SQL_MAX_LIFETIME=300
```

### 数据库备份（P0，2026-09-25 建立）
- 脚本：`/usr/local/bin/backup-newapi.sh`（docker exec pg_dump + gzip → /opt/backup/new-api/）
- Cron：每日 03:10（`10 3 * * *`），保留 7 天（`find -mtime +7 -delete`）
- 验证：首次备份 40MB / 43 张表 / gzip 完整 / users/channels/logs/tokens 均在。
- 恢复：`zcat /opt/backup/new-api/newapi-<TS>.sql.gz | docker exec -i postgres psql -U newapi -d new-api`

### 慢查询防护（v1.3.24-26）
- 带宽/流量排行已 SQL 聚合 + 60s Redis 缓存（SLOW SQL 从 25 条/2h → 0）。
- 排查命令：`docker logs new-api --since 2h | grep 'SLOW SQL'`；命中后查
  `controller/log.go` 对应 handler（queryModelBandwidthLeaderboard / GetBandwidthLeaderboard / GetLogsTraffic）。

## 7. 透传模式 token 计费说明（上游决定，v1.3.15 起）

### 现状（已确认）
- **结算/日志 token + 缓存 token 全部以上游 usage 为准**：`summary.CacheTokens = usage.PromptTokensDetails.CachedTokens`
  （service/text_quota.go:265），`usage_billing_path=upstream`（生产实测日志证据）。
- **网关不覆盖上游 token/cache 计数**；`local_count_tokens` 仅用于「上游未返回 usage」的降级路径
  （gemini/audio 等特殊渠道强制本地计数）。

### 预扣费估算（网关唯一"算 token"处）
- 预扣用 `EstimateRequestToken`（本地估算，`CountToken` env 默认 true）——**仅用于预扣防欠费**，
  结算按上游 usage 多退少补，**不改变最终计费**。
- 若希望透传渠道完全由上游决定 + 降低首字延迟/CPU：
  - compose 设 `CountToken=false` → 预扣估算返回 0（token_counter.go:182），预扣走最小额，
    结算仍按上游 usage 补扣。**权衡**：低额度用户瞬时可用额变大（结算前可能超用），
    免费/信任用户无影响。**回滚**：改回 true 重启。

### 推荐
- 免费/公益网关：`CountToken=false`（省 CPU、首字更快，预扣风险可接受）。
- 商业计费网关：保持 `CountToken=true`（预扣精度优先，避免超用）。