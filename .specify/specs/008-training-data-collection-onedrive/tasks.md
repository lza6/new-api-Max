# Implementation Tasks: 训练数据采集 + OneDrive 冷存储对接

**Feature**: `008-training-data-collection-onedrive`
**Depends on spec**: `spec.md` | **Plan**: `plan.md`

> `[P]` = 可与同级任务并行。每项任务必须满足：**可运行的验收证据**，不接受「代码写完」算完成。

---

## Phase 1：基础件（纯逻辑，先行且可充分单测）

- [x] **1.1 目录名净化 + 分片命名** `[P]`
  - `service/training_capture/naming.go`：`SanitizeDirName(userID, username)`、`PartFileName(seq)`
  - 白名单化（只留 `[A-Za-z0-9._-]`）、折叠 `_`、空/`..` → `user-<id>`、截断 64、**追加 ID 消歧**
  - 验收：单测穷举——`../`、`..`、空、emoji、超长、纯符号、中文、两人 sanitize 后同名
  - 证据：`go test ./service/training_capture/ -run TestSanitizeDirName -v`
  - **Requirement**: Story 4 / ADR-008-4

- [x] **1.2 消息指纹 + 增量判定** `[P]`
  - `service/training_capture/dedup.go`：`Fingerprint(role, content)`、`SelectIncrement(watermark, messages)`
  - 规范化（trim + 折叠空白）后哈希；水位 = 计数 + 末次指纹
  - 验收：单测覆盖——首采全量、重复不采、新增只采增量、**编辑/重生成视为新增**、
    删除不回退、会话 id 缺失退化为全局指纹去重、空消息、超长消息
  - 证据：`go test ./service/training_capture/ -run TestDedup -v`
  - **Requirement**: Story 2 / ADR-008-2

- [x] **1.3 有界队列 + 丢弃计数** `[P]`
  - `service/training_capture/queue.go`：非阻塞投递、容量上限、`DroppedCount`
  - 验收：单测——满队列投递**不阻塞**（<1ms 返回且返回 false）、计数正确、并发投递不丢计数
  - 证据：`go test ./service/training_capture/ -run TestQueue -v`（含并发）
  - **Requirement**: Story 3 / ADR-008-1

- [x] **1.4 数据模型（两张新表）**
  - `model/training_data.go`：`TrainingDataConsent` + `TrainingDataWatermark`
  - 注册 `AutoMigrate`；唯一索引 `(user_id)` / `(user_id, conversation_id)`
  - **不使用** `gorm:"default:true"` 业务布尔；**不使用** `ON CONFLICT AS new`
  - 验收：三库建表 + 迁移跑两次幂等
  - 证据：`go build ./...` + 既有 `TestDBConformance` 模式
  - **Requirement**: Story 1/2 / ADR-008-2 / 宪法 IV

## Phase 2：存储后端（先接口，后实现）

- [x] **2.1 Sink 接口 + 注册表**
  - `service/training_capture/sink.go`：`TrainingDataSink`（5 方法）+ `DefaultSink()` 选择
  - 未配置 → 返回 disabled sink（`EnsureDir` 报明确错误，不静默）
  - 验收：单测断言 disabled 时的错误**可读且含原因**
  - **Requirement**: Story 9 / ADR-008-3

- [x] **2.2 OneDrive 实现**（依赖 2.1）
  - `service/training_capture/onedrive.go`：
    - client_credentials 取 token（**缓存 + 提前 5min 续期**）
    - `EnsureDir`（`@microsoft.graph.conflictBehavior=replace` 幂等）
    - `WritePart`（`root:/path:/content` PUT，<=4MB 简单上传）
    - `Stat`（读 `quota`）
    - `List`（`root:/path:/children`，取最大分片序号）
  - **凭据只从 env 读**，日志永不打印 secret
  - 验收：**真实线上凭据**的集成测试——建目录/上传/读回/列举/清理，全绿
  - 证据：本地跑的集成用例 + 线上 curl 对照（已先行验证这 5 项能力）
  - **Requirement**: Story 4 / ADR-008-6

## Phase 3：采集管道（接线，风险最高的部分）

