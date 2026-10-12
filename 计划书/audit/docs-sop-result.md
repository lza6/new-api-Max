# 文档交付结果 — SOP / ADR / 黄金范例（+ 计划书索引）

- 执行者：技术文档工程师（agent `docs-sop`）
- 日期：2026-10-12
- 核对基线：`VERSION` = `v1.3.135`
- 仓库根：`C:\Users\Administrator.DESKTOP-EGNE9ND\Desktop\公益服务网关\new-api`

---

## 1. 新增的文件（4 个，未改动任何现有文件）

| # | 路径 | 行数 | 作用 |
|---|---|---|---|
| 1 | `计划书/ops/SOP-运维手册.md` | 501 | 运维 SOP：速查命令、蓝绿部署与回滚、按症状的排障清单、事故响应、明令禁止的操作 |
| 2 | `计划书/ops/ADR-架构决策记录.md` | 480 | 11 条架构决策记录（背景/决策/理由/后果含代价/证据 `file:line`） |
| 3 | `计划书/ops/黄金范例.md` | 458 | 14 个正面范例 + 反面清单 |
| 4 | `计划书/README.md` | 85 | `计划书/` 目录索引（可选项） |

- **未**在 `docs/` 下新增任何文件（遵守 `AGENTS.md:148`）。
- `计划书/ops/` 目录原已存在（内含 `deploy-zero-downtime.example.sh` / `disk-guard.example.sh` / `token-billing-group.md`）。
- `git status` 中 `计划书/` 下的 `D`/`M` 条目为本次会话开始前既有状态，**非本次产生**。

---

## 2. 每篇的章节骨架

### 2.1 `SOP-运维手册.md`

1. §0 三条纪律（不要在生产改计费做测试 / 本机 curl 不可用 / 「进不去」≠ 站点挂了）
2. §1 速查表：查版本、容器状态、健康（`/healthz` `/readyz` `/api/status`）、日志、磁盘、内存、渠道健康、`/metrics`；§1.1 磁盘自动守卫（cron）
3. §2 部署：蓝绿零停机流程（端口 3000/3001 交替 → Caddy 切流）、部署后自检清单、**回滚（按故障阶段分三条路径）**、§2.4 前端必须重建镜像
4. §3 Caddy 上游与安全头（含 CSP 必须放行 `blob:` 的坑）
5. §4 排障清单（症状 → 根因 → 查证命令），覆盖 **9 个症状**：
   网站打不开/连接重置、全站 503、首字慢/空响应、计费看起来不对、内存持续上涨、磁盘持续上涨、某渠道大量失败、会话随机过期、登录/OAuth 异常
6. §5 常见误判与坑（7 条，含「页面还是旧版本」「点了没反应」「`/v1/pricing` 429 是正常限流」）
7. §6 事故响应：生产数据被错误修改（5 步：停手 → 取证 → 算差额 → 补偿 → 公告复盘）
8. §7 明确禁止的操作（8 条表格）
9. §8 本文刻意没写的内容及原因

### 2.2 `ADR-架构决策记录.md`

| ADR | 标题 |
|---|---|
| ADR-001 | `relaykit/` 是独立 Go module |
| ADR-002 | 根模块所有 JSON 走 `common/json.go` 包装 |
| ADR-003 | 计费的额度转换必须走 `common/quota_math.go` |
| ADR-004 | 计费表达式「一个表达式，一个真相」 |
| ADR-005 | 响应缓存默认关 + 默认按用户隔离 + 宁可不命中不可错命中 |
| ADR-006 | 响应缓存默认走磁盘后端，进程内 LRU 有硬容量上界 |
| ADR-007 | 任务/媒体产物默认走「上游代理」，站内图床是可选 local 模式 |
| ADR-008 | 灰度开关用「原子快照」而不是可变全局 bool |
| ADR-009 | 公开统计端点用 SWR + 单飞，请求路径上不做同步聚合 |
| ADR-010 | 服务端 SEO 注入块不得包含页面上不存在的内容（cloaking） |
| ADR-011 | Redis 不可用时限流降级为内存，而不是 fail-closed 500 |

每条格式：**背景 / 决策 / 理由 / 后果（含代价）/ 证据 `file:line`**。
另有 §12 决策依赖关系图、§13「为什么只有这些 ADR」（未写决策的清单）。

### 2.3 `黄金范例.md`

