# 施工台账 — Batch-8 / Batch-9（G1 灰度闭环 + G2 自耗削减）

> 基线：`v1.3.123`（HEAD `8e5a29217`）｜指南：`计划书/下一步改进指南.md`
> 纪律：只记录**实际运行的命令与观察到的结果**；未验证项明确标「待验证」。

---

## 0. 批次契约与验收标准

| 批次 | 主题 | 交付物 | 验收标准 |
|---|---|---|---|
| **Batch-8** | G1 暗开关中心（地基） | 能力开关注册表 + 管理端页 + 度量 + 审计 | 开关可热更新且重启后保持；高风险二次确认；审计留痕；**开关全关时全站行为零变化** |
| **Batch-9** | G2 自耗削减 | 出站读取上限收口 + 出站客户端整体超时 | 上限有边界用例；超时断言 `Timeout > 0`；超限**报错不截断** |

**Batch-8 为什么排第一**（指南 §2.3）：它不产生任何业务行为变化，却解除后续所有批次
（G3 响应缓存 / G4 路由 enforce / G5 策略 enforce / G6 图床接线）的灰度能力瓶颈。

---

## 1. Batch-8 / G1 — 实际改动

### 1.1 后端

| 文件 | 性质 | 内容 |
|---|---|---|
| `common/feature_flag.go` | 新增 | 13 个开关 key 常量（字面量 = env 名，稳定契约）；`atomic.Pointer[map[string]string]` 发布**不可变快照**；`FeatureFlagValue/String`、`SetFeatureFlagOverride`、`ReplaceFeatureFlagOverrides`、`BoolFeatureFlagOverride`；4 个「值由 common/constant 持有」的开关 getter |
| `setting/feature_switch/feature_switch.go` | 新增 | 13 开关**元数据注册表**（标题/描述键、风险、依赖、指标、回滚提示、是否可编辑）+ 经 `setting/config` 持久化（选项 `feature_switch.values`）+ `IsEnabled/Set/Reset/List/SerializedValues/CurrentValue` |
| `setting/feature_switch/errors.go` | 新增 | 可被管理端渲染的拒绝原因：未知开关 / 不可编辑 / 非法取值（带合法集合）/ 依赖未满足（带缺失项） |
| `controller/feature_switch.go` | 新增 | `GET/PUT /api/option/feature-switches`；高风险启用**前置动作**（密钥加密迁移 / 登录密钥初始化）失败即回滚并补偿落库；`feature_switch.update` 审计 |
| `router/api-router.go` | 改 | 两条路由挂在既有 `optionRoute`（已由 `middleware.RootAuth()` 保护） |
| `controller/audit.go` | 改 | 新增审计模板 `feature_switch.update`（含 key / 前后值 / 风险） |
| **13 个能力模块** | 改 | 删掉**共享可变包级 bool**（管理端写 + 热路径读 = 无同步并发读写，属未定义行为），改为 getter 查原子快照 |

被改造的 13 个开关：`COMPLEXITY_ROUTING`、`TOOL_DRAWER_ENABLED`、`RELAY_AUDIT_ENABLED`、
`POLICY_ENGINE_MODE`、`CHANNEL_HEALTH_WEIGHTED_LB`、`DOMAIN_ROUTE_ENABLED`、
`MEMORY_INJECTION_ENABLED`、`CHANNEL_CIRCUIT_BREAKER`、`CHANNEL_KEY_ENCRYPTION`、
`PASSWORD_LOGIN_ENCRYPTION_ENABLED`、`CATALOG_SYNC_TASK_ENABLED`、`ERROR_LOG_ENABLED`、
`GET_MEDIA_TOKEN_NOT_STREAM`。

对应改动文件：`service/{complexity_router,tool_drawer,relay_audit,policy_engine,memory_injection,channel_circuit_breaker,domain_route}.go`、
`model/{channel_constraint,channel,channel_key_migration}.go`、`controller/{system_task_handlers,user,misc,relay}.go`、
`main.go`、`service/{security_verification,token_counter}.go`。

