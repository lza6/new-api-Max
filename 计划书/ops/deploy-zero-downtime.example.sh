#!/bin/bash
#
# 蓝绿零停机部署 —— **脱敏模板**。
#
# ⚠️ 这是模板，不是可直接运行的脚本。真实版本在服务器 `/opt/new-api/deploy-zero-downtime.sh`，
# 其中包含数据库/Redis 凭据，**不得提交到仓库**。本文件的用途是把「部署流程 + 磁盘清理策略」
# 记录下来，便于复现与审阅。所有密钥位点一律用 ${VAR} 占位。
#
# 用法：deploy-zero-downtime.sh <tag>
#   KEEP_IMAGES=<n> 覆盖镜像保留数量（默认 4：当前 + 3 个回滚窗口）
#
set -e

TAG="${1:?usage: deploy-zero-downtime.sh <tag>}"
DOMAIN="${DOMAIN:-freeapi.tingfengai.art}"
PROD_DIR="${PROD_DIR:-/opt/new-api}"
NETWORK="${NETWORK:-new-api_new-api-network}"

# 凭据从环境读取，不写死在脚本里。
: "${SQL_DSN:?SQL_DSN is required}"
: "${REDIS_CONN_STRING:?REDIS_CONN_STRING is required}"

NEW_PORT=3001
OLD_PORT=3000
CUR=$(grep -oE "127.0.0.1:300[0-9]" /etc/caddy/Caddyfile | head -1 | grep -oE "300[0-9]")
if [ "$CUR" = "3001" ]; then NEW_PORT=3000; OLD_PORT=3001; fi
echo "[deploy] tag=$TAG old=$OLD_PORT new=$NEW_PORT"

# 起新容器
docker rm -f new-api-new 2>/dev/null || true
docker run -d --name new-api-new \
  --network "${NETWORK}" \
  --env-file "${PROD_DIR}/.env" \
  -e SQL_DSN="${SQL_DSN}" \
  -e REDIS_CONN_STRING="${REDIS_CONN_STRING}" \
  -e TZ=Asia/Shanghai -e BATCH_UPDATE_ENABLED=true -e LOG_FLUSH_ENABLED=true \
  -e MEMORY_CACHE_ENABLED=true -e GOMEMLIMIT=512MiB \
  -p "${NEW_PORT}:3000" \
  -v "${PROD_DIR}/data:/data" -v "${PROD_DIR}/logs:/app/logs" \
  --restart unless-stopped \
  --memory 900m --memory-swap 900m "new-api:${TAG}"

# 等健康
echo "[deploy] 等新容器健康（<=90s）..."
for _ in $(seq 1 45); do
  sleep 2
  ok=$(curl -s -o /dev/null -w "%{http_code}" --max-time 5 "http://127.0.0.1:${NEW_PORT}/api/status" || true)
  [ "$ok" = "200" ] && echo "[deploy] 新容器 200" && break
done

# 切 Caddy 上游
sed -i "s/127.0.0.1:${OLD_PORT}/127.0.0.1:${NEW_PORT}/g" /etc/caddy/Caddyfile
systemctl reload caddy
sleep 2

# 外网自检（从本机以真实域名 + 真实 TLS 访问自己的公网入口）
code=$(curl -s -o /dev/null -w "%{http_code}" --max-time 10 \
  --resolve "${DOMAIN}:443:127.0.0.1" "https://${DOMAIN}/api/status" || true)
if [ "$code" != "200" ]; then
  echo "[deploy] 自检失败($code)，回滚 Caddy 上游"
  sed -i "s/127.0.0.1:${NEW_PORT}/127.0.0.1:${OLD_PORT}/g" /etc/caddy/Caddyfile
  systemctl reload caddy
  docker rm -f new-api-new
  exit 1
fi
echo "[deploy] 自检通过($code)，停旧容器并改名"
docker rm -f new-api || true
docker rename new-api-new new-api
echo "[deploy] 完成：new-api:${TAG} 在端口 ${NEW_PORT}"

# ---------------------------------------------------------------------------
# 自动清理：防止磁盘无限增长
#
# 背景（2026-10-11 实测）：每次热更新的 `docker build` 都会留下数 GB 的**构建缓存**，
# 且它永远不会自己消失；镜像 tag 也逐版本堆积。未清理时磁盘用掉 54G/78G（70%），
# 其中 **构建缓存独占 44.31G（可回收 39.53G）** —— 真正的元凶不是镜像（镜像只占 5.9G）。
# 清理后降到 8.3G（11%）。
#
# 策略保守：
#   - 构建缓存全清 —— 只影响下次构建速度，不影响任何已构建镜像；
#   - new-api 镜像保留最近 KEEP_IMAGES 个（默认 4 = 当前 + 3 个回滚窗口）；
#   - 正在运行的镜像**永不删除**（显式排除）；
#   - 构建源码目录/源码包只留最新一份。
# ---------------------------------------------------------------------------
KEEP_IMAGES=${KEEP_IMAGES:-4}
echo "[cleanup] 清理构建缓存与旧镜像（保留最近 ${KEEP_IMAGES} 个 new-api 版本）..."

docker builder prune -af >/dev/null 2>&1 || true

RUNNING_IMAGE=$(docker inspect new-api --format '{{.Config.Image}}' 2>/dev/null || true)
docker images new-api --format '{{.CreatedAt}}|{{.Tag}}' | sort -r \
  | tail -n +$((KEEP_IMAGES + 1)) | cut -d'|' -f2 | while read -r t; do
  [ -z "$t" ] && continue
  if [ "new-api:${t}" = "$RUNNING_IMAGE" ]; then
    echo "[cleanup]   跳过正在运行的 new-api:${t}"
    continue
  fi
  docker rmi "new-api:${t}" >/dev/null 2>&1 && echo "[cleanup]   已删 new-api:${t}" || true
done

ls -1 /root/newapi-src-*.tar.gz 2>/dev/null | sort | head -n -1 | xargs -r rm -f
rm -rf /root/newapi-build-* 2>/dev/null || true
docker image prune -f >/dev/null 2>&1 || true

echo "[cleanup] 完成。磁盘：$(df -h / | tail -1 | awk '{print $3" used / "$2" ("$5")"}')"
