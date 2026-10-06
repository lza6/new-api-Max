# Batch-5 施工状态（7 项迁移任务）

> 起始版本 v1.3.99。用户全量授权（2026-10-06）。来源：litellm / answer-me-with-html / kiwi-mem+aci / ccLoad / api-relay-audit 的可迁移能力。

## 任务拆解

| # | 任务 | 交付物 | 验收 | 风险 |
|---|---|---|---|---|
| T1 | 统一节省口径 + Savings Baseline | 反事实基准（对比未优化节省 X%）+ 端点 | 端点契约测试；E2E | L1 |
| T2 | 黑匣子日志分级透明化 | admin_info 延迟/压缩/亲和/quota_saturation 对用户分级开放（白话版/技术版） | 可见性测试；前端分级 | L2 |
| T3 | 可读输出/解释页（HTML） | 日志原理可视化 HTML 渲染 | 前端组件测试；E2E | L1 |
| T4 | 复杂度路由（规则版 7 维） | 本地打分→tier，可解释、<1ms | 单测（打分/边界/性能）；开关 | L2 |
| T5 | 工具抽屉 / 元函数三段式 | 按需注入工具描述省 token | 单测；开关；灰度 | L3（请求路径）|
| T6 | 健康排序 LB + 可配置冷却 | 健康分排序选择 + 冷却参数可配 | 单测；开关；回滚 | L2 |
| T7 | 中继一致性自检 | SSE 白名单/usage 单调/错误不泄漏/渠道指纹 | 单测；E2E | L2 |

## 复用盘点（侦察结论）
- T1：`model/model_compression_stat.go`（按模型压缩统计已持久化）+ `controller/rankings.go GetCompressionStats`
- T2：`model/log_other.go`（public/adminInfo/rootInfo 三级可见性已存在）+ `controller/cost_detail.go GetLogCostDetail`
- T3：`controller/cost_detail.go parseLogCostDetail` + 前端 `explain-breakdown`
- T4：**全新** `service/complexity_router.go`
- T5：**全新**，relay 请求路径（高风险）
- T6：`service/channel_select.go` + `service/channel_health_score.go` + `service/channel_cooldown.go`（已有冷却/健康分，缺排序与可配参数）
- T7：**全新** `service/relay_audit.go`

## 执行顺序（先自包含→后请求路径）
1. T4、T7（独立新文件，无冲突）
2. T1（复用压缩统计）
3. T6（增强既有选择/冷却，开关默认关）
4. T2、T3（日志透明化 + 解释页）
5. T5（请求路径，开关默认关，最后做）

## 验证台账（v1.3.100 施工后）

### 构建/静态
- `go build ./...` ✅；`go vet ./service/ ./model/ ./controller/ ./relay/channel/openai/` ✅
- gofmt 全部改动文件 ✅

### 单元/集成
- `go test ./service/ -count=1` ✅ ok 8.8s（含新 `batch5_test.go`：T4 打分/tier/性能(<0.5ms)、T7 错误泄漏/usage单调/模型指纹/SSE白名单、T1 反事实基准、T5 去重/指纹/元函数索引）
- `go test ./model/ -count=1` ✅ ok 56s（含新 T6 `TestWeightedSelectWithHealthBias`/`NoSamples`）
- `go test ./relay/channel/openai/ ./relay/helper/` ✅（T5 接入 adaptor 无回归）
- `go test ./controller/ -run TestV1Pricing|TestReadyz|TestHealthz` ✅

### 反向验证（先红后绿）
- **T5**：单测抓出真实 bug——`normalizeToolSchema` 首版「折叠空白」不足以让 `{ "a" : 1 }` ≡ `{"a":1}`；改为「字符串字面量外移除空白」的状态机（保留字符串内空白，避免语义不同 schema 误去重）→ 测试转绿。
- **T6**：临时把健康系数改 `factor=1.0`（忽略健康分）→ `TestWeightedSelectWithHealthBias` **FAIL**（健康渠道不再显著领先）；还原 → PASS。实测占比 19011/10045/944（健康 100/50/0 分）。

### 前端
- 新增 `transparency-panel.tsx`（T3 可读解释页）+ `api.ts` transparency 客户端 + details-dialog 接线
- `bun run typecheck` 我的文件 0 错；`vitest transparency-panel.test.tsx` 2/2 PASS
- i18n：新增 `Request Explanation`/`No explanation available...`/`Upstream wait`/`Error Logs Only` 等键，7 语言齐备，`i18n:check` 0 missing

### 开关（均默认关，零行为变化）
| env | 作用 |
|---|---|
| `COMPLEXITY_ROUTING` | T4 复杂度路由 |
| `TOOL_DRAWER_ENABLED` | T5 工具抽屉去重 |
| `CHANNEL_HEALTH_WEIGHTED_LB` | T6 健康加权 LB |
| `CHANNEL_HEALTH_MIN_WEIGHT_FACTOR` | T6 弱渠道保底比例（%，默认 5） |
| `CHANNEL_COOLDOWN_{AUTH,RATE_LIMIT,SERVER_ERROR,TIMEOUT}_SECONDS` | T6 可配冷却时长 |
| `RELAY_AUDIT_ENABLED` | T7 中继一致性自检 |
| `OUTBOUND_UPLOAD_BANDWIDTH_BPS` | T1 省时折算带宽（0=不折算） |

### 端点
- `GET /api/rankings/savings-baseline`（T1）
- `GET /api/log/usage/:id/transparency`（T2/T3）