### 1.2 前端

| 文件 | 性质 |
|---|---|
| `features/system-settings/feature-switches/{types.ts,api.ts,index.tsx,section-registry.tsx}` | 新增分区（复用既有 `SettingsPage` 骨架与 section-registry 范式） |
| `features/system-settings/feature-switches/components/feature-switch-list.tsx` | 新增列表：开关名/当前值/默认值/风险徽章/依赖/需否重启/回滚提示 + **效果度量** + 高风险二次确认 |
| `routes/_authenticated/system-settings/feature-switches/{index.tsx,$section.tsx}` | 新增路由（`routeTree.gen.ts` 由构建再生成） |
| `components/layout/config/system-settings.config.ts` | 侧边栏新增「实验功能」入口 |
| `i18n/locales/*.json` × 7 | **+61 键/语言**，纯文本行插入（未 JSON 再序列化） |

### 1.3 复用而非另造

- 持久化/热更新：复用 `setting/config` 的 `configWriteHook` —— 该钩子在**启动装载**
  （`InitOptionMap → loadOptionsFromDatabase → handleConfigUpdate`）与**管理端保存**
  两条路径上都会触发，因此「重启生效」与「立即热更新」共用一条链路，无需第二套机制。
- UI：复用 `SettingsPage` / `section-registry` / `RiskAcknowledgementDialog` / `Switch` /
  `Card` / `StatusBadge` 风格，未新建骨架。

---

## 2. Batch-8 — 验收证据（全部实际运行）

### 2.1 后端门禁

```
go build ./...                                            → exit 0
go vet ./setting/feature_switch/ ./controller/ ./common/   → exit 0
gofmt -l（本批改动文件）                                    → 无输出（干净）
```

### 2.2 单元测试

```
go test ./setting/feature_switch/ -count=1        → ok（12 个用例全 PASS）
go test ./controller/ -run FeatureSwitch|GetFeatureSwitches|UpdateFeatureSwitch -count=1 → ok（4 个用例）
go test ./service/ -count=1                        → ok 30.9s
go test ./model/ -count=1                          → ok 81.6s
go test ./common/ ./oauth/ ./relay/ -count=1       → 全 ok
```

**反向验证（RED，指南 §3.3 强制）**：把 `common.FeatureFlagValue` 的「覆盖优先」分支短路
（`if true || !ok { return fallback }`）后：

```
--- FAIL: TestIsEnabledPrefersPersistedValueOverEnvDefault
    Messages:   管理员配置必须优先于 env 默认值
--- FAIL: TestSerializedValuesRoundTrips
    Messages:   经持久化格式往返后仍必须生效
```

还原后两用例 PASS。→ 证明用例真的锁住了那条逻辑，而不是摆设。

### 2.3 全量后端回归（vs 指南 §14.1 既有噪声表）

```
go test ./... -count=1
失败包：controller / e2e / relay/channel / relay/channel/task/jsplugin
失败用例：TestAuditDatabaseMatrix、TestResetPassword*、TestSendEmailVerificationAntiEnumeration、
         TestKlingNativeRoute*、TestSessionLimitDoesNotRecordRejectedLoginAsSuccessful、
         TestGetSiteOverviewAggregatesConsumeLogs、TestSiteSubscriptionStatsAggregates、
         TestAdminSetUserSubscriptionTierInvalidatesCache、TestReadyzRedisDownFailSoft、
         TestTaskAdaptor*、TestUpstreamGetBody_HTTP2*、TestDocumentPluginRunsGenericBatchArtifactChain、
         TestSecurityAccountDeletionConcurrentRequestsHaveOneWinner
```

- 除最后一条外，**全部逐条命中指南 §14.1 的既有噪声表**（顺序依赖 / 共享 `model.DB` 污染 / 环境）。
- 最后一条经隔离验证为既有噪声：
  `go test ./controller/ -run TestSecurityAccountDeletionConcurrentRequestsHaveOneWinner -count=1` → **ok 18.5s**。
