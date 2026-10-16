<div align="center">

![new-api](./web/public/logo.png)

# New API

🍥 **新一代 LLM 网关与 AI 资产管理系统**

<p align="center">
  <a href="./README.zh_CN.md">简体中文</a> |
  <a href="./README.zh_TW.md">繁體中文</a> |
  <a href="./README.en.md">English</strong></a> |
  <a href="./README.fr.md">Français</a> |
  <a href="./README.ja.md">日本語</a>
</p>

<p align="center">
  <a href="https://github.com/lza6/new-api-Max/releases/latest">
    <img src="https://img.shields.io/github/v/release/lza6/new-api-Max?color=brightgreen&include_prereleases" alt="release">
  </a><!--
  --><a href="https://hub.docker.com/r/lza6/new-api-max">
    <img src="https://img.shields.io/badge/docker-dockerHub-blue" alt="docker">
  </a>
</p>

</div>

---

## 📝 项目说明

> [!IMPORTANT]
> - 本项目仅用于**合法授权**的 AI API 网关、组织级鉴权、多模型管理、用量分析、成本核算与私有化部署场景。
> - 使用者须合法取得上游 API 密钥、账号、模型服务与接口权限，并遵守上游服务条款及适用法律法规。
> - 面向公众提供生成式 AI 服务时，使用者应遵守所在司法辖区的监管要求，履行备案、许可、内容安全、实名认证、日志留存、税务与上游授权等义务。

---

## 本站部署增强（lza6/new-api-Max 生产分支）

以下为生产实例 `freeapi.tingfengai.art` 已落地并验证的能力，基于上游 New API 扩展：

### 订阅系统
- 套餐：天卡 ¥2 / 周卡 ¥25 / 月卡 ¥60（无限额度，CNY 1:1）；余额兑换与兑换码兑换均可用（充值开关关闭仍可余额兑换）
- 档位：并发 3/s、订阅 RPM 150（基础限速 3/s + 120RPM 管理员可控，分组/用户/单订阅三级覆盖）
- 自动升级分组（购买/兑换→`subscriber`）+ 分组订阅门禁 + 到期自动降级
- 订阅模型矩阵：套餐可用模型白名单，越权调用 403；续费顺延

### 统计与模型广场
- 模型卡片：今日/近 30 天调用与成功数；模型效果测试（含测试日期时间）整合进卡片
- 流量智能单位（B/KB/MB/GB/TB）、每日/模型带宽排行、站点权威统计（带宽/请求/Token）
- 订阅站点统计（v1.3.46）：`GET /v1/stats/subscriptions` 公开只读聚合（档位/订阅/生效中/7 天到期/30 天新增），定价页展示

### 安全与事件（v1.3.46）
- API Key 明文查看 step-up：`POST /api/token/:id/key` 与 `/batch/keys` 需安全验证
  （passkey/2FA/密码，一次性 proof 绑定 token 上下文）；keys 页/仪表盘 copy-curl/chat 链接均受保护
- 邮箱防枚举：发送验证码接口统一响应，不泄露注册状态
- 验证码存储：Redis 优先（TTL + 原子一次性消费）+ 内存兜底
- 通用 Webhook 子系统：`/api/admin/webhook/settings` 配置（默认关），事件订阅
  `epay.topup.success / epay.subscription.success / task.settled`，HMAC-SHA256 签名 + SSRF 防护

### 运维与本地化
- 前端 zh / zh-TW 汉化；系统信息页 CPU/内存/状态真实上报
- 部署/回滚/验收/E2E 复现：见 `计划书/ops/deployment-sop.md`、`计划书/workflow_status.md`、`计划书/audit/final-audit-v1.3.46.md`

