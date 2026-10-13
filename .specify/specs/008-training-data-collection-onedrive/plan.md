# Implementation Plan: 训练数据采集 + OneDrive 冷存储对接

**Feature**: `008-training-data-collection-onedrive` | **Spec**: `spec.md`
**Created**: 2026-10-14

## 1. 技术上下文（实测，非假设）

| 事实 | 实测值 | 来源 |
|---|---|---|
| 生产机 | `154.37.208.171`，4 核 4G，80G 盘，**75Mbps** 上行 | `计划书/ops/` + 服务器实测 |
| 容器内存限 | 900MB（`--memory 900m`） | `/opt/new-api/deploy-zero-downtime.sh` |
| 当前磁盘 | 9.2G / 78G（12%） | `df -h /` |
| 日志目录 | **870MB**，24 文件，单个最大 173MB，**无上限增长** | 实测 |
| OneDrive 配额 | **5TB 总 / 5.49TB 剩余** | Graph API `quota` |
| OneDrive 凭据 | client_credentials（应用凭据，无浏览器登录） | `HANDOFF_onedrive.md` |
| OneDrive 能力 | 建目录 ✅ 上传 ✅ **读回逐字一致** ✅ 列举 ✅ 删除 ✅ | 线上实测（含 `_probe2` 往返） |
| 三库 | SQLite / MySQL ≥5.7.8 / PostgreSQL ≥9.6 | 宪法 IV |
| 后端 | Go 1.25 + Gin + GORM v2 | `AGENTS.md` |
| 前端 | React 19 + TanStack + Base UI + Tailwind 4 | `web/AGENTS.md` |

## 2. 架构决策（含被否方案）

### ADR-008-1：采集必须完全脱离请求热路径

**决策**：请求结束时只做一件极轻的事——把「发生了一次对话」的信号投进**有界内存队列**
（非阻塞 `select`，满则丢弃并计数）。上传由独立 worker 消费。

**被否方案 A**：请求结束后同步写去重表。
→ 否：宪法 V 明确禁止热路径 DB 同步写；每请求一次写会在高并发下直接抬高 p95。

**被否方案 B**：定时扫 `logs` 表批量采集。
→ 否：`logs` 表**不含对话正文**（只有元数据/计费），扫不到内容；
且大表扫描与「慢查询猎杀」的目标冲突。

**被否方案 C**：用现有 `service/` 的 `backgroundLoop` 直接跑（无队列）。
→ 否：仍需一个「信号从请求线程传到 loop」的有界通道，否则要么忙轮询、要么漏采。

### ADR-008-2：去重采用「会话水位 + 消息指纹」双层

**决策**：
- **会话层水位**：`(conversationId) → uploadedMessageCount / lastFingerprint`
- **消息层指纹**：`hash(role + "\x00" + normalizedContent)`（规范化 = trim + 折叠空白）
- 上传时取「水位之后」的消息；**若该消息指纹已在上传集合中则跳过**

**为什么这样能解决用户点名的「长期会话 token 都一样」**：
长期会话的历史轮次**内容不变** → 指纹不变 → 被跳过；只有新增轮次指纹是新的。
用户说的「很多 token 都一样」正是**消息级去重能命中**的场景。

**被否方案**：整会话内容哈希。
→ 否：会话一旦新增一轮，整体哈希就变 → 整个会话重传，正是要避免的浪费。

**被否方案**：只存「已上传消息 id 集合」。
→ 否：集合会随对话量无限增长；水位（计数 + 末次指纹）是 O(1)。

**边界处理**：
- 编辑/重生成 → 产生新指纹 → 视为新增（**不丢内容**，宁可多传一次）
- 删除消息 → 水位不回退（**不重复传**已传内容）
- 会话 id 缺失 → 退化为「按内容指纹全局去重」，仍不重复

### ADR-008-3：云盘后端抽象为接口，OneDrive 为首个实现

**决策**：
```go
type TrainingDataSink interface {
    Name() string
    // EnsureDir 幂等：已存在返回 nil
    EnsureDir(ctx context.Context, path string) error
    // WritePart 写入一个分片；返回远端引用
    WritePart(ctx context.Context, path string, data []byte) (string, error)
    // Stat 返回配额与统计（用于管理员视图）；不支持时返回 ErrNotSupported
    Stat(ctx context.Context) (SinkStat, error)
    // List 列举某目录下的分片（用于重启后恢复分片序号）
    List(ctx context.Context, path string) ([]string, error)
}
```
**理由**：用户明确要求「扩展其他云盘的对接方式」。接口小（5 方法），
OneDrive / S3 / OSS / WebDAV 都能实现；未实现的后端**明确报错**。

### ADR-008-4：目录名必须白名单化（安全）

**决策**：用户名 → 目录名做 `sanitizeDirName`：
1. 只保留 `[A-Za-z0-9._-]`，其余 → `_`
2. 折叠连续 `_`，去掉首尾 `.`/`_`
3. 空结果或纯 `.`/`..` → `user-<id>`
4. 截断到 64 字符
5. **追加 `-<userID>` 消歧**（防两个不同用户 sanitize 后同名而互相覆盖）

