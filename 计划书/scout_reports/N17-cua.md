# N17 侦察报告：CUA/浏览器/移动/桌面 Agent（已完成，18 项详读）

## 关键项目

### browser-use（python|large）★ 4/5/5/4
- 事件驱动 CDP 会话管理器（"SINGLE SOURCE OF TRUTH"）；DOM 快照脱敏（password/one-time-code 字段"never leave the snapshot"）
- **vision 'auto'**：仅当 action result metadata 明确 include_screenshot 才带图；强制缩图重编码；vision_detail_level 透传
- **TokenCost 服务**：从 LiteLLM GitHub pricing JSON 拉价格（1 天缓存）+OpenRouter+CUSTOM_MODEL_PRICING 覆盖——"按模型实时定价计费"与 new-api 计费域同构
- MessageCompactionSettings（每 25 步压缩/keep_last 6/trigger 40k）；14 个 watchdog 自愈器
- **对 new-api**：它自己拉 LiteLLM 价格表——网关提供 /pricing 端点+实时价格镜像可成定价源（锁定机会）；usage 必须把 image tokens 与 text tokens 分开报

### sandbox-runtime（Anthropic srt，node）★ 2/5/5/4
- OS 级沙箱不依赖容器：macOS sandbox-exec、Linux bubblewrap+自生成 seccomp、**Windows srt-win（专用账户+SID 级 WFP+两跳启动，结构性关闭 surrogate-spawn 逃逸）**
- MITM 代理体系+域名白名单；**凭据哨兵**（credential-extract/sentinel——识别拦截密钥外流）；违规监控实时告警
- **对 new-api**：Go 单二进制常部署用户裸机形态，"无容器 OS 级沙箱"正是 jsplugin 长任务/外部命令执行的隔离路线；凭据 sentinel+域名白名单 MITM 可用于"插件密钥不落沙箱"

### OpenSandbox-main（阿里，node）★ 3/5/5/4
- **OSEP-0008 rootfs 快照暂停/恢复**：暂停=rootfs commit 成 OCI 镜像推 registry+删除沙箱释放算力，resume 从镜像重建且 sandboxId 稳定——长任务经济学最优解
- fast-sandbox（OSEP-0007）：gRPC Fast-Path+预热 Agent 池冷启动 <50ms；FQDN egress 白名单（OSEP-0001）
- **对 new-api**：jsplugin 长任务扩展路径=池化（预热实例池）+快照暂停；egress 白名单是插件联网权限现成设计

### CubeSandbox（腾讯，E2B 兼容）★ 3/5/4/4
- RustVMM+KVM 硬件隔离，<60ms 冷启动；**凭据保险库**（Agent 调外部 API 密钥永不进沙箱）+**AutoPause/AutoResume**+每沙箱流量令牌+策略路由出口；CubeCoW 事件级快照
- **对 new-api**："凭据保险库+流量令牌+AutoPause"是 CUA/代码执行增值最该抄的三件套——凭据引用与渠道密钥管理天然衔接

### cua（trycua，python）★ 3/5/5/4
- Fleet 云 Go 后端 56 handler：**metering→billing→billing_webhook→signedserviceurls 完整"agent 沙箱云计费栈"**
- cua-driver 权限三档+动作历史审计（metadata allowlist，绝不存截图/击键/剪贴板）；后台输入驱动不夺焦点
- **对 new-api**：沙箱即服务蓝图（令牌计量→计费 webhook→签名租约 URL）；合规审计模板

