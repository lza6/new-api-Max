# 审计验证账本（Audit Ledger）

> 目的：记录已执行的审计/测试及范围与结论，避免重复重跑；改动相关区域时优先引用。
> 追加规范：每次新增一行 [date] 范围/命令/结论/证据路径。

## P0-1 三库 conformance 契约测试套件（2026-09-21）

- 范围：SQLite + MySQL(9.6.0) + PostgreSQL(16.14) 三库真实实例；AutoMigrate 幂等、logs/task_events 索引、保留字列 group/key、布尔值、JSON(TEXT) 往返、lockForUpdate 真库锁、UsingMainDatabase/UsingLogDatabase 分支。
- 命令：`scripts/db-conformance.ps1`（本机服务模式）；`go test ./model/ -run TestDBConformance -v -count=1`。
- 结论：PASS=24，FAIL=0，SKIP=0（三库全过）；连续两次运行一致（可重复性验证）。
- 证据：`.deploy/refscan/` 无相关；本文件；测试文件 `model/db_conformance_test.go`。
- 数据库版本：PostgreSQL 16.14 (VC build 1944)；MySQL 9.6.0；SQLite（内置 glebarez）。
- 幂等：全模型 AutoMigrate 二次跑无 schema mutation（drop-before-migrate 保证库状态无关）。

## 加入资产（2026-09-21）
- `make db-check`：一条命令跑三库 conformance。
- CI：`.github/workflows/ci.yml` 新增 `db-conformance` job（services: mysql:8.4 + postgres:16）。
- 脚本：`scripts/db-conformance.ps1`（支持本机服务 与 -Docker 两模式）。