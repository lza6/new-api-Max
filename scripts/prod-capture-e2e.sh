#!/usr/bin/env bash
# 生产环境「训练数据采集」真实 E2E 探针。
#
# 做一件只用单测无法证明的事：**在真实网关上跑一次真实请求，然后去云盘上
# 把文件捞下来解压、逐行校验**。证明链路是：
#   注册 → 授权 → 计费请求 → 队列 → worker → 分片压缩 → 云盘落盘
#
# 安全边界：
#   - 只读 `/opt/new-api/.env` 里的凭据，**不打印任何密钥**；
#   - 用一次性用户（`capture_e2e_<时间戳>`），跑完由操作者决定是否清理；
#   - 只发**一次**极小的补全请求（max_tokens=16）。
#
# 用法（在生产服务器上）：bash prod-capture-e2e.sh [baseURL]
set -uo pipefail

BASE="${1:-http://127.0.0.1:3001}"
ENVF=/opt/new-api/.env
TS=$(date +%s)
# 生产关闭了密码注册（只允许第三方登录）。因此支持：
#   - 默认：走真实注册接口（本地/开放注册的环境）；
#   - 给了 E2E_USER / E2E_PASS：跳过注册，用已有账号（生产用 SQL 预建）。
U="${E2E_USER:-capture_e2e_$TS}"
P="${E2E_PASS:-CaptureE2E!2026}"
DAY=$(date -u +%F)

say() { echo "[e2e] $*"; }

# graph_token <accountIndex 1|2> → 打印 access_token（不回显）
graph_token() {
  local idx="$1" pre
  if [ "$idx" = "1" ]; then pre="TRAINING_ONEDRIVE_"; else pre="TRAINING_ONEDRIVE_${idx}_"; fi
  local T C S
  T=$(grep "^${pre}TENANT=" "$ENVF" | cut -d= -f2-)
  C=$(grep "^${pre}CLIENT_ID=" "$ENVF" | cut -d= -f2-)
  S=$(grep "^${pre}CLIENT_SECRET=" "$ENVF" | cut -d= -f2-)
  curl -s --max-time 25 -X POST "https://login.microsoftonline.com/$T/oauth2/v2.0/token" \
    --data-urlencode "client_id=$C" --data-urlencode "client_secret=$S" \
    --data-urlencode "scope=https://graph.microsoft.com/.default" \
    --data-urlencode "grant_type=client_credentials" |
    sed -n 's/.*"access_token":"\([^"]*\)".*/\1/p'
}

drive_id() {
  local idx="$1" pre
  if [ "$idx" = "1" ]; then pre="TRAINING_ONEDRIVE_"; else pre="TRAINING_ONEDRIVE_${idx}_"; fi
  grep "^${pre}DRIVE_ID=" "$ENVF" | cut -d= -f2-
}

# list_children <token> <drive> <path> → JSON
list_children() {
  local tok="$1" d="$2" p="$3"
  curl -s --max-time 25 -H "Authorization: Bearer $tok" \
    "https://graph.microsoft.com/v1.0/drives/$d/root:/${p}:/children?\$select=name,size,folder&top=50"
}

if [ -n "${E2E_USER:-}" ]; then
  say "1) 使用预建账号 $U（该环境已禁用密码注册）"
else
  say "1) 注册一次性用户 $U"
  reg=$(curl -s --max-time 20 -X POST "$BASE/api/user/register" -H 'Content-Type: application/json' \
    -d "{\"username\":\"$U\",\"password\":\"$P\"}")
  say "   register: $(printf '%s' "$reg" | head -c 140)"
fi

say "2) 登录"
login=$(curl -s --max-time 20 -X POST "$BASE/api/user/login" -H 'Content-Type: application/json' \
  -d "{\"username\":\"$U\",\"password\":\"$P\"}")
ATOK=$(printf '%s' "$login" | sed -n 's/.*"access_token":"\([^"]*\)".*/\1/p')
if [ -z "$ATOK" ]; then say "   ✗ 登录失败：$(printf '%s' "$login" | head -c 200)"; exit 1; fi
say "   ✓ 拿到 access_token"

say "3) 读取采集授权状态（应 available=true，因为云盘已配）"
st=$(curl -s --max-time 20 -H "Authorization: Bearer $ATOK" "$BASE/api/user/training-data/status")
AVAIL=$(printf '%s' "$st" | sed -n 's/.*"available":\([a-z]*\).*/\1/p')
PV=$(printf '%s' "$st" | sed -n 's/.*"policy_version":"\([^"]*\)".*/\1/p')
say "   available=$AVAIL policy_version=$PV"
if [ "$AVAIL" != "true" ]; then say "   ✗ 功能对用户不可用（云盘未生效？）"; exit 1; fi

say "4) 同意采集（真实授权接口）"
grant=$(curl -s --max-time 20 -X POST "$BASE/api/user/training-data/grant" \
  -H "Authorization: Bearer $ATOK" -H 'Content-Type: application/json' \
  -d "{\"policy_version\":\"$PV\"}")
