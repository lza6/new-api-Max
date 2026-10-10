# Batch-9 / G3 响应缓存 —— 真实端到端验收（mock 上游）

- 时间：2026-10-10T18:16:13.064Z
- 目标：http://127.0.0.1:3099（mock 上游 127.0.0.1:3999）
- 结果：**16/16 PASS**

| # | 断言 | 结果 | 详情 |
|---|---|---|---|
| 1 | mock upstream listening | ✅ | port 3999 |
| 2 | admin login | ✅ | {"data":{"access_expires_at":1791657072,"access_token":"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ0b2tlbl91c2UiOiJhY2Nlc3M |
| 3 | self-use mode enabled | ✅ | {"message":"","success":true} |
| 4 | local test instance started with SSRF_GUARD_DISABLED=1 (required for mock upstream) | ✅ |  |
| 5 | channel created | ✅ | {"message":"","success":true} |
| 6 | api token created | ✅ | {"data":{"id":1,"key":"v7HWdYJmVLS0GKIn3BM2Hdt3QJzT4LI5kw7iWcgfCTD7O3V5"},"message":"","success":true} |
| 7 | RESPONSE_CACHE_ENABLED turned on | ✅ | {"data":{"metrics":{"channel_circuit_open_total":0,"channel_health_score_avg":0,"channels_tracked":1,"policy_decision_total":0,"policy_engin |
| 8 | allowlist configured via option API | ✅ | {"message":"","success":true} |
| 9 | first call succeeded (cache miss) | ✅ | status=200 body={"id":"chatcmpl-mock-1","object":"chat.completion","created":1700000000,"model":"gpt-cache-e2e","choices":[{"index":0,"m |
| 10 | cache miss hit the upstream exactly once | ✅ | upstream calls: 0 -> 1 |
| 11 | second call succeeded (cache hit) | ✅ | status=200 |
| 12 | SECOND CALL DID NOT REACH THE UPSTREAM | ✅ | upstream calls stayed at 1 |
| 13 | both responses are byte-identical (usage truthfully replayed) | ✅ | len1=267 len2=267 |
| 14 | different prompt is NOT served from cache | ✅ | upstream calls: 2 |
| 15 | different prompt got a different response | ✅ |  |
| 16 | metrics report at least one cache hit | ✅ | hits=1 misses=2 live=2 |

**核心结论**：同一 prompt 连发两次，**上游只被调用了一次**，且两次响应体逐字节一致。
这证明缓存真的省掉了整次上游调用，而不是"函数能跑"。