- **本批改动的包 `common` / `setting` / `model` 在全量运行中全绿。**

### 2.4 前端门禁

```
cd web && bun run typecheck      → exit 0
cd web && bun run build          → exit 0（并再生成 routeTree.gen.ts，含新路由 26 处）
cd web && bunx oxlint -c .oxlintrc.json src/features/system-settings src/components/layout/config src/routes
                                 → 本批新增/改动文件 0 error / 0 warning
cd web && node scripts/check-missing-i18n.cjs → "i18n missing-key check: OK (0 missing)"
7 个 locale JSON 全部 JSON.parse 通过；受保护的 \uXXXX 转义键 projectAttributionSuffix **FOUND**（未被破坏）
```

> 说明：`bun run lint` 整体 **exit 1**，但其全部报错来自 `web/e2e-local/*.cjs`
> —— 历史会话遗留的**未跟踪**临时脚本（46 个未跟踪文件，指南 §14.5 列为待清理候选）。
> 本批文件不在报错清单内。

### 2.5 真实浏览器 E2E（本地全栈，指南 §3.2.2 要求的走查）

脚本：`计划书/e2e-evidence/batch-8/g1-feature-switch-browser-e2e.cjs`
证据：`RESULTS.md` + `01-feature-switches-page.png` + `02-relay-audit-enabled.png`

**结果：20/20 PASS，且连续两次运行均 20/20（用例幂等）**。覆盖：

- POST /api/setup 初始化 → 浏览器上下文内登录 → 直达新页（未被踢回登录）
- **13 个开关全部渲染**、效果度量区块、风险徽章、回滚提示、**中文 i18n 生效**
- 切换低风险开关：初始关 → 点击后开 → **度量出现数字** → 刷新后仍开且显示「已在管理端配置」
- 「重置为默认」按钮在 `configured=false` 时**不渲染**、`configured=true` 时渲染 → 点击后回到「使用环境默认值」
- 全程 0 未捕获页面错误

### 2.6 跨进程重启持久化（接口层真凭据）

脚本：`计划书/e2e-evidence/batch-8/persistence-restart-e2e.cjs`（自带起停服务进程）
证据：`RESULTS-persistence-restart.md`

**结果：10/10 PASS**。关键序列：

```
enable  → configured=true, effective=true
★ 重启进程
        → 仍 configured=true, effective=true   ← 这才是"真的落库"
reset   → configured=false
★ 重启进程
        → 仍 configured=false（回退 env 默认）
```

> 为什么必须单独做：浏览器刷新只证明**内存态**还在（服务进程没重启）。

### 2.7 生产部署与生产验收（**已上线**）

- 蓝绿零停机上线 `v1.3.124`（`[deploy] 完成：new-api:v1.3.124 在端口 3000`）；
  容器 healthy、`healthz/readyz=200`、Caddy 上游已切 3000、
  经公网域名 + 真实 TLS 访问 **200** 且版本头 `v1.3.124`、新接口无凭据 **401**。
- **生产真实浏览器验收 20/20 PASS**（`e2e-evidence/batch-8/prod-*.png`、`prod-RESULTS.md`、
  `prod-acceptance.md`）。最有分量的一条：开关打开期间，**线上真实流量让
  `relay_audit_findings_total` 从 4 → 11** —— 证明开关在生产链路里真的生效，不是 UI 假象。
- 验收结束已通过 UI「重置为默认」把开关复位，**生产行为与部署前一致**。
- **压测**（经生产实例）：新增接口 `/api/option/feature-switches` 并发 30×5 **150/150 全 200，
  p95 293ms**；并发 60×4 **0 个 5xx**。压测后容器 **mem 126.1 MiB / 900 MiB、restarts 0**。
  `/v1/pricing` 的 429 是既有限流器（60/min/IP）在单 IP 高频下的正常工作。
