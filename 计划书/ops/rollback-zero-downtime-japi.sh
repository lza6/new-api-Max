#!/usr/bin/env bash
# 听风API副站 回滚（蓝绿命名：把 Caddy 上游切回「另一个」容器名即可）
# 用法: ./rollback-zero-downtime.sh v1.3.74
#
# 前提：目标 tag 的镜像已在本机（docker images new-api:<tag>）。
# 语义：起一个用目标镜像的容器（用当前未占用的交替名），健康后切 Caddy 上游，
# 排空后回收旧容器。与 deploy-zero-downtime.sh 同一套蓝绿机制。
set -euo pipefail

TAG="${1:?用法: $0 <tag>  例: $0 v1.3.74}"
PROD=/opt/new-api
NET=new-api_new-api-network
CADDY=caddy
HOST_CADDYFILE="$PROD/caddy/Caddyfile"
NAME_A=new-api-a
NAME_B=new-api-b
DRAIN_SECONDS="${DRAIN_SECONDS:-12}"
PUBLIC_URL=https://japi.tingfengai.art
LOG=/tmp/rollback-zd-${TAG}.log

log()  { echo "[$(date +%H:%M:%S)] $*" | tee -a "$LOG"; }
fatal(){ log "错误: $*"; exit 1; }

docker image inspect "new-api:${TAG}" >/dev/null 2>&1 || fatal "本机无镜像 new-api:${TAG}"

CUR_NAME=$(grep -oE 'reverse_proxy (new-api-a|new-api-b):3000' "$HOST_CADDYFILE" | head -1 | sed -E 's/reverse_proxy (new-api-[ab]):3000/\1/' || true)
[ -z "$CUR_NAME" ] && fatal "Caddyfile 上游不是交替命名（new-api-a/b），请确认已迁移"
if [ "$CUR_NAME" = "$NAME_A" ]; then NEW_NAME="$NAME_B"; else NEW_NAME="$NAME_A"; fi
log "===== 副站回滚 → ${TAG} ====="
log "当前上游=${CUR_NAME} → 目标容器=${NEW_NAME}"

docker rm -f "$NEW_NAME" >/dev/null 2>&1 || true
ENV_ARGS=()
while IFS= read -r line; do
  [ -n "$line" ] && ENV_ARGS+=(-e "$line")
done < <(docker inspect "$CUR_NAME" --format '{{range .Config.Env}}{{println .}}{{end}}' \
         | grep -vE '^(PATH|HOSTNAME|HOME|GODEBUG|GOTRACEBACK)=')

docker run -d --name "$NEW_NAME" \
  --network "$NET" \
  "${ENV_ARGS[@]}" \
  -e NODE_TYPE=slave \
  -e NODE_NAME=new-api-japi-bg \
  -v "$PROD/data:/data" \
  -v "$PROD/logs:/app/logs" \
  "new-api:${TAG}" --log-dir /app/logs >>"$LOG" 2>&1 || fatal "容器启动失败"

for i in $(seq 1 45); do
  if docker exec "$CUR_NAME" wget -q -O /dev/null -T 3 "http://${NEW_NAME}:3000/api/status" 2>/dev/null; then
    log "新容器健康（第 ${i} 次探测）"; break
  fi
  [ "$i" = "45" ] && { docker rm -f "$NEW_NAME" >/dev/null 2>&1; fatal "容器未健康"; }
  sleep 2
done

cp "$HOST_CADDYFILE" "${HOST_CADDYFILE}.bak-$(date +%Y%m%d-%H%M%S)"
sed -i "s|${CUR_NAME}:3000|${NEW_NAME}:3000|g" "$HOST_CADDYFILE"
docker exec "$CADDY" caddy reload --config /etc/caddy/Caddyfile --adapter caddyfile >>"$LOG" 2>&1 || { log "reload 失败"; sed -i "s|${NEW_NAME}:3000|${CUR_NAME}:3000|g" "$HOST_CADDYFILE"; docker exec "$CADDY" caddy reload --config /etc/caddy/Caddyfile --adapter caddyfile >/dev/null 2>&1; docker rm -f "$NEW_NAME"; fatal "切换失败已回滚"; }
sleep 3
curl -sf -m 10 "${PUBLIC_URL}/api/status" >/dev/null 2>&1 || { sed -i "s|${NEW_NAME}:3000|${CUR_NAME}:3000|g" "$HOST_CADDYFILE"; docker exec "$CADDY" caddy reload --config /etc/caddy/Caddyfile --adapter caddyfile >/dev/null 2>&1; docker rm -f "$NEW_NAME"; fatal "外网自检失败已回滚"; }

log "排空 ${DRAIN_SECONDS}s 后回收旧容器"
sleep "$DRAIN_SECONDS"
docker rm -f "$CUR_NAME" >>"$LOG" 2>&1 || log "旧容器移除失败"
cd "$PROD" && sed -i "s|image: new-api:.*|image: new-api:${TAG}|" docker-compose.yml || true
log "===== 回滚完成：${TAG}（当前上游 ${NEW_NAME}）====="
