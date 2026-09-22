# 生产运维 SOP（new-api-Max / freeapi.tingfengai.art）

> 目标：让任何接手者都能按文档完成 部署/回滚/验收/排障，不靠口口相传。

## 1. 拓扑
- 香港 CN2 单机：Caddy(80/443) → new-api(3000) → PostgreSQL(newapi) / Redis
- 代码：`/opt/new-api-src`（git，tag 部署）；compose：`/opt/new-api/docker-compose.yml`
- 版本：`/opt/new-api/VERSION`；当前线上 **v1.2.92**

## 2. 部署（热更新）
```bash
TS=$(date +%Y%m%d-%H%M%S)
cd /opt/new-api && cp docker-compose.yml docker-compose.yml.bak.$TS
cd /opt/new-api-src && git fetch --tags origin && git checkout v<tag>
echo "v<tag>" > /opt/new-api-src/VERSION && git add VERSION && git commit -m "chore: bump VERSION to v<tag>" 2>/dev/null || true
docker build -t new-api:local-v<tag> .
cd /opt/new-api && sed -i "s|new-api:local-v[0-9.]*|new-api:local-v<tag>|" docker-compose.yml
docker compose up -d new-api
```
验收：`docker ps`（healthy）、`curl -s -o /dev/null -w "%{http_code}" http://127.0.0.1:3000/api/status`（200）

## 3. 回滚
```bash
cd /opt/new-api && sed -i "s|new-api:local-v<坏tag>|new-api:local-v<旧tag>|" docker-compose.yml.bak.$TS
cp docker-compose.yml.bak.$TS docker-compose.yml && docker compose up -d new-api
```

## 4. 常见运维命令
- DB 直连：`docker exec -e PGPASSWORD=<pw> postgres psql -U newapi -d new-api`
- 用户额度调整：兑换额度码（走真实缓存路径），勿直改 users.quota（缓存不一致）
- 订阅档位调整：后台 Security→Rate Limiting / 订阅 Tier Override，立即热更新
- 订阅到期扫描：主节点后台任务（每 60s），无需人工
- 登录排障：`docker logs new-api --tail 200`；日志表 `logs`（按 username 筛）

## 5. E2E 复现脚本（.codex/e2e-scratch/，未入库）
- `e2e-sub-live.mjs`：兑换→续费→并发3/s→RPM→429
- `e2e-group.mjs`：自动升级分组 + 门禁 403
- `e2e-tier.mjs`：管理员单订阅档位升级
- `e2e-bpay.mjs`：充值关闭余额兑换 + 模型矩阵 403
- 基线：`http://<SERVER_IP>:3000`（本机 HTTPS 出站被断）

## 6. 敏感信息红线
- 生产 IP/凭据不入库（占位符）；DB 密码仅在服务器 .env/compose
- E2E 临时管理员用后即删（含 user_sessions）