### 生产安全加固与报表（007 批次，2026-10-04，**未部署**）
- **登录暴力破解防护**：登录限流默认开启（`login_rate_limit.enabled` 默认 true）；**失败登录写审计**（`audit_logs`，`success=false` + `status` + `reason`，去敏不含密码）——OWASP 反暴力破解基线
- **账号级登录失败锁定**（OWASP Authentication Cheat Sheet — Account Lockout：失败计数必须归属**账号**而非来源 IP）：`login_lockout`（默认开启，`threshold`=20 次 / `window_seconds`=1200s，退避 `base_lock_seconds`=300s → `max_lock_seconds`=900s 封顶）。仅拒绝该账号的**密码**登录（OAuth/passkey/2FA 不受影响），锁定期内尝试不累加、不延长锁定，任意一次成功登录立即清零——因此**不能被用来永久锁死他人账号**。锁定写审计（`account_locked`，解锁由成功登录的既有审计体现），面向调用方的响应与"密码错误/账号不存在"完全一致（反枚举）
- **私网判定一致**：`common.IsPrivateIP` 补齐 CGNAT `100.64.0.0/10`、link-local、保留段，与 SSRF 判定同集
- **用量/成本报表**：`GET /api/log/report?group_by=model|channel|day&start=&end=`（管理员）+ `/api/log/report/export` CSV 流式导出；dashboard 新增「Usage Report」section
- **CORS 合规**：`CORS_ALLOWED_ORIGINS` 白名单，消除 `*` + credentials 非法组合，默认仅同源
- **故障隔离**：渠道级熔断器（连续失败摘除 + 半开探测），`CHANNEL_CIRCUIT_BREAKER` 开关
- **优雅关闭**：后台 loop 与批量额度更新纳入 SIGTERM 停机序列，不丢账
- **索引治理**：删除无收益的 `idx_logs_traffic`（聚合无谓词，仅增写放大）
- **站内图床（本地磁盘产物存储）**：`TASK_ARTIFACT_STORE_MODE=local` 启用；存生成产物与参考素材；
  **防盗刷**（HMAC 签名 capability URL + 下载限流 10/min + `attachment`/`nosniff` + 防目录穿越）；
  **每 5 分钟自动清理**已完成任务超过保留期的素材（`TASK_ARTIFACT_RETENTION_SECONDS`，默认 300s）
- 规范：`.specify/specs/007-prod-safety-audit-report/`（spec/plan/tasks）
- **领域感知路由（4.8.1，默认关）**：`DOMAIN_ROUTE_ENABLED` + `DOMAIN_ROUTE_MAP=tag:group,...`；请求头 `X-Route-Tag` 命中白名单即覆盖分组（**不越权**：仅限用户可用分组）
- **轻量用户记忆（4.7.1）**：记录用户最近使用模型（`UserSetting.last_used_model`，JSON 列无需迁移），供前端回填默认模型
- **媒体能力注册表（4.6.1）**：`GET /api/system-info/media-providers` 返回媒体能力目录（文生视频/图生视频/图像生成/TTS/ASR × 渠道）
- **端点适配（421 契约）**：`/v1/videos/generations` 等价别名（提交+查询）、`/v1/messages/count_tokens` 重新启用（返回 `{"input_tokens":N}`）、`/v1/sub2api/billing` 查询密钥分组倍率与计费口径
- **可观测性**：`GET /api/system-info/channel-health`（RootAuth）返回每渠道健康分 + 熔断状态（closed/open/half_open + 连续失败数）
- **服务探活**：`GET /healthz`（存活，进程活着即 200，极轻量）/ `GET /readyz`（就绪，主库+日志库可达才 200，否则 503）——供 K8s/Caddy/Docker 健康检查；推荐用 `/healthz` 替换较重的 `/api/status` 作健康检查
- **报表安全**：CSV 导出防公式注入；参数非法返回 400；内部错误不泄漏到响应

### 🔒 安全响应头（CSP）与反向代理

仓库提供 `deploy/Caddyfile.example`——生产用的 **Caddy 反向代理 + 安全响应头**模板（HSTS / nosniff / X-Frame-Options / Referrer-Policy / Permissions-Policy + CSP）。

**应用步骤（生产 Caddy 在宿主机，需管理员操作）**：
1. 复制模板：`cp deploy/Caddyfile.example /etc/caddy/Caddyfile`
2. 替换占位：`YOUR_DOMAIN`、`admin@example.com`、上游地址（`127.0.0.1:3000`，蓝绿部署时可能是 `3001`）。
3. 校验并生效：`caddy validate --config /etc/caddy/Caddyfile && systemctl reload caddy`
4. 验证头：`curl -sI https://YOUR_DOMAIN/ | grep -iE "content-security-policy|strict-transport|x-frame"`

