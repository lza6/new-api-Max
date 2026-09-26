# 生产 503 应急诊断与救援（freeapi.tingfengai.art）

> 更新时间：2026-09-26 21:35 ｜ 线上事实：Caddy 443 正常；:3000/:3002 后端 /api/status 全部 503（空 body，非应用 JSON）。
> 判定：后端容器不健康（进程僵死/卡死/OOM/DB 断连），Caddy 健康检查已按 v1.3.39 参数摘流 → 503。

## 一、线上观察（本机已验证）
| 探测 | 结果 |
|---|---|
| https://freeapi.tingfengai.art/api/status | HTTP 503, Server: Caddy, Content-Length: 0 |
| http://103.233.252.213:3000/api/status | 503 空 body（非应用响应，非 JSON） |
| http://103.233.252.213:3002/api/status | 503 空 body |
| TCP 443/3000/3002/80/22 | 全通（防火墙/端口正常） |
| HTTP :80 | 308 → https（Caddy 正常） |

## 二、服务器一键诊断（SSH 后逐条执行）
```bash
cd /opt/new-api
# 1) 容器状态（先看谁挂了）
docker ps -a --format 'table {{.Names}}\t{{.Status}}\t{{.Ports}}'
docker compose ps

# 2) 容器日志尾部（定根因：panic / OOM / db 连接失败 / postgres 拒绝）
docker logs --tail 100 new-api 2>&1 | tail -60
docker logs --tail 100 new-api-next 2>&1 | tail -60

# 3) 资源（是否 OOM / 磁盘满）
free -h; df -h /; docker stats --no-stream | head -20

# 4) 依赖（PG / Redis 是否活着）
docker compose ps postgres redis
docker exec new-api sh -c 'echo "select 1" | PGPASSWORD=$PGPASSWORD psql -h postgres -U newapi -d new-api' 2>&1 | head -5

# 5) 应用健康端点（容器内直测）
docker exec new-api curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:3000/api/status
```

## 三、一键救援（最快恢复，分钟级）
```bash
cd /opt/new-api
# 简单重启（若容器仍在但僵死）：先重启 standby 观察，再重启 master
docker restart new-api-next && sleep 8 && curl -sf http://127.0.0.1:3002/api/status >/dev/null && echo STANDBY_OK
docker restart new-api && sleep 8 && curl -sf http://127.0.0.1:3000/api/status >/dev/null && echo MASTER_OK

# 若 restart 不够（容器 Exited）：recreate
docker compose up -d new-api-next new-api

# 救援后验证
curl -sf https://freeapi.tingfengai.art/api/status | head -c 200; echo
```

## 四、若容器反复挂（根因在代码/DB）
- 磁盘满：`docker system prune -af --filter "until=24h"` 后再 up。
- PG 连不上：看 `docker logs new-api` 中 SQL_DSN 报错；确认 `docker compose ps postgres` Healthy。
- OOM：`docker inspect new-api --format '{{.State.OOMKilled}}'`；compose mem_limit 2g 是否被触发（日志 `Killed`）。
- 版本回退：`cp docker-compose.yml.bak-pre-<旧版> docker-compose.yml && docker compose up -d new-api`。

## 五、根因归属（诚实标注）
- 已确认：后端容器不健康（非 Caddy 配置问题，v1.3.39 健康参数已生效但无法拯救僵死后端）。
- 未确认：容器挂的具体原因（panic/OOM/DB）需服务器日志定位——本机无 SSH 凭据，等授权后按 §二 执行。


## 六、2026-09-27 Caddyfile 损坏事故（v1.3.43 后，根路径空 body）

### 现象
- 公网 https://freeapi.tingfengai.art/ 返回 200 但空 body（Content-Length: 0）；/api/status 200、后端容器 healthy——仅首页空白。
- 服务器 curl /api/status 200 success=True version=v1.3.43；容器内 wget / 返回完整 HTML；经 Caddy 代理后 / 变空。

### 根因（部署脚本缺陷，非运行时问题）
- 上一轮 blue-green 部署用 python re.sub 改写 Caddyfile，正则 [^}]* 贪婪匹配到 health 配置块并将其删除 → Caddyfile 结构损坏（缺 health、缺闭合、缺 header_up），reload 后 Caddy 对 / 代理返回空 body。
- 证据：/etc/caddy/Caddyfile 时间戳 04:32 仅 670 字节；备份 bak-zd-*（1192B）结构完整。

### 修复
1. 整文件 heredoc 写入正确 Caddyfile（双后端 3000+3002、health 2s/2s/3/2、header_up X-Real-IP/X-Forwarded-For、transport h1/h2、/assets immutable）。
2. caddy validate → systemctl reload caddy → 验证 / 返回 1302B HTML。
3. 公网复核：ROOT 200 size=1302；/api/status success=True v1.3.43；index.js 200 4.42MB；css 200 427KB；logo 200；连续 10 次健康全 200。

### 纪律（防复发）
- Caddyfile 禁止片段正则替换：一律整文件 heredoc 全量写入（脚本 gen_caddy 模式）。
- 部署后必须验证 "curl -s https://域名/ | wc -c" 非 0 且含 <html（不仅看 200）。
- 保留最近 2 份完整 Caddyfile 备份（bak-zd-* 1192B 的是完整版）。