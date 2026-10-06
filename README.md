<div align="center">

![new-api](/web/public/logo.png)

# New API

🍥 **Next-Generation LLM Gateway and AI Asset Management System**

<p align="center">
  <a href="./README.zh_CN.md">简体中文</a> |
  <a href="./README.zh_TW.md">繁體中文</a> |
  <strong>English</strong> |
  <a href="./README.fr.md">Français</a> |
  <a href="./README.ja.md">日本語</a>
</p>

<p align="center">
  <a href="https://raw.githubusercontent.com/lza6/new-api-Max/main/LICENSE">
    <img src="https://img.shields.io/github/license/lza6/new-api-Max?color=brightgreen" alt="license">
  </a><!--
  --><a href="https://github.com/lza6/new-api-Max/releases/latest">
    <img src="https://img.shields.io/github/v/release/lza6/new-api-Max?color=brightgreen&include_prereleases" alt="release">
  </a><!--
  --><a href="https://hub.docker.com/r/lza6/new-api-max">
    <img src="https://img.shields.io/badge/docker-dockerHub-blue" alt="docker">
  </a>
</p>

<p align="center">
  <br><!--
  -->
</p>

<p align="center">
  <a href="#-quick-start">Quick Start</a> •
  <a href="#-key-features">Key Features</a> •
  <a href="#-deployment">Deployment</a> •
  <a href="#-documentation">Documentation</a> •
  <a href="#-help-support">Help</a>
</p>

</div>

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
- 常用改动请先读 `.claude/skills/new-api-add-feature/SKILL.md`（项目可复用开发工作流技能）

### 生产安全加固与报表（007 批次，2026-10-04，**未部署**）
- **登录暴力破解防护**：登录限流默认开启（`login_rate_limit.enabled` 默认 true）；**失败登录写审计**（`audit_logs`，`success=false` + `status` + `reason`，去敏不含密码）——OWASP 反暴力破解基线
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
- **排障**：`ERR_CONNECTION_RESET` 见 `计划书/ops/connection-reset-sop.md`（大陆直连香港源站 IP:443 被 RST，非服务器故障）
- **领域感知路由（4.8.1，默认关）**：`DOMAIN_ROUTE_ENABLED` + `DOMAIN_ROUTE_MAP=tag:group,...`；请求头 `X-Route-Tag` 命中白名单即覆盖分组（**不越权**：仅限用户可用分组）
- **轻量用户记忆（4.7.1）**：记录用户最近使用模型（`UserSetting.last_used_model`，JSON 列无需迁移），供前端回填默认模型
- **媒体能力注册表（4.6.1）**：`GET /api/system-info/media-providers` 返回媒体能力目录（文生视频/图生视频/图像生成/TTS/ASR × 渠道）
- **端点适配（421 契约）**：`/v1/videos/generations` 等价别名（提交+查询）、`/v1/messages/count_tokens` 重新启用（返回 `{"input_tokens":N}`）、`/v1/sub2api/billing` 查询密钥分组倍率与计费口径
- **可观测性**：`GET /api/system-info/channel-health`（RootAuth）返回每渠道健康分 + 熔断状态（closed/open/half_open + 连续失败数）
- **服务探活**：`GET /healthz`（存活，进程活着即 200，极轻量）/ `GET /readyz`（就绪，主库+日志库可达才 200，否则 503）——供 K8s/Caddy/Docker 健康检查；推荐用 `/healthz` 替换较重的 `/api/status` 作健康检查
- **报表安全**：CSV 导出防公式注入；参数非法返回 400；内部错误不泄漏到响应

### 🔒 Security Headers (CSP) & Reverse Proxy

仓库提供 `deploy/Caddyfile.example`——生产用的 **Caddy 反向代理 + 安全响应头**模板（HSTS / nosniff / X-Frame-Options / Referrer-Policy / Permissions-Policy + CSP）。

**应用步骤（生产 Caddy 在宿主机，需管理员操作）**：
1. 复制模板：`cp deploy/Caddyfile.example /etc/caddy/Caddyfile`
2. 替换占位：`YOUR_DOMAIN`、`admin@example.com`、上游地址（`127.0.0.1:3000`，蓝绿部署时可能是 `3001`）。
3. 校验并生效：`caddy validate --config /etc/caddy/Caddyfile && systemctl reload caddy`
4. 验证头：`curl -sI https://YOUR_DOMAIN/ | grep -iE "content-security-policy|strict-transport|x-frame"`

