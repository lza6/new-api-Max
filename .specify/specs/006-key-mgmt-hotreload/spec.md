# 006 — 密钥自主管理 · 渠道 Key 运维 · 零停机热更新

> Ratified: 2026-09-29 ｜ 依赖：001（终局审计）｜ 目标版本：v1.3.59

## 1. 问题陈述（Problem）

生产 v1.3.58 上线后暴露三类真实用户/管理员痛点：

1. **用户查看自己的密钥被迫二次验证**：用户已通过 session 登录，点「显示密钥」仍要过 step-up（密码/2FA/passkey）。归属校验已由后端 `GetTokenByIds(id, userId)` 保证，step-up 属重复验证，体验受损且造成「用户前端复制时出现要验证」的线上投诉。
2. **管理员渠道 Key 运维能力缺失**：(a) 多 key 模式下无法直接新增 key；(b) 无法一眼看出哪些 key 仍有效；(c) 无法单独测试某个 key 是否能调用；(d) 无法批量一键测试所有 key；(e) 查看 key 需 root + step-up，运维效率低。
3. **迭代期请求中断**：2C2G 单实例 + `docker compose up` 重建容器造成 ~12s 全站不可用；用户迭代版本期间请求中断。

## 2. 用户故事（User Stories）

### US-1 用户查看自己的密钥（P0）
作为**已登录用户**，我在「API 密钥」页点「显示/复制」自己的密钥，应立即显示，**不再弹出二次验证**。
- 验收：session 登录态 → 单条 reveal → 200，无 `SECURITY_PROOF_REQUIRED`
- 验收：批量 reveal ≤100 条 → 200，无 step-up
- 验收：**越权仍被拦截**：用户 A 无法读取用户 B 的密钥（后端归属校验不变）
- 验收：审计日志仍记录 `token.key_view` / `token.key_view_batch`

### US-2 管理员渠道 Key 运维（P1）
作为**管理员**，我能在渠道多 key 面板：
- (a) 新增 key（追加，不覆盖既有）
- (b) 看到每个 key 的有效/禁用状态、禁用原因与时间
- (c) 单独测试某个 key 是否可调用
- (d) 一键批量测试所有 key
- (e) 查看渠道 key 无需 step-up（管理端，已由 AdminAuth/RootAuth 把关）

### US-3 零停机热更新（P1）
作为**运维**，我部署新版本时不希望用户请求中断。
- 验收：发布流程改为「先构建新镜像 → 起新容器 → 健康检查通过 → 切流量 → 停旧容器」
- 验收：发布窗口内 `curl /api/status` 连续请求 0 失败（除正常长连接）
- 验收：失败自动回滚到上一版本，不出现「新版本起不来且旧版本已停」

## 3. 非功能需求
- **兼容性**：不改变既有 API 响应结构；`X-Security-Proof` 头仍被接受。
  **已知边界（实现与承诺的差异，如实记录）**：站点把
  `require_verification_to_read_own_key` 设为 true 时，**新前端**会回退弹验证
  （先不带 proof → 收到 SECURITY_PROOF_* → 带 proof 重试）；但**旧版本前端**
  （仅「带 proof 才请求」的实现）会直接 403。即该开关的强制模式下不兼容旧前端。
  默认（false）下新旧前端都正常。
- **安全**：越权、枚举、审计、限流不得削弱；管理端 key 读取仍限 root/admin 权限
- **性能**：reveal 少一次 step-up 往返（少 1-2 次 RTT）
- **可维护**：配置项走既有 `operation_setting` 注册机制（热更新）
- **可回滚**：3 项均可用配置开关关闭，恢复旧行为

## 4. 成功指标（2026-09-30 按实测回填）

- [x] `POST /api/token/:id/key` 无 proof 时返回 200（开关默认关闭态）。
  **证据**：`middleware/secure_verification.go:70`（开关关 → `c.Next()` 直接放行）；`middleware/secure_verification_test.go:20` `TestOwnTokenKeyReadSkipsStepUpByDefault`；本地真实浏览器 E2E「查看自己的密钥未弹二次验证」「完整密钥确实被揭示（已解锁 toast）」两项 `ok:true`（`计划书/e2e-evidence/v1.3.60/results.json`）。
  批量：`POST /api/token/batch/keys` ≤100 条同样免 proof（`middleware/secure_verification.go:105` 上限）。
- [x] 多 key 面板可新增 / 单测 / 批量测试，且有可核验的 API 证据。
  **证据**：路由 `router/channel-router.go:55-56`；`controller/channel-test.go:1205` / `:1234`；单测 `controller/channel_test_internal_test.go:490`（add_keys）、`:609`/`:620`/`:638`（批量上限/空渠道/超时）。
  **生产 E2E**：单 key 4/4 `ok:true`、批量 `ok_count:4 fail_count:0`、查看渠道 key 200 无 `SECURITY_PROOF_REQUIRED`、add_keys 幂等拒绝重复 —— 记载于 `计划书/change-report-v1.3.59.html` 第 5 节（**该批未在仓库归档原始结果 JSON**）。
  **本地 E2E**：`计划书/e2e-evidence/v1.3.60/`（13 项全 ok，含 `c1-fallback-dialog.png` 回退弹窗截图 + `backup-export.sqljson.gz` 真实导出件）。
- [x] 发布窗口 `/api/status` 连续请求 100% 200，中断 < 1s。
  **证据（实测口径，与目标的差异如实标注）**：实际执行的是 **25 连打 25/25 = 200**（非 30 连打），数字来自 `计划书/ops/deployment-sop.md:39` 与 `计划书/workflow_status.md` 第九节；切流耗时 4 秒，发布窗口内非 200 计数为 **0**。
  **边界**：脚本 `/opt/new-api/deploy-zero-downtime.sh` 在生产机、不在仓库，本指标无法在仓库内复现 —— 标注「待复现」。

## 5. 明确不在范围（Out of Scope）
- 多实例/负载均衡改造（2C2G 单机不适用，仅在文档给出升级路径）
- 渠道 46 上游 400（用户已确认属上游问题，不处理）
- 真实付费 API（图片/视频生成）的 E2E

## 6. 风险与回滚
| 风险 | 缓解 | 回滚 |
|---|---|---|
| 放宽 step-up 被滥用 | 仅限**自己**的 token（后端归属校验）+ 审计 + 限流 | `token_key_read_verification_enabled=true` |
| 多 key 新增写坏既有 key | append 模式，事务 + 回归测试 | 恢复 key 列 |
| 热更新脚本失败 | 旧容器不停直到新容器 healthy | 脚本自动 abort，旧容器继续服务 |
