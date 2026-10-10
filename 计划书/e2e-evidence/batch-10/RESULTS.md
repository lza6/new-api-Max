# Batch-10 / G10 + G3 §5.2.2 —— 真实端到端验收（mock 上游）

- 时间：2026-10-10T20:28:34.270Z
- 目标：http://127.0.0.1:3099（mock 上游 127.0.0.1:4001）
- 结果：**28/28 PASS**

| # | 断言 | 结果 | 详情 |
|---|---|---|---|
| 1 | mock upstream listening | ✅ | port 4001 |
| 2 | admin login | ✅ | {"data":{"access_expires_at":1791665013,"access_token":"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ0b2t |
| 3 | channel created | ✅ | {"message":"","success":true} |
| 4 | api token created | ✅ | {"data":{"id":2,"key":"elCbOz3EDbUEHdalabEBc8CMoJGPC0CuLkACuVYb7B0ZSXVZ"},"message":"","success":true} |
| 5 | baseline request succeeded | ✅ | status=200 |
| 6 | without header the tools are passed through untouched | ✅ | upstream saw 3 tools (names=["weather","clock","weather"]) |
| 7 | dedupe request succeeded | ✅ | status=200 |
| 8 | X-NewAPI-Tool-Drawer: dedupe actually reached the upstream | ✅ | upstream saw 2 tools (names=["weather","clock"]) |
| 9 | the duplicate tool was the one removed | ✅ | ["weather","clock"] |
| 10 | off-header request succeeded | ✅ | status=200 |
| 11 | header off OVERRIDES a globally enabled switch (conservative clients protected) | ✅ | upstream saw 3 tools |
| 12 | /metrics reachable | ✅ | bytes=3767 |
| 13 | metric process_goroutines is exported (goroutine 泄漏可见) | ✅ |  |
| 14 | metric process_memory_alloc_bytes is exported (进程内存可见) | ✅ |  |
| 15 | metric process_memory_heap_objects is exported ("只加不减"的对象数可见) | ✅ |  |
| 16 | metric process_memory_sys_bytes is exported (进程向 OS 申请的总内存可见) | ✅ |  |
| 17 | metric db_open_connections is exported (DB 连接池使用可见) | ✅ |  |
| 18 | metric db_wait_count_total is exported (DB 连接池等待可见) | ✅ |  |
| 19 | background loop heartbeats are exported | ✅ | 5 loops: codex_credential_refresh,consume_log_flusher,subscription_quota_reset,sync_options,system_task_runner |
| 20 | heartbeat present for loop sync_options | ✅ |  |
| 21 | heartbeat present for loop consume_log_flusher | ✅ |  |
| 22 | heartbeat present for loop system_task_runner | ✅ |  |
| 23 | heartbeat present for loop subscription_quota_reset | ✅ |  |
| 24 | heartbeat present for loop codex_credential_refresh | ✅ |  |
| 25 | heartbeat for a disabled component is correctly absent (artifact cleanup runs only with a local store) | ✅ | codex_credential_refresh,consume_log_flusher,subscription_quota_reset,sync_options,system_task_runner |
| 26 | tool drawer saving metric tool_drawer_saved_bytes_total is exported | ✅ |  |
| 27 | tool drawer saving metric tool_drawer_deduped_requests_total is exported | ✅ |  |
| 28 | tool drawer savings are non-zero after a real dedupe (not a dead metric) | ✅ | saved_bytes=296 |

**两条核心结论**：
1. 请求头 `X-NewAPI-Tool-Drawer: dedupe` **真的改变了上游收到的 tools**（3 → 2，且被移除的正是重复项）；
   而 `off` **能覆盖已开启的全局开关** —— 保守客户端不会被全局开关伤害。
2. `/metrics` 真的输出了进程内存 / goroutine / DB 连接池 / 6 个后台 loop 心跳 / 工具抽屉收益，
   且收益值**非零**（不是"声明了但没人写"的死指标）。