**注意**：模板的 CSP 已包含 SPA 所需的 `'unsafe-inline'`（React hydration）与 `'unsafe-eval'`（前端依赖库运行时用到 `new Function`/`eval`，实测有 1 处 eval 违规），**不含**外部 CDN。若接入 GA/Umami，需在 `script-src`/`connect-src` 追加其域名后启用。参考 `~/.claude/rules/web/security.md`。

---

## 🚀 快速开始

### 使用 Docker Compose（推荐）

`docker-compose.yml` 默认拉起三个服务：`new-api`（网关，映射 `3000:3000`）、
`redis`（缓存）、`postgres`（数据库，镜像 `postgres:15`）。

```bash
# 克隆项目
git clone https://github.com/lza6/new-api-Max.git
cd new-api-Max

# 编辑 docker-compose.yml 配置（务必修改默认密码！）
# 其中 SQL_DSN 默认指向 compose 内的 postgres 服务
vim docker-compose.yml

# 启动服务
docker compose up -d
```

### 使用 SQLite（默认，无需外部数据库）

不设置 `SQL_DSN` 时使用内置 SQLite，数据文件在容器内 `/data` 目录：

```bash
docker run -d --name new-api \
  -p 3000:3000 \
  -v /your/data:/data \
  -e TZ=Asia/Shanghai \
  ghcr.io/lza6/new-api-max:latest
```

### 使用 MySQL

把 `docker-compose.yml` 里的 `SQL_DSN` 换成 MySQL 形式
（compose 中已给出注释形式的示例）：

```yaml
- SQL_DSN=root:123456@tcp(mysql:3306)/new-api
```

> MySQL 需要 **≥ 5.7.8**（本项目的建表/迁移语句依赖该版本起的 JSON 与 `ON DUPLICATE KEY UPDATE` 行为）。

### 从源码构建

```bash
# 后端（Go 1.25+）
go build -o new-api .

# 前端（Bun + Rsbuild）
cd web && bun install --frozen-lockfile && bun run build
```

`make build-web` 会完成前端构建，`make start-api` 以 `go run main.go` 启动后端；
`make all` 等价于「构建前端 + 启动后端」。

---

## ⚙️ 配置

配置以**环境变量**为主，后台可改的项存在数据库 `options` 表。

### 必改项

| 变量 | 说明 |
|------|------|
| `PORT` | 监听端口，默认 `3000` |
| `SQL_DSN` | 数据库连接串。**不设置则用 SQLite**（文件在 `common.SQLitePath`，默认 `one-api.db`，也可用 `SQLITE_PATH` 指定） |
| `LOG_SQL_DSN` | 可选。**独立日志库**连接串；设置后主库不再建 `logs` 表（便于分离日志与业务库，也支持 ClickHouse） |
| `SESSION_SECRET` | 会话签名密钥 |
| `CRYPTO_SECRET` | 敏感字段加密密钥（用于加密渠道密钥等） |
| `TZ` | 时区，例如 `Asia/Shanghai` |
| `GIN_MODE` | `release` 关闭调试输出（生产建议设置） |

### 存储与缓存

| 变量 | 说明 |
|------|------|
| `REDIS_CONN_STRING` | Redis 连接串；设置后用于缓存与验证码存储（未设置时用内存兜底） |
| `MEMORY_CACHE_ENABLED` | 启用内存缓存 |
| `SYNC_FREQUENCY` | 渠道缓存同步周期（秒） |
| `CHANNEL_UPDATE_FREQUENCY` | 渠道余额自动更新周期 |
| `BATCH_UPDATE_ENABLED` | 批量额度更新（`true` 启用，降低 DB 写压力） |

### HTTP 与安全

