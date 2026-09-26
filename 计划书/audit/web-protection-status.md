# Web 防护状态审计（T3 · web-protection-status.md）

> 生成：2026-09-25 · 更新：2026-09-26（v1.3.35 T3 闭环）
> 范围：`middleware/web-protection.go`、`setting/operation_setting/web_protection_setting.go`、
> `service/web_protection_tracker.go` / `web_protection_cleanup.go`、`controller/web_protection.go`、
> `router/web-protection-router.go`、`web/src/features/web-protection/*`、`web/src/i18n/locales/{en,zh,zh-TW,fr,ru,ja,vi}.json`。
> 结论：**策略配置化（T3-1 v1.3.34）+ 实时状态页（T3-2 v1.3.35：在线请求数 + 近期封禁事件表 + UI 解封）已落地**；
> 后端单测 + 前端单测 + 真实浏览器 E2E（截图入 `计划书/e2e-evidence/browser-e2e-web-protection/`）全部通过。
> 剩余未落地项（多实例全局统一限流、地域维度）如实标注，不宣称完成。

## 1. 已落地（代码证据）

### 1.1 策略配置化（热更新）
- 配置项注册：`config.GlobalConfig.Register("web_protection", &webProtectionSetting)`（web_protection_setting.go:39-45）
- 可配字段（setting/operation_setting/web_protection_setting.go:11-37）：
  - `enabled` 总开关、`limit_per_second` 每 IP 每秒、`burst` 令牌桶容量、`auto_ban` 自动封禁
  - `auto_ban_threshold_per_minute`（60s 窗口被 429 拒绝次数阈值）、`auto_ban_minutes`、`log_enabled`、`window_seconds`（默认 60）
- 前端表单可保存并回读：`web-protection-page.tsx` SettingsTab（enabled/rate/burst/auto-ban/log 全字段 + 三档 PRESET 快捷档），
  `PUT /api/admin/web-protection/settings`（web-protection/api.ts:54-58）
- 非法值回退默认：`GetWebProtectionLimit`（perSec<=0→10，burst<perSec→perSec）、`GetAutoBanThreshold`（<=0→20）、`GetAutoBanMinutes`（<=0→1440）

### 1.2 中间件行为
- 入口：`middleware/web-protection.go` WebProtection()（全流量计吞吐 + TrackWebRequestBegin/End）
- 判定链（service/web_protection_tracker.go:182-243）：`/v1` 前缀完全放行（模型 API 不受影响，走 token 级限流）→
  IP 封禁缓存检查（5s TTL，isIPBannedCached:165）→ 每-IP 令牌桶（rate=perSec/s, cap=burst）→
  超限 429 计数，达阈值自动 `model.BanIP(ip,"auto:web_rate_limit","auto",minutes)`（214-222）
- 拒绝响应：429 + `Retry-After: 60` + error type `ip_banned` / `rate_limited`（writeWebProtectionReject:409）
- 日志聚合：每窗口按 IP+路径落库（closeWindowLocked），每 30s 批量 flush（flushIfDue:329），异步写库失败仅告警
- 清理：StartWebProtectionMaintenanceLoop 每小时 flush + 清理过期封禁 + 7 天日志（web_protection_cleanup.go:31-47；main.go:380 启动，仅主节点）

### 1.3 状态页
- 接口：`GET /api/admin/web-protection/server-stats`（router/web-protection-router.go:15），仅 AdminAuth
- 内容（controller/web_protection.go:79-107）：
  - `network_in/out_mbps`：service.GetNetworkThroughput 进程内原子采样（全流量含 /v1，仅计量不限流）
  - `banned_count`（model.CountActiveBannedIPs）+ `today_request_count / today_bytes_sent / today_bytes_received`
  - `instance`：本节点 system_instances 最近上报（CPU/内存/磁盘/运行时/主机名）+ uptime
- 鉴权锁：TestServerStatsAdminOnly（controller/web_protection_test.go:21）
- 前端：`web-protection-page.tsx` ServerStatsCard 每 1.5s 轮询展示（网络出/入、CPU、内存、磁盘、主机、运行时、uptime、封禁数、今日请求/流量）

### 1.4 封禁管理
- 接口齐全：settings GET/PUT、server-stats、maintenance、web-request-logs（+detail）、banned-ips CRUD + unban + 批量解封（router/web-protection-router.go:13-23）
- 前端 BannedTab：手动封禁（IP/时长/原因）、解封、批量解封

