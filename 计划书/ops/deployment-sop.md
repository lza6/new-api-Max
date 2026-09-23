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