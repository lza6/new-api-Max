# 独立审查报告：v1.3.129 – v1.3.132（+ 接线）

- 审查员：独立 Critic（未修改任何代码文件）
- 审查基线：`4b24b558e..15ffde409`（`git log --oneline 72086c084..HEAD`）
- 覆盖提交：`4b24b558e`(v1.3.129) / `fbc793bbc`(v1.3.130) / `35984e2ce`(v1.3.131) / `6b8adc876`(v1.3.132) / `296c256b4`(docs)
- 审查日期：2026-10-12

> 说明：工作区当前存在**其它 agent 未提交**的改动（`controller/spa_seo.go` +238、`service/site_visitor_tracker.go` +239，mtime 00:02/00:05）。本报告结论**不含**这些未提交改动；仅在其能佐证已提交代码的问题时被引用。

---

## 1. 结论一句话

方向正确、可上生产（og:url 事故修复、价目表容器修复、产物 200 空体修复都成立且测试真跑通），但有 **1 个数据正确性缺陷（P1-1）** 与 **1 个公开端点并发超时缺陷（P1-2）** 建议修复后再上线，另有 **1 个「声称已接线、实际零调用」的死代码（P1-3）**。

**无 P0。**

---

## 2. 问题清单

### P1-1 单次 flush 超过 500 行会**静默永久丢弃**访客

- **证据**
  - `service/site_visitor_tracker.go:377` —— `collectRowsLocked` 把 `t.pending` 整体替换为新 map（已取出的行不再保留）。
  - `model/site_visitor.go:100-103` —— `UpsertSiteDailyVisitors` 直接 `rows = rows[:siteVisitorMaxBatch]` 截断，**无日志、无重试、无剩余队列**。
  - `visitorBufferMaxEntries = 50_000`（`service/site_visitor_tracker.go:48`）允许 pending 远超 500。
- **影响**：30 秒窗口内出现 >500 个新 IP 时，超出部分**不会**在下一次 flush 补写，整条访客记录（含首访来源 `source` / 落地 `path`）丢失 —— 与被丢弃的增量不同，这不是「只损失回访计数」。与文件头 :34-44 的说明（"只损失这次的回访计数，不会产生一条错误的访客记录"）不符。攻击面：换源 IP 洪泛即可人为压低 UV 或造成统计抖动。
- **复现方式**：向 `pending` 灌入 501 条不同 `ipHash` → 调用 `FlushSiteVisitors()` → 查库仅有 500 行，且第 501 条在下次 flush 也不会补上。
- **建议修法**：`UpsertSiteDailyVisitors` 内部按 500 分批循环 upsert（同一事务内）；或 `collectRowsLocked` 只取前 500 条并把剩余留在 `pending` 下轮再写。二者择一，并加用例锁「>500 行时总数不丢」。

### P1-2 `/api/site/visitors` 缓存过期即击穿，公开端点可打到 30s 超时

- **证据**
  - 提交版 `service/site_visitor_tracker.go:491-544` —— 读缓存未命中就**同步**跑完整聚合，无 singleflight、无 stale-while-revalidate。
  - 聚合包含 2 次 `COUNT(DISTINCT user_id)` 打 `logs`（`model/site_visitor.go:355-362`）。
  - `logs` 的可用复合索引 `idx_log_user_type_created` 前导列是 `user_id`（`model/log.go:63-65`），**无法**为该 WHERE（`type = ? AND created_at >= ?`）提供范围扫描。
  - 佐证：工作区未提交改动自带实测记录 —— 12 并发 × 20 请求下 `/api/site/visitors` 出现 **11 次 30s 超时**，单次聚合 **6.3 秒**。
- **影响**：Redis 未启用、或缓存刚过期的瞬间，N 个并发请求各自跑一遍 6.3s 聚合并互相争抢 DB → 少数请求撞上 30s 超时。这是挂在公开路由（`router/api-router.go:39-42`，无认证）上的端点，匿名即可触发。原注释「查询本身都是小表索引扫描，可接受」与实测不符。
- **复现方式**：关闭 Redis（或等 TTL 过期）后并发 `GET /api/site/visitors`；或直接对 `logs` 表执行该 WHERE 看 explain plan。
- **建议修法**：singleflight + 后台刷新（只要手里有值就立即返回旧值，仅冷启动同步算一次）。工作区已有未提交实现，需评审后提交。

### P1-3 `ForgetChannelHealth` 是死代码（声称已接线，实际零调用）

