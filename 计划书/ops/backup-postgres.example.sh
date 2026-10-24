#!/bin/bash
#
# 每日 PostgreSQL 备份 —— 对应改进指南 T-E1 / SOP-备份与恢复.md
#
# 为什么需要它：2026-10-24 审计发现生产**零备份、零 WAL 归档**（仓库内 rg pg_dump 零命中）。
# 2026-10-23 已发生过一次 PG 被 OOM kill 后崩溃恢复、统计信息被清空的事故——离数据丢失只差一步。
#
# 设计取舍：
#   1. 用 `pg_dump -Fc`（自定义格式，自带压缩 + 可用 pg_restore 选择性恢复），不用纯 SQL 文本；
#   2. 走 `docker exec` 进 postgres 容器，避免宿主机装 client 版本不匹配；
#   3. 备份前先查剩余空间，宁可跳过并告警，也不写满磁盘（磁盘满会连带把服务打挂）；
#   4. 只删**本脚本自己命名规则**的旧备份，绝不 `find -delete` 模糊匹配；
#   5. 备份完立刻用 `pg_restore --list` 做结构可读性校验（这是最低限度的"不是空文件"证明）。
#
# 安装（服务器，二选一）：
#   install -m 0700 backup-postgres.example.sh /opt/new-api/backup-postgres.sh
#   cat > /etc/cron.d/new-api-backup <<'EOF'
#   SHELL=/bin/bash
#   PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
#   30 3 * * * root /opt/new-api/backup-postgres.sh >> /var/log/new-api-backup.log 2>&1
#   EOF
#   chmod 644 /etc/cron.d/new-api-backup
#
# 手动跑一次（验证用）：
#   /opt/new-api/backup-postgres.sh; echo "exit=$?"; ls -lh /opt/backup/
#
set -uo pipefail

# ── 配置（可用环境变量覆盖）────────────────────────────────────────────
PG_CONTAINER="${PG_CONTAINER:-postgres}"
PG_USER="${PG_USER:-root}"
PG_DB="${PG_DB:-new-api}"
BACKUP_DIR="${BACKUP_DIR:-/opt/backup}"
KEEP_DAYS="${KEEP_DAYS:-7}"
MIN_FREE_MB="${MIN_FREE_MB:-2048}"   # 剩余空间低于此值就跳过本次备份
PREFIX="newapi-db"

STAMP="$(date +%F_%H%M%S)"
TARGET="${BACKUP_DIR}/${PREFIX}-${STAMP}.dump"
LOG_TAG="[backup $(date '+%F %T')]"

log() { echo "${LOG_TAG} $*"; }
fail() { echo "${LOG_TAG} ERROR: $*" >&2; exit 1; }

# ── 0. 前置检查 ────────────────────────────────────────────────────────
mkdir -p "$BACKUP_DIR" || fail "无法创建 ${BACKUP_DIR}"
chmod 700 "$BACKUP_DIR"

command -v docker >/dev/null 2>&1 || fail "找不到 docker"
docker ps --format '{{.Names}}' | grep -qx "$PG_CONTAINER" \
  || fail "容器 ${PG_CONTAINER} 未运行（docker ps 里看不到）"

FREE_MB=$(df -Pm "$BACKUP_DIR" | awk 'NR==2 {print $4}')
if [ -z "$FREE_MB" ] || [ "$FREE_MB" -lt "$MIN_FREE_MB" ]; then
  fail "剩余空间 ${FREE_MB:-未知}MB < ${MIN_FREE_MB}MB，跳过本次备份（避免写满磁盘）"
fi
log "剩余空间 ${FREE_MB}MB，开始备份 ${PG_DB}"

# ── 1. 备份 ────────────────────────────────────────────────────────────
# -Fc = custom format（压缩 + 支持 pg_restore 选择性恢复）
# 先写 .part 再 mv：避免"备份到一半被杀"留下一个看起来正常的半截文件
docker exec "$PG_CONTAINER" pg_dump -U "$PG_USER" -d "$PG_DB" -Fc -f "/tmp/${PREFIX}-${STAMP}.dump" \
  || fail "pg_dump 失败（检查容器内数据库是否可连）"
docker cp "${PG_CONTAINER}:/tmp/${PREFIX}-${STAMP}.dump" "${TARGET}.part" \
  || fail "docker cp 失败"
docker exec "$PG_CONTAINER" rm -f "/tmp/${PREFIX}-${STAMP}.dump" >/dev/null 2>&1
mv "${TARGET}.part" "$TARGET" || fail "重命名失败"
chmod 600 "$TARGET"

SIZE_BYTES=$(stat -c '%s' "$TARGET" 2>/dev/null || echo 0)
if [ "$SIZE_BYTES" -lt 10240 ]; then
  fail "备份文件只有 ${SIZE_BYTES} 字节，判定异常（保留文件待人工检查：${TARGET}）"
fi
log "备份完成 ${TARGET}（$((SIZE_BYTES/1024/1024))MB）"

# ── 2. 结构可读性校验（不是"文件存在"就算完）────────────────────────────
# pg_restore --list 会解析归档目录，能列出表条目才说明 dump 没坏。
# 注意：它在**容器内**执行，所以先把文件塞回容器。
docker cp "$TARGET" "${PG_CONTAINER}:/tmp/verify.dump" >/dev/null 2>&1
TABLE_COUNT=$(docker exec "$PG_CONTAINER" pg_restore --list /tmp/verify.dump 2>/dev/null | grep -c 'TABLE DATA')
docker exec "$PG_CONTAINER" rm -f /tmp/verify.dump >/dev/null 2>&1
if [ "${TABLE_COUNT:-0}" -lt 10 ]; then
  fail "pg_restore --list 只解析出 ${TABLE_COUNT:-0} 张表，判定备份不可用"
fi
log "结构校验通过：可解析 ${TABLE_COUNT} 张表的数据段"

# ── 3. 轮转（只删自己命名规则的旧文件）──────────────────────────────────
DELETED=0
while IFS= read -r old; do
  [ -z "$old" ] && continue
  rm -f "$old" && DELETED=$((DELETED+1))
done < <(find "$BACKUP_DIR" -maxdepth 1 -type f -name "${PREFIX}-*.dump" -mtime "+${KEEP_DAYS}" 2>/dev/null)
log "轮转完成：删除 ${DELETED} 个超过 ${KEEP_DAYS} 天的备份"

log "OK"
