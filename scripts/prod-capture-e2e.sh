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
#   bash prod-capture-e2e.sh [baseURL]
#   E2E_USER=xxx E2E_PASS=yyy bash prod-capture-e2e.sh          # 预建账号，不改额度
#   E2E_USER=xxx E2E_PASS=yyy E2E_SET_QUOTA=1 bash ...          # 预建账号并补足额度
#   E2E_DAY=2026-10-17 bash ...                                 # 指定日期目录
set -euo pipefail

BASE="${1:-http://127.0.0.1:3001}"
ENVF="${ENVF:-/opt/new-api/.env}"
TS=$(date +%s)
DAY="${E2E_DAY:-$(date -u +%F)}"
U="${E2E_USER:-capture_e2e_$TS}"
P="${E2E_PASS:-CaptureE2E!2026}"

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

# RootPath 每个账号可以不同（默认 training-data）。**不能写死**。
root_path() {
  local idx="$1" pre r
  pre="$(acct_prefix "$idx")"
  r=$(env_get "${pre}ROOT_PATH")
  echo "${r:-training-data}"
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
    body=$(curl -sS --max-time 25 -H "Authorization: Bearer $tok" "$url")
    printf '%s\n' "$body"
    url=$(nextlink_of "$body")
    n=$((n + 1))
  done
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
  say "   额度已补足到 2000000（仅当原值更低）"
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
  -d "{\"model\":\"deepseek-v4.1-flash\",\"messages\":[{\"role\":\"user\",\"content\":\"$PROBE\"}],\"max_tokens\":16,\"stream\":false}")
say "   relay: $(printf '%s' "$resp" | head -c 220)"
printf '%s' "$resp" | grep -q '"choices"' || die "补全请求失败（额度/模型/渠道？）"

say "8) 等待后台 worker 上传（最多 90s，逐账号查找）"
found_acct=""; found_root=""; found_dir=""
for _ in $(seq 1 18); do
  for IDX in "${ACCOUNTS[@]}"; do
    TK=$(graph_token "$IDX" || true); DR=$(drive_id "$IDX")
    [ -n "$TK" ] && [ -n "$DR" ] || continue
    ROOT=$(root_path "$IDX")
    body=$(list_children_all "$TK" "$DR" "$ROOT/$DAY")
    if printf '%s' "$body" | grep -q "$U"; then
      found_acct="$IDX"; found_root="$ROOT"
      found_dir=$(printf '%s' "$body" |
        sed -n "s/.*\"name\":\"\([^\"]*${U}[^\"]*\)\".*/\1/p" | head -1)
      break 2
    fi
  done
  sleep 5
done
[ -n "$found_acct" ] || die "90s 内云盘上没找到 $U 的目录（docker logs new-api | grep -i 'training capture'）"
say "   ✓ 落在账号 #$found_acct 的目录：$found_root/$DAY/$found_dir"

say "9) 下钻到模型目录并校验分片"
TK=$(graph_token "$found_acct"); DR=$(drive_id "$found_acct")
sub=$(list_children_all "$TK" "$DR" "$found_root/$DAY/$found_dir")
model_dir=$(printf '%s' "$sub" | sed -n 's/.*"name":"\([^"]*\)","folder".*/\1/p' | head -1)
[ -n "$model_dir" ] || die "用户目录下没有模型子目录"
say "   模型目录：$model_dir"
files=$(list_children_all "$TK" "$DR" "$found_root/$DAY/$found_dir/$model_dir")
say "   分片列表：$(printf '%s' "$files" | head -c 260)"
part=$(printf '%s' "$files" | sed -n 's/.*"name":"\(part-[^"]*\)".*/\1/p' | head -1)
[ -n "$part" ] || die "模型目录下没有分片"

# 下载并解压（Graph 的 /content 会 302 到下载地址，-L 跟随）
curl -sSL --max-time 60 -H "Authorization: Bearer $TK" \
  "https://graph.microsoft.com/v1.0/drives/$DR/root:/$found_root/$DAY/$found_dir/$model_dir/$part:/content" \
  -o "$TMP/part.gz"
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
for l in lines:
    rec = json.loads(l)
    roles.append(rec.get("role"))
    print(f"     - role={rec.get('role')} kind={rec.get('kind','-')} len={len(rec.get('content',''))}")
assert any(r == "request" for r in roles), "缺少请求侧记录"
print("   ✓ 分片可独立解压、每行都是合法 JSON、含请求侧记录")
PY

say "完成。一次性用户：$U (id=$UID_)"
say "清理命令（确认后执行）："
echo "  docker exec postgres psql -U newapi -d new-api -c \"delete from tokens where user_id=$UID_\""
echo "  docker exec postgres psql -U newapi -d new-api -c \"delete from training_data_consents where user_id=$UID_\""
echo "  docker exec postgres psql -U newapi -d new-api -c \"delete from training_data_watermarks where user_id=$UID_\""
echo "  docker exec postgres psql -U newapi -d new-api -c \"delete from users where id=$UID_\""
echo "  # 云盘目录（用上面的账号 token）：DELETE .../root:/$found_root/$DAY/$found_dir:/"
