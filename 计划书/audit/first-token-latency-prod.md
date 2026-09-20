# 线上首字延迟根因实证（2026-09-20）

> 结论：首字慢的主因在**上游**（yunshuzhilian.asia 自身慢 + 超大上下文 prefill），**网关开销 ≈ 0~300ms**。

## 一、线上真实数据（生产 PG / API）
- 最近 60 条 consume 日志：`frt`(首字) **p50=26.9s / p95=52s / max=80s**；`use_time`(总) p50=34s
- 每请求 token 量：**25万~51万**（deepseek-v4-flash，agent/批量超大上下文负载）
- 模型 deepseek-v4-flash **只有 1 个活跃渠道**（channel 20 yunshuzhilian.asia，status=1）；grok/futureppo 渠道为禁用态 → 无备份/负载均衡
- perf-metrics：avg_latency_ms=44160、success_rate≈98%、avg_tps≈64

## 二、成对基准（服务器端同网络，网关 vs 直连上游，curl time_starttransfer）
| 场景 | 网关首字节(median) | 直连首字节(median) | 网关附加 |
|---|---|---|---|
| 小 prompt("hi", stream) 5 次 | ~2.08s | ~2.68s | **≈0~0.3s** |
| 中 prompt(300 词) 3 次 | ~2.90s | ~2.76s | ≈0（2/3 网关更快） |
- 直连上游连 "hi" 都要 **1.8~4.4s** 首字节 → 上游本身慢
- 生产 25-51 万 token 上下文 → 上游 prefill 长达 20-80s 才出首字（模型+负载固有时延，网关无法消减）

## 三、为何“看起来不对”
- 网关侧：RequestId/鉴权/渠道选择/预扣都在请求头之前完成，实测附加 ≤300ms（此前 v1.2.34 已压到 ~19ms 顺序开销）
- 本地 e2e 的 2-22s 波动与本次线上 26-80s 同源：**上游供应商超卖/排队 + 超大 prefill**

## 四、可落地建议（非网关代码 bug）
1. **为 deepseek-v4-flash 增加第二/三活跃渠道**（启用 channel 4/7 或新增），开启基于健康分的优先级路由与负载分流（现有 health_score + cooldown 机制可复用）
2. 若 yunshuzhilian.asia 长期超卖，评估更换/增加更快的 deepseek 供应商
3. agent/批量超大上下文（30 万+ token）建议拆分或换用小模型，降低 prefill 硬成本
4. RELAY_TIMEOUT=300 已配置（慢请求不再无限占连接）
5. 监控：/api/perf-metrics 已有 TTFT 曲线（avg_latency_ms/success_rate），可据此跟踪改善