## 2. 缺口（未落地 / 待办）

| 缺口 | 现状 | 建议 | 优先级 |
|---|---|---|---|
| 策略维度（UA/路径/白名单） | 已闭环（v1.3.34）：operation_setting 增 allowed_paths/blocked_paths/ua_allowlist，中间件判定链实现（默认空=全放行），单测覆盖参数变化与非法值回退 | —— | DONE |
| 在线并发/队列维度 | 已闭环（v1.3.35）：service.TrackWebRequestBegin/End 原子计数，GetWebProtectionInFlight；server-stats 返回 in_flight；前端「当前在线请求」卡 | —— | DONE |
| 近 5 分钟封禁事件流 | 已闭环（v1.3.35）：server-stats 返回 recent_bans（最新生效 10 条）；前端「最近封禁」列表 + 一键解封按钮 | —— | DONE |
| 地域/国家过滤 | 未落地：需 geo 数据源与判定位 | 后续版本单独评估 | P2（待办） |
| 多实例全局统一限流 | 未落地：令牌桶/封禁缓存为进程内单例，多主节点各实例独立计数；封禁写入共享 DB 后 5s 缓存收敛 | 如需全局统一限流需 Redis 计数（本期不做） | P2（待办） |


## 3. 与 T3 任务卡对照（下一步改进指南 §T3）
- 原子任务 1「策略配置化补全」：**已完成（v1.3.34）**——启停/阈值/封禁时长/路径白黑名单/UA 白名单全字段可配，
  配置热更单测（web_protection_test.go: TestWebProtectionPathAndUAPolicy / TestWebProtectionPathAndUAGettersSanitize）+ 前端表单保存回读测试通过。
- 原子任务 2「实时状态页」：**已完成（v1.3.35）**——server-stats 增 in_flight + recent_bans；
  前端 ServerStatsCard 增「当前在线请求」卡 +「最近封禁」列表（IP/原因/时间/解封按钮）；7 语言 i18n。
- 原子任务 3「封禁-解封 E2E」：**已完成（v1.3.35 真实浏览器）**——mock 策略触发封禁 → 状态页 recent_bans 出现 →
  UI 解封 → 页面实时消失；截图 5 张入 `计划书/e2e-evidence/browser-e2e-web-protection/`。

## 4. 验证结果（v1.3.35 已运行）
- 后端：`go test ./service/ -run TestWebProtectionInFlight -count=1` → 4/4 PASS；
  `go test ./controller/ -run "TestServerStats|TestWebProtection" -count=1` → 全 PASS（含新增 TestServerStatsIncludesInFlightAndRecentBans 封禁-解封字段断言）；
  `go vet ./...` / `go build ./...` → 无错误。
- 前端：`bun run typecheck` → 通过；`bunx vitest run src/features/web-protection` → 2/2 PASS（含在线计数 + 最近封禁解封 UI 断言）；`bun run build` → 成功。
- 真实 E2E（本机网关 v1.3.35 + Chrome headless CDP，截图在 e2e-evidence/browser-e2e-web-protection/）：
  - 策略真实生效：Mozilla+/dashboard → 200；curl+/dashboard → 429；Mozilla+/admin/users → 429
  - 并发 30/40 请求下 in_flight 实时 >0（状态页「当前在线请求」= 1）
  - 封禁 → recent_bans 出现（API 与浏览器 UI 均验证）→ UI 解封 → 页面实时消失
  - 截图：realtime-status-page.png / realtime-inflight.png / banned-tab.png / recent-ban-visible.png / recent-ban-cleared.png

## 5. 结论
- Web 防护已从「裸中间件」升级为「可热更配置（launch+热更）+ 实时状态页（在线数/最近封禁/吞吐/资源）+ 封禁管理闭环（封禁/解封/批量解封/维护）」，且 /v1 零开销隔离设计正确。
- T3 全部原子任务已闭环（v1.3.34 策略维度 + v1.3.35 实时状态页 + 浏览器 E2E 证据）。
- 诚实边界：地域过滤、多实例全局统一限流为显式待办；in-flight 为进程内计数（多实例下各实例独立，与令牌桶语义一致）。
- 所有结论均有源码行号 / 测试输出 / 真实浏览器截图佐证。