# workflow_status.md — 训练数据采集（多账号归属安全）终局闭环

> 最后更新：2026-10-19 06:00（Asia/Shanghai）
> 当前生产：`new-api:v1.3.154`（`freeapi.tingfengai.art` / 154.37.208.171，容器 healthy，上游 3001）

## 1. 任务契约（本轮）

| 编号 | 任务 | 交付物 | 验收标准 | 状态 |
|---|---|---|---|---|
| T1 | 多账号归属判定 fail-closed | `sink_multi.resolve` | **任一**账号列出失败即拒绝写入；B1/N4 用例通过 | done |
| T2 | 读侧兜底不翻转写归属 | `ReadFile` + `ErrFileNotFound` | 瞬时错误原样上抛；兜底成功也不改 `dirIdx`；N2 用例通过 | done |
| T3 | 删除真实落到所有账号 | `DeleteFile` 404→哨兵 + 逐账号删除 | 重复副本全部删除；瞬时错误必须上报；N3 用例通过 | done |
| T4 | 重复目录取最大 part 序号 | `resolve` + 严格 `maxPartSeq` | N4 用例 + 边界表通过；ERROR 日志 | done |
| T5 | Stat/列表暴露部分失败 | `Stat` + controller | 部分数据 + 错误 + `truncated=true`；N5 用例通过 | done |
| T6 | 失败信号有上限重试 | `worker.go` | 3 次上限；N1 用例通过 | done |
| T7 | 生产探针加固 | `scripts/prod-capture-e2e.sh` | `set -e`、分页、多账号/RootPath 继承、kind 断言、缓存失效、trap | done |
| T8 | 提交/发版/部署 | tag v1.3.154 | 干净构建 + 测试；生产 healthy | done |
| T9 | 生产真实 E2E（双账号） | 探针输出 | 用户 h1→账号#2、h4→账号#1；分片可解压、kind 合法 | done |
| T10 | 测试数据清理 | DB + 云盘 | users/tokens/consents/logs 0；两账号目录 0 | done |
| R1 | 独立审查（6 维） | 审查报告 | 发现 P1×2、P2×6、P3×7；已逐条处理 | done |
| R2 | 前后端衔接审计 | 缺口清单 | 无 P0/P1；P2×6、P3×10；已修 6 项 | done |
| R3 | 文档/运维一致性审计 | 缺口清单 | P0×1、P1×5、P2×11；已修主要项 | done |
| R4 | 修复审查发现并复验 | commit 901c66490 | 全量包测试 + 前端测试 + 构建通过 | done |
| R5 | HTML 变更报告 + 测验 | `计划书/变更报告-终局审计-20261018.html` | 覆盖 v1.3.151/v1.3.154 + 24 题 | done |

## 2. 独立审查结果摘要

### 代码审查（6 维）
- **P1-1**：`remember()` 在读写兜底后改写写归属 → 已删除该机制；新增
  `TestMultiSinkReadFallbackDoesNotChangeWriteOwnership`。
- **P1-2**：重复目录 + 有账号不可读时仍选可读账号 → 改为**任一账号不可读即拒绝**；
  新增 `TestMultiSinkRefusesToGuessWhenAnyAccountUnreadable`。
- P2-1 删除只删一份 → 改为逐账号删除；P2-3 live test 断言过时 → 已更新；
  P2-4 探针 `grep -q` SIGPIPE / curl 中止 → 已修；P2-5 `truncated` → 已修；
  P2-6 OneDrive 404 映射缺单测 → 新增假 Graph 用例。
- **未修（如实）**：P2-2 重试信号回到队尾，极端情况下可能重传整段（重复而非丢失）；
  P3-2 worker 重试注释的退避数值仍不精确。两者都需要改 `worker.go` 的消费循环，
  而该文件同时被另一会话的租约功能占用，本轮未冒险改动；已记录为已知限制。

### 前后端衔接
- 无 P0/P1 断链；15 条路由与前端调用逐条对齐，有契约测试。
- 已修：授权状态 30s 轮询（P2-1）、云盘 `last_error` 在配额已知时不可见（P2-2）、
  列表截断标志丢失（P2-3）、每日历史空态+错误态并现（P2-4）、
  `retried` 未展示（P2-5）、声明升版后重试死循环（P2-6）、
  非分片文件也显示分析/删除（P3-1）、`resumable` 字段未使用（P3-4）、
  下载字节数统一格式（P3-3）。
