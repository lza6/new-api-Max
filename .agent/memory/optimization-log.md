# optimization-log.md — 性能与体积记录

> 只记**实测数字**，不记估计值。没有命令输出的数字一律标 `未实测`。

## 前端体积 / 首屏

| 项 | 数字 | 条件 | 出处 |
|---|---|---|---|
| 语言包整包 | 175KB gzip | v1.3.167 期 | 旧指南 §15 |
| 语言包 common 子集 | 12KB gzip | 同上 | 同上 |
| LCP | 3980ms → 3448ms | 阻塞语言包 −160KB gzip 后 | 同上 |
| `--font-numeric`（JetBrains Mono Variable）增量 | **未实测** | 需 `bun run build` 前后对比 | T-C1 验收项 |

## 数据库

| 项 | 数字 | 条件 |
|---|---|---|
| 生产冷查询（大日志表） | 12.2s → 1.67s | 加覆盖索引 `idx_log_type_created_finance`（PG Index Only Scan） |
| 接口冷/热 | 2.99s / 0.002s | 同上 |
| 三库 conformance 耗时 | 103s | SQLite + MySQL + PG |

## 服务端

| 项 | 数字 | 条件 |
|---|---|---|
| 轻压测 | QPS 74.4，0 错误 | 20 并发 / 15s，服务器本机（非真实上游），v1.3.169 |
| 单进程全量前端测试 | **>25 分钟未完成** | 2026-10-23 本机，197 测试文件 |
| 分片后 | 待 T-A1 全量结果 | `web/scripts/test-shards.mjs` |

## 构建耗时（本机，可作回归基线）

| 命令 | 耗时 |
|---|---|
| `go build ./...` | 54.7s |
| `go build -o new-api-dump.exe .` | ~1 min |
| `bun run typecheck` | 4.9s |
| `scripts/gen-db-structure.mjs` | <1s（55 表 / 637 列） |