| 变量 | 说明 |
|------|------|
| `CORS_ALLOWED_ORIGINS` | 允许的跨域来源白名单（**默认仅同源**；切勿与 credentials 一起用 `*`） |
| `SHUTDOWN_TIMEOUT_SECONDS` | 优雅关闭超时（秒），默认 `120` |
| `CHANNEL_CIRCUIT_BREAKER` | 渠道级熔断开关 |
| `ENABLE_PPROF` | 启用 pprof（`127.0.0.1:8005`），仅排障时开 |
| `METRICS_ENABLED` | 启用 Prometheus 指标端点 `/metrics` |

### 产物存储与日志

| 变量 | 说明 |
|------|------|
| `TASK_ARTIFACT_STORE_MODE` | 生成产物存储模式。默认 `upstream`（**不在本站存图**）；`local` 才落本地磁盘 |
| `TASK_ARTIFACT_RETENTION_SECONDS` | `local` 模式下产物的保留时长，默认 `300` 秒 |
| `LOG_MAX_SIZE_MB` | 单个日志文件大小上限（MB），默认 `256`；超过即分片，配合保留策略（最多 14 个文件 / 总计 1GB）自动清理 |
| `LOG_DIR` | 日志目录（也可用命令行参数 `--log-dir`） |
| `UMAMI_WEBSITE_ID` / `UMAMI_SCRIPT_URL` | 接入 Umami 统计（可选） |
| `GOOGLE_ANALYTICS_ID` | 接入 Google Analytics（可选） |

### 训练数据采集（可选功能，默认关闭）

用户明确授权后，把成功的请求异步采集到云盘，用于训练数据积累。
**不配置 `TRAINING_DATA_SINK` 时该功能完全关闭**（不会往云盘传任何用户对话）。

| 变量 | 说明 |
|------|------|
| `TRAINING_DATA_SINK` | 存储后端。`onedrive` 启用；留空或 `disabled` 关闭（默认） |
| `TRAINING_ONEDRIVE_TENANT` | Azure AD 租户（`xxx.onmicrosoft.com`） |
| `TRAINING_ONEDRIVE_CLIENT_ID` | 应用 client_id（`client_credentials` 流程，**无需浏览器登录**） |
| `TRAINING_ONEDRIVE_CLIENT_SECRET` | 应用 client_secret（**只从环境变量读，不落库**） |
| `TRAINING_ONEDRIVE_DRIVE_ID` | 目标 drive id |
| `TRAINING_ONEDRIVE_ROOT_PATH` | 可选。云盘内的根目录，便于与其它内容隔离 |
| `TRAINING_CAPTURE_BUDGET_PCT` | 采集模块可用内存占机器内存的百分比，默认 `80` |
| `TRAINING_CAPTURE_MIN_BUDGET_MB` | 预算下限（MB），默认 `128`。检测不到 cgroup 与宿主机内存时用它兜底 |
| `TRAINING_CAPTURE_MAX_TEXT_MB` | 单条采集文本上限（MB），默认随预算自适应（最小 64KB，最大 8MB） |
| `TRAINING_CAPTURE_MAX_PART_MB` | 单个分片**压缩前**的原始上限（MB），默认 `16` |
| `TRAINING_CAPTURE_QUEUE_CAP` | 采集队列容量（条），默认按预算自适应（上限 4096） |

**内存预算是 cgroup 感知的**：容器里读 `mem.VirtualMemory()` 拿到的是**宿主机**内存，
按它算 80% 会让受限容器被 OOM killer 杀掉（表现为无征兆重启）。
因此检测顺序是 cgroup v2 → cgroup v1 → 宿主机 → 安全下限，并取 min。
启动日志会打印实际预算与来源（`cgroupv2` / `cgroupv1` / `host` / `floor`）。

**云盘数据布局**：`<ROOT_PATH>/<YYYY-MM-DD>/<用户名>-<用户ID>/<模型名>/part-0001.jsonl.gz`
（分片为 gzip 压缩的 JSONL，读取方需先解压）。

**记录格式**（每行一条 JSON）：

```json
{"role":"request","content":"<该轮新增的请求文本>"}
{"role":"assistant","content":"<模型对该轮的回复>"}
```

