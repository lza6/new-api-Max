# 首字延迟诊断 SOP（new-api 网关）

> 更新：2026-10-02 · 适用 v1.3.70+（延迟拆解已落地）
> 场景：用户反馈「首字很慢」「某模型延迟大」「并发时变慢」

---

## 0. 一句话结论（先记住）

**首字延迟 `frt = 上游 prefill 为主（80%+）`，不是上传带宽、不是网关 CPU。**
判断依据永远是「延迟拆解」的两个读数：

- `upstream_upload_ms` = 网关→上游上传耗时
- `upstream_ttfb_ms` = 上游首个响应头耗时（含 prefill）

**哪个大，瓶颈就在哪。** 别猜。

---

## 1. 看哪里（两个入口）

### A. 实时面板（进行中请求）
系统信息页 → 「实时请求详情」：
- 每行「首字」列带**分段条**（amber=上传 / blue=上游），hover 看明细
- 聚合卡「平均首字」附「上传 X · 上游 Y」
- 顶部：实时网络 MB/s、并发水位 active/limit/waiting

### B. 历史日志（任意请求）
使用日志 → 点开某条 → 「请求时间线」→「延迟拆解」区块（同款分段条）。
消费日志字段：`upstream_connect_ms` / `upstream_upload_ms` / `upstream_ttfb_ms`。

---

## 2. 诊断决策树

```
首字慢
├─ 面板/日志显示 upstream_ttfb_ms 占大头（>70%）
│   → 上游 prefill 慢。压缩无用。
│   → 动作：换更快上游 / 就近机房 / 减少 prompt 体积 / 检查上游是否过载
│
├─ upstream_upload_ms 占大头
│   → 上传慢。检查：
│     ① 渠道是否开 request_compression（大 body 压缩后 ~1%）
│     ② 出口带宽是否被打满（面板网络 out 持续接近 5Mbps 上限）
│     ③ body 是否过大（>5MB 的 prompt 上传本身就要时间）
│   → 动作：开压缩 / 减 prompt / 升带宽
│
├─ connect_ms 占大头
│   → 建连慢（DNS/TCP/TLS）。检查上游可达性、是否跨区。
│
└─ 分段都正常但 frt 大
    → 看 frt 与 upload+ttfb 的差（差 = 网关内排队/读体/计费）
    → 检查并发水位是否排队、是否有大 body 读体开销
```

---

## 3. 快速数据查询（服务器 SQL）

```sql
-- 按 body 桶看 frt + 延迟拆解（近 5 分钟）
SELECT CASE
    WHEN (other::json->>'request_bytes')::bigint < 262144 THEN 'a_lt256k'
    WHEN (other::json->>'request_bytes')::bigint < 1048576 THEN 'b_256k-1MB'
    ELSE 'c_gt1MB' END b,
  count(*) n,
  round(avg((other::json->>'frt')::bigint)/1000.0,1) avg_frt_s,
  round(avg((other::json->>'upstream_upload_ms')::bigint)) avg_upload_ms,
  round(avg((other::json->>'upstream_ttfb_ms')::bigint)) avg_ttfb_ms
FROM logs WHERE type=2 AND created_at > extract(epoch from now())-300
  AND other::json->>'upstream_ttfb_ms' IS NOT NULL
GROUP BY 1 ORDER BY 1;
```

```sql
-- 压缩是否在工作（近 5 分钟压缩次数）
-- 网关日志： docker logs new-api --since 5m | grep -c "request compression"
```

---

## 4. 上游 A/B（判定是不是上游自身慢）

从服务器**直连上游**（不经网关），测「到首个真实 token」：

```bash
K=<渠道 key>
# 注意：上游会发 ": heartbeat" 注释行，curl 的 time_starttransfer 会提前触发，
# 必须用 python 跳到第一个含 content/tool_call 的 delta 才算真实首 token。
# 简易版（够用）：看 total 时间随 body 增大是否线性增长 → 是则 prefill 主导。
for KB in 30 300 800; do
  # 生成 ~KB 的 prompt，POST 上游，记录 time_starttransfer / time_total
done
```

**经验值**（2026-10 实测，上游 LA）：30KB→3.1s、300KB→8.5s、800KB→21.8s（prefill 线性）。

---

## 5. 已知陷阱

1. **`: heartbeat` 注释行**：curl 的 ttfb 会被它提前触发，误判「上游很快」。真实首 token 要跳过空 delta。
2. **frt 与 ttfb 口径**：`frt` 从 StartTime（读完 body 后）算；`upstream_ttfb` 从请求发出算（含上传）。故 `ttfb` 可能 ≥ `frt - connect - upload` 不总成立。
3. **压缩不能救 prefill**：压缩只省上传字节。上游 prefill 占大头时，开压缩改善有限。
4. **大上传占满出口**：≥5MB 的 body 上传会占满 5Mbps 上行，拖慢**并发**小请求（面板网络 out 会顶满）。
5. **400 `input[N].call_id`**：多为上游问题（客户端空 tool_call id，pass_through 转发后上游自转 Responses 被拒）。直连上游可复现。

---

## 6. 一句话行动清单

- 瓶颈是 **上游 prefill** → 换上游/就近机房/减 prompt（压缩无用）
- 瓶颈是 **上传** → 开渠道压缩 + 查出口带宽 + 减 body
- 瓶颈是 **建连** → 查上游可达性/跨区
- 瓶颈是 **排队** → 看并发水位，调 global_concurrency 或上游容量
