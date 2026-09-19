# GO 全面代码评审报告（new-api-Max）

> 日期：2026-09-20 ｜ 范围：根 Go 模块（relay/ relaykit/ service/ controller/ model/ common/ middleware/ router/ setting/ pkg/）
> 方式：主控代理单代理直审（子代理工具本轮不可用，改为本人直接执行只读扫描 + go vet + 定向测试，诚实声明）
> 目标：对照「全面 Go 代码评审」15 节清单做取证级核查，产出可验证 findings，不改任何代码

## 0. 执行方式与验证记录

| 验证项 | 命令/方式 | 结果 |
|---|---|---|
| 编译器静态检查 | `go vet ./common/... ./model/... ./service/... ./middleware/... ./router/... ./pkg/billingexpr/...` | ✅ 全绿（无输出） |
| 计费不变量测试 | `go test ./common/ -run Quota` / `go test ./pkg/billingexpr/...` / `go test ./service/ -run "Billing\|TaskBilling\|Quota"` | ✅ 全部 ok（0.8s/0.9s/1.7s） |
| 全仓模式扫描 | rg 统计 interface{}/panic/json 直用/_ = 吞错/弱加密/默认口令 | 见下文各节 |
| 未执行项 | `-race` 全量、goleak、govulncheck、线上 EXPLAIN、controller 全套测试（Windows 宿主机 t.TempDir 已知环境问题） | ⏳ 见 §边界 |

## 1. 总体结论

**未发现 CRITICAL 级问题**（在扫描/工具可覆盖范围内）。计费不变量单元测试与 go vet 全绿，SQL 全参数化无注入面，认证令牌/TOTP/口令盐均用 crypto/rand。
存在 **1 项 P1 运维风险（默认 root 口令）、若干 MEDIUM（错误被吞集中在支付/任务/余额路径、panic stub）、若干 P2（JSON 直用约定违反、`_ =` 系统性吞错、可维护性）**。

## 2. Findings 汇总（按严重度）

### [MEDIUM] P1-1 默认 root 口令 123456 且启动日志明文打印
- 位置：`model/main.go:74-75`
- 证据：`common.SysLog("no user exists, create a root user for you: username is root, password is 123456")` + `Password2Hash("123456")`
- 影响：首次部署若未立即改密，公网实例存在被接管风险；日志明文打印默认口令属信息泄漏。
- 建议：生产首登强制改密；启动日志只提示"已创建 root 用户，请立即修改密码"，不打印口令。

### [MEDIUM] P1-2 支付/余额/任务关键路径错误被吞（静默失败）
- 位置：
  - `controller/subscription_payment_epay.go:116` `_ = model.ExpireSubscriptionOrder(...)`（支付回调后状态更新失败静默 → 订阅权益漂移）
  - `controller/channel-billing.go:617` `_ = updateAllChannelsBalance()`
  - `controller/codex_usage.go:134` `_ = model.DB...Update("key", ...).Error`（渠道 key 轮换失败静默）
  - `relay/relay_task.go:81,124` `_ = common.Unmarshal(originTask.Data, &taskData)`（任务数据解析失败静默 → 结算/归属错配）
- 建议：支付/计费/任务路径的错误必须显式处理（log + 返回非 2xx 触发重试 或 走失败分支），禁止 `_ =`。

### [MEDIUM] P1-3 渠道适配器 panic stub（与同文件风格不一致）
- 位置：`relay/channel/zhipu/adaptor.go:28`、`relay/channel/mistral/adaptor.go:27` `panic("implement me")`
- 影响：同文件其他未实现方法均返回 `errors.New("not implemented")`，唯独这两个 `ConvertClaudeRequest` 直接 panic；一旦 Claude 协议请求被路由到该渠道，将触发 500（Gin recover）而非优雅错误。
- 建议：统一为 `return nil, errors.New("not implemented")`。

### [MEDIUM] P1-4 TOTP 备用码模偏差
- 位置：`common/totp.go` `generateRandomBackupCode`（`charset[int(randomBytes[0])%36]`，256%36≠0，字母数字分布略不均）
- 影响：备用码熵略低于理想（已用 crypto/rand，安全底线 OK，但可改进）。
- 建议：rejection sampling（拒绝 >= 252 的值）消除偏差。