GRANTED=$(printf '%s' "$grant" | sed -n 's/.*"granted":\([a-z]*\).*/\1/p')
say "   granted=$GRANTED"
if [ "$GRANTED" != "true" ]; then say "   ✗ 授权失败：$(printf '%s' "$grant" | head -c 200)"; exit 1; fi

say "5) 给一次性用户加额度（新用户默认 0）"
UID_=$(docker exec postgres psql -U newapi -d new-api -tAc "select id from users where username = '$U'" 2>/dev/null | tr -d '[:space:]')
if [ -z "$UID_" ]; then say "   ✗ 找不到刚注册的用户"; exit 1; fi
docker exec postgres psql -U newapi -d new-api -c "update users set quota=2000000 where id=$UID_" >/dev/null 2>&1
say "   user_id=$UID_ quota=2000000"

say "6) 创建 API 密钥"
tok=$(curl -s --max-time 20 -X POST "$BASE/api/token/" -H "Authorization: Bearer $ATOK" \
  -H 'Content-Type: application/json' \
  -d '{"token":{"name":"e2e-capture","unlimited_quota":true,"expired_time":-1,"group":"default"}}')
KEY=$(printf '%s' "$tok" | sed -n 's/.*"key":"\([^"]*\)".*/\1/p')
if [ -z "$KEY" ]; then say "   ✗ 建密钥失败：$(printf '%s' "$tok" | head -c 200)"; exit 1; fi
say "   ✓ 密钥已创建"

say "7) 发一次真实补全请求（max_tokens=16）"
# 请求内容里带时间戳：无会话 id 时采集走**全局指纹去重**，
# 两次跑完全相同的内容会被正确判为重复而不再上传 ——
# 探针必须用唯一内容，否则会把「去重生效」误读成「采集没生效」。
PROBE="CAPTURE-E2E-$TS 请只回复两个字：收到"
resp=$(curl -s --max-time 120 -X POST "$BASE/v1/chat/completions" \
  -H "Authorization: Bearer sk-$KEY" -H 'Content-Type: application/json' \
  -d "{\"model\":\"deepseek-v4.1-flash\",\"messages\":[{\"role\":\"user\",\"content\":\"$PROBE\"}],\"max_tokens\":16,\"stream\":false}")
say "   relay: $(printf '%s' "$resp" | head -c 220)"

say "8) 等待后台 worker 上传（最多 60s）"
found_acct=""; found_dir=""
for i in $(seq 1 12); do
  for IDX in 1 2; do
    TK=$(graph_token "$IDX"); DR=$(drive_id "$IDX")
    [ -z "$TK" ] && continue
    body=$(list_children "$TK" "$DR" "training-data/$DAY")
    if printf '%s' "$body" | grep -q "$U"; then
      found_acct="$IDX"
      found_dir=$(printf '%s' "$body" | sed -n "s/.*\"name\":\"\([^\"]*${U}[^\"]*\)\".*/\1/p" | head -1)
      break 2
    fi
  done
  sleep 5
done

if [ -z "$found_acct" ]; then
  say "   ✗ 60s 内云盘上没找到 $U 的目录"
  say "   排查：docker logs new-api | grep -i 'training capture'"
  exit 1
fi
say "   ✓ 落在账号 #$found_acct 的目录：$found_dir"

say "9) 下钻到模型目录并校验分片"
TK=$(graph_token "$found_acct"); DR=$(drive_id "$found_acct")
sub=$(list_children "$TK" "$DR" "training-data/$DAY/$found_dir")
model_dir=$(printf '%s' "$sub" | sed -n 's/.*"name":"\([^"]*\)","folder".*/\1/p' | head -1)
say "   模型目录：$model_dir"
files=$(list_children "$TK" "$DR" "training-data/$DAY/$found_dir/$model_dir")
say "   分片列表：$(printf '%s' "$files" | head -c 260)"
part=$(printf '%s' "$files" | sed -n 's/.*"name":"\(part-[^"]*\)".*/\1/p' | head -1)
if [ -z "$part" ]; then say "   ✗ 模型目录下没有分片"; exit 1; fi

# 下载并解压（Graph 的 /content 会 302 到下载地址，-L 跟随）
tmp=$(mktemp)
curl -sL --max-time 60 -H "Authorization: Bearer $TK" \
  "https://graph.microsoft.com/v1.0/drives/$DR/root:/training-data/$DAY/$found_dir/$model_dir/$part:/content" -o "$tmp"
say "   分片大小：$(wc -c <"$tmp") 字节"

python3 - "$tmp" <<'PY'
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
echo "  docker exec postgres psql -U newapi -d new-api -c \"delete from users where id=$UID_\""
echo "  docker exec postgres psql -U newapi -d new-api -c \"delete from tokens where user_id=$UID_\""
echo "  docker exec postgres psql -U newapi -d new-api -c \"delete from training_data_consents where user_id=$UID_\""
echo "  docker exec postgres psql -U newapi -d new-api -c \"delete from training_data_watermarks where user_id=$UID_\""
