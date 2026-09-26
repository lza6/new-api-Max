#!/bin/bash
# new-api blue-green zero-downtime update v3
# Fix over v2: never kill a live upstream that Caddy still routes to.
#   state A: switch to :3000-only (master) BEFORE replacing standby
#   state B: switch to :3002-only BEFORE replacing master
#   state C: switch back to dual :3000 :3002
# Rollback: ERR trap restores a config whose backends are known-healthy.
set -euo pipefail
TAG="${1:?usage: rolling-update-newapi-v3.sh <IMAGE_TAG>}"
IMAGE="ghcr.io/lza6/new-api-max:${TAG}"
cd /opt/new-api
CA=/etc/caddy/Caddyfile

gen_caddy() { # gen_caddy UPSTREAMS
  python3 - "$1" <<'PY'
import sys
up = ' '.join('127.0.0.1:' + h for h in sys.argv[1].split())
open('/etc/caddy/Caddyfile','w').write("""{
    email admin@tingfengai.art
    servers {
        protocols h1 h2
    }
}

freeapi.tingfengai.art {
    encode zstd gzip

    header {
        Strict-Transport-Security "max-age=31536000; includeSubDomains"
        X-Content-Type-Options "nosniff"
        Referrer-Policy "strict-origin-when-cross-origin"
        -Server
    }

    handle_path /assets/* {
        header Cache-Control "public, max-age=31536000, immutable"
        reverse_proxy %(up)s {
            health_uri /api/status
            health_interval 1s
            health_timeout 2s
            health_fails 3
            health_passes 2
            lb_policy round_robin
        }
    }

    reverse_proxy %(up)s {
        health_uri /api/status
        health_interval 1s
        health_timeout 2s
        health_fails 3
        health_passes 2
        lb_policy round_robin
        flush_interval -1
        header_up X-Real-IP {remote_host}
        header_up X-Forwarded-For {header.X-Forwarded-For}
        transport http {
            versions 1.1 2
        }
    }
}
""" % {"up": up})
PY
}
caddy_reload() {
  caddy validate --config "$CA" --adapter caddyfile >/dev/null 2>&1 || { echo "ERROR: config invalid, NOT reloading"; return 1; }
  systemctl reload caddy
  sleep 2
  systemctl is-active caddy >/dev/null
}
restore_master() { gen_caddy "3000"; caddy_reload || true; }
restore_standby() { gen_caddy "3002"; caddy_reload || true; }
trap 'restore_master' ERR

SQL_PW=$(grep -oP 'SQL_DSN=postgresql://newapi:\K[^@]+' docker-compose.yml | head -1)
REDIS_PW=$(grep -oP 'REDIS_CONN_STRING=redis://:\K[^@]+' docker-compose.yml | head -1)
SQL_DSN="postgresql://newapi:${SQL_PW}@postgres:5432/new-api"
REDIS_DSN="redis://:${REDIS_PW}@redis:6379"

echo "==> pulling ${IMAGE}"
docker pull "${IMAGE}"

# A) if a standby(:3002) exists and is live-routed, switch to master-only first
if docker inspect new-api-next >/dev/null 2>&1; then
  echo "==> pre-switch: Caddy -> :3000 only (so standby can be replaced safely)"
  gen_caddy "3000"
  caddy_reload || { echo "ERROR: pre-switch failed"; exit 1; }
fi

echo "==> (re)create standby on :3002 (new version)"
docker rm -f new-api-next >/dev/null 2>&1 || true
docker run -d --name new-api-next --restart always \
  -p 3002:3000 \
  -v /opt/new-api/data:/data -v /opt/new-api/logs:/app/logs \
  --network new-api_new-api-network \
  --env-file /opt/new-api/.env \
  --env SQL_DSN="${SQL_DSN}" --env REDIS_CONN_STRING="${REDIS_DSN}" \
  --env TZ=Asia/Shanghai --env ERROR_LOG_ENABLED=true --env BATCH_UPDATE_ENABLED=true \
  --env NODE_NAME=new-api-next --env NODE_TYPE=slave --env MEMORY_CACHE_ENABLED=true \
  --env LOG_FLUSH_ENABLED=true --env LOG_FLUSH_INTERVAL=1000 --env LOG_FLUSH_BATCH=500 \
  --env RELAY_TIMEOUT=900 --env SQL_MAX_OPEN_CONNS=32 --env SQL_MAX_IDLE_CONNS=8 --env SQL_MAX_LIFETIME=300 \
  "${IMAGE}" --log-dir /app/logs >/dev/null
ok=""
# 预热：连续 3 次健康通过才算 ready，避免切流到半启动实例导致 503。
passes=0
for i in $(seq 1 60); do
  if curl -sf http://127.0.0.1:3002/api/status >/dev/null 2>&1; then
    passes=$((passes+1))
    if [ "$passes" -ge 3 ]; then echo "standby healthy after ${i}s (3x)"; ok=1; break; fi
  else
    passes=0
  fi
  sleep 1
done
[ -n "$ok" ] || { echo "ERROR: standby not healthy; restoring master-only"; restore_master; exit 1; }

# B) switch to standby-only
echo "==> atomic switch: Caddy -> :3002 only"
gen_caddy "3002"
caddy_reload || { restore_master; echo "ERROR: switch failed, restored master-only"; exit 1; }

# replace master
echo "==> replacing master via compose (new image)"
sed -i "s#ghcr.io/lza6/new-api-max:.*#${IMAGE}#" docker-compose.yml
docker compose up -d --no-deps new-api
ok=""
# 预热：连续 3 次健康通过才算 ready。
passes=0
for i in $(seq 1 90); do
  if curl -sf http://127.0.0.1:3000/api/status >/dev/null 2>&1; then
    passes=$((passes+1))
    if [ "$passes" -ge 3 ]; then echo "master healthy after ${i}s (3x)"; ok=1; break; fi
  else
    passes=0
  fi
  sleep 1
done
[ -n "$ok" ] || { echo "ERROR: master not healthy; staying on :3002"; restore_standby; exit 1; }

# C) dual
echo "==> atomic switch: Caddy -> dual :3000 :3002"
gen_caddy "3000 3002"
caddy_reload || { restore_standby; echo "ERROR: dual switch failed, restored :3002"; exit 1; }

echo "==> removing legacy standby :3001"
docker rm -f new-api-rolling >/dev/null 2>&1 || true

echo "==> done. public:"
curl -s -o /dev/null -w 'status=%{http_code}\n' --max-time 8 https://freeapi.tingfengai.art/api/status
docker ps --format '{{.Names}} {{.Image}} {{.Status}}' | grep new-api