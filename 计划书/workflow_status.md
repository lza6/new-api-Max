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