**注意**：模板的 CSP 已包含 SPA 所需的 `'unsafe-inline'`（React hydration）与 `'unsafe-eval'`（前端依赖库运行时用到 `new Function`/`eval`，实测有 1 处 eval 违规），**不含**外部 CDN。若接入 GA/Umami，需在 `script-src`/`connect-src` 追加其域名后启用。参考 `~/.claude/rules/web/security.md`。

## 📝 Project Description

> [!IMPORTANT]
> - This project is intended solely for lawful and authorized AI API gateway, organization-level authentication, multi-model management, usage analytics, cost accounting, and private deployment scenarios.
> - Users must lawfully obtain upstream API keys, accounts, model services, and interface permissions, and must comply with upstream terms of service and applicable laws and regulations.
> - Users should ensure their use complies with upstream terms of service and applicable laws and regulations.
> - When providing generative AI services to the public, users should comply with applicable regulatory requirements and fulfill all filing, licensing, content safety, real-name verification, log retention, tax, and upstream authorization obligations required by their jurisdiction.

---

## 🤝 Trusted Partners

<p align="center">
  <em>No particular order</em>
</p>

<p align="center">
  <a href="https://www.cherry-ai.com/" target="_blank">
    <img src="./docs/images/cherry-studio.png" alt="Cherry Studio" height="80" />
  </a><!--
  --><a href="https://github.com/iOfficeAI/AionUi/" target="_blank">
    <img src="./docs/images/aionui.png" alt="Aion UI" height="80" />
  </a><!--
  --><a href="https://bda.pku.edu.cn/" target="_blank">
    <img src="./docs/images/pku.png" alt="Peking University" height="80" />
  </a><!--
  --><a href="https://www.compshare.cn/?ytag=GPU_yy_gh_newapi" target="_blank">
    <img src="./docs/images/ucloud.png" alt="UCloud" height="80" />
  </a><!--
  --><a href="https://www.aliyun.com/" target="_blank">
    <img src="./docs/images/aliyun.png" alt="Alibaba Cloud" height="80" />
  </a><!--
  --><a href="https://io.net/" target="_blank">
    <img src="./docs/images/io-net.png" alt="IO.NET" height="80" />
  </a>
</p>

---

## 🙏 Special Thanks

<p align="center">
  <a href="https://www.jetbrains.com/?from=new-api" target="_blank">
    <img src="https://resources.jetbrains.com/storage/products/company/brand/logos/jb_beam.png" alt="JetBrains Logo" width="120" />
  </a>
</p>

<p align="center">
  <strong>Thanks to <a href="https://www.jetbrains.com/?from=new-api">JetBrains</a> for providing free open-source development license for this project</strong>
</p>

---

## 🚀 Quick Start

### Using Docker Compose (Recommended)

```bash
# Clone the project
git clone https://github.com/lza6/new-api-Max.git
cd new-api

# Edit docker-compose.yml configuration
nano docker-compose.yml

# Start the service
docker-compose up -d
```

<details>
<summary><strong>Using Docker Commands</strong></summary>

```bash
# Pull the latest image
docker pull calciumion/new-api:latest

# Using SQLite (default)
docker run --name new-api -d --restart always \
  -p 3000:3000 \
  -e TZ=Asia/Shanghai \
  -v ./data:/data \
  calciumion/new-api:latest

# Using MySQL
docker run --name new-api -d --restart always \
  -p 3000:3000 \
  -e SQL_DSN="root:123456@tcp(localhost:3306)/oneapi" \
  -e TZ=Asia/Shanghai \
  -v ./data:/data \
  calciumion/new-api:latest
```

> **💡 Tip:** `-v ./data:/data` will save data in the `data` folder of the current directory, you can also change it to an absolute path like `-v /your/custom/path:/data`

</details>

---

🎉 After deployment is complete, visit `http://localhost:3000` to start using!

> [!WARNING]
> When operating this project as a public generative AI service or API resale service, users should first complete all required filing, licensing, content safety, real-name verification, log retention, tax, payment, and upstream authorization obligations.

