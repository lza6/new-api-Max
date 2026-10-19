#!/usr/bin/env bash
# 生产环境「训练数据采集」真实 E2E 探针。
#
# 做一件只用单测无法证明的事：**在真实网关上跑一次真实请求，然后去云盘上
# 把文件捞下来解压、逐行校验**。证明链路是：
#   注册/登录 → 授权 → 计费请求 → 队列 → worker → 分片压缩 → 云盘落盘
#
# 安全边界：
#   - 只读 `/opt/new-api/.env` 里的凭据，**不打印任何密钥**；
#   - 默认用一次性用户（`capture_e2e_<时间戳>`）；也支持 E2E_USER/E2E_PASS；
#   - 只发**一次**极小的补全请求（max_tokens=16）；
#   - 创建的 API 密钥 1 小时后过期、额度 100000（不是无限额度）；
#   - 临时文件用 trap 清理。
#
# 用法（在生产服务器上）：
#   bash prod-capture-e2e.sh [baseURL]   # 不传则自动探测 3000/3001（蓝绿会交替）
#   E2E_USER=xxx E2E_PASS=yyy bash prod-capture-e2e.sh          # 预建账号，不改额度
#   E2E_USER=xxx E2E_PASS=yyy E2E_SET_QUOTA=1 bash ...          # 预建账号并补足额度
#   E2E_DAY=2026-10-17 bash ...                                 # 指定日期目录（**UTC** 日期）
#   E2E_MODEL=xxx bash ...                                      # 指定在线模型
#
# 前置条件：
#   - 生产通常禁用密码注册 → 必须先用 SQL 预建 E2E_USER；
#   - 新用户 quota=0，补额度后需删除 Redis `user:<id>`（脚本会做）；
#   - 依赖容器名 `postgres` / `redis` 与库名 `new-api`（与生产 compose 一致）；
#   - 服务器有 python3（校验 gzip/JSONL）。
set -euo pipefail

BASE="${1:-}"
if [ -z "$BASE" ]; then
  for p in 3000 3001; do
    if curl -s -o /dev/null --max-time 2 "http://127.0.0.1:$p/api/status"; then
      BASE="http://127.0.0.1:$p"; break
    fi
  done
fi
[ -n "$BASE" ] || { echo "[e2e] ✗ 无法探测本地端口（试过 3000/3001），请显式传入 baseURL" >&2; exit 1; }
ENVF="${ENVF:-/opt/new-api/.env}"
TS=$(date +%s)
DAY="${E2E_DAY:-$(date -u +%F)}"
U="${E2E_USER:-capture_e2e_$TS}"
P="${E2E_PASS:-CaptureE2E!2026}"
MODEL="${E2E_MODEL:-deepseek-v4.1-flash}"

# 用户名会拼进 SQL（预建账号查找）与云盘目录名；限制字符集，避免引号破坏语句。
case "$U" in
  *[!A-Za-z0-9_.-]*) echo "[e2e] ✗ E2E_USER 只允许字母/数字/_.-：$U" >&2; exit 1 ;;
esac

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

say() { echo "[e2e] $*"; }
die() { echo "[e2e] ✗ $*" >&2; exit 1; }

# env_get <key>：读 .env 的值，缺失时输出空串且**不因 set -e 中断**。
env_get() { grep -m1 "^$1=" "$ENVF" 2>/dev/null | cut -d= -f2- || true; }

# 账号前缀：1 → TRAINING_ONEDRIVE_；N → TRAINING_ONEDRIVE_<N>_
acct_prefix() {
  if [ "$1" = "1" ]; then echo "TRAINING_ONEDRIVE_"; else echo "TRAINING_ONEDRIVE_$1_"; fi
}

# 发现实际配置了几个账号（与 LoadOneDriveConfigsFromEnv 的规则一致：以 TENANT 为准）。
ACCOUNTS=()
if [ -z "$(env_get "TRAINING_ONEDRIVE_TENANT")" ]; then
  die "$ENVF 里没有 TRAINING_ONEDRIVE_TENANT（云盘未配置）"