### [LOW/P2] P2-1 业务代码直用 encoding/json（违反 AGENTS.md 统一约定）
- 位置（抽样 40+ 处）：`relay/mjproxy_handler.go:128,170,177,184,290,...`、`controller/channel.go:597,1047,2125,...`、`controller/misc.go:284`（NewDecoder）、`relay/channel/zhipu/relay-zhipu.go:193-243`、`relay/channel/xunfei/relay-xunfei.go:146-231`、`setting/user_usable_group.go:30,42`、`controller/deployment.go:69`、`pkg/ionet/*`（client.go:104、container.go:293 等）
- 约定：根模块 marshal/unmarshal 必须走 `common.Marshal/Unmarshal/DecodeJson`；relaykit 走 `kitutil.*`。
- 影响：行为不一致 + 统一错误日志/审计缺失（非安全漏洞）。
- 建议：分批迁移，低风险逐步替换。

### [LOW/P2] P2-2 全仓 `_ =` 吞错约 104 处（relay 32 / controller 22 / model 25 / service 15 / relaykit 10）
- 抽样：多数为流式 flush/close（`relay/helper/common.go:73,84,137`、`stream_scanner.go:110,202`）可忽略；但支付/计费/任务/缓存路径的吞错需按 P1-2 处理。
- 建议：系统性审计，规则=非 I/O 幂等类错误不得吞。

### [INFO] 其他
- `interface{}`：22 个文件（多为 adapter/转换器），可逐步收窄，无高危点。
- `panic(`：19 个文件。`relaykit/relayconvert/*registry.go` 与 `router/task-plugin-protocol-router.go:19` 均为启动期 fail-fast（配置错误即崩，属设计内）；渠道 stub panic 见 P1-3。
- `context.TODO()`：生产代码 0 处 ✅；`gorm:query_option`：仅 `model/locking.go:13` 注释（无遗留使用）✅。
- 弱加密：`crypto/sha1` 仅用于支付引用 ID（`controller/*stripe/creem/topup*`）与渠道亲和哈希（`service/channel_affinity.go:419`），**未用于凭据** → 可接受（LOW/INFO）。
- `math/rand`：6 处，全部为非安全用途（抖动、抽渠道、签到额度）✅；密钥/Token/盐均 crypto/rand（`common/utils.go:231-248`、`common/totp.go:4`、`model/auth_flow.go:4`、`common/account_password.go:4`、`service/email_binding.go:4`）。
- SQL：全参数化，`applyExplicitLogTextFilter` 正确转义 LIKE 通配符；未发现拼接注入面 ✅。
- 并发/资源：本轮为静态取证，未发现明确 goroutine 泄漏/未关 Body 的确定性证据；需 `-race` + goleak 运行时验证（见 §边界）。

## 3. 优先级矩阵

| 优先级 | 项 | 动作 |
|---|---|---|
| **P0** | 无（无需停机修复项） | — |
| **P1** | 默认口令+日志、支付/任务/余额吞错、panic stub、备用码模偏差 | 4 个修复项，改动小、可单文件级修复 + 定向测试 |
| **P2** | JSON 统一（40+ 处）、`_ =` 系统性清理、interface{} 收窄、`-race`/goleak/govulncheck 基线 | 分批技术债 |
| **P3/边界** | controller 套件 Windows t.TempDir 环境问题（Linux CI 正常） | 并入 CI 技术债清单 |

## 4. 修复建议（供下轮执行，均小改动）
1. `model/main.go`：日志去口令 + 首登强制改密标记。
2. 支付/任务/余额吞错 4 处：显式错误处理（log + 语义化返回）。
3. `zhipu/mistral adaptor`：panic → errors.New。
4. `common/totp.go`：rejection sampling。
5. JSON 直用：按目录分批迁移到 common.*/kitutil.*。

## 5. 边界与后续（诚实声明）
- 未执行：`-race` 全量、goleak 泄漏测试、govulncheck 依赖漏洞扫描、线上库 EXPLAIN、controller 全套测试（Windows 宿主 t.TempDir 已知问题，Linux CI 不受影响）。
- 性能类（§7）与 HTTP 细节（§13）为静态取证，未做压测；建议下轮补 `go test -race ./service/...`、`go test ./relay/... -run "Fallover|Cooldown"` 与线上压测。
- 本轮**未修改任何代码**；findings 均附绝对路径:行号，可复核。