**为什么必须做**：用户名直接拼路径 = 路径穿越（`../`）+ 覆盖他人数据。
OneDrive 的 `root:/{path}:/content` 形式对路径字符敏感，必须提前净化。

### ADR-008-5：分片命名与恢复

**决策**：`part-0001.jsonl`（零填充 4 位）、`part-0002.jsonl` …
- 零填充 → 字典序 = 数字序（列举后取最大即可恢复下一个序号）
- 达到 `MaxPartBytes`（默认 8MB）即开新分片
- 重启后：`List(dir)` 取最大序号继续，**不重头覆盖**

**为什么不用单文件追加**：Graph API 的简单上传不支持远端追加；
且单文件会无限增大（用户明确要求避免）。

### ADR-008-6：凭据只从环境变量读，DB 只存非敏感引用

**决策**：`ONEDRIVE_CLIENT_ID` / `ONEDRIVE_CLIENT_SECRET` / `ONEDRIVE_TENANT` /
`ONEDRIVE_DRIVE_ID` 走 env；管理端只显示「已配置 / 未配置」+ 连通性测试结果。

**理由**：与既有安全基线一致（渠道密钥加密未启用时不落明文）；
明文密钥落 DB 会让「DB 备份泄漏 = 云盘失守」。

### ADR-008-7：管理员总开关关闭 → 用户开关隐藏

**决策**：用户端接口返回 `available: false`（含原因），前端据此**不渲染**开关。

**理由**：显示一个点了无效的开关，正是用户明确反对的「假功能」。

### ADR-008-8：日志分片 + 双上限保留（已实现）

**已落地**：`logger/retention.go` + 13 用例。按数量（14）+ 总字节（1GB）双上限，
保护当前写入文件，只删本程序命名的文件。

---

## 3. 数据模型

### 3.1 授权表（新）

```
training_data_consents
  id            PK
  user_id       FK users(id) UNIQUE   -- 一用户一条
  granted       bool                  -- 当前是否授权
  granted_at    int64                 -- 最近一次同意时间（unix）
  revoked_at    int64                 -- 最近一次撤回时间
  policy_version varchar(32)          -- 同意的声明版本（版本变更需重新同意）
  created_at / updated_at
INDEX (user_id) UNIQUE
```

### 3.2 去重水位表（新）

```
training_data_watermarks
  id                  PK
  user_id             FK
  conversation_id     varchar(128)
  uploaded_count      int             -- 已上传消息数（水位）
  last_fingerprint    varchar(64)     -- 末次消息指纹（校验水位一致性）
  last_uploaded_at    int64
  created_at / updated_at
UNIQUE INDEX (user_id, conversation_id)
```

**三库注意**：`conversation_id` 用 `varchar(128)` + 唯一索引；
**不用** `ON CONFLICT ... AS new`（MySQL 5.7 不支持），
统一用 `ON DUPLICATE KEY UPDATE ... VALUES(col)` 风格或先查后写（在 worker 内，非热路径）。

### 3.3 采集统计（进程内 + 落库）

进程内原子计数（今日用户数/对话数/字节），
管理员视图实时读；云盘配额**实时查 Graph API**（带短 TTL 缓存，避免打爆 API）。

---

## 4. 组件设计

| 组件 | 文件 | 职责 |
|---|---|---|
| 采集核心（纯逻辑） | `service/training_capture/dedup.go` | 指纹计算、增量判定（**可单测，无 IO**） |
| 目录名净化 | `service/training_capture/naming.go` | `sanitizeDirName` / 分片命名（**纯函数**） |
| 队列 + worker | `service/training_capture/queue.go` | 有界队列、自适应节奏、丢弃计数 |
| 存储接口 | `service/training_capture/sink.go` | `TrainingDataSink` 接口 + 注册表 |
| OneDrive 实现 | `service/training_capture/onedrive.go` | Graph API：token/建目录/上传/配额/列举 |
| 授权服务 | `service/training_capture/consent.go` | 授权读写、声明版本校验 |
| 用户端接口 | `controller/training_data.go` | 查询状态 / 授权 / 撤回（服务端强制） |
| 管理端接口 | `controller/training_data_admin.go` | 总开关、统计、云盘状态、按日期查询 |
| 前端用户页 | `web/src/features/profile/...` | 开关 + 声明弹窗 + 小字说明 |
| 前端管理页 | `web/src/features/system-settings/...` | 总开关 + 统计面板 |

**接线点（关键）**：采集信号从**请求结束**处投递。
必须先定位「对话请求成功的唯一收口点」，只在那里投递一次（避免重复采集）。

---

## 5. 性能策略

| 关注点 | 措施 |
|---|---|
| 请求延迟 | 投递 = 非阻塞 channel send；队列满 → 丢弃 + 计数（**绝不阻塞**） |
| 上传带宽 | 令牌桶限速（默认取 75Mbps 的保守比例，可配）；仅在「无用户请求压力」时提速（可选） |
| 内存 | 队列容量硬上限 + 单次上传大小上限；超限不排队 |
| 云盘 API | token 缓存（提前 5 分钟续期）；配额查询带 TTL 缓存 |
| 并发 | worker 单 goroutine（避免并发写同一分片）；目录创建容忍「已存在」竞态 |
| 降级 | 云盘不可达 → 指数退避 + 有界重试；持续失败**不影响网关**，只上报 |