- 发布：tag `v1.3.124` + GitHub Release
  <https://github.com/lza6/new-api-Max/releases/tag/v1.3.124>（`gh release create` 缺
  `workflow` scope，改用 `gh api … /releases` 创建成功）。

---

## 3. Batch-9 / G2 — 实际改动与结论

| 项 | 指南要求 | 本轮处置 |
|---|---|---|
| 4.2.3 无界 `io.ReadAll` → `LimitReader` | 9 处 | ✅ **已做，且做了 15 处**（侦察发现不止 9 处） |
| 4.2.4 出站 `http.Client` 补 `Timeout` | 2 处 | ✅ 已做（2/2） |
| 4.2.1 压缩参数引导 | 改注释 + 管理端提示 + 压测对比 | ⏭ **未做**（纯配置引导，收益需真实压测数据支撑；见 §5 未闭环披露） |
| 4.2.2 首包 commit 参数化 | 可选项，取证不足则**弃做** | ⏭ **弃做**（指南允许；未取得「自耗 24% 主要来自 commit 策略」的证据） |

### 3.1 新增统一收口

`common/read_limit.go`（新增）：`ReadAllLimited(r, maxBytes)` + 4 个上下文常量
（上游任务响应 8 MB / OAuth 64 KB / 上游错误体 1 MB / models 元数据 8 MB）。
语义：`maxBytes+1` 探界 → 刚好等于上限**通过**，超出**报错**且**不返回部分数据**。

### 3.2 实际接线的 15 处

`service/`：`codex_models.go`、`codex_wham_usage.go`×3、`error.go`、`midjourney.go`、
`task_polling.go`×2、`task_unconfirmed_resolution.go`
`controller/`：`channel-billing.go`、`channel_upstream_update.go`、`midjourney.go`、`topup_creem.go`
`oauth/`：`generic.go`×2、`github.go`
`relay/`：`relay_task.go`×2

> `relay/` 下仍有约 50 处渠道 handler 的裸 `io.ReadAll`（指南未列，本轮未做）——
> **未闭环，见 §5**。收口点是 `relay/channel/api_request.go`，那是下一批的事。

### 3.3 出站客户端整体超时

- `controller/model_sync.go`：新增 `syncHTTPTotalTimeout = 120s`，`newHTTPClient()` 补 `Timeout`。
- `controller/ratio_sync.go`：抽出 `newRatioSyncHTTPClient()`（**行为等价**，仅移动代码），
  新增 `ratioSyncHTTPTotalTimeout = 60s` 并补 `Timeout`。
- **刻意保留自定义 `Transport`**：两者都为 `github.io` 做 IPv4 优先的 `DialContext`，
  换成共享池会丢掉该行为（指南 §4.2.4 亦允许按既有实现判断）。
  理由已写进代码注释，满足验收项「剩余 `&http.Client{` 命中均有明确理由」。

> 为什么需要整体 `Timeout`：`Dialer` / `TLSHandshake` / `ResponseHeader` 三个超时
> **都不覆盖「读完 body」**——上游慢速吐字节时请求会永久挂住（goroutine + 连接泄漏）。

### 3.4 测试

```
go test ./common/ -run "TestReadAllLimited|TestNormalizeHTTPMethodLabel|TestMetricsIncCapsSeriesCount" -count=1 -v
  → TestReadAllLimited（5 子用例）PASS / TestNormalizeHTTPMethodLabel PASS / TestMetricsIncCapsSeriesCount PASS
go test ./controller/ -run TestOutboundSyncClientsHaveTotalTimeout -count=1 -v → PASS
```

**RED 反向验证**：短路 `MetricsInc` 的序列上限判断后：
```
Error: "6000" is not less than or equal to "2000"      ← 单个指标的序列数必须有硬上界
Error: "0" is not positive                             ← 溢出计数器未生效
```
还原后 PASS。

