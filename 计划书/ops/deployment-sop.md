# 部署 / 回滚 SOP（new-api-Max）

> 更新：2026-10-02 · 覆盖双站
> 适用：fork 仓库 `lza6/new-api-Max`（origin main）
> **主站**：`https://freeapi.tingfengai.art` · `103.233.252.213`（2C2G · 5Mbps · Ubuntu 20.04）
>   Caddy 在**宿主机**；new-api + postgres + redis 在 Docker
> **副站**：`https://japi.tingfengai.art` · `103.110.80.198`（4C4G · CentOS 7）
>   Caddy 在**容器内**（compose 管理）；new-api + postgres + redis 也在 Docker
>
> 两站**都支持零停机**（各自的脚本不同，机制差异见 §2）；`deploy.sh`（先停后起，5-15s 中断）仅在必要时用。

---

## 0. 铁律

1. **发布默认用零停机脚本**；`deploy.sh` 会先停后起（5-15s 中断），仅在必要时用。
2. **不做自动部署**：只有用户明确要求时才动生产。
3. **镜像本地构建**：不依赖 ghcr / watchtower。
4. **发布前先备份** 配置；任一步失败脚本自动回滚，旧容器全程在服务。
5. 发版顺序：`VERSION` bump → commit → push main → `git push origin <tag>` → 服务器部署。

---

## 1. 标准发布（零停机，双站）

| 站点 | 脚本 | 机制 |
|---|---|---|
| 主站 | `/opt/new-api/deploy-zero-downtime.sh <tag>` | 宿主 Caddy + **host 端口蓝绿**（3000/3001 交替） |
| 副站 | `/opt/new-api/deploy-zero-downtime.sh <tag>` | 容器内 Caddy + **容器名蓝绿**（new-api-a/new-api-b 交替） |
| 副站回滚 | `/opt/new-api/rollback-zero-downtime.sh <tag>` | 同副站蓝绿机制 |

### 主站流程（脚本内自动）
1. 探测 Caddy 当前上游**端口**（3000/3001）→ 选空闲端口
2. `git fetch --force` tag + `checkout` 源码
3. `docker build`（约 6-8 分钟）
4. `docker run` 新容器（临时名 `new-api-next`，`NODE_TYPE=slave`，绑定空闲端口）
5. 轮询 `http://127.0.0.1:<port>/api/status` 健康
6. `sed` 改 **宿主** `/etc/caddy/Caddyfile` 上游端口 + `caddy reload`
7. 外网自检 → 停旧容器 → `docker rename` 新容器为 `new-api`

### 副站流程（脚本内自动）
1. 读 `caddy/Caddyfile` 当前上游**容器名**（new-api-a / new-api-b）→ 目标是另一个
2. 构建镜像 → `docker run` 新容器（目标名，`slave`、复用旧容器 env、同网络）
3. 用当前容器（自带 wget）在**容器网络内**探测新容器健康
4. `sed` 改 `caddy/Caddyfile` 上游容器名 + `docker exec caddy caddy reload`
5. 外网自检 → **等待 12s 排空**（关键）→ 停旧容器

> ⚠️ **副站架构关键约束（踩坑换来的，勿破）**：
> - 副站 Caddy 的 Caddyfile 必须**目录挂载**（`./caddy:/etc/caddy:ro`），**不能单文件挂载**。
>   单文件 `:ro` bind mount 绑定挂载时刻的 inode，宿主改文件容器**永远读不到** →
>   `caddy reload` 永远加载旧配置 → 切流失败/删旧容器 502。
> - 切换后**必须等待 ~12s 再删旧容器**：Caddy 与旧上游的 keep-alive 连接/健康检查器
>   需时间排空，立即删会 502。
> - 交替命名（a/b）**不要用 rename**：rename 切换名字有一瞬窗口。


**前置检查**：可用内存 > 400MB（脚本自动判断，不足则 abort）。

**实测基线（v1.3.58 → v1.3.59）**：构建期外部 25 连打 **25/25=200**；切流耗时 **4 秒**；数据完整。

