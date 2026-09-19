# 线上生产 E2E 验收（v1.2.34 部署，2026-09-20）

## 部署方式（CI 镜像队列卡死 11h+，绕行）
- 服务器本地 `git clone --depth 1 --branch v1.2.34` + `docker build -t new-api:local-v1.2.34 .`
- 备份：docker-compose.yml.bak-pre-v1234 + docker tag new-api-max:latest → latest-pre-v1234-backup
- compose：image→new-api:local-v1.2.34；新增 MEMORY_CACHE_ENABLED/SYNC_FREQUENCY/LOG_FLUSH_ENABLED/INTERVAL/BATCH/SQL_MAX_OPEN_CONNS=64/IDLE=16/LIFETIME=300/RELAY_429_*
- `docker compose up -d new-api` → Up healthy；env 8 项生效确认

## 验收结果（真实请求）
- /api/status version = v1.2.34；容器 healthy
- 管理端登录 → 现有 token(536 IDEVS) → 真实上游 deepseek-v4-flash 流式：
  - HTTP 200 ✅ / [DONE] ✅ / **首字 2.46s / 总 2.69s**（上游本身 2-22s，网关透传正常）
- RESULT: PASS

## 回滚
- cp docker-compose.yml.bak-pre-v1234 docker-compose.yml && docker compose up -d new-api（回到 latest-pre-v1234-backup 镜像）