---

## 4. G10（提前做的部分）— 无界增长点审计

用户质疑「很多地方靠运行内存兜底」。据此把指南 §12.2.1（原排 Batch-13）的审计提前，
对 `common/` `middleware/` `service/` `model/` `relay/` `controller/` 做了逐条排查。

### 4.1 找到并已修的**真实**无界点

| 位置 | 问题 | 处置 |
|---|---|---|
| `common/metrics.go` `MetricsInc` 的 `counters[name][labelKey]` | label **含 HTTP method**（`middleware/logger.go:55` 传 `param.Method`，直接来自请求行，任意 token 都算）→ 标签组合**无界且无任何淘汰** | ✅ 双层修复：① `NormalizeHTTPMethodLabel` 归一化到有限集合（未知→`OTHER`）；② `MetricsInc` 增加 `maxMetricLabelSeries=2000` 硬上界，超限样本计入 `<name>_label_series_overflow_total`（**不静默丢**） |

### 4.2 确认**有界且已验证**（抽样）

`common/rate-limit.go`（定时 `delete`）、`middleware/{user,token,subscription}-rate-limit`（释放即删）、
`service/live_request_tracker.go`（active ≤2000 + 最旧驱逐）、`service/user_memory.go`（cap 10000 即清空）、
`service/channel_cooldown.go`（周期清扫）、`service/web_protection_tracker.go` 的 `ipState`（10min 闲置删）、
`common/feature_flag.go`（键 ≤13，写入前 `Meta`+`normalize` 校验，未知键丢弃）。

### 4.3 已识别但**未处理**（诚实披露，见 §5）

- `service/tokenizer.go` `tokenEncoderMap`：键 = 模型名（`IsOpenAITextModel` 子串匹配 → 任意含 "gpt" 的名字），
  **无淘汰**，且值是**大对象 codec** → 内存放大明显。
- `service/web_protection_tracker.go` `banCache`：键 = **客户端 IP**，**无淘汰**。
- `service/url_guard.go` `ssrfCache`、`service/channel_affinity.go` 的正则缓存、
  `relay/channel/api_request.go` 的 header 正则缓存：配置驱动、**改配置后旧项永久残留**。
- `service/channel_health_score.go`、`service/channel_circuit_breaker.go`：键有界（渠道 id）但渠道删除后残留。
- 进程内存 / goroutine 数 / 后台 loop 上次执行时间的 gauge（指南 §12.2.2）**未加** →
  目前「跑三天会不会变慢」**仍不可观测**。

---

## 5. 未闭环 / 待验证（不假装完成）

| # | 项 | 状态 | 缺什么 |
|---|---|---|---|
| 1 | `fr / ru / ja / vi` 四个语言的 **61 个新键为英文占位值** | ⚠️ 键齐全、不缺回退，但**未真正翻译**（en/zh/zh-TW 已完整翻译） | 翻译 |
| 2 | `relay/` 下约 50 处渠道 handler 裸 `io.ReadAll` | ❌ 未做 | 下一批 |
| 3 | G2 §4.2.1 压缩参数调优（改前/改后延迟 + CPU 对比） | ❌ 未做 | 真实压测数据 |
| 4 | G10 §12.2.2 资源 gauge（内存/goroutine/loop 时间戳） | ❌ 未做 | 下一批 |
| 5 | G10 §4.3 列出的其余无界点 | ❌ 未做 | 下一批 |
| 6 | `common/metrics.go` gauges 无上限（但有全量重置） | ◐ 已评估为**可接受** | — |

---

## 6. 下一步建议（按 ROI）

