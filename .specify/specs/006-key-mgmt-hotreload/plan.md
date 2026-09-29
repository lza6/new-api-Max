# Plan — 006 密钥自主管理 · 渠道 Key 运维 · 零停机热更新

## 技术栈
- 后端：Go 1.25（Gin + GORM），复用 `setting/operation_setting` 注册机制（热更新）
- 前端：React 19 + TS，复用 `CopyButton` / `ConfirmDialog` / `Dialog` / `StaticDataTable`
- 部署：Docker Compose 单机，改用「先起新容器 → 健康检查 → 切 Caddy → 停旧容器」

## 架构决策

### AD-1 用户自己的 token 读取不再要求 step-up（可配置）
新增 `TokenSetting.RequireVerificationToReadOwnKey bool`（默认 **false** = 不需验证）。
- `middleware.SecureTokenKeyVerificationRequired`：校验 token 归属当前 session 用户；归属成立且开关为 false → 直接放行。
- 归属不成立（读别人的）→ 保持原 step-up 语义（防御纵深）。
- 后端归属校验 `GetTokenByIds(id, userId)` 保持不变 = 真正的安全边界。

### AD-2 渠道 key 运维补「测试单个 key / 批量测试」
- 新增 `POST /api/channel/:id/key/test`，body `{"key_index": N}`：用第 N 个 key 向该渠道发一次最小请求（复用 `channel-test.go` 的测试逻辑，指定 key index）。
- 新增 `POST /api/channel/:id/keys/test`：并发批量测试全部 key，返回 `[{index, ok, latency_ms, error}]`。
- 复用既有 `GetNextEnabledKey` 的 key 解析与 `processChannelError` 的健康分记录。

### AD-3 查看渠道 key 去 step-up（管理端）
`POST /api/channel/:id/key` 的 `SecureVerificationRequired` 改为可配置：
`ChannelSetting.RequireVerificationToReadKey bool`（默认 false）。管理端已由 `AdminAuth + RootAuth + RequirePermission(ChannelSensitiveWrite)` 把关。

### AD-4 零停机热更新
新增 `deploy-zero-downtime.sh`：
1. build 新镜像 `new-api:<tag>`
2. `docker run` 新容器（临时名 `new-api-next`，端口 3001），等 `/api/status` healthy
3. 改 Caddy upstream 指向 3001，`caddy reload`（毫秒级）
4. 停旧容器，把新容器改名为正式名 + 改回 3000
5. 任一步失败 → 不改 Caddy，直接删新容器（旧容器全程在服务）
2C2G 约束：新旧容器短暂共存（各 ~350MB），需先确认可用内存 > 800MB；不足则回退滚动方式并提示。

## 数据模型
无 schema 变更。`TokenSetting` / `ChannelSetting` 为 JSON 配置列（options 表），热更新。

## 安全
- token 归属校验为唯一安全边界，**不削弱**
- 管理端 key 读取限制 root/admin
- 审计事件保留

## 验证策略
1. 单测：`middleware`（归属+开关矩阵）、`controller`（key test）
2. 前端：vitest（reveal 不再触发验证弹窗）
3. E2E：本地起服务 + 生产 HTTPS 验证 reveal 200、key test 200
4. 热更新：生产 dry-run（不切流量）验证脚本分支