- 未修（P3）：导出列表分页、下载记录 100 条上限提示、`date`/`max_bytes`/`ttl_seconds`
  等后端能力的 UI 入口、错误双重提示、后端原因串 i18n。均不影响核心链路。

### 文档/运维
- **P0 已修**：README 记录格式补 `kind`（消费方契约）。
- P1 已修：归属/删除语义、多实例租约现状、探针 RootPath 继承、部署前构建步骤、
  `NODE_TYPE` 双 master 现状、SOP 版本。
- 其他已修：`.env.example` 补 `TRAINING_*`；目录示例；METRICS 已开启；
  回滚路径统一；audit-ledger 的租约矛盾；README 语言链接闭合。
- 未修（P2/P3）：翻译版 README 仍滞后；滚动更新旧脚本未标废弃；
  健康检查仍用 `/api/status` 而非 `/readyz`；`controller/training_data_bootstrap.go`
  与 `worker.go` 中仍有过时的多实例注释（属另一会话文件/需同步改动）。

## 3. 验证日志（真实命令与结果）

| 时间 | 命令 | 结果 |
|---|---|---|
| 2026-10-18 | `go build ./service/training_capture/` | exit 0 |
| 2026-10-18 | `go test ./service/training_capture/ -count=1` | ok（76.8s） |
| 2026-10-18 | `go test -race ./service/training_capture/ -count=1` | 4 次中 3 次 ok；1 次全 PASS 后二进制非零（无 DATA RACE/FAIL），未复现 |
| 2026-10-18 | 干净 worktree（HEAD+patch）`go build ./...` + 包测试 | exit 0 / ok（51.3s） |
| 2026-10-19 | `go build ./...`（含工作区） | exit 0 |
| 2026-10-19 | `go test ./service/training_capture/ -count=1` | ok（80.5s） |
| 2026-10-19 | `go test ./controller/ -run 'TrainingData\|TrainingCapture'` | ok（32.9s） |
| 2026-10-19 | `bun run vitest run`（training-data + profile settings） | 2 files / 10 tests passed |
| 2026-10-19 | `bun run i18n:check` | OK（0 missing） |
| 2026-10-19 | `bun run build`（web） | 成功 |
| 2026-10-19 | `bash -n scripts/prod-capture-e2e.sh` | 通过 |
| 2026-10-19 | `deploy-zero-downtime.sh v1.3.154` | healthy；外网 200；版本头 v1.3.154 |
| 2026-10-19 | 生产 E2E h1（hash→1） | 账号 #2；136B；request kind=message |
| 2026-10-19 | 生产 E2E h4（hash→0） | 账号 #1；176B；request + assistant |
| 2026-10-19 | 清理回读 | DB 剩余 0；账号#1/#2 剩余目录 0 |
| 2026-10-19 | 生产二进制 /metrics | `cannot safely resolve` 命中；metrics 200，24 条 capture 指标 |

证据：`计划书/e2e-evidence/prod-capture-e2e-v13154-20261019.txt`

## 4. 已知噪声/边界

- 工作树若同时存在另一会话的未提交代码，整树构建可能被其未完成改动影响；
  判定本轮必须用 `git status` 区分文件归属。
- `-race` 有一次无法解释的「全 PASS 但二进制非零」；后续若复现，用 `go test -json` 抓
  `Action=fail`。
- 生产探针新用户 quota=0；补额度后必须 `DEL user:<id>`（已内置到脚本）。
- 探针无 assistant 行可能是模型只回 reasoning（正文为空），不一定是采集失败；
  h4 用例已证明 assistant 行可正常写入。

## 5. 阻塞项

- 无（本轮范围内）。
- 范围外/下一轮：P2-2 重试顺序与重复（需与租约改动一起做）；
  Claude 计费两处决策待用户；累计充值金额（需三库验证）；p95 压测；
  导出列表分页与下载记录上限提示等 P3 UI 项。