1. **Batch-11 / G6**：`TaskArtifactStore.Persist` 接线。侦察结论：**产物的"唯一来源"并不存在**
   —— 全仓没有任何地方下载产物字节，轮询只把上游 URL 存进 `task.PrivateData.ResultURL`。
   所以这不是"调用一个现成函数"，而是**要新增一次产物抓取**（挂点：`service/task_polling.go`
   的 `task.StatusSuccess` 分支，与 `service/task_unconfirmed_resolution.go`）。
   顺带修一个真 bug：`controller/task.go:331` 用 `_ = artifactStore.Serve(...)` **吞掉错误后直接 return**
   → 文件缺失时返回 **200 空体**而不是回退上游代理。
2. **G10 剩余项 + §12.2.2 资源 gauge**（用户明确关切，收益是"可观测"而非"更快"）。
3. **Batch-9 剩余**：`relay/` 渠道 handler 的读取收口（在 `relay/channel/api_request.go` 统一做）。
4. 四个语言的翻译补齐。

---

# Batch-9 / G3 — 网关精确响应缓存（追加，v1.3.125）

## 交付物

| 文件 | 性质 |
|---|---|
| `service/response_cache.go` / `_redis.go` | 新增：带 TTL 的进程内 LRU（**硬容量上界**，清扫在写入路径分批做、**不常驻 goroutine**）+ 可选 Redis 后端 |
| `setting/response_cache_setting/` | 新增：TTL / 容量 / 模型白黑名单 / 跨用户共享 / 命中计费 / 后端类型 |
| `setting/feature_switch` | 第 **14** 个开关 `RESPONSE_CACHE_ENABLED`（管理端可见、可灰度、可回滚） |
| `relay/response_cache_hook.go` | 新增：**取数钩子** |
| `relay/compatible_handler.go` | 改：两处 `adaptor.DoRequest` 改为走钩子 |
| `relay/channel/openai/relay-openai.go` | 改：**写入钩子**（非流式 + 200 + usage 存在；命中时跳过重复写回） |
| `controller/{feature_switch,metrics}.go` | 改：把开关声明的 3 个指标**真正产出**（`/metrics` 与「实验功能」页同源） |

## 两个关键设计决定（都来自真实证据）

1. **归一化必须保留值的原始字节**：顶层解成 `map[string]json.RawMessage`，而不是 `map[string]any`。
   否则 `seed: 12345678901234567890` 这类大整数会被 float64 舍入，两个**不同的**请求
   可能算出同一个键 → **错命中**。刻意不递归排序嵌套键（那要求解成 any），
   代价是嵌套键序不同的等价请求不命中 —— 方向正确：**宁可漏命中，不可错命中**。

2. **钩子必须包在 `adaptor.DoRequest` 外层，伪造 200 响应喂给既有链路**。
   我最初把它放在 `controller/relay.go`（命中即自己回写 + 自己调 `PostTextConsumeQuota`）：
   **单测全绿、接口测试也看不出，真实 E2E 抓到它每次命中都 panic**（3 次命中 → 3 次 panic），
   因为 `PostTextConsumeQuota` 依赖 relay handler 逐层填充的 relayInfo 状态。
   而且那条路径**不会写消费日志** —— 管理员会看到「上游调用量下降但日志里什么都没有」。
   改成伪造响应后：**16/16 PASS 且 panic 归零**。

## 验收证据

```
go build ./... / go vet（relay service controller）        → 干净
go test ./service/ -run TestResponseCache|TestExtractUsage → 11 用例全 PASS
RED：把 userID 从缓存键去掉 → TestResponseCacheIsolatesUsersByDefault 如期 FAIL
真实 E2E（mock 上游，零付费调用）                          → 16/16 PASS，panic 0
  · 同一 prompt 连发两次 → 上游调用数 0→1→**停在 1**（第二次由缓存服务）
  · 两次响应体逐字节一致（usage 如实回填）
  · 换内容 → 上游被调用第 2 次（键确实随内容变化）
  · 指标 hits=1 / misses=2 / live_entries=2
go test ./service/ ./relay/ ./setting/... -count=1          → 全绿
go test ./controller/ -count=1                             → 失败项与基线既有噪声（§14.1）一致
```