助手回复通过**包装 ResponseWriter 做有界 tee** 从响应体提取
（覆盖 OpenAI / Claude / Gemini 的流式与非流式形态），
因此**无需为每个渠道适配**。提取失败时该条记录只有请求侧 ——
不会因为提取失败而少采或写坏。

**已知边界（如实声明）**：
- 请求文本是上游 `TokenCountMeta.CombineText` —— 把整段对话的 role 与文本
  拼接后的**一个字符串**，不可逆，因此**逐轮角色只能启发式还原**；
- 文本超限时保留**头 + 尾**并插入省略标记
  （`…[training-capture: N bytes elided]…`），中间内容会丢失；
- 响应体捕获上限 256KB，超长回复的尾部会被截掉。

**管理端**：`/training-data` 页面提供总开关、今日采集统计、云盘配额与文件浏览，
并支持对某段对话做深度分析（**会真实调用模型、消耗管理员额度**）。

> 完整清单以代码为准：后端读环境变量的位置集中在 `common/`、`main.go` 与各 `setting/` 包中。

---

## 🧑‍💻 开发

### 前端（`web/`，Bun + Rsbuild）

```bash
cd web
bun install

bun run dev           # 开发服务器（rsbuild dev）
bun run build         # 生产构建
bun run build:check   # 类型检查 + 构建
bun run typecheck     # 仅类型检查（tsgo -b）
bun run lint          # oxlint
bun run test          # vitest run
bun run i18n:check    # 校验各语言缺失的文案键
bun run i18n:sync     # 同步 i18n 键
bun run format        # 统一格式化（保留受保护的文件头）
bun run copyright:check  # 校验版权头
```

### 后端（Go）

```bash
go build ./...                    # 构建全部包
go test ./...                     # 运行测试
make test                         # 根模块 + relaykit 子模块一起测
make dev-api                      # 用 docker-compose.dev.yml 起开发依赖
make db-check                     # 三库一致性契约测试（SQLite + MySQL + PostgreSQL）
```

> `make db-check` 需要真实可连的 MySQL / PostgreSQL 实例，通过 `TEST_MYSQL_DSN` /
> `TEST_POSTGRES_DSN` 指定；未配置的方言会被跳过。

### 代码规范

- 后端：`gofmt`；改动数据库相关代码必须通过三库一致性契约测试（`go test ./model/ -run TestDBConformance`）。
- 前端：提交前跑 `bun run typecheck` 与 `bun run lint`，两者都不得有 error。
- 前端详细规范见 [`web/AGENTS.md`](./web/AGENTS.md)。

---

## 📚 文档

| 资源 | 链接 |
|------|------|
| 官方文档 | <https://docs.newapi.pro/zh/docs> |
| 常见问题 | <https://docs.newapi.pro/zh/docs/support/faq> |
| 反馈问题 | <https://docs.newapi.pro/zh/docs/support/feedback-issues> |
| Issue 反馈 | <https://github.com/lza6/new-api-Max/issues> |

---

## ✨ 主要特性

### 🎨 核心功能

- 🎨 全新的用户界面
- 🌍 多语言支持
- 💾 支持多种数据库（SQLite / MySQL / PostgreSQL）
- 🔄 支持多种渠道，可配置渠道优先级
- ⚖️ 支持加权随机负载均衡
- 🔁 支持失败自动重试
- 💰 支持按次数计费与按量计费
- 🔐 支持令牌分组与模型限速
- 🧠 支持推理模型（reasoning）
- 💬 支持流式输出
- 🔑 支持多种鉴权方式（API Key / OAuth / Passkey / 2FA）

### 💰 授权用量与成本管理

- 按用户 / 令牌 / 渠道 / 模型多维统计
- 额度充值、兑换码、订阅套餐
- 用量与成本报表（管理员，支持 CSV 导出）

### 🔐 授权与安全

- 组织级多用户与权限管理
- 令牌细粒度授权（模型 / 分组 / 额度 / 过期）
- 登录限流、失败审计与账号级锁定
- 敏感操作 step-up 安全验证
- SSRF 防护、CORS 白名单、安全响应头模板

### 🚀 高级功能