📖 For more deployment methods, please refer to [Deployment Guide](https://docs.newapi.pro/en/docs/installation)

---

## 📚 Documentation

<div align="center">

### 📖 [Official Documentation](https://docs.newapi.pro/en/docs) | [![Ask DeepWiki](https://deepwiki.com/badge.svg)](https://deepwiki.com/lza6/new-api-Max)

</div>

**Quick Navigation:**

| Category | Link |
|------|------|
| 🚀 Deployment Guide | [Installation Documentation](https://docs.newapi.pro/en/docs/installation) |
| ⚙️ Environment Configuration | [Environment Variables](https://docs.newapi.pro/en/docs/installation/config-maintenance/environment-variables) |
| 📡 API Documentation | [API Documentation](https://docs.newapi.pro/en/docs/api) |
| ❓ FAQ | [FAQ](https://docs.newapi.pro/en/docs/support/faq) |
| 💬 Community Interaction | [Communication Channels](https://docs.newapi.pro/en/docs/support/community-interaction) |

---

## ✨ Key Features

> For detailed features, please refer to [Features Introduction](https://docs.newapi.pro/en/docs/guide/wiki/basic-concepts/features-introduction)

### 🎨 Core Functions

| Feature | Description |
|------|------|
| 🎨 New UI | Modern user interface design |
| 🌍 Multi-language | Supports Simplified Chinese, Traditional Chinese, English, French, Japanese |
| 🔄 Data Compatibility | Fully compatible with the original One API database |
| 📈 Data Dashboard | Visual console and statistical analysis |
| 🔒 Permission Management | Token grouping, model restrictions, user management |

### 💰 Authorized Usage Accounting and Billing

- ✅ Internal top-up and quota allocation for lawful authorized scenarios (EPay, Stripe)
- ✅ Organization-level per-request, usage-based, and cache-hit cost accounting
- ✅ Cache billing statistics for OpenAI, Azure, DeepSeek, Claude, Qwen, and supported models
- ✅ Flexible billing policies for internal management or authorized enterprise customers

### 🔐 Authorization and Security

- 😈 Discord authorization login
- 🤖 LinuxDO authorization login
- 📱 Telegram authorization login
- 🔑 OIDC unified authentication
- 🔍 Key quota query usage (with [new-api-key-tool](https://github.com/lza6/new-api-Max-key-tool))

### 🚀 Advanced Features

**API Format Support:**
- ⚡ [OpenAI Responses](https://docs.newapi.pro/en/docs/api/ai-model/chat/openai/create-response)
- ⚡ [OpenAI Realtime API](https://docs.newapi.pro/en/docs/api/ai-model/realtime/create-realtime-session) (including Azure)
- ⚡ [Claude Messages](https://docs.newapi.pro/en/docs/api/ai-model/chat/create-message)
- ⚡ [Google Gemini](https://doc.newapi.pro/en/api/google-gemini-chat)
- 🔄 [Rerank Models](https://docs.newapi.pro/en/docs/api/ai-model/rerank/create-rerank) (Cohere, Jina)

**Intelligent Routing:**
- ⚖️ Channel weighted random
- 🔄 Automatic retry on failure
- 🚦 User-level model rate limiting

**Format Conversion:**
- 🔄 **OpenAI Compatible ⇄ Claude Messages**
- 🔄 **OpenAI Compatible → Google Gemini**
- 🔄 **Google Gemini → OpenAI Compatible** - Text only, function calling not supported yet
- 🚧 **OpenAI Compatible ⇄ OpenAI Responses** - In development
- 🔄 **Thinking-to-content functionality**

**Reasoning Effort Support:**

<details>
<summary>View detailed configuration</summary>

**OpenAI series models:**
- `o3-mini-high` - High reasoning effort
- `o3-mini-medium` - Medium reasoning effort
- `o3-mini-low` - Low reasoning effort
- `gpt-5-high` - High reasoning effort
- `gpt-5-medium` - Medium reasoning effort
- `gpt-5-low` - Low reasoning effort

**Claude thinking models:**
- `claude-3-7-sonnet-20250219-thinking` - Enable thinking mode

**Google Gemini series models:**
- `gemini-2.5-flash-thinking` - Enable thinking mode
- `gemini-2.5-flash-nothinking` - Disable thinking mode
- `gemini-2.5-pro-thinking` - Enable thinking mode
- `gemini-2.5-pro-thinking-128` - Enable thinking mode with thinking budget of 128 tokens
- You can also append `-low`, `-medium`, or `-high` to any Gemini model name to request the corresponding reasoning effort (no extra thinking-budget suffix needed).

</details>

---

## 🤖 Model Support

> For details, please refer to [API Documentation - Gateway Interface](https://docs.newapi.pro/en/docs/api)

| Model Type | Description | Documentation |
|---------|------|------|
| 🤖 OpenAI-Compatible | OpenAI compatible models | [Documentation](https://docs.newapi.pro/en/docs/api/ai-model/chat/openai/createchatcompletion) |
| 🤖 OpenAI Responses | OpenAI Responses format | [Documentation](https://docs.newapi.pro/en/docs/api/ai-model/chat/openai/createresponse) |
| 🎨 Midjourney-Proxy | [Midjourney-Proxy(Plus)](https://github.com/novicezk/midjourney-proxy) | [Documentation](https://doc.newapi.pro/api/midjourney-proxy-image) |
| 🎵 Suno-API | [Suno API](https://github.com/Suno-API/Suno-API) | [Documentation](https://doc.newapi.pro/api/suno-music) |
| 🔄 Rerank | Cohere, Jina | [Documentation](https://docs.newapi.pro/en/docs/api/ai-model/rerank/creatererank) |
| 💬 Claude | Messages format | [Documentation](https://docs.newapi.pro/en/docs/api/ai-model/chat/createmessage) |
| 🌐 Gemini | Google Gemini format | [Documentation](https://docs.newapi.pro/en/docs/api/ai-model/chat/gemini/geminirelayv1beta) |
| 🔧 Dify | ChatFlow mode | - |
| 🎯 Custom upstream | Supports configuring legally authorized upstream endpoints | - |

### 📡 Supported Interfaces

<details>
<summary>View complete interface list</summary>

- [Chat Interface (Chat Completions)](https://docs.newapi.pro/en/docs/api/ai-model/chat/openai/createchatcompletion)
- [Response Interface (Responses)](https://docs.newapi.pro/en/docs/api/ai-model/chat/openai/createresponse)
- [Image Interface (Image)](https://docs.newapi.pro/en/docs/api/ai-model/images/openai/post-v1-images-generations)
- [Audio Interface (Audio)](https://docs.newapi.pro/en/docs/api/ai-model/audio/openai/create-transcription)
- [Video Interface (Video)](https://docs.newapi.pro/en/docs/api/ai-model/videos/sora/createvideo)
- [Embedding Interface (Embeddings)](https://docs.newapi.pro/en/docs/api/ai-model/embeddings/createembedding)
- [Rerank Interface (Rerank)](https://docs.newapi.pro/en/docs/api/ai-model/rerank/creatererank)
- [Realtime Conversation (Realtime)](https://docs.newapi.pro/en/docs/api/ai-model/realtime/createrealtimesession)
- [Claude Chat](https://docs.newapi.pro/en/docs/api/ai-model/chat/createmessage)
- [Google Gemini Chat](https://docs.newapi.pro/en/docs/api/ai-model/chat/gemini/geminirelayv1beta)

</details>

---

## 🚢 Deployment

> [!TIP]
> **Latest Docker image:** `calciumion/new-api:latest`

### 📋 Deployment Requirements

| Component | Requirement |
|------|------|
| **Local database** | SQLite (Docker must mount `/data` directory)|
| **Remote database** | MySQL ≥ 5.7.8 or PostgreSQL ≥ 9.6 |
| **Container engine** | Docker / Docker Compose |
| **System architecture** | 64-bit only (amd64 / arm64); 32-bit systems are not supported |

### ⚙️ Environment Variable Configuration

<details>
<summary>Common environment variable configuration</summary>

| Variable Name | Description | Default Value |
|--------|------|--------|
| `SESSION_SECRET` | Authentication signing secret; must be identical on every node | - |
| `SESSION_COOKIE_SECURE` | `false`/unset disables the refresh/logout OriginGuard for local HTTP dev proxies; `true` enables the Secure cookie and strict Origin checks | `false` |
| `SESSION_COOKIE_TRUSTED_URL` | Required with Secure mode: comma-separated exact HTTPS Origins allowed to call refresh/logout; not a relay CORS allowlist | - |
| `TRUSTED_PROXIES` | Unset/blank trusts loopback, RFC 1918 and IPv6 ULA with a startup warning; `none` trusts no proxies; an explicit proxy IP/CIDR list replaces the defaults | `127.0.0.0/8, ::1, 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16, fc00::/7` |
| `USER_SESSION_ACTIVE_LIMIT` | Maximum active login Sessions per user | `50` |
| `USER_SESSION_ISSUANCE_LIMIT` | Maximum Sessions created per user within the issuance window, including revoked Sessions | `100` |
| `USER_SESSION_ISSUANCE_WINDOW_SECONDS` | Per-user Session issuance window; clamped to the revoked retention period when configured higher | `86400` |
| `USER_SESSION_REVOKED_RETENTION_DAYS` | Days to retain revoked Session rows for audit and issuance accounting | `7` |
| `USER_SESSION_HOURLY_ALERT_THRESHOLD` | Global Sessions created per hour that triggers an alert only; it never blocks login | `5000` |
| `CRYPTO_SECRET` | HMAC secret for cache keys; nodes sharing Redis must use the same effective value | Defaults to `SESSION_SECRET` |
| `SQL_DSN` | Database connection string | - |
| `REDIS_CONN_STRING` | Redis connection string | - |
| `SUBSCRIPTION_ACTIVE_CACHE_SECONDS` | 订阅档位正缓存 TTL（秒）：限流中间件对「有 active 订阅」用户每 TTL 才查询一次 DB，订阅变更即时失效；`0` = 关闭（逐位回退基线，每请求直查 DB）。多实例为进程内缓存，与 RPM 并发计数同口径 | `10` |
| `SUBSCRIPTION_STATS_CACHE_SECONDS` | 公开订阅统计端点（`/v1/stats/subscriptions`）短缓存 TTL（秒）；`0` = 关闭实时聚合。公开只读端点已用宽松 `PublicReadRateLimit`（60/min/IP） | `30` |
| `METRICS_ENABLED` | 是否开放 Prometheus 文本格式 `/metrics` 端点（默认关；含请求量/延迟直方图、限流命中、自动封禁、事件总线投递计数，以及**渠道健康分/熔断状态/冷却/队列深度** gauge）。**鉴权**：仅可信来源（环回/私网 `IsTrustedSourceIP`）或 root 会话可读，公网匿名 401 | `false` |
| `READYZ_CHECK_REDIS` | `/readyz` 就绪探针是否将 Redis 可达性计入就绪码：`true`（默认）= Redis 宕机返回 503（fail-hard）；`false` = fail-soft（仅报告 Redis 状态，不影响就绪码）。未配置 `REDIS_CONN_STRING` 时 Redis 视为「未依赖」恒就绪 | `true` |
| `SLOW_REQUEST_THRESHOLD_MS` | 慢请求采样阈值（毫秒）：超过则输出 `[SLOW] request-id=...` 日志，供按 request-id 聚合慢链路；`0` = 全部采样 | `3000` |
| `QUOTA_WARN_THRESHOLDS` | 额度预警多档位（逗号分隔，从大到小）：用户**未显式设置** `QuotaWarningThreshold`（注册注入的默认 80% 视为未显式设置）时按此多档分级提醒；解析失败或全空时回退默认档位 | `1000,500,100` |
| `QUOTA_REMIND_THRESHOLD`（旧） | **已弃用**：旧单阈值由多档 `QUOTA_WARN_THRESHOLDS` 取代，预警路径不再读取此值；保留仅为系统设置 UI 兼容显示，新部署无需配置 | `1000` |
| `RELAY_IDLE_CONN_TIMEOUT` | Idle keep-alive timeout for relay HTTP clients, seconds. Defaults to Go standard library behavior; set `0` to disable | `90` |
| `RELAY_RESPONSE_HEADER_TIMEOUT` | How long the relay waits for upstream **response headers**, seconds; set `0` to disable. Only bounds the header wait -- streaming after the headers arrive is unaffected. Note that non-streaming upstreams usually send headers only once generation finishes, so leave headroom | `1800` |
| `STREAMING_TIMEOUT` | Streaming timeout (seconds) | `300` |
| `RELAY_REQUEST_COMPRESSION_ENABLED` | Global kill-switch for outbound request-body compression. When `true`, channels with `setting.request_compression=true` gzip the upstream request body (≥ threshold). Useful when upstream upload bandwidth is the first-token bottleneck | `true` |
| `RELAY_REQUEST_COMPRESSION_THRESHOLD_KB` | Request-body size (KB) below which compression is skipped (not worth the CPU). 256KB covers the common 300KB–1MB prompt range at ~44ms CPU cost | `256` |
| `STREAM_SCANNER_MAX_BUFFER_MB` | Max per-line buffer (MB) for the stream scanner; increase when upstream sends huge image/base64 payloads | `64` |
| `MAX_REQUEST_BODY_MB` | Max request body size (MB, counted **after decompression**; prevents huge requests/zip bombs from exhausting memory). Exceeding it returns `413` | `32` |
| `AZURE_DEFAULT_API_VERSION` | Azure API version | `2025-04-01-preview` |
| `ERROR_LOG_ENABLED` | Error log switch | `false` |
| `PYROSCOPE_URL` | Pyroscope server address | - |
| `PYROSCOPE_APP_NAME` | Pyroscope application name | `new-api` |
| `PYROSCOPE_BASIC_AUTH_USER` | Pyroscope basic auth user | - |
| `PYROSCOPE_BASIC_AUTH_PASSWORD` | Pyroscope basic auth password | - |
| `PYROSCOPE_MUTEX_RATE` | Pyroscope mutex sampling rate | `5` |
| `PYROSCOPE_BLOCK_RATE` | Pyroscope block sampling rate | `5` |
| `HOSTNAME` | Hostname tag for Pyroscope | `new-api` |
| `CORS_ALLOWED_ORIGINS` | Comma-separated exact Origins allowed to make cross-origin browser requests (credentials enabled). Unset/blank = same-origin only (no CORS headers). Wildcard is never combined with credentials (spec-invalid) | - |
| `CHANNEL_CIRCUIT_BREAKER` | Enable per-channel circuit breaker (`on`/`true`): a channel is tripped after `CHANNEL_CIRCUIT_FAILURE_THRESHOLD` consecutive upstream failures and re-probed after the open window. Off = zero behavior change | `false` |
| `CHANNEL_CIRCUIT_FAILURE_THRESHOLD` | Consecutive upstream failures (auth/rate-limit/5xx/timeout) before a channel is tripped | `5` |
| `CHANNEL_CIRCUIT_OPEN_SECONDS` | Circuit-open window in seconds before a half-open probe is allowed | `120` |
| `TASK_ARTIFACT_STORE_MODE` | 站内图床模式：`upstream`（默认，产物走上游代理，零变化）/ `local`（本地磁盘存储生成产物与参考素材） | `upstream` |
| `TASK_ARTIFACT_STORE_DIR` | 本地图床根目录（`local` 模式）；默认 `<工作目录>/data/task-artifacts` | - |
| `TASK_ARTIFACT_MAX_FILE_MB` | 本地图单单文件上限（MB），超限拒绝落盘 | `64` |
| `TASK_ARTIFACT_RETENTION_SECONDS` | 产物/参考素材保留期（秒）：后台每 5 分钟清理「任务已完成且超过保留期」的目录 | `300` |
| `DOMAIN_ROUTE_ENABLED` | 领域感知路由开关（默认关）。开启后请求头 `X-Route-Tag` 命中 `DOMAIN_ROUTE_MAP` 时可覆盖分组（仅限用户可用分组） | `false` |
| `DOMAIN_ROUTE_MAP` | 领域路由映射（逗号分隔 `tag:group`），如 `medical:medical-group,legal:legal-group` | - |
| `CHANNEL_KEY_ENCRYPTION` | 渠道密钥**加密存储**开关（AES-256-GCM，主密钥由 `CRYPTO_SECRET` 派生）。默认关；开启后**下次保存渠道即加密**、读取自动解密，旧明文 fail-open 兼容。**多节点须一致** | `false` |
| `PRICING_SYNC_TASK_ENABLED` | 上游**价目**定时同步开关（倍率/价目，幂等 merge）。上游源走 `PRICING_SYNC_UPSTREAMS`（JSON 数组，`[{name,base_url,path}]`）；未配置时任务空转 | `true` |
| `PRICING_SYNC_TASK_INTERVAL_MINUTES` | 价目同步周期（分钟，最小 1） | `360` |
| `CATALOG_SYNC_TASK_ENABLED` | **模型目录**定时同步开关（B2-2）：上游 llm-metadata 的 models/vendors 增删与字段对齐。默认关（目录变更是有副作用的写）；仅自动应用 create（新建）与 update（本地 `sync_official=1` 的官方条目），不删本地条目 | `false` |
| `CATALOG_SYNC_TASK_INTERVAL_MINUTES` | 目录同步周期（分钟，最小 1） | `720` |
| `CATALOG_SYNC_LOCALE` | 目录同步拉取的上游语言（`zh` / `en` / `ja`） | `zh` |
| `LOG_STAT_MAX_DAYS` | 管理端日志统计（`SumUsedQuota`）的时间窗口**天数上限**：传入无上界/超长窗口时收敛到该边界，避免对全历史做无界 SUM 聚合；`0` = 关闭收敛 | `366` |
| `COMPLEXITY_ROUTING` | 复杂度路由开关（规则版 7 维打分：长度/代码/数学/推理/工具/多模态/多轮 → simple/medium/complex）。默认关 | `false` |
| `TOOL_DRAWER_ENABLED` | 工具抽屉开关：对 `tools` 定义做**等价去重**（同 name/schema 指纹只留一份），省 prompt token。默认关 | `false` |
| `CHANNEL_HEALTH_WEIGHTED_LB` | 健康加权负载均衡：渠道选择按「基础权重 × 健康系数」加权，健康渠道更常被选中。默认关 | `false` |
| `CHANNEL_HEALTH_MIN_WEIGHT_FACTOR` | 健康分为 0 的渠道保留的最小权重比例（%），避免彻底饿死、保留探测恢复 | `5` |
| `CHANNEL_COOLDOWN_AUTH_SECONDS` | 渠道鉴权失败（401/403）冷却时长（秒）；`0`=用默认 300 | `300` |
| `CHANNEL_COOLDOWN_RATE_LIMIT_SECONDS` | 渠道限流（429，无 Retry-After 时）冷却时长（秒）；`0`=用默认 60 | `60` |
| `CHANNEL_COOLDOWN_SERVER_ERROR_SECONDS` | 渠道 5xx 冷却时长（秒）；`0`=用默认 60 | `60` |
| `CHANNEL_COOLDOWN_TIMEOUT_SECONDS` | 渠道超时冷却时长（秒）；`0`=用默认 30 | `30` |
| `RELAY_AUDIT_ENABLED` | 中继一致性自检开关（SSE 白名单/usage 单调/错误不泄漏/渠道指纹）。默认关 | `false` |
| `OUTBOUND_UPLOAD_BANDWIDTH_BPS` | Savings Baseline 折算省时用的上行带宽（bps）；`0` = 只报字节口径不折算 | `0` |

📖 **Complete configuration:** [Environment Variables Documentation](https://docs.newapi.pro/en/docs/installation/config-maintenance/environment-variables)

</details>

### 🔧 Deployment Methods

<details>
<summary><strong>Method 1: Docker Compose (Recommended)</strong></summary>

```bash
# Clone the project
git clone https://github.com/lza6/new-api-Max.git
cd new-api

# Edit configuration
nano docker-compose.yml

# Start service
docker-compose up -d
```

</details>

<details>
<summary><strong>Method 2: Docker Commands</strong></summary>

**Using SQLite:**
```bash
docker run --name new-api -d --restart always \
  -p 3000:3000 \
  -e TZ=Asia/Shanghai \
  -v ./data:/data \
  calciumion/new-api:latest
```

**Using MySQL:**
```bash
docker run --name new-api -d --restart always \
  -p 3000:3000 \
  -e SQL_DSN="root:123456@tcp(localhost:3306)/oneapi" \
  -e TZ=Asia/Shanghai \
  -v ./data:/data \
  calciumion/new-api:latest
```

> **💡 Path explanation:**
> - `./data:/data` - Relative path, data saved in the data folder of the current directory
> - You can also use absolute path, e.g.: `/your/custom/path:/data`

</details>

<details>
<summary><strong>Method 3: BaoTa Panel</strong></summary>

1. Install BaoTa Panel (≥ 9.2.0 version)
2. Search for **New-API** in the application store
3. One-click installation

📖 [Tutorial with images](./docs/BT.md)

</details>

### ⚠️ Multi-machine Deployment Considerations

> [!WARNING]
> - All nodes must use the same primary database and the same `SESSION_SECRET`; otherwise Access Tokens, refresh sessions, and temporary authentication flows cannot be verified consistently.
> - Nodes connected to the same Redis must also use the same `CRYPTO_SECRET`, or their cache-key digests will differ and shared entries cannot be reused consistently.

The database is authoritative for login Sessions and for the per-user active/issuance limits. Redis Session entries are short-lived caches whose TTL follows `SYNC_FREQUENCY` (60 seconds by default) and never exceeds the Session's remaining lifetime.

| Redis topology | Session propagation | Rate limiting |
| --- | --- | --- |
| Shared Redis | Revocations and version publications normally propagate immediately | Redis limits are shared across nodes |
| Independent Redis per node | Nodes converge from the database within the effective `SYNC_FREQUENCY`; a newly rotated token may receive a temporary 401 on a node with stale cache | Each node has its own allowance, so aggregate capacity can reach roughly the configured limit multiplied by the node count |
| No Redis | Every Session validation reads the database | In-memory limits are independent per node |

A shorter `SYNC_FREQUENCY` reduces the independent-Redis staleness window but causes one additional primary-key Session lookup per active SID, per node, per TTL. These guarantees make Session authentication bounded-stale across the supported topologies; rate limits and other Redis-backed control-plane caches remain topology-dependent.

See [User authentication and login sessions](./docs/authentication.md) for the token, Origin-check and PAT contracts.

### 🔄 Channel Retry and Cache

**Retry configuration:** `Settings → Operation Settings → General Settings → Failure Retry Count`

**Cache configuration:**
- `REDIS_CONN_STRING`: Redis cache (recommended)
- `MEMORY_CACHE_ENABLED`: Memory cache

---

## 🔗 Related Projects

### Upstream Projects

| Project | Description |
|------|------|
| [One API](https://github.com/songquanpeng/one-api) | Original project base |
| [Midjourney-Proxy](https://github.com/novicezk/midjourney-proxy) | Midjourney interface support |

### Supporting Tools

| Project | Description |
|------|------|
| [new-api-key-tool](https://github.com/lza6/new-api-Max-key-tool) | Key quota query tool |
| [new-api-horizon](https://github.com/lza6/new-api-Max-horizon) | New API high-performance optimized version |

---

## 💬 Help Support

### 📖 Documentation Resources

| Resource | Link |
|------|------|
| 📘 FAQ | [FAQ](https://docs.newapi.pro/en/docs/support/faq) |
| 💬 Community Interaction | [Communication Channels](https://docs.newapi.pro/en/docs/support/community-interaction) |
| 🐛 Issue Feedback | [Issue Feedback](https://docs.newapi.pro/en/docs/support/feedback-issues) |
| 📚 Complete Documentation | [Official Documentation](https://docs.newapi.pro/en/docs) |

### 🤝 Contribution Guide

Welcome all forms of contribution!

- 🐛 Report Bugs
- 💡 Propose New Features
- 📝 Improve Documentation
- 🔧 Submit Code

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

## 🌟 Star History

<div align="center">

[![Star History Chart](https://api.star-history.com/svg?repos=lza6/new-api-Max&type=Date)](https://star-history.com/#lza6/new-api-Max&Date)

</div>

---

<div align="center">

### 💖 Thank you for using New API

If this project is helpful to you, welcome to give us a ⭐️ Star！

**[Official Documentation](https://docs.newapi.pro/en/docs)** • **[Issue Feedback](https://github.com/lza6/new-api-Max/issues)** • **[Latest Release](https://github.com/lza6/new-api-Max/releases)**

<sub>Built with ❤️ by lza6</sub>

</div>
