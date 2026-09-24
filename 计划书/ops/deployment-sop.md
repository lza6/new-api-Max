# 部署 / 回滚 SOP（new-api-Max · v1.3.x）

> 适用：fork 仓库 lza6/new-api-Max（origin main），服务器 /opt/new-api（docker compose，端口 3000，
> 主库 PG + Redis + 独立日志库）。fork 的 push 触发不可靠 → 构建/发布用 `workflow_dispatch`。

## 1. 发布链路（build → tag → release → 部署 → 验收）
1. 代码：main 分支推 **主题 commit**（禁 `-f`）；本地与远端 SHA 核对一致。
2. 版本：更新 `VERSION`（如 v1.3.12）→ 主题 commit。
3. 镜像（fork 环境二选一）：
   - CI：`gh workflow run docker-build.yml --repo lza6/new-api-Max -f tag=<TAG>`（手动 dispatch，勿依赖 push 触发）。
   - 服务器直连：`cd /opt/new-api-src && git checkout <TAG> && docker build -t new-api:local-<TAG> .`
4. 备份当前 compose（回滚用）：`cp /opt/new-api/docker-compose.yml /opt/new-api/docker-compose.yml.bak-pre-v<OLD>`；
   旧镜像打备份标签：`docker tag new-api:local-<OLD> new-api:latest-pre-v<OLD>-backup`。
5. 换镜像：编辑 compose 镜像为 `new-api:local-<TAG>`（或 `ghcr.io/lza6/new-api-max:<TAG>`），`docker compose up -d new-api`。
6. 健康检查：`docker compose ps`（Up healthy）；`docker exec new-api env | grep -E 'VERSION|RELAY_|MEMORY_|LOG_'` 核对环境。
7. 线上验收：`curl -s https://freeapi.tingfengai.art/api/status | grep version` 与响应头 `X-New-Api-Version: v1.3.x`；抽样真实请求（流式 200+DONE）。
8. 证据存档：`计划书/e2e-evidence/prod-v<TAG>-acceptance.json` + 本 SOP 记录。

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
- 注意：fork push 触发不可靠，watchtower 只负责「镜像已更新」后的自动拉取；镜像构建仍用 dispatch。

## 4. 运维/排障
- 磁盘防满：服务器 systemd/cron `docker system prune -af --filter "until=48h"`（保留最近 1-2 个备份标签手动清）。
- 慢首字：确认 compose 已注入 `MEMORY_CACHE_ENABLED=true`、`LOG_FLUSH_ENABLED=true`、连接池收敛、429 退避 env。
- 5xx=上游超时：`RELAY_TIMEOUT`（>max prefill）与 `relay.non_stream_first_byte_timeout`；超时语义 504（非 500）。
- 渠道 429/502：单渠道超卖 → 增加健康渠道 + `CHANNEL_HEALTH_ROUTING`（灰度后可开）。

## 5. 演练与备份
- 干跑：STAGING 同版本模拟「build→up→curl→回滚」；恢复演练记录留 `计划书/ops/`。
- 备份：compose 快照 + DB dump（pg_dump）+ 计划书证据目录；保留策略：最近 1-2 个备份。

## 5. 透传模式 token 计费说明（上游决定）

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

## 6. 生产加固基线（2026-09-25 v1.3.26 实机验证）

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