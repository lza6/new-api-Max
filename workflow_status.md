# workflow_status.md — 训练数据采集（多账号归属安全）终局闭环

> 最后更新：2026-10-19（Asia/Shanghai）
> 当前生产：`new-api:v1.3.151`（`freeapi.tingfengai.art` / 154.37.208.171，容器 healthy）

## 1. 任务契约（本轮）

| 编号 | 任务 | 交付物 | 验收标准 | 状态 |
|---|---|---|---|---|
| T1 | 多账号归属判定 fail-closed | `sink_multi.resolve` | 任一账号 List 失败且无人有数据时拒绝写入；B1 用例通过 | done |
| T2 | 读侧兜底不翻转写归属 | `ReadFile` + `ErrFileNotFound` | 瞬时错误原样上抛；N2 用例通过 | done |
| T3 | 删除真实落到文件所在账号 | `DeleteFile` 404→哨兵 | 非归属账号文件可删；瞬时错误必须报错；N3 用例通过 | done |
| T4 | 重复目录取最大 part 序号 | `resolve` + `maxPartSeq` | N4 用例通过；打 ERROR 日志 | done |
| T5 | Stat 暴露部分失败 | `Stat` + controller | 返回部分数据 + 错误；N5 用例通过 | done |
| T6 | 失败信号有上限回队 | `worker.go` | 3 次上限；N1 用例通过 | done |
| T7 | 生产探针加固 | `scripts/prod-capture-e2e.sh` | `set -e`、分页、多账号/RootPath、退出码、trap | done |
| T8 | 提交/发版/部署 | commit/tag/v1.3.151 | 干净 worktree 构建+测试通过；生产 healthy | done |
| T9 | 生产真实 E2E（双账号） | 探针输出 | 用户 A→账号#1、用户 B→账号#1、lb3→账号#2；分片可解压 | done |
| T10 | 测试数据清理 | DB + 云盘 | users/tokens/consents/logs 0；两账号目录 0 | done |
| R1 | 独立审查（6 维） | 审查报告 | 只读，不修改代码 | in_progress |
| R2 | 前后端衔接审计 | 缺口清单 | 每个 UI 动作对应真实端点与状态 | in_progress |
| R3 | 文档/运维一致性审计 | 缺口清单 | README/env/部署/探针一致 | in_progress |
| R4 | 修复审查发现并复验 | commit | P0/P1 关闭 | pending |
| R5 | HTML 变更报告 + 测验 | `计划书/变更报告-终局审计-20261018.html` | 覆盖 v1.3.151 + 测验题 | pending |

## 2. 验证日志（真实命令与结果）

| 时间 | 命令 | 结果 |
|---|---|---|
| 2026-10-18 | `go build ./service/training_capture/` | exit 0 |
| 2026-10-18 | `go test ./service/training_capture/ -count=1` | ok（76.8s） |
| 2026-10-18 | `go test -race ./service/training_capture/ -count=1` | 4 次中 3 次 ok；1 次全部用例 PASS 后二进制非零（无 DATA RACE/无 FAIL 用例），未复现，记为噪声 |
| 2026-10-18 | 干净 worktree（HEAD+patch）`go build ./...` | exit 0 |
| 2026-10-18 | 干净 worktree 包测试 | ok（51.3s） |
| 2026-10-18 | `bash -n scripts/prod-capture-e2e.sh` | 通过 |
| 2026-10-19 | 生产 `deploy-zero-downtime.sh v1.3.151` | healthy；外网 200；版本头 v1.3.151 |
| 2026-10-19 | 生产启动日志 | `multi-account sink active with 2 accounts` + `worker started` |
| 2026-10-19 | 生产 E2E：`capture_e2e_a_...` | 账号 #1；gzip 分片 1 行 request |
| 2026-10-19 | 生产 E2E：`capture_e2e_b_...` | 账号 #1；2 行（request + assistant） |
| 2026-10-19 | 生产 E2E：`capture_e2e_lb3_...`（hash→index 1） | 账号 #2；1 行 request |
| 2026-10-19 | 清理回读 | DB users 0；账号#1/#2 剩余 `capture_e2e_` 目录 0 |

## 3. 已知噪声/边界

- 工作树整树 `go build ./...` 会失败于另一会话未提交的
  `controller/metrics.go`（`SkippedLease` 未定义）；判定本轮必须用 HEAD+patch 干净 worktree。
- `-race` 有一次无法解释的「全 PASS 但二进制非零」；后续若复现，用
  `go test -json` 抓 `Action=fail`。
- 生产探针创建的测试用户默认不改额度；本轮新用户额度为 0，必须先补额度并
  **删除 Redis `user:<id>` 缓存**，否则网关仍读到旧值（实测踩到）。

## 4. 阻塞项

- 无（本轮范围内）。
- 范围外未闭环（历史遗留，非本轮）：多实例租约由另一会话实现中；
  Claude 计费两处决策待用户；累计充值金额（需三库验证）；p95 压测。