新增**反伪闭环守卫用例** `TestFeatureSwitchDeclaredMetricsAreActuallyProduced`：
注册表里每个开关声明的 `MetricKeys` 必须真的被产出，否则用例 FAIL。
它当场抓出 `CHANNEL_HEALTH_WEIGHTED_LB` 的 `channel_health_score_avg` 在无渠道时缺失
（已修成恒定产出）。

## 未闭环（诚实披露）

1. v1 **不缓存流式**（指南范围外）。
2. 写入侧只接在 **OpenAI 适配器路径**（覆盖绝大多数 OpenAI 兼容渠道）；
   原生 Claude/Gemini 适配器的非流式路径**尚未接写入**，这些请求只会读、不会写。
3. `fr/ru/ja/vi` 的新增 3 个键仍是英文占位值（en/zh/zh-TW 已完整翻译）。
4. 响应缓存**参数**（TTL/容量/白名单）目前只能经选项 API 设置，**还没有专属管理端页面**
   （总开关在「实验功能」页可见可切换）。

## Batch-9 / G3 — 生产部署与生产验收（已完成）

- 蓝绿零停机上线 `v1.3.125`（`[deploy] 完成：new-api:v1.3.125 在端口 3001`）；
  容器 healthy、`healthz/readyz=200`、Caddy 上游已切 3001、经公网域名 + 真实 TLS **200**、
  新接口无凭据 **401**、内存 **29.66 MiB / 900 MiB**、重启 **0**。
- **生产真实浏览器验收 20/20 PASS**（现覆盖 **14** 个开关）：全部渲染、切换即时生效、
  刷新后仍显示「已在管理端配置」、重置回到环境默认；开关打开期间**线上真实流量**使
  `relay_audit_findings_total = 10`。验收后已复位。
- **`RESPONSE_CACHE_ENABLED` 默认关** → 生产行为与部署前**完全一致**，只是多了一个可打开的开关。
- **并发安全**：`go test ./service/ -run TestResponseCache -race` 全 12 用例 PASS，
  含 `TestResponseCacheIsConcurrencySafe`（16 goroutine × 200 次并发读写），**无 data race**。
- Release：<https://github.com/lza6/new-api-Max/releases/tag/v1.3.125>

---

# Batch-10 / G10 + G3 §5.2.2（追加）

## 一、G10 §12.2.2 —— 让「跑久会不会变慢」从靠猜变成可观测

新增 `process_*` / `db_*` 资源 gauge（`controller/metrics.go`，仅 /metrics 渲染时执行）：

| 指标 | 判读方式 |
|---|---|
| `process_goroutines` | 持续上升不回落 → 大概率 goroutine 泄漏（如未 stop 的 ticker） |
| `process_memory_alloc_bytes` / `_heap_bytes` / `_sys_bytes` / `_stack_bytes` | 绝对量与增长趋势 |
| `process_memory_heap_objects` | 持续上升且 alloc 也在涨 → 大概率有「只加不减」的 map/slice |
| `process_gc_cycles_total` / `process_gc_pause_total_seconds` | 增速上升 → GC 压力变大 |
| `db_open_connections` / `db_in_use_connections` / `db_idle_connections` / `db_max_open_connections` | 池子是否够用 |
| `db_wait_count_total` / `db_wait_duration_seconds_total` | 增长说明池偏小或被慢查询占满 |
| `background_loop_last_run_timestamp_seconds{loop=…}` | **某条曲线不再前进 = 那个 loop 卡死了** |

后台 loop 心跳（新增 `common/loop_heartbeat.go`）已接入 **6** 个主要常驻任务：
`sync_options`、`system_task_runner`、`subscription_quota_reset`、`task_artifact_cleanup`、
`codex_credential_refresh`、`consume_log_flusher`。
登记表的键是**编译期常量 + 128 条硬上界**（超出拒绝登记并告警）—— 这张表自己
绝不能成为新的无界增长点，上界是被强制的而非靠约定。