fi
ACCOUNTS+=(1)
for n in $(seq 2 9); do
  pre="$(acct_prefix "$n")"
  if [ -n "$(env_get "${pre}TENANT")" ]; then ACCOUNTS+=("$n"); fi
done
say "云盘账号数：${#ACCOUNTS[@]}（索引：${ACCOUNTS[*]}）"

# graph_token <accountIndex> → 打印 access_token（不回显）
graph_token() {
  local idx="$1" pre T C S
  pre="$(acct_prefix "$idx")"
  T=$(env_get "${pre}TENANT"); C=$(env_get "${pre}CLIENT_ID"); S=$(env_get "${pre}CLIENT_SECRET")
  [ -n "$T" ] && [ -n "$C" ] && [ -n "$S" ] || return 1
  curl -sS --max-time 25 -X POST "https://login.microsoftonline.com/$T/oauth2/v2.0/token" \
    --data-urlencode "client_id=$C" --data-urlencode "client_secret=$S" \
    --data-urlencode "scope=https://graph.microsoft.com/.default" \
    --data-urlencode "grant_type=client_credentials" |
    sed -n 's/.*"access_token":"\([^"]*\)".*/\1/p'
}

drive_id() {
  local idx="$1" pre
  pre="$(acct_prefix "$idx")"
  env_get "${pre}DRIVE_ID"
}

# RootPath：主账号未配置 → drive 根目录（空）；第 2+ 账号未配置 → 继承主账号。
# 与 service/training_capture/onedrive.go 的实现一致，**不能**硬编码 training-data，
# 否则生产改了根目录就会误报「没找到」。
root_path() {
  local idx="$1" pre r
  pre="$(acct_prefix "$idx")"
  r=$(env_get "${pre}ROOT_PATH")
  if [ -z "$r" ] && [ "$idx" != "1" ]; then
    r=$(env_get "TRAINING_ONEDRIVE_ROOT_PATH")
  fi
  printf '%s' "$r" | sed 's|^/*||; s|/*$||'
}

# join_path <root> <child>：root 为空时不要产生前导斜杠。
join_path() {
  if [ -z "$1" ]; then printf '%s' "$2"; else printf '%s/%s' "$1" "$2"; fi
}

# names_of <json> → 逐行输出所有 "name":"..."（Graph 的字段顺序不保证，
# 不要用「name 后面紧跟 folder/size」这种顺序敏感的 sed）。
names_of() {
  printf '%s' "$1" | grep -o '"name":"[^"]*"' | sed 's/^"name":"//; s/"$//'
}

# nextlink_of <json> → 取出 @odata.nextLink（处理 & 与 \/ 转义）
nextlink_of() {
  printf '%s' "$1" |
    sed -n 's/.*"@odata.nextLink":"\([^"]*\)".*/\1/p' | head -1 |
    sed 's/\\u0026/\&/g; s/\\\//\//g'
}

# list_children_all <token> <drive> <path> → 把所有分页的 JSON 依次打印出来。
# 只取第一页会漏掉 >200 项的目录（真实数据集里很常见）。
list_children_all() {
  local tok="$1" d="$2" p="$3" url body n=0
  url="https://graph.microsoft.com/v1.0/drives/$d/root:/${p}:/children?\$select=name,size,folder&top=200"
  while [ -n "$url" ] && [ "$n" -lt 20 ]; do
    # 单次网络抖动不应中止整个探针：轮询循环会在下一轮重试。
    body=$(curl -sS --max-time 25 -H "Authorization: Bearer $tok" "$url" || true)
    if [ -z "$body" ] || printf '%s' "$body" | grep -q '"error"'; then
      # 把错误体打出来（不含凭据），让「读失败」与「目录为空」可区分。
      printf '%s\n' "$body"
      return 0
    fi
    printf '%s\n' "$body"
    url=$(nextlink_of "$body")
    n=$((n + 1))
  done
  if [ "$n" -ge 20 ]; then
    echo "WARN: 分页超过 20 页，结果可能不完整" >&2
  fi
}

