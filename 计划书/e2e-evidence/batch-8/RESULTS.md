# Batch-8 / G1 浏览器 E2E 结果

- 时间：2026-10-10T18:44:45.036Z
- 目标：https://freeapi.tingfengai.art
- 结果：**20/20 PASS**，失败 0

| # | 断言 | 结果 | 详情 |
|---|---|---|---|
| 1 | production mode: /api/setup skipped | ✅ | E2E_PROD=1 |
| 2 | login (UI form) | ✅ | https://freeapi.tingfengai.art/dashboard/overview |
| 3 | page not redirected to login | ✅ | https://freeapi.tingfengai.art/system-settings/feature-switches/switches |
| 4 | page shows section title | ✅ |  |
| 5 | all 13 switches rendered | ✅ | missing=[] |
| 6 | effect metrics block present | ✅ |  |
| 7 | risk badges present | ✅ |  |
| 8 | rollback hint present | ✅ |  |
| 9 | chinese i18n applied | ✅ |  |
| 10 | relay-audit switch is off initially | ✅ | on=false |
| 11 | reset button hidden while unconfigured | ✅ | count=0 |
| 12 | relay-audit switch turned on | ✅ | on=true |
| 13 | effect metric shows a number after enabling | ✅ | after="relay_audit_findings_total = 0" |
| 14 | server reports the switch as configured (reset button appeared) | ✅ | count=1 |
| 15 | switch persisted after reload (configured in admin console) | ✅ | 中继一致性自检 \| 低风险 \| 每次中继后做只读检查（SSE 事件白名单、用量单调性、上游错误泄漏、模型指纹），发现记录在日志条目上。 \| 当前值: true \| 环境默认值: false \| 已在管理端配置 \| 开关: RELAY_AUDIT_ENABLED \| 重置为默认 \| 效果度量 \| rel |
| 16 | switch still on after reload | ✅ |  |
| 17 | reset button present before reset | ✅ |  |
| 18 | reset returns the switch to the environment default | ✅ | 中继一致性自检 \| 低风险 \| 每次中继后做只读检查（SSE 事件白名单、用量单调性、上游错误泄漏、模型指纹），发现记录在日志条目上。 \| 当前值: false \| 环境默认值: false \| 使用环境默认值 \| 开关: RELAY_AUDIT_ENABLED \| 效果度量 \| relay_audit |
| 19 | switch is off again after reset | ✅ | on=false |
| 20 | no uncaught page errors | ✅ | [] |

截图：`01-feature-switches-page.png`（全页）、`02-relay-audit-enabled.png`（启用后）