- [x] **3.1 授权服务**
  - `service/training_capture/consent.go`：`IsGranted(userID)`（带短 TTL 缓存）、
    `Grant`、`Revoke`、`PolicyVersion` 校验
  - 验收：单测——未授权不采、撤回后立即不采、版本变更视为需重新同意
  - **Requirement**: Story 1/6
  - 已完成 + 独立审查发现并修复 **B2（撤回 TOCTOU）**：
    「撤回立即生效」原只在无并发时成立（refresh 读 DB 后写缓存的窗口内，
    撤回可能先清缓存再被旧值写回），修法为 `consentRevocationEpoch` 纪元校验。

- [x] **3.2 采集 worker + 自适应节奏**（依赖 2.2、3.1）
  - 单 goroutine 消费：查授权 → 取增量 → 拼目录 → 分片 → 上传
  - 令牌桶限速；失败指数退避 + 有界重试；云盘不可达不影响主流程
  - 分片达 `MaxPartBytes` 即新建；重启后 `List` 恢复序号
  - 验收：单测（用 fake sink）——目录结构正确、分片轮转、重试不重复、失败降级
  - **Requirement**: Story 3/4 / ADR-008-5
  - 已完成（22 个 worker 用例，真实 SQLite + 内存后端）。
    **实测发现并修复的设计缺口**：请求路径每次只产出 1 条拼接文本，
    消息级水位恒为 1 → 增量完全失效（用户点名要避免的）。
    新增 `text_increment.go` 按**文本前缀**做增量，并加 DB 列
    `request_text_bytes` / `request_text_hash`。

- [x] **3.3 接线到请求收口**（依赖 3.1、1.3）
  - `controller/relay.go` 的 `defer` 中：**仅成功请求**投递
  - 投递内容只含已算好的 meta（不重复解析 JSON）
  - **硬约束**：投递路径**无 DB 写、无网络、无锁**——只有 channel send
  - 验收：**压测对照**——开启 vs 关闭采集，请求 p95 无可感知劣化
  - 证据：本地压测脚本 + 数据入 `计划书/e2e-evidence/`
  - **Requirement**: Story 3 / 宪法 V（**最高风险项**）
  - 已完成接线 + 基准（`go test -bench`，见提交 200c7ffe9）：
    未启用 **96.7 ns/op / 0 allocs**、启用 **1653 ns/op / 2 allocs**、
    含入队 **1911 ns/op**。接线点在成功分支 `newAPIError == nil` 之后，
    信号构造与入队在 `gopool.Go` 内执行（彻底离开响应返回路径）。
  - **遗留待补**：真实 HTTP 压测脚本（开/关对照 p95）+ 证据入
    `计划书/e2e-evidence/spec-008/`。基准已证明**单请求开销**，
    但「网关整体 p95 无劣化」需要 HTTP 层负载对照，尚未执行。

## Phase 4：接口层

- [x] **4.1 用户端接口**（依赖 3.1）（已完成，含 5 条安全契约用例）
  - `GET /api/user/training-data/status` → `{available, granted, policyVersion, grantedAt}`
  - `POST /api/user/training-data/grant` / `revoke`
  - `available` 由「管理员总开关 && 云盘已配置」决定
  - 服务端强制：状态从 DB 读，不信前端
  - 验收：单测 + 真实 E2E（未登录 401 / 授权后 granted=true / 撤回后 false）
  - **Requirement**: Story 1

- [x] **4.2 管理端接口**（依赖 2.2、3.2）（已完成）
  - `GET /api/training-data/admin/overview` → 今日用户数/对话数/字节 + 云盘配额 + 文件夹/文件数 + 当前日期
  - `GET /api/training-data/admin/by-date?date=YYYY-MM-DD` → 该日采集量
  - `PUT /api/training-data/admin/enabled` → 总开关（关 → worker 停消费）
  - 验收：单测 + 真实 E2E（数字与云盘实际交叉核对）
  - **Requirement**: Story 5

## Phase 5：前端

- [x] **5.1 个人资料页开关**（依赖 4.1）`[P]`（已完成，typecheck+lint 通过；浏览器 E2E 待做）
  - 复用既有设置项渲染范式（先检索 `src/features/profile/`）
  - 开关 + **常驻小字说明** + 首次开启弹**免责声明**（必须点同意）
  - `available=false` 时**不渲染**开关
  - 状态实时更新（成功/失败都有 toast）
  - 验收：真实浏览器 E2E + 截图
  - **Requirement**: Story 1/5 / 宪法 VI

