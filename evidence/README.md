# evidence/ — 本轮验收证据

| 文件 | 内容 |
|---|---|
| `e2e-full-20261017.json` | 真实浏览器 E2E 7/7：登录、Key curl、MCP 付费审批、审批页、`/en/pricing` canonical/hreflang、无致命控制台错误 |
| `stress-relay-20261017.json` | `/v1/chat/completions` 10 并发压测（SQLite，50 并发版本见下） |
| `stress-relay-20261017-c50.json` | 50 并发压测：0 错误，QPS/P50/P95/P99 见文件 |
| `../计划书/e2e-evidence/v1.3.148/` | 截图与 `results.json` |
| `../计划书/e2e-evidence/v1.3.88/` | a11y/响应式 Playwright 7/7 截图与结果 |

> 压测环境：本机 SQLite + 本机 mock 上游（18080）+ Go 网关（18131）。
> SQLite 单写者会显著限制吞吐；生产 PostgreSQL 的结果需在部署后复测。