### 其他
- UI-TARS-desktop：Operator 抽象五执行面；模型输出文本 action 序列；@tarko/agent-snapshot 录制回放测试。3/5/5/4
- agent-browser：**截图质量预检**（空白/模糊评分，废图不喂 LLM 直接省 token）；会话 TTL+trace step 持久化。2/3/4/3
- mcp-chrome：扩展复用登录态零冷启动；native messaging 16MB 上限——**超大 base64 报文需 chunked 切分是多模态网关真实痛点**。3/4/4/3
- chrome-devtools-mcp（Google 官方）：screencast 产生"连续视频分段"流量（比单截图大 100 倍）；slim/full 工具分层；遥测开关默认开启提醒网关应给租户提供遥测开关。3/4/5/4
- mobile-mcp：a11y 树优先零 image token（文本快照比图便宜一个数量级）；TuriX 同样 AX 树为主截图为辅双通道。2/3/4/3
- MobileAgent（阿里）：四角色分工+InfoPool 共享状态；25 步任务=25 次多模态请求——**CUA 天然高 ARPU**；多角色扇出需要"会话级速率限制按 agent 计数"。3/4/3/2
- agent-device（callstack）：**设备租约状态机**（new/recover/continue/cancel/release/supersede+ResourceOwnershipFence 世代号+journal 持久化）——"稀缺资源并发分配"顶级参考；keyed-lock/fence/journal 可平移到渠道级并发租约（recover/continue 语义对渠道熔断恢复贴切）。2/4/5/4
- clickclickclick：planner/finder 双模型分工（廉价模型跑 finder）；quality=45 低质 JPEG 即可工作——"自动图像压缩换更低成本"有客户需求。2/2/2/2
- daytona：仓库已归档（v0.190.0 冻结，开发转私有）——**沙箱赛道商业价值高但纯开源托管难以为继，new-api 切入应以网关协同（计费+路由+密钥托管）而非再造沙箱为定位**。2/3/4/2
- Pake：Tauri 注入式定制（auth.js 保持登录/style.js 改皮肤）；new-api 保持 Electron 但可借鉴窗口状态持久化+启动兜底细节。2/3/4/3
- tabby：ee/ 目录开源/商业分层（商业化可参考目录级分层而非分叉）。1/2/5/2
- camofox：反检测在 C++ 层——UA 层反爬无意义

## 汇总一：CUA/沙箱流量对网关的要求清单（12 条）
1. 超大单报文（base64 截图数 MB）→ 提高请求体上限避免全文缓冲两次拷贝
2. 长连接/流式（WebSocket 指令+SSE+PTY）→ 稳定 SSE 透传+心跳+断线续传
3. 高频短请求扇出 → "步进式"配额与按会话聚合速率窗口
4. vision 参数透传（vision_detail_level/llm_screenshot_size）+ usage 如实反映 image tokens
5. **usage 中 image 与 text token 分列**（对账刚需，粒度不足致高价值客户流失）
6. **定价数据可编程获取**（/pricing 镜像+自定义计价覆盖→成为 agent 定价源形成锁定）
7. **会话级预算熔断**（screencast 单截图 100 倍流量）
8. 任务级/会话级账单聚合（按 task_id/header 追踪，一个任务=上百请求）
9. CUA 请求对首 token 延迟敏感→渠道选择给 vision 请求优先级权重
10. **密钥引用而非传递**（凭据保险库+sentinel）
11. egress FQDN 白名单（合规底线）
12. 暂停/恢复与快照（rootfs→OCI+AutoPause）

## 汇总二：沙箱隔离技术路线对比

| 维度 | 进程级（OS 原语） | 容器级 | 微 VM | 进程内解释器 |
|---|---|---|---|---|
| 代表 | srt、cua-driver | OpenSandbox Docker、camofox | CubeSandbox RustVMM/KVM、gVisor/Kata/Firecracker | new-api jsplugin（sobek） |
| 冷启动 | ~ms | 百 ms-秒 | 50-90ms（预热线池 <50ms） | 零 |
| 隔离强度 | 中（srt-win 接近容器级） | 中-高 | 最高（内核边界） | 低-中（共享崩溃域） |
| GUI/设备 | 可（AX/UIA） | 可（Xvfb/VNC） | 可（VNC） | 不可 |
| Windows | srt-win 最完整 | 弱 | 弱 | 现状即此 |
| 适用 | MCP 工具/插件命令/CUA 驱动 | 浏览器农场/代码执行 | 多租户 SaaS 沙箱 | 请求级 hook |

**结论**：jsplugin 维持进程内解释器处理请求级 hook（5s 超时定位正确），不承担长任务——长任务扩展路径：先进程级（srt 模式跨平台零依赖）→容器级（池化+rootfs 快照，OpenSandbox OSEP 蓝图）→微 VM 仅多租户 SaaS 化时考虑（CubeSandbox 为 E2B 兼容首选）。
**CUA 增值最小可行组合**：vision token 明细分列（计费）+会话级预算熔断（风控）+凭据引用（安全）+/pricing 镜像（锁定）。