- 渠道熔断与自动禁用/启用
- 通用 Webhook 事件订阅（HMAC-SHA256 签名）
- 服务探活 `/healthz`（存活）与 `/readyz`（就绪）
- 可观测性指标与渠道健康分
- 请求级实时面板与延迟拆解

---

## 🤖 模型支持

覆盖主流上游的 OpenAI 兼容、Claude、Gemini、Midjourney、Suno、Rerank、
Embedding、TTS/ASR 与视频生成等接口，具体可接入的模型以渠道配置为准。

### 📡 支持的接口（节选）

| 路径 | 说明 |
|------|------|
| `/v1/chat/completions` | OpenAI 兼容对话 |
| `/v1/messages` | Claude 兼容对话 |
| `/v1/embeddings` | 向量化 |
| `/v1/rerank` | 重排序 |
| `/v1/images/generations` | 图像生成 |
| `/v1/videos/generations` | 视频生成（含等价别名） |
| `/v1/messages/count_tokens` | Token 计数 |
| `/api/…` | 后台管理接口 |

---

## 🚢 部署

### 📋 部署要求

- **Go** 1.25+（从源码构建时）
- **Bun**（构建前端时）
- **数据库**：SQLite（零依赖）／ MySQL **≥ 5.7.8** ／ PostgreSQL **≥ 9.6**
- **Redis**（可选，推荐用于缓存与验证码）

### 🔧 部署方式

1. **Docker Compose**（推荐）：`docker compose up -d`
2. **单容器**：`docker run` 挂载 `/data` 持久化 SQLite
3. **从源码**：`go build` 后端 + `bun run build` 前端，再用 systemd/supervisor 托管
4. **反向代理**：参考 `deploy/Caddyfile.example`（含安全响应头与 CSP）

### ⚠️ 多机部署注意

- 多实例必须共用同一数据库，且建议设置 `REDIS_CONN_STRING` 以共享缓存。
- 主从节点通过 `NODE_TYPE` 区分（从节点不执行迁移等主控任务）。

### 🔄 渠道重试与缓存

- 上游失败按渠道优先级与重试次数自动切换渠道，可配置 429 退避。
- 渠道信息有本地缓存，`SYNC_FREQUENCY` 控制同步周期；多机部署时同步周期不宜过大。

---

## 📜 License

This project is licensed under the [GNU Affero General Public License v3.0 (AGPLv3)](./LICENSE).

Additional terms under AGPLv3 Section 7 apply. Modified versions must preserve
the author attribution notice `Frontend design and development by New API
contributors.` in the appropriate legal notices and in any prominent about,
legal, footer, or attribution location presented by the user interface.

Modified versions that present a user interface must also preserve a visible
link to the original project: <https://github.com/lza6/new-api-Max>.

This is an open-source project developed based on [One API](https://github.com/songquanpeng/one-api) (MIT License).

If your organization's policies do not permit the use of AGPLv3-licensed software, or if you wish to avoid the open-source obligations of AGPLv3, please contact us at: [support@quantumnous.com](mailto:support@quantumnous.com)

---

<div align="center">

### 💖 感谢使用 New API

如果这个项目对你有帮助，欢迎给我们一个 ⭐️ Star！

**[官方文档](https://docs.newapi.pro/zh/docs)** • **[Issue 反馈](https://github.com/lza6/new-api-Max/issues)** • **[最新版本](https://github.com/lza6/new-api-Max/releases)**

<sub>Built with ❤️ by lza6</sub>

</div>

> ⚠️ **多实例部署的重要约束**：训练数据采集模块**目前不做跨实例协调**。
> 若你以多实例（或蓝绿/多副本）方式部署并开启采集，会出现两类问题：
> ① 两个实例可能给同一会话算出同一个分片序号并互相覆盖（静默丢数据）；
> ② 总开关与用户授权缓存都是**进程内**状态，A 实例关闭后 B 实例最长 30 秒仍在采集。
> **请只在单个实例上开启采集**，或等跨实例协调（云盘条件写 / DB 租约）落地后再多实例启用。
> 启动日志会打印这条告警。