14 个范例，按要求的 6 类覆盖（部分类别给出 2 条）：

| 类别 | 范例位置 |
|---|---|
| 带反向锁的回归测试 | `controller/spa_seo_test.go:233-263`；`controller/task_generic_test.go:601-668` |
| 有界缓存 / 防无界增长 | `common/loop_heartbeat.go:27-69`；`common/metrics.go:51-58,63-104`；`service/response_cache.go:69-89` |
| 有界读写（不静默丢数据） | `common/read_limit.go:9-63`；`model/site_visitor.go:105-129` |
| 配置驱动的开关默认值安全 | `setting/system_setting/task_artifact_store.go:52-75`；`setting/response_cache_setting/response_cache_setting.go:19-22` |
| 前端三态处理 | `web/src/features/home/components/traffic-stats.tsx:29-51` |
| 错误处理不吞错 | `controller/task.go:331-347` |
| 注释解释「为什么」而非「是什么」 | `common/json.go:12-17`；`common/quota_math.go:10-12`；`service/site_visitor_tracker.go:70-77` |

末尾另有 §15 反面清单（9 条本项目明确不认可的写法，均来自真实事故或审计结论）。

### 2.4 `计划书/README.md`

子目录一览（`ops/` `audit/` `e2e-evidence/` `reports/`）+ 各目录文件表 + 根级文件表 + 「阅读顺序建议」（按目的给路径）。**只描述真实存在的文件**。

---

## 3. 引用核对结果

### 3.1 方法

1. 用脚本对 **4 篇文档**提取全部 `file:line` / `file:line-line` 引用；
2. 逐条 `Read`（或直接打印）**该行真实内容**并人工比对论断，**不是抽查**；
3. 每条引用同时校验：**文件存在** 且 **行号在文件行数范围内**。

命令：

```
node /tmp/verify_refs.cjs    # 存在性 + 行号范围
node /tmp/dump_refs.cjs      # 逐条打印该行真实内容，供人工比对
```

### 3.2 结果

| 指标 | 数值 |
|---|---|
| 核对引用数（去重） | **194 条** |
| 覆盖文件数 | **38 个** |
| 最终有效 | **194 / 194**（文件存在、行号在范围内、行内容与论断一致） |
| **修正数** | **26 条** |

### 3.3 修正明细

**(A) 行号错误（引用的行内容与论断不符）—— 7 条**

| 引用 | 原引用指向的实际内容 | 改为 | 说明 |
|---|---|---|---|
| `pkg/billingexpr/expr.md:9` | `### Core Principles`（小节标题） | `:11` | 应为「1. Expression is self-contained」 |
| `pkg/billingexpr/expr.md:13` | 「2. Variables are opt-in」 | `:15` | 应为「3. Prices are real prices」 |
| `pkg/billingexpr/expr.md:15` | 「3. Prices are real prices」 | `:17` | 应为「4. Upstream-agnostic」 |
| `pkg/billingexpr/expr.md:17` | 「4. Upstream-agnostic」 | `:19` | 应为「5. Version-aware」 |
| `service/channel_circuit_breaker.go:26` | 「复用既有健康快照做过滤点…」注释 | `:29` | 行为开关声明实在 `:29` |
| `service/channel_circuit_breaker.go:44-46` | 注释 + 另一个变量（`circuitBreakerEnvDefault`） | `:47-48` | 阈值变量在 `:47`（`CHANNEL_CIRCUIT_FAILURE_THRESHOLD`）、`:48`（`CHANNEL_CIRCUIT_OPEN_SECONDS`） |
| `middleware/logger.go:55` | 一行注释 | `:54-58` | 实际代码调用在 `:57`（`NormalizeHTTPMethodLabel(param.Method)`） |

**(B) 引用路径不完整（只写裸文件名，无法独立定位）—— 19 条**

| 原写法 | 条数 | 改为 |
|---|---|---|
| `deploy-zero-downtime.example.sh:…` | 13（SOP 12 + ADR 1） | `计划书/ops/deploy-zero-downtime.example.sh:…` |
| `expr.md:…` | 4（与 A 组重叠） | `pkg/billingexpr/expr.md:…` |
| `traffic-stats.tsx:…` | 2 | `web/src/features/home/components/traffic-stats.tsx:…` |

**(C) 补充引用 —— 2 条**