if [ -n "${E2E_USER:-}" ]; then
  say "1) 使用预建账号 $U（该环境已禁用密码注册）"
else
  say "1) 注册一次性用户 $U"
  reg=$(curl -sS --max-time 20 -X POST "$BASE/api/user/register" -H 'Content-Type: application/json' \
    -d "{\"username\":\"$U\",\"password\":\"$P\"}")
  say "   register: $(printf '%s' "$reg" | head -c 140)"
fi

say "2) 登录"
login=$(curl -sS --max-time 20 -X POST "$BASE/api/user/login" -H 'Content-Type: application/json' \
  -d "{\"username\":\"$U\",\"password\":\"$P\"}")
ATOK=$(printf '%s' "$login" | sed -n 's/.*"access_token":"\([^"]*\)".*/\1/p')
[ -n "$ATOK" ] || die "登录失败：$(printf '%s' "$login" | head -c 200)"
say "   ✓ 拿到 access_token"

say "3) 读取采集授权状态（应 available=true，因为云盘已配）"
st=$(curl -sS --max-time 20 -H "Authorization: Bearer $ATOK" "$BASE/api/user/training-data/status")
AVAIL=$(printf '%s' "$st" | sed -n 's/.*"available":\([a-z]*\).*/\1/p')
PV=$(printf '%s' "$st" | sed -n 's/.*"policy_version":"\([^"]*\)".*/\1/p')
say "   available=$AVAIL policy_version=$PV"
[ "$AVAIL" = "true" ] || die "功能对用户不可用（云盘未生效？）"

say "4) 同意采集（真实授权接口）"
grant=$(curl -sS --max-time 20 -X POST "$BASE/api/user/training-data/grant" \
  -H "Authorization: Bearer $ATOK" -H 'Content-Type: application/json' \
  -d "{\"policy_version\":\"$PV\"}")
GRANTED=$(printf '%s' "$grant" | sed -n 's/.*"granted":\([a-z]*\).*/\1/p')
say "   granted=$GRANTED"
[ "$GRANTED" = "true" ] || die "授权失败：$(printf '%s' "$grant" | head -c 200)"

say "5) 检查额度"
UID_=$(docker exec postgres psql -U newapi -d new-api -tAc \
  "select id from users where username = '$U'" 2>/dev/null | tr -d '[:space:]' || true)
[ -n "$UID_" ] || die "找不到用户 $U"
QUOTA=$(docker exec postgres psql -U newapi -d new-api -tAc \
  "select quota from users where id=$UID_" 2>/dev/null | tr -d '[:space:]' || echo 0)
say "   user_id=$UID_ quota=$QUOTA"
if [ -n "${E2E_USER:-}" ] && [ "${E2E_SET_QUOTA:-0}" != "1" ]; then
  say "   预建账号：**不修改其额度**（需要补额度请显式设置 E2E_SET_QUOTA=1）"
else
  # 只补不降：绝不动比 2000000 更高的真实余额。
  docker exec postgres psql -U newapi -d new-api \
    -c "update users set quota=2000000 where id=$UID_ and quota < 2000000" >/dev/null 2>&1 ||
    die "补额度失败"
  # **直接改库后必须清用户缓存**：用户信息有 Redis 缓存（user:<id>，TTL 60s），
  # 不清就会继续读到旧额度 —— 请求仍报「额度不足」，很容易被误判成「充值没生效」。
  # Redis 连接串只在容器 env 里（.env 没有），从容器读，且不回显密码。
  REDIS_URL=$(docker exec new-api printenv REDIS_CONN_STRING 2>/dev/null | head -1 || true)
  if [ -n "$REDIS_URL" ]; then
    RPASS=$(printf '%s' "$REDIS_URL" | sed -n 's|redis://:\([^@]*\)@.*|\1|p')
    # 用 REDISCLI_AUTH 传密码，避免 `-a` 把口令暴露在进程参数里。
    docker exec -e REDISCLI_AUTH="$RPASS" redis redis-cli DEL "user:$UID_" >/dev/null 2>&1 || true
  fi
  say "   额度已补足到 2000000（仅当原值更低），并已清除用户缓存"