---

## 6. 安全与合规

- 服务端强制：未授权用户的数据**绝不入队**（在投递点校验授权状态，带短 TTL 缓存）
- 不采集：密钥、令牌、验证码、密码类字段（在序列化时**白名单**字段，非黑名单）
- 凭据：仅 env；日志中永不打印 `client_secret`
- 授权留痕：`granted_at` + `policy_version`
- 撤回文案**诚实**：明确「停止采集」≠「删除已采集数据」

---

## 7. 验收方式（对齐宪法「生产优先」）

| 层级 | 方式 |
|---|---|
| 单元 | 去重算法 / 目录名净化 / 分片命名 / 队列边界（纯函数，穷举边界） |
| 集成 | sink 接口 + OneDrive 实现：真实建目录/上传/读回（线上实测凭据） |
| E2E | 本地网关真实登录 → 用户开启授权 → 发真实对话 → 断言云盘出现 `日期/用户/模型/part-*.jsonl` 且内容正确 |
| 压测 | 开启采集 vs 关闭采集：请求 p95 对比（证明确无劣化） |
| 线上 | 部署后真实用户路径验证 + 管理员视图数字与云盘交叉核对 |
| 三库 | 两张新表在三库上建表 + 迁移幂等 |

---

## 8. 任务分解

见 `tasks.md`。

## 9. 风险与回滚

| 风险 | 回滚手段 |
|---|---|
| 采集影响延迟 | 管理员总开关一键关闭（**立即停止 worker**）；或 env `TRAINING_DATA_ENABLED=false` 重启 |
| 云盘被写脏 | 分片写入独立日期目录；删除只需删该目录（不影响其它） |
| 新表迁移问题 | 两张新表均为**新增**，不修改既有表；回滚只需 `DROP TABLE`（无数据依赖） |
| 队列内存 | 硬上限，最坏情况丢弃采集而非 OOM |

---

## 10. 关键实现约束（读码实测，决定了采集机制）

### 10.1 `dto.Request` 接口**读不到消息数组**

```go
// relaykit/dto/request_common.go:8
type Request interface {
    GetTokenCountMeta() *types.TokenCountMeta
    IsStream(c *http.Request) bool
    SetModelName(modelName string)
}
```

`RelayInfo.Request` 是这个**窄接口**。它**没有** `Messages` 之类的访问器，
所以在通用 relay 路径上**无法**拿到结构化的消息数组（role/content 逐条）。

### 10.2 但**能**拿到全格式统一的文本

`GetTokenCountMeta()` 返回的 `TokenCountMeta.CombineText` 是
「所有消息拼接后的文本」，且**每个 relay 格式都实现**了它（OpenAI/Claude/Gemini/…）。
另有 `MessagesCount`（消息条数）。

实测确认（`relaykit/dto/openai_request.go:161`）：OpenAI 格式的 `CombineText`
由 `prompt` + `input` + 各消息内容拼接而成；Claude/Gemini 各有对应实现。

### 10.3 由此确定采集机制（两段式）

| 段 | 位置 | 拿什么 | 代价 |
|---|---|---|---|
| **请求段** | `Relay` 的 `defer`（请求结束） | `CombineText` + `MessagesCount` + `OriginModelName` + `UserId` + `UsingGroup` | 读已算好的 meta，**不额外解析 JSON** |
| **响应段** | 同上（响应已完成） | 助手回复文本 | 需要新引入一个轻量累加器 |

**响应文本的获取**：各 adaptor 内部有 `ResponseText strings.Builder`，
但它们**不在同一处**、且不都暴露出来。为避免改 40+ 个 adaptor（违反宪法 VII 保守改动），
采用**最小侵入**方案：新增一个 context 累加器 `ContextKeyTrainingResponseText`，
在**少数几个**主流格式（OpenAI 兼容 / Claude / Gemini）的收口处追加；
其余格式**接受只有 prompt 侧**（并在文档明确标注该边界，**不假装全都有**）。

> 这一条是**诚实边界**：不同格式的覆盖率不同，`spec` 里不承诺 100%，
> 但保证「拿到了什么就存什么，且字段白名单一致」。

### 10.4 为什么不用 `logs` 表

`logs` 表**不含对话正文**（只有模型名/渠道/token 数等元数据），
且扫大表与宪法 V（禁热路径 DB 操作）冲突。

### 10.5 投递点

`controller/relay.go` 的 `func Relay(...)` 开头已有唯一 `defer`（错误处理用）。
在其后追加一个**只读**的投递：
```go
defer func() {
    if newAPIError == nil {                 // 只采成功的请求
        servicetraining.MaybeCapture(relayInfo)   // 内部：非阻塞投递，满则丢弃
    }
}()
```
**成功才采**：失败请求没有助手回复，采集无意义且会引入噪声。
