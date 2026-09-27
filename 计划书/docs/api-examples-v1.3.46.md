# 调用示例 · v1.3.46 新增端点（供调用方/接入方直接使用）

> 位置：网关 `/v1`（公开，轻量限流）与 `/api`（管理端，Bearer session token）。
> 签名规则：HMAC-SHA256，请求头 `X-New-API-Webhook-Signature: sha256=<hex>`，覆盖完整请求体。

## 1. API Key 明文查看（step-up 全流程）

```bash
BASE=http://localhost:18100
TOKEN="<登录返回的 access_token>"

# ① 查询可用验证方式（本用户只有密码 → methods=[password]）
curl -H "Authorization: Bearer $TOKEN" "$BASE/api/verify/methods?scope=token.key.read"

# ② 用密码获取一次性 proof（上下文绑定 {token_id:1}，1 分钟内有效）
PROOF=$(curl -s -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"method":"password","scope":"token.key.read","context":{"token_id":1},"password":"<你的密码>"}' \
  "$BASE/api/verify" | jq -r '.data.proof_token')

# ③ 带证明取回明文 key；无证明/证明过期/上下文不匹配 → 403
curl -s -X POST -H "Authorization: Bearer $TOKEN" -H "X-Security-Proof: $PROOF" "$BASE/api/token/1/key"
# 批量：POST /api/token/batch/keys  body {"ids":[1,2,3]}，proof context={"token_ids":[1,2,3]}
```

## 2. 订阅站点统计（公开只读）

```bash
curl -s "$BASE/v1/stats/subscriptions"
# → {"data":{"total_plans":..,"total_subscriptions":..,"active_subscriptions":..,
#           "expiring_soon_7d":..,"new_last_30d":..,"by_plan":[{"plan_id":..,"title":..,"total":..,"active":..}]}}
```

## 3. 公开定价（复核既有）

```bash
curl -s "$BASE/v1/pricing"
```

## 4. Webhook 配置（管理员）

```bash
# GET/PUT /api/admin/webhook/settings（PUT body 示例）
curl -s -X PUT -H "Authorization: Bearer $ADMIN_TOKEN" -H "Content-Type: application/json" -d '{
  "enabled": true,
  "url": "https://your-receiver.example/hooks/new-api",
  "secret": "your-hmac-secret",
  "events": ["epay.topup.success","epay.subscription.success","task.settled"]
}' "$BASE/api/admin/webhook/settings"
```

## 5. 接收方签名校验（Go 示例）

```go
func verify(secret string, body []byte, sigHeader string) bool {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hmac.Equal([]byte(strings.TrimPrefix(sigHeader, "sha256=")), []byte(hex.EncodeToString(mac.Sum(nil))))
}
```

事件信封：`{"event_type":"epay.topup.success","event_id":"<tradeNo>","payload":{...},"timestamp":<unix>}`；
`task.settled` 的 `event_id` = 任务 TaskID，`payload.status` = success/failure。

## 6. 说明
- Webhook 默认关闭；启用保存时即校验 URL（http(s) + SSRF 拒绝私网/环回/云元数据）。
- 每个事件同一 event_id 60 秒内只通知一次（幂等窗口）；失败最多重试 3 次（1s/2s 退避）。
- 批量 key proof 上下文为 `{"token_ids":[...]}`（1..100 个，去重排序）；>100 请分批。