fi

say "6) 创建短期、限额度 API 密钥（1 小时 / 100000 额度）"
tok=$(curl -sS --max-time 20 -X POST "$BASE/api/token/" -H "Authorization: Bearer $ATOK" \
  -H 'Content-Type: application/json' \
  -d "{\"token\":{\"name\":\"e2e-capture-$TS\",\"remain_quota\":100000,\"unlimited_quota\":false,\"expired_time\":$((TS + 3600)),\"group\":\"default\"}}")
KEY=$(printf '%s' "$tok" | sed -n 's/.*"key":"\([^"]*\)".*/\1/p')
[ -n "$KEY" ] || die "建密钥失败：$(printf '%s' "$tok" | head -c 200)"
TOKEN_ID=$(printf '%s' "$tok" | sed -n 's/.*"id":\([0-9]*\).*/\1/p' | head -1)
say "   ✓ 密钥已创建 (id=${TOKEN_ID:-?})"

say "7) 发一次真实补全请求（max_tokens=16）"
# 请求内容里带时间戳：无会话 id 时采集走**全局指纹去重**，
# 两次跑完全相同的内容会被正确判为重复而不再上传 ——
# 探针必须用唯一内容，否则会把「去重生效」误读成「采集没生效」。
PROBE="CAPTURE-E2E-$TS 请只回复两个字：收到"
resp=$(curl -sS --max-time 120 -X POST "$BASE/v1/chat/completions" \
  -H "Authorization: Bearer sk-$KEY" -H 'Content-Type: application/json' \
  -d "{\"model\":\"$MODEL\",\"messages\":[{\"role\":\"user\",\"content\":\"$PROBE\"}],\"max_tokens\":16,\"stream\":false}")
say "   relay: $(printf '%s' "$resp" | head -c 220)"
printf '%s' "$resp" | grep -q '"choices"' || die "补全请求失败（额度/模型/渠道？可用 E2E_MODEL 指定在线模型）"

say "8) 等待后台 worker 上传（最多 90s，逐账号查找）"
found_acct=""; found_root=""; found_dir=""
for _ in $(seq 1 18); do
  for IDX in "${ACCOUNTS[@]}"; do
    TK=$(graph_token "$IDX" || true); DR=$(drive_id "$IDX")
    [ -n "$TK" ] && [ -n "$DR" ] || continue
    ROOT=$(root_path "$IDX")
    DAY_PATH=$(join_path "$ROOT" "$DAY")
    body=$(list_children_all "$TK" "$DR" "$DAY_PATH")
    names=$(names_of "$body" || true)
    # 用 here-string 而不是 `names_of | grep -q`：后者在匹配后提前退出会让
    # 上游 sed 收到 SIGPIPE（141），pipefail 下整个条件变成假 → 误判「没找到」。
    if grep -qF -- "$U" <<<"$names" || grep -qF -- "-$UID_" <<<"$names"; then
      found_acct="$IDX"; found_root="$ROOT"
      found_dir=$(grep -F -- "$U" <<<"$names" | head -1 || true)
      [ -n "$found_dir" ] || found_dir=$(grep -F -- "-$UID_" <<<"$names" | head -1 || true)
      break 2
    fi
  done
  sleep 5