新增 `main.go:43`、`main.go:46`，用于准确标注 `go:embed web/dist` 的真实出处
（原仅引 `Dockerfile:27,39`，只够支撑「构建时拷贝产物」，不足以支撑「embed 进二进制」这一论断）。

### 3.4 命令/脚本路径存在性

文档中出现的脚本与命令均来自**真实存在的文件**，已 `ls` 确认：

- `计划书/ops/deploy-zero-downtime.example.sh`、`计划书/ops/disk-guard.example.sh`
- `deploy/Caddyfile.example`
- 服务器侧路径（`/opt/new-api/deploy-zero-downtime.sh`、`/opt/new-api/disk-guard.sh`）来自脚本内注释与既有事故记录，**已在文中标注为「服务器上的脱敏模板对应的真实位置」**，未杜撰。

---

## 4. 刻意没写的内容及原因

| # | 未写内容 | 原因 |
|---|---|---|
| 1 | **SLA / 可用性百分比 / MTTR** | 仓库内**没有任何真实的历史可用性度量数据**（生产 `METRICS_ENABLED` 未开启、无外部监控接入）。编造数字只会误导后续排障 |
| 2 | 生产 `.env` 内容、任何凭据、生产 Caddyfile 原文 | ① 安全边界；② 仓库内只有脱敏模板（`*.example.sh`）与示例副本（`deploy/Caddyfile.example`），引用会与线上不一致 |
| 3 | `/opt/new-api/rollback.sh` 的完整步骤 | 该脚本出现在 2026-09-28 的旧 `docker compose` 部署方案里；当前蓝绿流程用的是 `deploy-zero-downtime.sh` 内建回滚。**两者当前关系待现场确认**，故只写蓝绿回滚（三条路径），不写该脚本步骤 |
| 4 | 数据库三引擎（SQLite/MySQL/PG）同支持的**选型理由**、Gin/GORM/React/Rsbuild 的**选型理由** | 仓库里只有**约束条文**（`AGENTS.md:92`）与技术栈清单（`AGENTS.md:11-17`），**找不到任何「为什么这么选」的记录**。只有约束没有理由，不构成 ADR |
| 5 | Sobek 执行 JS 任务插件、限流器为何用固定窗口、Caddy 健康检查参数取值的**决策依据** | 同上：只找到「是什么」，未找到书面的设计权衡 |
| 6 | 多实例部署的完整配置 | 当前生产是**单实例**（`deploy-zero-downtime.example.sh` 起单个容器），多实例相关的 `SESSION_SECRET` 一致性要求只在 SOP §4.8 作为根因提示 |

上述 4/5 两项已在 `ADR-架构决策记录.md` §13 逐条列出并标注「待确认」，**不冒充已决**。

---

## 5. 与旧记忆不一致、已按代码实际行为更正的现状

排障根因一律落在**真实代码或既有事故记录**上，并对以下两处**已修复的历史缺陷**按当前代码行为描述（避免运维按过时记忆误判）：

| 项 | 旧记忆中的状态 | 当前代码实际行为 | 证据 |
|---|---|---|---|
| 限流器在 Redis 故障时 | fail-closed 直接 500（2026-09-29 事故 246 次 500） | **降级为进程内内存限流** | `middleware/rate-limit.go:112-131` |
| `GET /api/log/traffic` | 把窗口内全量 44 万行 / 290MB+ 塞进 Redis → OOM | **SQL 侧按日聚合**；Redis 缓存值只允许按日行（≤days 组） | `controller/log.go:170-200` |

另：生产 `/metrics` 返回前端 SPA HTML（`METRICS_ENABLED` 未开启），已在 SOP §5.6 标注，并说明「是否开启需决策」。

---

## 6. 剩余风险与未做的验证

- **未跑构建/测试**：本次只新增 Markdown，不触及任何代码，故未运行 `go build` / `bun run typecheck` / `bun run build`（按「按风险适度验证」原则）。
- **`计划书/ops/*.example.sh` 与服务器真实脚本可能存在差异**：脱敏模板中未包含 `NODE_TYPE=slave`（既有记忆明确要求新容器必须带该变量）。已在 SOP §2.1 标注「以服务器上的真实脚本为准」，并列出该约束的来源。
- **并发写入风险**：编辑期间收到过「文件在磁盘上已被修改」提示（同批次有多个 agent 在写）。本次只写自己新建的 4 个文件 + 本报告文件，未触碰他人文件。