- **证据**
  - 定义：`service/channel_health_score.go:103`；其注释 :100 明确写「由渠道删除路径调用」。
  - 实际：`controller/channel.go:748-758` 的 `DeleteChannel` 成功分支只调用 `model.InitChannelCache()` / `service.InvalidateProxyClient(...)` / `service.ResetProxyClientCache()`，**没有** `ForgetChannelHealth`。
  - 全仓检索：`grep -rn ForgetChannelHealth --include=*.go | grep -v _test` → 仅定义处，无调用点。
- **影响**：渠道删除后样本环（32 个样本槽）与快照缓存**永不释放**，正是该函数存在的唯一理由；重复本项目已记录过的「库全绿、生产零调用」教训。
- **复现方式**：上述 grep；或删除渠道后观察 `channelHealthTable` 仍保留该 id 条目。
- **建议修法**：在 `DeleteChannel` 成功分支（`controller/channel.go:757` 附近）加 `service.ForgetChannelHealth(id)`；如需更强保证，加一条断言调用点的用例。

### P2-1 三库只验了 SQLite；MySQL 分支的注释理由自相矛盾

- **证据**
  - `model/db_conformance_test.go` 未覆盖 `SiteDailyVisitor`；`controller/main_test.go:57-62`、`service/task_billing_test.go` 的 TestMain 也只用 SQLite。
  - 新增的是**手写多值 INSERT + 两条方言分支**：`model/site_visitor.go:159-167`（ON CONFLICT）与 `:169-177`（ON DUPLICATE KEY）。
  - `:172-175` 使用 MySQL 的 `VALUES(col)`，该语法自 MySQL **8.0.20 起已弃用**（CI 与 compose 用的是 mysql:8.4 / 8.2）。
  - 而 `:93-95` 的注释正是以「GORM 生成的 `VALUES(col)` 已被 8.0.20 弃用」为由拒绝使用 `clause.OnConflict` —— 理由与自身实现矛盾。
- **影响**：AGENTS.md 明确要求 SQLite/MySQL/PostgreSQL 三库真实验证，本批未做；MySQL 分支的正确性目前只有静态阅读支撑。弃用不影响当前（含 8.4）可运行，但注释中的技术理由不成立，会误导后续维护者。
- **复现方式**：`TEST_MYSQL_DSN=... TEST_POSTGRES_DSN=... go test ./model/ -run TestDBConformance`（当前无用例覆盖该表，等于未验证）。
- **建议修法**：给 `UpsertSiteDailyVisitors` 补三库 conformance 用例（含「同批重复键」「hits 累加」「is_bot AND 语义」）；把 :93-95 的注释改写为「需兼容 MySQL 5.7.8，因此保留 `VALUES()`，忽略 8.0.20+ 的弃用告警」。

### P2-2 计费标签与前端**仍不一致**（任务型模型）

- **证据**
  - 前端权威判据 `web/src/features/pricing/lib/billing-mode.ts:34-38`：`hasTaskUsageSchema` 为**最高优先级**，命中则显示 `Task billing`。
  - `hasTaskUsageSchema` = `Object.keys(model.billing_usage_schema ?? {}).length > 0`（`web/src/features/pricing/lib/dynamic-price.ts:139-141`）。
  - `controller/spa_seo.go:419-437` 的 `spaPricingModeLabel` **没有**该分支，只判表达式 / quota_type。
- **影响**：对配置了 `billing_usage_schema` 的模型（任务型计费），服务端价目表会标成「按用量表达式计费 / 按次计费 / 按 token 计费」，而前端 badge 显示「Task billing」→ 与 v1.3.132 commit message 与 :412-418 注释所主张的「与前端 `getBillingModeLabelKey` 一致」不符。
- **复现方式**：构造 `billing_usage_schema` 非空的模型，比较 `/pricing` 服务端表格与其前端卡片 badge。
- **建议修法**：Go 侧 `model.Pricing` 已有 `BillingUsageSchema` 字段（`model/pricing.go:44`），可在 `spaPricingModeLabel` 前置一个 `len(p.BillingUsageSchema) > 0 → 任务计费 / Task billing` 分支，并补用例。

### P2-3 未配置密钥时 `ip_hash` 不稳定，去重**静默失效**

- **证据**
  - `common/init.go:67-71`：`CRYPTO_SECRET` 未设时 `CryptoSecret = SessionSecret`。
  - `common/constants.go:37`：`SessionSecret = uuid.New().String()` 是**进程启动时随机生成**；`common/init.go:59-66` 仅打印告警，不中止。
  - 使用点：`service/site_visitor_tracker.go:128-137` 的 `VisitorIPHash` 用 `common.CryptoSecret` 做 HMAC 密钥。
  - 仓库其它位置已把该状态视为不可信：`model/channel_key_migration.go:90` 明确排除 `common.CryptoSecret == "" || == common.SessionSecret`。
