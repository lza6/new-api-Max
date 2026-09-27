# 渠道健康分概览审计（T2 · channel-health-status.md）

> 生成：2026-09-25 · 只读审计（代码证据 文件:行号）+ 本机测试运行，不改业务代码。
> 范围：`service/channel_health_score.go`、`service/channel_select.go`、`model/channel_constraint.go`、
> `controller/channel_health.go`、`router/channel-router.go`、`service/probe/*`（B4 探测）、
> `web/src/features/channels/*`。
> 结论：**健康分聚合 + 路由过滤 + 前端徽章/冷却 hover 已落地**；**策略参数化与聚合概览卡未见实现**——与指南任务卡"健康分策略参数化 / 前端健康分概览"不等同于完成。

## 1. 已落地（代码证据）

### 1.1 健康分聚合（service/channel_health_score.go）
- 每渠道进程内滑动窗口 256 条（近 1h TTL）：`channelHealthRingSize=256`、`channelHealthWindowTTL=time.Hour`（:29-31）
- 统一记录入口：`RecordChannelOutcome(channelId, success, latency, class)`（:73-88）
- 冷却事件独立计数：`RecordChannelCooldownMatchWithClass`（:92-104）+ 最近一次冷却错误类（B5-2 hover 原因）
- 打分公式：`score = successRate × (70 + 延迟分)`；延迟分基于成功请求 P95，1.5s 满分 30 / 10s 零分（computeHealthScore:206-224）
- 快照字段：score / success_rate / p50 / p95 / cool_count / sample_count / cooling_down / cool_until / last_cool_class（:116-130）
- init 注册 model.ChannelHealthProbe 回调（:134-140），无样本渠道 fail-open

### 1.2 路由过滤（model/channel_constraint.go + service/channel_select.go）
- 过滤语义 FilterChannelHealth（:111-133）：`CHANNEL_HEALTH_ROUTING=on|true`（**默认 on**）→
  冷却中剔除（HealthCoolingExclude）→ 分数阈值剔除（HealthMinScore<=0 = 仅冷却剔除）→ 无样本 fail-open
- GetChannelConstraints 自动注入冷却剔除默认过滤（service/channel_select.go:23-31）
- env 开关为进程级一次性读取（channelHealthRoutingEnabled 快照，热更不覆盖 env），测试可 SetChannelHealthRoutingEnabled 覆盖（:144-157）
- 回归测试：TestChannelHealthRoutingFilter（model/channel_constraint_test.go:222）
- 实测：`go test ./service/ -run "TestChannelHealth|TestChannelCooldown" -count=1` → **5/5 PASS**（1.39s）

### 1.3 管理端 API
- `GET /api/channel/health_scores`（router/channel-router.go:45，authz.ChannelRead）
- controller.GetChannelHealthScores：Pluck 全部渠道 id → 逐渠道快照 map（controller/channel_health.go:13-31）；
  无样本渠道返回零值快照

### 1.4 前端概览（web/src/features/channels）
- ChannelsProvider 统一拉取健康分 map（60s staleTime）：channels-provider.tsx:110-115，注入 context
- 渠道表 Health 列：ChannelHealthCell（channels-columns.tsx Health column）——探针 grade 徽章 > 健康分徽章 > "Not probed"
  （channel-health-cell.tsx:225-285）；Popover 展示探针用例结果 + 健康分详情（成功率/P50/P95/样本数/冷却次数）
- 冷却 hover：启用且冷却中渠道状态徽章旁 Tooltip「冷却至 X · 原因」（channels-columns.tsx:1015-1058，60s 缓存复用）
- 前端测试：channel-health-cell.test.tsx（Popover 内容）；lib/channel-health.ts 提供 grade/score 到徽章 variant 映射与健康数据解析

### 1.5 B4 渠道验真探测（补充）
- service/probe/*：真实 OpenAI 兼容 chat 探测题（model-id/cache/capability/consistency/billing/stream），A-F grade 存 `channels.probe_result`（model/channel.go:50）
- 调度：每日定时探测默认 off（channel_setting.ProbeScheduleEnabled，controller/channel_probe.go:127-135）；手动「立即探测」不受开关限制
- 前端：ChannelTestDialog（连通性测试）+ Health 列探针徽章复用 probe_result

## 2. 缺口（未落地 / 待办）

| 缺口 | 现状 | 建议 | 优先级 |
|---|---|---|---|
| 健康分策略参数化 | 窗口/权重/最低分数阈值是代码常量 + env 开关，未进 operation_setting 热更 | 扩 channel_setting：window_seconds、success_weight、latency_best/worst_ms、min_score；改后打分变化 + 非法值回退默认的测试 | P1 |
| 前端健康分聚合概览 | 只有逐渠道徽章/详情，无「均值/最差渠道/近期可用率」聚合卡 | channel 页新增 Overview 卡片：全部渠道健康分均值、最差 N 渠道、近期可用率趋势（复用 Provider 已拉数据，不加请求） | P2 |
| 多渠道建议（单点提示） | 无「某模型仅 1 渠道」管理端提示 | 管理端按模型统计可用渠道数=1 时提示「存在单点，建议添加渠道」；用户侧错误文案区分「无渠道」与「唯一渠道过载」 | P2 |
| 多实例健康分共享 | 窗口在进程内存，restart 清零，多实例各自统计 | 如需跨实例聚合：纳入探针结果（DB probe_result）或 Redis 滑动窗口（本期不做，注明语义即可） | P2 |
| 浏览器证据 | 组件测试在，未跑真实浏览器截图 | STAGING 起网关+渠道，三断点截图入 e2e-evidence/browser-e2e-channel-health/ | P3（用户要求时） |

## 3. 与 T2 任务卡对照（下一步改进指南 §T2）
- 原子任务 1「健康分策略参数化」：**未落地**（无设置项）
- 原子任务 2「前端健康分概览」：**部分落地**——逐渠道徽章/详情/hover 已有；**聚合概览卡未落地**
- 原子任务 3「多渠道建议」：**未落地**

## 4. 本机验证（已运行）
- `go test ./service/ -run "TestChannelHealth|TestChannelCooldown" -count=1 -v` → 5/5 PASS（1.39s）
  （TestChannelHealthScoreFormula / RingAggregation / SnapshotRealData / SnapshotCoolingReflectsCooldown / SnapshotEmptyReturnsZero）
- 模型层路由过滤测试 TestChannelHealthRoutingFilter 已存在（未单独重跑，此前批次已锁）

## 5. 结论
- T2 的「健康分路由 + 前端徽章」主体已完成并有测试锁（TestChannelHealthRoutingFilter + service 5 测试）；
  健康分路由默认 on（CHANNEL_HEALTH_ROUTING=true），具备冷却剔除 + 低分剔除 + fail-open。
- 剩余工作集中在：参数化（P1）、聚合概览卡（P2）、多渠道建议（P2）、浏览器证据（P3）。
- 未验证项均显式标注；不宣称"健康分概览已完成"（指南任务卡要求的三件套只完成了一件半）。