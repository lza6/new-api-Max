# SOP — 排障：训练数据采集不工作

> 面向「用户说采集没生效」的场景。按**真实代码路径**从外到内逐层排查，每层给出判据与命令。
> 证据来源全部标 `file:line`；无证据的推断标「待验证」。
> 最后核对：2026-10-16（对照 `VERSION` = `v1.3.147`）。

---

## 0. 先看「设计上就采不到」的三种正常情况（不是故障）

| 现象 | 根因（设计如此） | 依据 |
|---|---|---|
| 纯图片 / 纯 embedding / 音频请求没有采集 | `BuildSignal` 拿不到文本就返回 nil，不存空记录 | `service/training_capture/hotpath.go:139-143` |
| 助手回复没被采集 | 热路径只拿得到 `TokenCountMeta.CombineText`；流式回复拼接代价高，默认不填 `ResponseText` | `service/training_capture/hotpath.go:28-54,122-124` |
| 只能做「整段文本前缀」增量，没有逐轮 role 标注 | 网关 `dto.Request` 只暴露拼接后的 `CombineText`（不可逆） | `service/training_capture/hotpath.go:28-54` |

发现用户期望的是上述「设计边界」内的东西 → 如实说明，不要当 bug 查。

---

## 1. 采集为什么「开了却采不到」：四道闸门

投递链路的完整顺序（任一不满足即不采集）：

```
请求完成 → controller/relay.go goroutine 调 BuildSignal → MaybeCapture → IsGranted → TryEnqueue → worker → 切分/压缩/上传
```

### 闸门 1：总开关 + 云盘是否配置

- 总开关存 `options` 表，键 `TrainingDataCaptureGatewayEnabled`（`service/training_capture/gateway_option.go:19-23`）。
- 前端对外状态由 `GetGatewayStatus()` 组装（`service/training_capture/consent.go:144-158`）：
  **总开关开 且 云盘已配置**才报告 enabled；云盘没配时即使 DB 里开着也报告为不可用。
- 命令：`curl -s http://127.0.0.1:3000/api/admin/training-data/overview`（需管理员 session）
  看 `enabled` / `can_enable` / `reason`（reason 会明说「尚未配置云盘存储后端」）。

### 闸门 2：进程内快速开关

- `captureHotPathEnabled`（`service/training_capture/hotpath.go:63-73`）短路绝大多数请求。
- 它由 `SyncHotPathFlag()` 维护，判据**完全等同于闸门 1**（`service/training_capture/consent.go:120-124`）。
- 触发同步的时机：总开关变更、后端配置变更、启动完成（`consent.go:120`）。
- 若热路径开关与真实状态不一致（例如改了云盘配置但没触发同步）→ 表现为「总开关看着开、就是采不到」。
  排查点：确认改配置后是否走了 `SyncHotPathFlag`（DB 载入处 `service/training_capture/gateway_option.go:30-50`）。

### 闸门 3：用户是否授权

- 服务端强制：投递点读 `TrainingDataConsent` 表判定，不信任前端（`service/training_capture/consent.go:14-37`）。
- 判据：`IsGranted(userId)` = 总开关开 + 云盘已配置 + 用户已授权 + 同意的 `policyVersion` == 当前版本
  （`service/training_capture/consent.go:170-190`）。
- 缓存 TTL 默认 **30s**（`consent.go:39`）；撤回会**主动失效缓存**。所以：
  - 刚授权后最坏等 30s 才生效（正常）。
  - 撤回后正常路径立即停止（若仍在上传，说明走到了 worker 消费复查之前，见闸门 5）。

### 闸门 4：请求本身有没有可采的文本

`BuildSignal`（`service/training_capture/hotpath.go:134-179`）在下列情况返回 nil：

- `in.UserId <= 0` 或 `in.Meta == nil`（`hotpath.go:135-137`）；
- `CombineText` trim 后为空（`hotpath.go:138-143`）——**最常见的「采不到」**：
  - `CountToken=false`（关闭 token 计数时没有 `CombineText`）；
  - 或请求本身没有文本内容（见 §0）。
- `captureHotPathEnabled == false`（`hotpath.go:145-147`）。

命令：确认请求是否真的产生了 token 计数；`CountToken` 配置与本请求是否有文本。

---

## 2. 进了队列之后

### 2.1 队列是否在丢

- 队列满时 `TryEnqueue` 返回 false 并计入丢弃：`QueueStats.Dropped`（`service/training_capture/queue.go:199-217`）。
- `Dropped` 持续增长 = 采集量超过 worker 吞吐（云盘慢 / 限速低 / 预算小）。
- 命令：管理端 overview 里的队列/丢弃指标（`CaptureStats`）；或看启动日志里的**实际**队列容量
  （`service/training_capture/resources.go:285-293`）。

### 2.2 worker 是否在跑 / 是否 panic

- worker 有顶层 `defer recover()`，panic 不会杀进程，但会打日志
  `training capture: worker panic recovered (capture continues, gateway unaffected): ...`
  （`service/training_capture/worker.go:269-273`）。
- worker 消费前会**复查授权**（`worker.go:313-321`）——撤回后即使用户请求已入队也不会再上传。
- 命令：`docker logs --since 30m new-api 2>&1 | grep -i "training capture"` 看上面这类日志。

### 2.3 增量判定是否把内容「判成无新增」

- 有会话 id（`prompt_cache_key`）→ 精确增量（`hotpath.go:111-116`）。
- 无会话 id → 退化为**全局指纹去重**（去重粒度变粗，但不会误丢）。
- 文本增量规则：只在「新文本以已上传前缀开头」时裁剪；否则**全量重传**（`text_increment.go:39-78`）。
  → 若看到每次都全量重传，通常是 prefix hash 不匹配（历史被编辑/截断），这是**保守正确行为**，非 bug。

---

## 3. 切分、压缩与上传

- 顺序硬约束：**先按原始 JSONL 的行切，再对每块各自 gzip**（`service/training_capture/worker.go:465-488`）。
  切错顺序 → 云盘上是坏 gzip 碎片 → 预览显示二进制垃圾（详见 ADR-003）。
- 压缩失败会降级为明文上传并记 `training capture: gzip failed, uploading plain: ...`
  （`worker.go:490-494`）。
- watermark 保存失败会记 `training capture: save watermark failed: ...`（`worker.go`）。
- 云盘写入失败 → 看 `write part ...` 返回的错误（`worker.go:504-506`）。
- 云盘侧确认：管理端 `/api/admin/training-data/files`（`router/training-data-router.go:40`）。

---

## 4. 一条命令收集证据

```
docker logs --since 30m new-api 2>&1 | grep -i "training capture"
```

配合管理端 overview（`/api/admin/training-data/overview`）看四类数字：
总开关状态 / 队列长度与丢弃 / 今日上传量 / 云盘剩余。

---

## 5. 快速分诊表

| 症状 | 先查 | 判据 |
|---|---|---|
| 开关亮但 0 数据 | 云盘是否配置 | `GetGatewayStatus().CanEnable`（`consent.go:144-158`） |
| 刚开启没反应 | 等 30s 缓存 | `consentCacheTTL`（`consent.go:39`） |
| 某些请求采不到 | 请求有没有文本 | `hotpath.go:138-143` |
| 有丢弃计数 | 队列/worker 吞吐 | `queue.go:199-217` |
| 云盘有文件但乱码 | 切分/压缩顺序 | `worker.go:465-488`（ADR-003） |
| 撤回后还上传 | worker 复查授权 | `worker.go:313-321` |