- **影响**：未配密钥时，**进程重启后同一 IP 在同一天会得到不同 hash**，`(day, ip_hash)` 唯一键挡不住 → UV 虚高、行数膨胀；多实例部署时每个实例各算一遍。隐私侧的「不可字典反查」仍成立，但统计口径失守且**无任何日志提示**。
- **复现方式**：不设 `SESSION_SECRET`/`CRYPTO_SECRET` 启动，访问两次（中间重启），观察同一 IP 当天出现两行。
- **建议修法**：启动时若 `CryptoSecret` 不稳定（空或等于随机 `SessionSecret`），记录一次明确的 warn，或按 `model/channel_key_migration.go` 的先例禁用/降级该统计。生产同时确认已配置 `SESSION_SECRET`。

### P2-4 SEO 注入块在 React 挂载前对用户**可见**（窗口 = 首屏时长）

- **证据**（均为静态确认）
  - `web/src` 全目录与 `web/index.html` 中**不存在**任何针对 `#spa-seo-content` 的样式；容器也没有 `hidden` 属性或内联样式（`controller/spa_seo.go:486`）。
  - 全仓 `spa-seo` 关键词命中仅集中在 `controller/spa_seo.go`（25x 行移除脚本 / :324 注释 / :374 表格 id / :486 容器）。**无 CSS 隐藏**。
  - 移除脚本要等 `#root` 首次出现子节点才删除（`controller/spa_seo.go:249-256`）。
  - 验收脚本 `计划书/e2e-evidence/batch-13/seo-pricing-invisible-e2e.cjs:86-95` 先在 `waitForFunction(root.childElementCount>0)` **之后**才取样状态，因此**无法**证伪这段窗口。
- **影响**：注释（:477-478）与提交信息宣称「用户看不到，爬虫看得到」。按作者自测数据（移动端 h1 可见约 5.4s），这段窗口足以让首屏出现爬虫内容块；`/pricing` 上还包含最多 120 行价目表。属于「宣称与事实不一致」，也影响真实用户的 CWV/首屏观感。
- **复现方式**：`curl` 原始 HTML 可见容器无隐藏样式；用浏览器在 `domcontentloaded`（React 挂载前）截图即可看到块内容。
- **建议修法**：二选一 ——（a）接受该窗口，把注释/文档改为「首屏期间可能短暂可见」，并让验收脚本在挂载**前**也取样以记录事实；（b）对容器加视觉隐藏（注意：不得做成对爬虫/用户展示不同内容的 cloaking；若要隐藏需确保爬虫仍能读到文本，例如 `position:absolute` + 屏外偏移而非 `display:none`/`visibility:hidden`）。

### P3（建议，不阻塞）

- **P3-1** `model/site_visitor.go:238` `dayFloor = today - 10000` 实为「去年同日」，而保留策略是 400 天（`service/site_visitor_tracker.go:53`）→ 第 366–400 天的数据被保留却**不计入 `Total`**，与该字段注释「自本表启用以来的累计」不符。
- **P3-2** 验收脚本 `seo-pricing-invisible-e2e.cjs:150` 期望非动态行一律「按次计费」，而 Go 对纯 token 模型输出「按 token 计费」，该断言与实现不符；其「15/15 PASS」需在生产实际数据下核实（若生产 `/pricing` 含 token 模型则应为 FAIL）。
- **P3-3** 缓存键 `site_visitor_summary:v1`（`service/site_visitor_tracker.go:492`）缺 `newapi:` 前缀，对照 `service/response_cache_redis.go:17` 的 `newapi:rc:` 约定；多站点共用 Redis 会串号。
- **P3-4** `isLoaded: query.isFetched` 语义已改（`web/src/features/home/hooks/use-home-page-content.ts:89-98`），而唯一消费者 `web/src/features/home/index.tsx:39` 已不再使用它；`active_users_24h/30d`（`types.ts`）返回但 UI（`traffic-stats.tsx`）未展示。

---

## 3. 我不确定的地方（「待验证」，不做猜测）

