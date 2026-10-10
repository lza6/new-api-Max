# Batch-8 / G1 跨进程重启持久化验证

- 时间：2026-10-10T14:25:43.406Z
- 目标：http://127.0.0.1:3099（二进制 C:/Users/ADMINI~1.DES/AppData/Local/Temp/newapi-b8.exe，工作目录 C:/Users/ADMINI~1.DES/AppData/Local/Temp/b8e2e）
- 结果：**10/10 PASS**

| # | 断言 | 结果 | 详情 |
|---|---|---|---|
| 1 | server started | ✅ | http://127.0.0.1:3099 |
| 2 | setup/login available | ✅ | {"message":"系统已经初始化完成","success":false} |
| 3 | login | ✅ | {"data":{"access_expires_at":1791643230,"access_token":"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ0b2t |
| 4 | enable switch via API | ✅ | {"data":{"metrics":{"channel_circuit_open_total":0,"channels_tracked":0,"policy_decision_total":0,"policy_engine_eval_total":0,"relay_audit_findings_total":0}," |
| 5 | switch on before restart | ✅ | {"key":"RELAY_AUDIT_ENABLED","kind":"bool","value":"true","env_default":"false","configured":true,"effective":true,"title_key":"Relay Consistency Audit","description_key":"Read-onl |
| 6 | server restarted | ✅ | http://127.0.0.1:3099 |
| 7 | switch STILL ON after process restart (persisted to DB) | ✅ | {"key":"RELAY_AUDIT_ENABLED","kind":"bool","value":"true","env_default":"false","configured":true,"effective":true,"title_key":"Relay Consistency Audit","description_key":"Read-onl |
| 8 | reset switch via API | ✅ | {"data":{"metrics":{"channel_circuit_open_total":0,"channels_tracked":0,"policy_decision_total":0,"policy_engine_eval_total":0,"relay_audit_findings_total":0}," |
| 9 | server restarted (2nd) | ✅ | http://127.0.0.1:3099 |
| 10 | reset ALSO persisted across restart (back to env default) | ✅ | {"key":"RELAY_AUDIT_ENABLED","kind":"bool","value":"false","env_default":"false","configured":false,"effective":false,"title_key":"Relay Consistency Audit","description_key":"Read- |