done
[ -n "$found_acct" ] || die "90s 内云盘上没找到用户 $U 的目录（docker logs new-api | grep -i 'training capture'）"
FOUND_DAY_PATH=$(join_path "$found_root" "$DAY")
say "   ✓ 落在账号 #$found_acct 的目录：$FOUND_DAY_PATH/$found_dir"

say "9) 下钻到模型目录并校验分片"
TK=$(graph_token "$found_acct" || true)
[ -n "$TK" ] || die "账号 #$found_acct 取 token 失败"
DR=$(drive_id "$found_acct")
sub=$(list_children_all "$TK" "$DR" "$FOUND_DAY_PATH/$found_dir")
model_dir=$(names_of "$sub" | grep -v '^part-' | head -1 || true)
[ -n "$model_dir" ] || die "用户目录下没有模型子目录"
say "   模型目录：$model_dir"
files=$(list_children_all "$TK" "$DR" "$FOUND_DAY_PATH/$found_dir/$model_dir")
say "   分片列表：$(printf '%s' "$files" | head -c 260)"
part=$(names_of "$files" | grep '^part-' | head -1 || true)
[ -n "$part" ] || die "模型目录下没有分片"

# 下载并解压（Graph 的 /content 会 302 到下载地址，-L 跟随）
curl -fsSL --max-time 60 -H "Authorization: Bearer $TK" \
  "https://graph.microsoft.com/v1.0/drives/$DR/root:/$FOUND_DAY_PATH/$found_dir/$model_dir/$part:/content" \
  -o "$TMP/part.gz" || die "下载分片失败（HTTP 错误或网络中断）"
say "   分片大小：$(wc -c <"$TMP/part.gz") 字节"

# 校验失败必须让整个探针失败（早期版本没有检查退出码，
# python 报错后脚本仍然打印“完成”——把失败当成功）。
python3 - "$TMP/part.gz" <<'PY' || die "分片内容校验失败"
import gzip, json, sys
raw = open(sys.argv[1], 'rb').read()
try:
    plain = gzip.decompress(raw)
except Exception as e:
    print(f"   ✗ 不是合法 gzip: {e}")
    sys.exit(1)
lines = [l for l in plain.decode('utf-8').splitlines() if l.strip()]
print(f"   解压后 {len(lines)} 行 JSONL")
roles = []
kinds = []
for l in lines:
    rec = json.loads(l)
    roles.append(rec.get("role"))
    if rec.get("role") == "request":
        kinds.append(rec.get("kind"))
    print(f"     - role={rec.get('role')} kind={rec.get('kind','-')} len={len(rec.get('content',''))}")
assert any(r == "request" for r in roles), "缺少请求侧记录"
# `kind` 是消费方契约（full/suffix/message），缺失会让清洗脚本静默污染数据。
assert kinds and all(k in ("full", "suffix", "message") for k in kinds), \
    f"request 行缺少合法 kind（实际 {kinds}）"
if "assistant" in roles:
    print("   ✓ 含 assistant 行：回复采集链路已生效")
else:
    print("   ⚠ 本次没有 assistant 行：可能是模型只回了 reasoning/工具调用（正文为空），"
          "不一定是采集失败 —— 需要用有正文的模型再测")
print("   ✓ 分片可独立解压、每行都是合法 JSON、request 行含合法 kind")
PY

say "完成。一次性用户：$U (id=$UID_)"
say "清理命令（确认后执行）："
echo "  docker exec postgres psql -U newapi -d new-api -c \"delete from tokens where user_id=$UID_\""
echo "  docker exec postgres psql -U newapi -d new-api -c \"delete from training_data_consents where user_id=$UID_\""
echo "  docker exec postgres psql -U newapi -d new-api -c \"delete from training_data_watermarks where user_id=$UID_\""
echo "  docker exec postgres psql -U newapi -d new-api -c \"delete from users where id=$UID_\""
echo "  # 云盘目录（该账号 token）：DELETE .../root:/$FOUND_DAY_PATH/$found_dir:/   # 注意 ROOT_PATH 为空时没有前导目录"