1. **MySQL / PostgreSQL 分支未实跑**：本机无 `TEST_MYSQL_DSN` / `TEST_POSTGRES_DSN`，只有 SQLite 基线。P2-1 的所有结论止于「静态确认」。
2. **生产密钥与代理配置未验证**：无法确认生产是否配置了 `SESSION_SECRET` / `CRYPTO_SECRET`（决定 P2-3 是否真实发生）；也无法确认 `TRUSTED_PROXIES` 前置代理是否落在默认私网段内 —— 若不在，`c.ClientIP()` 会把所有访客折叠成同一个 IP，UV 恒为 1，而现有 E2E 从单机隧道发起**测不出这一点**。
3. **生产 `/pricing` 实际是否含 token 模型**：决定 P3-2 是否会让验收脚本失败。
4. **E2E 报告的 21/21 与 15/15 未由我复跑**（需隧道与浏览器），因此仅作为对方的声明引用，未独立确认。
5. **工作区未提交改动**（`spa_seo.go` +238、`site_visitor_tracker.go` +239）未纳入本次评审；其中 singleflight/SWR 改动若合并，需重新评审其正确性（如 `refreshing` 标志的失败路径是否会导致永不刷新）。

---

## 4. 我认为做得对的地方

- **og:url 回归锁**（`controller/spa_seo_test.go` 的 `TestSPASEOSetsWellFormedOGURL`）同时断言「标签存在」与「语义正确」，并含反向锁 `NotContains("X content=")` —— 正是对本项目「CI 全绿但线上坏」教训的正确落地。
- **`SetTaskArtifactStoreForTest` 注入点**让「清理竞态」分支真正可测；且 `os.Open` 失败（`Stat` 与 `Open` 之间文件被删）也落在 `os.ErrNotExist → 回退`，逻辑闭环（`controller/task.go:341-348` + `service/task_artifact_store_local.go:177-181,190-196`）。
- **v1.3.132 的自我回退我独立核对过，结论是回退正确**：`model/pricing.go:389-398` 表明 `BillingExpr` 非空时必伴随 `billing_mode=tiered_expr`；`service/tiered_settle.go:102,165,204` 表明实际扣费确实走表达式。回退不是引入新错误。
- **pending 存增量而非累计值**（`service/site_visitor_tracker.go:73-84`）是对「SQL 侧 `hits = hits + excluded.hits`」这一写入语义的正确适配，用例 `TestTrackSiteVisitAccumulatesAcrossFlushes` 锁住了它。
- **`is_bot` 累积 AND 语义**（`:287`、`model/site_visitor.go:165,175`）与其三态用例（`TestTrackSiteVisitBotFlagIsCumulative`）在语义上是正确的：只有当天全部访问都是爬虫才标记为爬虫。
- **前端组件与服务端契约的一致性**：`web/src/features/home/components/traffic-stats.tsx` 的口径注释、三态处理（loading 骨架 / 失败整块隐藏）与 `site-stats.tsx` 既有约定保持一致。
- **接线真实性**：`TrackSiteVisit` 挂在 `router/web-router.go:43-46`（`static.Serve` 之前，`NoRoute` 链上），自身过滤 `/api/`、`/v1/`、`/assets`、带扩展名的资源与非 GET/HEAD（`service/site_visitor_tracker.go:227-256`）；`StartSiteVisitorLoop`（`main.go:433`）与 `StopSiteVisitorLoop`（`main.go:270`）成对存在，且 `backgroundLoop.stop` 会 `wg.Wait()` 阻塞到 goroutine 真正退出，停止前会 flush 未落库数据（`:447-449`），无孤儿 goroutine。

---

## 5. 我实际执行的验证（证据）

| 命令 | 结果 |
|---|---|
| `go test ./service/ -run 'TestTrackSiteVisit\|TestUpsertSiteDailyVisitors\|TestGetSiteVisitorStats\|TestDayOffset\|TestAddDelta\|TestCollectRows\|TestIsBotUserAgent\|TestTrafficSource\|TestIsVisitorTrackablePath\|TestVisitorIPHash\|TestCountRegisteredUsers\|TestDeleteSiteDailyVisitors' -count=1` | `ok github.com/lza6/new-api-Max/service 22.729s` |
| `go test ./controller/ -run 'TestGetSiteVisitorSummary\|TestSPA\|TestTaskArtifact' -count=1` | `ok github.com/lza6/new-api-Max/controller 14.719s` |
| `bun run typecheck`（`web/`） | 退出码 0 |
| 七个语言包 16 个新 i18n 键逐个校验（en/zh/zh-TW/fr/ru/ja/vi，`translation` 命名空间） | 全部存在 |
| `grep -rn ForgetChannelHealth --include=*.go \| grep -v _test` | 仅定义处，零调用 |

**未执行**：MySQL/PostgreSQL conformance、生产 E2E（隧道 + 浏览器）、生产环境探测。
