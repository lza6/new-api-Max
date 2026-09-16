# 真实 E2E 验收报告（Batch-1/5/6 关键链路）

> 时间：2026-09-16 · 环境：本机网关（SQLite 测试库，:3000）+ 服务器公网 mock 上游（103.233.252.213:18080）
> 原则：不调真实付费 API（红线），用本地/服务器自建 OpenAI 兼容 mock 走完整网关链路。

## 1. 完整链路（真实请求 → 200）

```
本机网关 :3000 → 渠道校验/计费 → 公网 mock 上游 :18080
```

```bash
POST /v1/chat/completions
Authorization: Bearer <token>
{"model":"gpt-4o-mini","messages":[...],"stream":false}
→ HTTP 200
→ {"choices":[{"message":{"content":"Hello from server mock"}}],
   "usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}
```

## 2. B5-1 费用解释卡数据（消费日志 other.explain）

```
type=2 model=gpt-4o-mini quota=2 prompt=10 completion=5
  billing_source=wallet
  explain={"facts":[
    {"label":"prompt_tokens","value":10},
    {"label":"completion_tokens","value":5},
    {"label":"model_ratio","value":0.075},
    {"label":"completion_ratio","value":4},
    {"label":"group_ratio","value":1},
    {"label":"channels_considered","value":1}
  ],"inferences":[]}
```

## 3. B1-2 SSRF 防护真实生效

- 渠道 base_url=127.0.0.1:18080 → 请求被拦截
  `{"error":{"message":"upstream address rejected ...","code":"upstream_unavailable"}}`（HTTP 500）
- 渠道 base_url=103.233.252.213:18080（公网）→ 放行，请求成功
- 结论：私网/环回永不放行（设计如此），公网正常，符合 B1-2 验收

## 4. B1-3 密钥回显收敛

- 令牌列表 API 返回 `kHgo**********Y40s`（脱敏）
- 明文需专用接口 `POST /api/token/:id/key`（B6 安全验证）
- 渠道列表 `Omit("key")`，`key` 字段空

## 5. B6-2 错误归类

- SSRF 拦截错误 → `code:"upstream_unavailable"`（机器可读，前端可人话映射）

## 6. B4-1 渠道验真探测（真实 A-F 分级实证）

触发：`POST /api/channel/probe/{id}`（管理员），真实请求 → 公网 mock 上游。

**良渠道**（正确应答 + 流式多 chunk + usage 自洽 + 缓存）：
```
grade=A score=100/100
  model_identity: true 25   capability: true 20
  cache: true 20            consistency: true 15
  stream_integrity: true 10 billing_consistency: true 10
```

**劣渠道**（答非所问 + 非流式 + usage 打架 + 参数丢弃）：
```
grade=F score=25/100
  model_identity: false 0   capability: false 0  (n=2 → 1 choice)
  cache: false 10           consistency: true 15
  stream_integrity: false 0 (单 chunk)  billing_consistency: false 0 (15!=999)
```

结论：六维 100 分制 A-F 分级真实可用，良=A、劣=F，符合 B4-1 验收。

## 7. B6-2 五类错误→人话映射（真实 E2E）

后端归一化（真实上游错误 → 稳定 error.type）：
```
e-badkey    HTTP 401  type=key_invalid
e-overquota HTTP 403  type=insufficient_quota   ← 403 单独区分，不误判为坏 key
e-ratelimit HTTP 429  type=rate_limited
e-updown    HTTP 502  type=upstream_unavailable
e-content   HTTP 400  type=content_filtered     ← content_filter 变体归一
```

前端人话映射（friendly-error-mapping 同源 regex）：
```
e-badkey    → "The key is invalid or expired. Please rotate it on the channels page."
e-overquota → "Insufficient quota. Please top up or redeem a quota card."
e-ratelimit → "Too many requests. Please try again later."
e-updown    → "The upstream service is temporarily unavailable. Please try again later."
e-content   → "The content was blocked by a safety policy."
```
结果：5/5 类错误全部命中人话首行。关键修复：getFriendlyErrorMessage 读取
response.data.error.type 稳定枚举 + 403 超额单独归 insufficient_quota。

## 8. B6-2 浏览器真实 UI 验证（Playwright 实机截图×5）

环境：本机网关 + 全新 build 的前端（内含 B6-2 映射），Chrome headless 登录后
在 Playground 逐模型触发真实错误，sonner toast 首行即人话。

| 模型 | HTTP | error.type | toast 首行（截图） |
|------|------|-----------|--------------------|
| e-badkey | 401 | key_invalid | 密钥无效或已过期，请到渠道页轮换。 |
| e-overquota | 403 | insufficient_quota | 额度不足，请充值或兑换额度卡。 |
| e-ratelimit | 429 | rate_limited | 请求过于频繁，请稍后再试。 |
| e-updown | 502 | upstream_unavailable | 上游服务暂不可用，请稍后再试。 |
| e-content | 400 | content_filtered | 内容被安全策略拦截。 |

结果：**5/5 类错误浏览器 toast 首行均为翻译后人话**（截图见 `b6-2/*.png`）。
补充：正则表扩 `insufficient balance` / `safety|moderation` 变体，仅 message
无 error.type 时同样命中；zh-TW/fr/ja/ru/vi 补齐 5 条人话翻译。