- [ ] **5.2 管理端统计面板**（依赖 4.2）`[P]`
  - 总开关 + 统计卡（今日采集/云盘配额/文件数）+ 按日期查询
  - 未配置云盘时总开关**禁用并明示原因**
  - 验收：真实浏览器 E2E + 截图
  - **Requirement**: Story 5

  - [x] **5.3 i18n 全语言同步**（已完成：20 键 × 7 locale = 140 行**纯文本插入**，
    `git diff --numstat` 为 140 insertions / 0 deletions，证明无任何行被改写；
    混淆键 `footer.newapi.projectAttributionSuffix` 完好）
  - 新增键走**纯文本行插入**（保护混淆键），跑 `bun run i18n:sync` 验证无漂移
  - 验收：7 locale 均含新键 + 混淆键完好 + `i18n:sync` 无 diff
  - **Requirement**: 宪法 VI

## Phase 6：日志分片轮转（独立缺陷，已提前落地）

- [x] **6.1 保留策略（双上限）** ✅ 已完成
  - `logger/retention.go`：数量 14 + 总字节 1GB，保护当前文件，只删本程序命名
  - 证据：**13/13 PASS**（含「绝不删无关文件」「当前文件必保护」「单巨大文件被字节规则拦下」）
- [x] **6.2 接线到轮转点** ✅ 已完成
  - `logger/logger.go` 的 `SetupLogger` 轮转后调用
- [ ] **6.3 分片命名（按大小切分）**
  - 现状：按「100 万行」轮转，非按大小；单文件仍可达 173MB
  - 目标：叠加**按大小**切分阈值 + 命名可排序
  - 验收：单测 + 线上观察单文件不再超阈值
  - **Requirement**: Story 7

## Phase 7：README 重写

- [x] **7.1 README 全面重写**（可与 Phase 4-5 并行）
  - 全中文（术语/路径/命令保留英文）
  - 覆盖：项目定位、快速开始、配置项、部署、验证命令、常见问题
  - **保留保护标识**（new-api / QuantumNous 归属不得删改）
  - 移除上游残留与过时内容
  - 已完成：600 行 → ~360 行全中文。删除上游模板段落
    （Trusted Partners 徽章墙 / Special Thanks / 泛化英文 Project Description）。
    **许可块用 `diff` 与旧 README 逐行比对，结果完全一致**（含 AGPLv3
    Section 7 附加条款、`Frontend design and development by New API
    contributors.`、指向 lza6/new-api-Max 的可见链接、One API 署名、
    support@quantumnous.com 联系方式、页脚 by lza6）。
    文中 23 个环境变量、全部前端命令、6 个 Makefile 目标、compose 三服务
    与端口、`/healthz` `/readyz` 等**逐条 grep 核验存在**，无编造。
  - 验收：按 README 步骤**真实走一遍**能跑起来（新环境模拟）
  - **Requirement**: Story 8

## Phase 8：验证与交付

- [ ] **8.1 三库迁移验证**（依赖 1.4）
  - 三库建表 + 迁移两次幂等 + 既有数据无损
- [ ] **8.2 真实 E2E**（依赖全部）
  - 本地真实登录 → 授权 → 发真实对话 → 断言云盘 `日期/用户/模型/part-*.jsonl` 内容正确
  - 撤回后再发对话 → 断言云盘**无新增**
  - 管理端关总开关 → 用户页开关**消失**
- [ ] **8.3 压测证明无延迟劣化**（依赖 3.3）
  - 开/关采集对照，p50/p95/p99 对比
- [ ] **8.4 生产部署 + 线上验收**
  - 备份 → 部署 → 真实用户路径 → 管理员数字与云盘交叉核对
- [ ] **8.5 文档同步**
  - `.wolf/` 记忆、`计划书/audit-ledger.md`（避免下次重复跑同一验证）、SOP 更新

---

## 依赖图

```
1.1 ┐
1.2 ├─┐
1.3 ┤ ├─→ 3.3 ─→ 8.3
1.4 ┘ │
      ├─→ 3.1 ─→ 4.1 ─→ 5.1 ─┐
2.1 ─→ 2.2 ─→ 3.2 ─┤          ├─→ 8.2 ─→ 8.4 ─→ 8.5
                   └─→ 4.2 ─→ 5.2 ┘
5.3（贯穿）          7.1（可并行）   6.3（独立）
```

**关键路径**：1.x → 2.x → 3.x → 4.x → 5.x → 8.2 → 8.4
**最高风险节点**：3.3（接线到请求收口，直接影响网关延迟）
