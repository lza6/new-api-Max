#!/usr/bin/env bash
# 听风API副站 零停机发布（蓝绿交替命名 + Caddy 热切换 + 排空后再回收）
# 用法: ./deploy-zero-downtime.sh v1.3.76
#
# 架构：副站 Caddy 在容器内，反代走 docker 网络名（new-api:3000），host 端口仅
# 绑 127.0.0.1 容器不可达 → 蓝绿也走容器网络。
#
# 【为什么这样设计（实测得来，勿轻易改）】
#   - 用 caddy reload 换上游名：切换瞬间**不中断**（实测 FAIL=0）。
#   - 但切换后**立即删旧容器会 502**：Caddy 与旧上游间的 keep-alive 连接 / 主动
#     健康检查器（health_checker）尚未排空，旧容器一删即报 no upstreams available。
#   - 因此：切换 → **等待 DRAIN 秒**（默认 12s，给连接排空 + checker 过期）→ 才删旧容器。
#   - 交替命名（new-api-a / new-api-b）避免 rename 造成的一瞬 502 窗口。
#
# 流程：
#   1. 读 Caddyfile 当前上游名 → 目标名 = 另一个
#   2. 构建镜像 → 起新容器（目标名，NODE_TYPE=slave、复用旧容器完全一致的 env）
#   3. 用当前容器（自带 wget）在网络内探测新容器健康
#   4. 改 Caddyfile 上游名 + caddy reload（热切换，不中断）
#   5. 外网自检 → 等待 DRAIN 排空 → 停旧容器
#   6. 任一步失败 → 恢复 Caddyfile + reload + 删新容器（旧容器全程未停）
set -euo pipefail

TAG="${1:?用法: $0 <tag>  例: $0 v1.3.76}"
SRC=/opt/new-api-src
PROD=/opt/new-api
NET=new-api_new-api-network
CADDY=caddy
HOST_CADDYFILE="$PROD/caddy/Caddyfile"
LOG=/tmp/deploy-zd-${TAG}.log
PUBLIC_URL=https://japi.tingfengai.art
NAME_A=new-api-a
NAME_B=new-api-b
DRAIN_SECONDS="${DRAIN_SECONDS:-12}"

log()  { echo "[$(date +%H:%M:%S)] $*" | tee -a "$LOG"; }
fatal(){ log "错误: $*"; exit 1; }

[ -f "$PROD/.env" ] || fatal "缺少 $PROD/.env"
set -a; . "$PROD/.env"; set +a

# 当前上游名（new-api-a / new-api-b / 首次的 new-api）
CUR_NAME=$(grep -oE 'reverse_proxy (new-api-a|new-api-b|new-api):3000' "$HOST_CADDYFILE" | head -1 | sed -E 's/reverse_proxy (new-api-[ab]|new-api):3000/\1/' || true)
if [ -z "$CUR_NAME" ]; then
  if docker inspect new-api >/dev/null 2>&1; then
    CUR_NAME=new-api
  else
    fatal "既无 new-api 也无 new-api-a/b 容器"
  fi
fi
if [ "$CUR_NAME" = "$NAME_A" ]; then NEW_NAME="$NAME_B"; else NEW_NAME="$NAME_A"; fi

log "===== 副站零停机发布 ${TAG} ====="
log "当前上游=${CUR_NAME} → 新容器=${NEW_NAME}（排空等待 ${DRAIN_SECONDS}s）"

AVAIL=$(free -m | awk '/^Mem:/{print $7}')
log "可用内存 ${AVAIL}MB"
[ "$AVAIL" -lt 600 ] && fatal "可用内存不足（${AVAIL}MB），请改用普通 deploy.sh"

docker rm -f "$NEW_NAME" >/dev/null 2>&1 || true

log "① 更新源码 → ${TAG}"
cd "$SRC"
git fetch --force origin "+refs/tags/${TAG}:refs/tags/${TAG}" >/dev/null 2>&1 || log "  (tag 已存在)"
git checkout -f "$TAG" >/dev/null 2>&1
log "   $(git describe --tags 2>/dev/null || echo "$TAG")"

log "② 构建镜像 new-api:${TAG}（约 5-8 分钟）"
docker build -t "new-api:${TAG}" . >>"$LOG" 2>&1 || fatal "镜像构建失败（详见 ${LOG}）"

