# Spec-008 三库迁移验证证据

**日期**：2026-10-14
**代码版本**：`4fc813c9f`（含 T1.4 两张新表）
**执行环境**：生产机 `154.37.208.171` 上的 Docker（本机无 Docker/MySQL）

## 执行命令

```bash
docker run -d --name mysql-conformance --network new-api_new-api-network \
  -e MYSQL_ROOT_PASSWORD=conformance -e MYSQL_DATABASE=newapi_conformance_test \
  -p 3307:3306 mysql:5.7

docker exec postgres psql -U newapi -d postgres \
  -c "CREATE DATABASE \"newapi_conformance_test\";"

docker run --rm --network new-api_new-api-network -v /root/conf-src:/src -w /src \
  -e TEST_MYSQL_DSN="root:conformance@tcp(mysql-conformance:3306)/newapi_conformance_test?charset=utf8mb4&parseTime=true&loc=Local" \
  -e TEST_POSTGRES_DSN="postgres://newapi:...@postgres:5432/newapi_conformance_test?sslmode=disable" \
  golang:1.25 sh -c "go test ./model/ -run TestDBConformance -count=1"
```

## 结果

```
ok  github.com/lza6/new-api-Max/model  10.318s
```

逐用例（每个都跑 sqlite / mysql / postgres 三遍）：
- `TestDBConformanceAutoMigrateIdempotent` —— **PASS ×3**
  （含新增的 `TrainingDataConsent` / `TrainingDataWatermark`：
  首次建表有 schema 变更，**二次迁移零 ALTER/DROP/CREATE INDEX**）
- `TestDBConformanceLogsIndexes` —— PASS ×3
- `TestDBConformanceLogsTrafficIndexDropped` —— PASS ×3
- `TestDBConformanceUsageReport` —— PASS ×3
- `TestDBConformanceEventDeliveryDedup` —— PASS ×3
- `TestDBConformanceWebhookEndpointRoundTrip` —— PASS ×3

## 关键点

1. **MySQL 用的是 5.7**（宪法 IV 的最低支持版本），不是 8.0 ——
   这直接验证了「不用 `ON CONFLICT ... AS new`」这条决策的必要性。
2. 新增两张表**已登记进 `conformanceModels()`**：否则这个测试根本覆盖不到它们
   （初版就漏了，是一处真实的覆盖缺口，已补）。
3. PostgreSQL 首跑失败是**环境问题**（测试库不存在），非代码问题；
   建库后通过。