### ⚠️ 关键约束
- 新容器**必须** `NODE_TYPE=slave`：后台任务（`subscription_reset_task` / `auth_cleanup` / `authz.Init` / task event cleanup / web protection cleanup）由 `common.IsMasterNode` 门控，master 会与旧容器**重复执行**。
- 环境变量：脚本用 `--env-file /opt/new-api/.env` + 显式 `SQL_DSN`/`REDIS_CONN_STRING`（`.env` 内不含这两项）。
- **部署后 Caddy 上游可能是 3001**（端口交替）：排查前先 `grep reverse_proxy /etc/caddy/Caddyfile`。

---

## 2. 回滚（零停机，1 分钟内）

```bash
cd /opt/new-api
./rollback.sh v1.3.58          # 目标 tag 镜像必须已在本机（docker images new-api）
```

机制与发布相同（蓝绿 + Caddy reload），失败自动恢复 Caddy 上游，旧容器不停。
若目标 tag 镜像不在本机：`cd /opt/new-api-src && git checkout <tag> && docker build -t new-api:<tag> .`

### 安全类放宽的回滚（无需重新部署）
两个验证开关改回 `true` 即恢复旧行为（系统设置 → 安全 → Token 限制，热更新）：
- `token_setting.require_verification_to_read_own_key`
- `token_setting.require_verification_to_read_channel_key`

---

## 3. 应急：普通滚动（`deploy.sh`，会有短暂中断）

```bash
cd /opt/new-api
./deploy.sh v1.3.60     # 见 §1 步骤 2-5，但用 docker compose up（先停后起，5-15s 中断）
```
仅在零停机脚本不可用（如内存不足、磁盘告急）时使用。

---

## 4. 发版前质量门（本地）

```bash
# 后端
go build ./... && go vet ./... && (cd relaykit && GOWORK=off go build ./...)
go test ./controller/ ./middleware/ ./common/ ./setting/... -count=1
# 前端
cd web && bun run typecheck && bunx oxlint -c .oxlintrc.json <改动文件> && bunx vitest run <相关目录>
bun run i18n:sync    # 新增文案必须 7 语言回填，locale-consistency 测试须通过
```

---

## 5. 线上验收（每次发布后）

```bash
curl -sI https://freeapi.tingfengai.art/api/status | grep -i x-new-api-version   # 版本号
for i in $(seq 1 30); do curl -s -o /dev/null -w '%{http_code} ' -m 8 https://freeapi.tingfengai.art/api/status; done; echo
docker ps --format '{{.Names}}\t{{.Image}}\t{{.Status}}'                          # 容器 healthy
docker exec postgres psql -U newapi -d new-api -t -c 'SELECT count(*) FROM users;'  # 数据完整
docker logs new-api --since 10m 2>&1 | grep -cE '\| 5[0-9]{2} \|'                 # 5xx 计数
```
证据存档：`计划书/e2e-evidence/prod-v<tag>-acceptance.json`。

---

## 6. 排障速查

| 现象 | 先查 |
|---|---|
| 全站 503 | `free -m` / `docker exec redis redis-cli INFO memory` / Caddy 日志 `no upstreams available` |
| 单模型「无可用渠道」503 | 渠道是否被删/禁用；`abilities` 表该模型是否有 `enabled=t` 且渠道 status=1 |
| Redis 内存高但 DBSIZE 小 | `docker exec redis redis-cli -a <pwd> CLIENT LIST \| grep monitor`（MONITOR 连接吃内存） |
| 版本没变 | Caddy 上游端口（3000/3001）；`docker inspect new-api --format '{{.Config.Image}}'` |
| 迭代期请求中断 | 用 `deploy-zero-downtime.sh`（先起新后停旧） |

---

## 7. 已知生产约束（不可变更）

- **2C2G / 5Mbps**：任何「双实例常驻 + 构建 + 数据库」组合都会触发 swap 风暴 → 503。动 compose 前先算内存总账。
- Redis `maxmemory 48mb` + `client-output-buffer-limit normal 32mb 16mb 60`：**任何把 DB 全量结果塞 Redis 的代码都是定时炸弹**（`RedisSet` 已有 1MiB 硬上限）。
- 构建期（`docker build` 大 Go 项目）会吃满内存，构建期 load 可到 40 —— 零停机脚本已把构建放在起新容器之前。