# 复用当前容器 env（保证配置完全一致）
ENV_ARGS=()
while IFS= read -r line; do
  [ -n "$line" ] && ENV_ARGS+=(-e "$line")
done < <(docker inspect "$CUR_NAME" --format '{{range .Config.Env}}{{println .}}{{end}}' \
         | grep -vE '^(PATH|HOSTNAME|HOME|GODEBUG|GOTRACEBACK)=')

log "③ 启动新容器 ${NEW_NAME}（NODE_TYPE=slave，接入 ${NET}）"
docker run -d --name "$NEW_NAME" \
  --network "$NET" \
  "${ENV_ARGS[@]}" \
  -e NODE_TYPE=slave \
  -e NODE_NAME=new-api-japi-bg \
  -v "$PROD/data:/data" \
  -v "$PROD/logs:/app/logs" \
  "new-api:${TAG}" --log-dir /app/logs >>"$LOG" 2>&1 || fatal "新容器启动失败"

wait_healthy() {
  for i in $(seq 1 45); do
    if docker exec "$CUR_NAME" wget -q -O /dev/null -T 3 "http://${NEW_NAME}:3000/api/status" 2>/dev/null; then
      log "新容器健康（第 ${i} 次探测，约 $((i*2))s）"; return 0
    fi
    sleep 2
  done
  return 1
}
if ! wait_healthy; then
  log "--- 新容器日志尾部 ---"; docker logs --tail 30 "$NEW_NAME" 2>&1 | tee -a "$LOG"
  docker rm -f "$NEW_NAME" >/dev/null 2>&1 || true
  fatal "新容器未在 90s 内健康，已清理；旧容器继续服务"
fi

cp "$HOST_CADDYFILE" "${HOST_CADDYFILE}.bak-$(date +%Y%m%d-%H%M%S)"
restore_caddy() {
  log "恢复 Caddy 上游 → ${CUR_NAME}:3000"
  sed -i "s|${NEW_NAME}:3000|${CUR_NAME}:3000|g" "$HOST_CADDYFILE"
  docker exec "$CADDY" caddy reload --config /etc/caddy/Caddyfile --adapter caddyfile >>"$LOG" 2>&1 || log "caddy reload 失败，请手工检查"
}

log "④ 切换 Caddy 上游 → ${NEW_NAME}:3000"
sed -i "s|${CUR_NAME}:3000|${NEW_NAME}:3000|g" "$HOST_CADDYFILE"
if ! docker exec "$CADDY" caddy reload --config /etc/caddy/Caddyfile --adapter caddyfile >>"$LOG" 2>&1; then
  restore_caddy; docker rm -f "$NEW_NAME" >/dev/null 2>&1 || true
  fatal "Caddy 切换失败，已清理新容器"
fi
sleep 3
if ! curl -sf -m 10 "${PUBLIC_URL}/api/status" >/dev/null 2>&1; then
  restore_caddy; docker rm -f "$NEW_NAME" >/dev/null 2>&1 || true
  fatal "切流后外网自检失败，已回滚"
fi
log "外网自检通过（流量已在新容器 ${NEW_NAME}）"

# 关键：排空后再回收旧容器，避免 keep-alive 连接/健康检查器未排空造成 502
log "⑤ 等待 ${DRAIN_SECONDS}s 让 Caddy 连接/健康检查器排空……"
sleep "$DRAIN_SECONDS"
log "⑥ 停止并移除旧容器（${CUR_NAME}）"
docker rm -f "$CUR_NAME" >>"$LOG" 2>&1 || log "旧容器移除失败，请手工处理"

# 同步 compose 镜像引用（便于 rollback.sh；容器已脱离 compose，仅保持元数据一致）
cd "$PROD"
[ -f docker-compose.yml ] && sed -i "s|image: new-api:.*|image: new-api:${TAG}|" docker-compose.yml || true

log "===== 完成：${TAG} 上线（零停机；当前上游 ${NEW_NAME}）====="
curl -sf -m 10 "${PUBLIC_URL}/api/status" >/dev/null 2>&1 && log "外网 /api/status: 200" || log "外网自检异常，请检查"
