# 专项分析 · T2/T3 缺口方案选型（健康分参数化 & Web 防护策略维度）

> 定位：主指南 §T2/§T3 的**方案纵深**，供执行 AI 落地时直接采用；只读整理，未改业务代码。
> 生成：2026-09-25 · 锚点：`计划书/audit/channel-health-status.md`、`计划书/audit/web-protection-status.md`、perf-ledger 0009/0008。

## 1. T2-1 健康分策略参数化

### 现状（证据）
- 打分：`service/channel_health_score.go` 窗口 256 条 / 1h TTL（:29-31），公式 `successRate×(70+延迟分)`，延迟分 P95 1.5s 满分 30 / 10s 零分（:206-224）。
- 过滤：`model/channel_constraint.go:111-133`，env `CHANNEL_HEALTH_ROUTING`（默认 on）快照一次性读取（:144-157）。

### 方案（最小改动）
1. `setting/operation_setting` 新增 `channel_health_setting` 热更结构：
   - `window_seconds`（默认 3600）、`ring_size`（默认 256）、`success_weight`（默认 70，配合延迟权重=100-success_weight）、
   - `latency_best_ms`（默认 1500 满分）、`latency_worst_ms`（默认 10000 零分）、`min_score`（默认 0 = 仅冷却剔除）。
2. `service/channel_health_score.go`：打分/窗口初始化从设置读取，非法值回退默认（沿用 web_protection_setting 模式）。
3. `model/channel_constraint.go`：过滤读 `min_score` 设置（保留 env 快照兼容，env 优先或设置优先需显式定义）。
4. 测试：设置变更 → 打分变化；非法值回退；无样本 fail-open 不变；过滤 min_score 边界。

### 风险与回滚
- 无 schema 变更（operation_setting JSON）；设置热更热生效；回滚=恢复默认设置 + commit 回退。
- 需补 `channel-health-status.md` 状态更新；不破坏现有测试（`TestChannelHealth|TestChannelCooldown` 5/5、`TestChannelHealthRoutingFilter`）。

## 2. T3-1 Web 防护策略维度

### 现状（证据）
- 判定链：`middleware/web-protection.go` + `service/web_protection_tracker.go:182-243`（/v1 放行 → IP 封禁缓存 → 每-IP 令牌桶 → 429 计数/自动封禁）。
- 设置：`operation_setting/web_protection_setting.go`（enabled/limit/burst/auto_ban/…）。

### 方案（最小改动）
1. `web_protection_setting` 扩字段（JSON，默认全放行）：
   - `allowed_paths []string` / `blocked_paths []string`（glob 或前缀，二选一明确）、`ua_allowlist []string`（子串匹配，默认空=不启用）、`geo_mode`（`off|block|allow` + `geo_codes []string`；无 GeoIP 库时先不做或读 X-Forwarded-For 头映射简化版）。
2. `web_protection_tracker.go` 判定链插入新维度（默认不启用，零行为变化）：路径前缀白/黑名单 → UA 子串白名单 → （可选）地域。
3. 前端 `web-protection-page.tsx` SettingsTab 加字段输入 + i18n；`web/src/features/web-protection/api.ts` 类型同步。
4. 测试：每维度「未配置=放行 / 配置后=拒绝/放行」；与 IP 令牌桶叠加顺序（先维度后速率或反之，明确写进测试）。

### 风险与回滚
- 默认全放行 → 线上零行为变化；设置热更；回滚=清空新增字段 + commit 回退。
- 多实例一致性说明：令牌桶仍每实例独立（文档化，不做 Redis 全局计数）。

## 3. 通用落地纪律（两主题共用）
- 设置结构走 `config.GlobalConfig.Register(...)` 热更模式（web_protection_setting.go 参照）。
- 测试用 `github.com/stretchr/testify/require/assert`；不回退现有测试；改完跑 service+model+middleware 相关包。
- 文档回填：`audit/channel-health-status.md` / `audit/web-protection-status.md` 状态段 + perf-ledger 追加记录；`db_structure.md` 仅当加列（本方案不加）。
- 浏览器证据：STAGING 截图入 `计划书/e2e-evidence/browser-e2e-channel-health/`、`browser-e2e-web-protection/`（P3，用户要求时）。

## 闭环状态（2026-09-27，v1.3.44 回填）
- ✅ T2-1 健康分参数化已落地：setting/operation_setting/channel_health_setting.go（window/ring/success_weight/latency_best/worst/min_score），channel_health_score.go 已接线（:102 ring size、:235 success weight）。
- ✅ T3-1 Web 防护维度：allowed_paths/blocked_paths/ua_allowlist 已落地；ip_allowlist（CIDR/单 IP）v1.3.43 新增 + 前端设置页 + i18n 7 语言。
- ⚪ geo_mode 明确不做：无 GeoIP 库，X-Forwarded-For 简化版误判风险高；保留为后续可配项。