## 二、G10 §12.2.1 —— 修掉 4 处已确认的无界点

| 位置 | 问题 | 修复 |
|---|---|---|
| `service/web_protection_tracker.go` `banCache` | 键 = **客户端 IP**（外部输入驱动），**只读侧判 TTL、从不删除** | 过期**就地删除** + `webProtectionBanCacheMaxEntries = 8192` 硬上界（达界后不缓存新条目，只损失命中率） |
| `service/url_guard.go` `ssrfCache` | 键 = 上游 host，过期不删 | 过期**就地删除** + `ssrfCacheMaxEntries = 4096` 硬上界 |
| `service/tokenizer.go` `tokenEncoderMap` | 键 = **请求里的模型名**（子串匹配，任意含 "gpt" 的名字都会进表）；原注释「won't grow after initialization」**不成立** | `tokenEncoderMaxEntries = 512` 上界；达界后不缓存（codec 是共享词表对象，不缓存只是多一次查找） |
| `middleware/token-rate-limit.go` `tokenQBSBuckets` | token 删除后条目**永远残留**（只加不减） | 空闲 > 10min 的桶在写入路径分批淘汰（每 1024 次写入扫一次）；**因桶空闲足够久必已回填满，淘汰不改变限流语义** |

**未处理（诚实披露）**：3 处按正则串索引的 `sync.Map`（`channel_affinity` / `openai_chat_responses_mode` /
`relay/channel/api_request.go`）仍无配置变更失效钩子；`channelHealthTable` / `circuitTable`
在渠道删除后仍残留（键有界 = 渠道数，无界风险低但会残留）。二者均只随**配置变更**增长，
不随流量增长，风险等级低于上面 4 处。

## 三、G3 §5.2.2 —— 工具抽屉改为请求级 opt-in

- 新增请求头 `X-NewAPI-Tool-Drawer: off|dedupe|meta`，**优先级：请求头 > 全局开关**。
- `off` 能覆盖**已开启**的全局开关 → 依赖完整 schema 的保守客户端不再被全局开关伤害
  （这正是原注释自认的风险）。
- `meta` **本版未实现**：它需要宿主侧拦截「模型回调元函数要 schema」那一轮，是独立的一整块能力。
  声明 `meta` 时**安全降级到等价去重**（语义相同、只是省得更少）并记一次日志 —— 不静默假装做了。
- 新增收益度量 `tool_drawer_deduped_requests_total` / `tool_drawer_saved_bytes_total`，
  并**兑现**在「实验功能」页与 /metrics（只在真的移除了内容时才计数）。

## 四、验收证据

```
go build ./... / go vet（common model service middleware controller）  → 干净
common:    TestLoopHeartbeat*                     6 用例 PASS（含 16 goroutine 并发）
service:   TestSSRFCache* / TestResolveToolDrawerMode* / TestDedupToolDefs* / TestToolDrawerSavings*  PASS
middleware: TestTokenQBS*                         3 用例 PASS
controller: 反伪闭环守卫 TestFeatureSwitchDeclaredMetricsAreActuallyProduced PASS
真实 E2E（mock 上游，零付费调用）                  → 28/28 PASS
  · 不带请求头 → 上游收到 **3** 个工具（原样透传）
  · 带 dedupe   → 上游收到 **2** 个工具，且被移除的正是重复的 weather
  · 全局开关**开着**时带 off → 上游仍收到 **3** 个（请求头确实能覆盖全局）
  · /metrics 输出 process_* / db_* / 5 个 loop 心跳 / 工具抽屉收益（saved_bytes=296，非死指标）
```

**E2E 纠正的两处"我的断言写错"**（不是产品 bug）：指标真名是 `process_memory_heap_objects`；
`task_artifact_cleanup` 的心跳在默认配置下**本就应当缺席**（图床模式为 `upstream` 时该 loop 不启动）
—— 现已把这两条写进断言，使"缺席"也成为被验证的行为。
