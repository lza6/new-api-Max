#!/bin/bash
#
# 每日磁盘守卫 —— 与 deploy-zero-downtime.sh 里的清理**互补**。
#
# 分工：
#   - 部署脚本的清理只在「部署时」跑，管住构建缓存与镜像堆积这条主路径；
#   - 本脚本每日常驻，兜住「有人在脚本之外手动 docker build / 留下临时目录」这类情况，
#     并在磁盘使用率越过阈值时**输出告警**（只告警，不擅自删数据）。
#
# 安装（服务器）：
#   install -m 0755 disk-guard.example.sh /opt/new-api/disk-guard.sh
#   cat > /etc/cron.d/new-api-disk-guard <<'EOF'
#   SHELL=/bin/bash
#   PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
#   17 4 * * * root /opt/new-api/disk-guard.sh >> /var/log/new-api-disk-guard.log 2>&1
#   EOF
#
# 可用环境变量覆盖：KEEP_IMAGES / WARN_PERCENT / BUILD_KEEP_DAYS
#
set -u

KEEP_IMAGES=${KEEP_IMAGES:-4}
WARN_PERCENT=${WARN_PERCENT:-85}
BUILD_KEEP_DAYS=${BUILD_KEEP_DAYS:-7}

echo "[$(date -Is)] disk-guard start; before: $(df -h / | tail -1)"

# 1) 构建缓存：只清 72 小时前的（保留近期构建的复用收益）
docker builder prune -f --filter until=72h >/dev/null 2>&1 || true

# 2) 旧 new-api 镜像：保留最近 N 个（正在运行的永不删）
RUNNING_IMAGE=$(docker inspect new-api --format '{{.Config.Image}}' 2>/dev/null || true)
docker images new-api --format '{{.CreatedAt}}|{{.Tag}}' | sort -r \
  | tail -n +$((KEEP_IMAGES + 1)) | cut -d'|' -f2 | while read -r t; do
  [ -z "$t" ] && continue
  [ "new-api:${t}" = "$RUNNING_IMAGE" ] && continue
  docker rmi "new-api:${t}" >/dev/null 2>&1 || true
done

# 3) 悬空镜像 + 停止超过 7 天的容器
docker image prune -f >/dev/null 2>&1 || true
docker container prune -f --filter until=168h >/dev/null 2>&1 || true

# 4) 源码构建残留：超过 BUILD_KEEP_DAYS 天的目录与包
find /root -maxdepth 1 -name 'newapi-build-*' -type d -mtime +"$BUILD_KEEP_DAYS" -exec rm -rf {} + 2>/dev/null || true
find /root -maxdepth 1 -name 'newapi-src-*.tar.gz' -mtime +"$BUILD_KEEP_DAYS" -delete 2>/dev/null || true

# 5) journald 收敛（不设上限时它会一直涨）
journalctl --vacuum-size=200M >/dev/null 2>&1 || true

AFTER=$(df -h / | tail -1)
PCT=$(df / | tail -1 | awk '{print $5}' | tr -d '%')
echo "[$(date -Is)] disk-guard done; after: ${AFTER}"
if [ "$PCT" -ge "$WARN_PERCENT" ]; then
  echo "[$(date -Is)] WARNING: disk usage ${PCT}% >= ${WARN_PERCENT}% — 请检查 docker / 日志 / 数据目录"
fi
