# 交互反馈盘点表刷新（只读审计产物 · 2026-09-26）

> 审计范围：`web/src/features` 当前 31 个 features（实际目录清单），按操作 → 加载/成功/失败/空态反馈现状 → 证据（文件:行号） → 缺口 → 优先级(P0/P1/P2) 逐项盘点。
> 依据：`web/AGENTS.md` 3.9 错误处理（强制 `handleServerError`/`getFriendlyErrorMessage`）、3.6 API、3.7 表单、加载/空态/touch target 规范；`计划书/docs/t7-ux-a11y-path.md` 与 `计划书/下一步改进指南.md` T7 内容（旧版 `计划书/audit/ux-interaction-ledger.md` 在 HEAD 已被删除，本表为刷新版）。
> 说明：静态只读核对（未运行浏览器/测试）；「L=加载 / S=成功 / E=失败 / 空=空态」列以 √/- 标记；查询类分页表格共享 `DataTable` 的 `isLoading`/`emptyTitle` 实现，全局兜底见第 0 节。

### 0. 全局基础设施
| Feature | 证据 | 说明 |
|---|---|---|
| 全局错误兜底 | `web/src/lib/handle-server-error.ts:38-73` | sonner toast + WeakSet 去重 + 人话映射（B6-2） |
| HTTP 401 会话 | `web/src/lib/http-client.ts:56-113` | 401 自动刷新令牌，失败 toast+跳 /sign-in |
| 列表加载/空态 | `web/src/components/data-table/core/data-table-view.tsx` + 各表 `emptyTitle` | DataTable 统一 isLoading + emptyTitle |

### 1. 按 feature 逐项（31 个实际目录）

| # | Feature | 操作 | 加载 | 成功 | 失败 | 空态 | 证据 | 缺口 | 优先级 |
|---:|---|---|:--:|:--:|:--:|:--:|---|---|---|
| 1 | about | 加载关于信息 | √ Skeleton | -（无写操作） | × 无 isError 分支 | - | `web/src/features/about/index.tsx:118-135` | 数据加载失败无错误提示 | P2 |
| 2 | auth | 邮箱密码登录 | √ disabled 按钮 | √ toast.success(Welcome back!) | √ handleServerError x4 | n/a | `web/src/features/auth/sign-in/components/user-auth-form.tsx:180-186,310-333` | OAuth callback 页除文案外无失败重试 UI | P1 |
| 2b | auth | OAuth/Telegram 登录 | √ 按钮 disabled | √ 各 hook toast | √ handleServerError | n/a | `web/src/features/auth/components/oauth-providers.tsx:162-168`、`use-oauth-login.ts:97-166`、`oauth-callback-screen.tsx:80` | 弹窗无 spinner；callback 无重试 | P1 |
| 2c | auth | 注册 | √ 按钮 isLoading | √ toast.success | √ handleServerError+校验 toast | n/a | `web/src/features/auth/sign-up/components/sign-up-form.tsx:174-180` | - | P2 |
| 2d | auth | 忘记密码 | √ Loader2 | √ toast.success(Reset email sent) | √ handleServerError | n/a | `web/src/features/auth/forgot-password/components/forgot-password-form.tsx:85-93,124-129` | - | P2 |
| 2e | auth | OTP/Passkey | √ 按钮 disabled | √ toast.success | √ handleServerError/设备不支持 | n/a | `web/src/features/auth/otp/components/otp-form.tsx:78-81`、`user-auth-form.tsx:242-294` | 设备不支持仅 toast | P2 |
| 3 | channels | 创建/编辑渠道 | √ drawer isSubmitting + section Skeleton | √ toast.success(hook onSuccess) | √ handleServerError | √ emptyTitle No Channels Found | `use-channel-mutate-form.ts:112-117`、`channel-mutate-drawer.tsx:1334-1360,1568-1633`、`channels-table.tsx:417-419`、`channel-editor-loading-state.tsx:38-42` | - | P2 |
| 3b | channels | 渠道测试 | √ toast.loading 进度+isDeletingFailed | √ 逐项 success | √ toast.error/handleServerError | √ emptyContent | `channel-test-dialog.tsx:323-382,701-799,1113-1137` | - | P2 |
| 3c | channels | 拉取模型 | √ 按钮 loading | √ toast.success(Fetched N models) | √ handleServerError | - | `fetch-models-dialog.tsx:152-198` | - | P2 |
| 4 | chat | 聊天预设跳转 | - | - | - | - | `web/src/features/chat/hooks/use-chat-presets.ts` | 无 UI 交互点；跳转 FluentRead 在 keys 侧有 toast | P2 |
| 5 | dashboard | 加载+复制 | √ panel Skeleton | √ toast.success(Copied) | √ handleServerError(复制) | √ 公告/FAQ/Uptime empty | `overview-dashboard.tsx:303-319`、`announcements-panel.tsx:75-76`、`faq-panel.tsx:52-53`、`uptime-panel.tsx:109-110`、`flow-charts.tsx:663` | chart 区错误态未单独渲染 | P2 |
| 6 | docs | 展示文档 | - | - | - | - | `web/src/features/docs/index.tsx` | 纯展示+CopyButton | - |
| 7 | errors | 错误页 | - | - | - | - | `web/src/features/errors/*.tsx` | 静态错误页 | P2 |
| 8 | home | 站点统计 | √ Skeleton | - | × 空态 fallback 注释 | √ API 不可用空态 | `web/src/features/home/components/site-stats.tsx:21-35` | hero 图片 onError 静默 | P2 |
| 9 | keys | API 密钥 CRUD/复制/状态 | √ DataTable isLoading+移动 Skeleton | √ toast.success x3 | √ handleServerError | √ emptyTitle No API Keys Found | `api-keys-mutate-drawer.tsx:294-332`、`api-keys-table.tsx:113,345-347,375`、`api-keys-delete-dialog.tsx:51-58`、`data-table-row-actions.tsx:108-162,229-247` | - | - |
| 10 | legal | 协议/隐私 | √ Skeleton | - | × 失败按空态渲染 | √ message/外链按钮 | `legal-document.tsx:58-83` | 失败与空未区分 | P2 |
| 11 | model-pricing | 定价编辑 | √ LoadingState+isSaving | √ toast.success | √ handleServerError | - | `model-pricing-panel.tsx:98-100,119,131,256` | - | P2 |
| 12 | models | 模型 CRUD/切换/同步/批处理 | √ drawer LoadingState+isPending、表 isLoading | √ toast.success x14 | √ handleServerError（model-actions 全批量分支） | √ emptyTitle x4+EmptyText | `model-mutate-drawer.tsx:223-232,306-307`、`data-table-row-actions.tsx:88-118`+`lib/model-actions.ts:88`、`models-table.tsx:239-241`、`vendor-operation-dialog.tsx:178`、`missing-models-dialog.tsx:141-156` | - | - |
| 13 | onboarding | 引导 | - | - | - | - | `web/src/features/onboarding/components/onboarding-guide.tsx` | 纯本地 guide | - |
| 14 | performance-metrics | 数据 API | - | - | - | - | `web/src/features/performance-metrics/{api,types}.ts` | 无 UI | - |
| 15 | playground | 发消息/停止/重生成/编辑/清空 | √ isGenerating+isLoadingMessages+isModelLoading | √ toast.success(Conversation cleared) | √ handleServerError+消息内错误行 | √ PlaygroundEmptyState | `index.tsx:65-110`、`use-chat-handler.ts:222-237,322-331`、`playground-chat.tsx:198-206`、`playground-input-tools.tsx:92` | 删除消息无确认；流错误 toast 后无内联重试入口 | P2 |
| 16 | pricing | 筛选/搜索/详情 | √ LoadingSkeleton | - | × usePricingData.error 未用于 UI | √ EmptyState(搜索) | `index.tsx:52-151`、`pricing-table.tsx:94-95`、`use-pricing-data.ts:30-75` | 查询失败当成空列表，无错误/重试 | P1 |
| 17 | profile | 签到/语言/侧栏/通知 | √ Skeleton x2 | √ toast.success x4 | √ handleServerError x4 | √ insights 空态 | `checkin-calendar-card.tsx:149-169`、`language-preferences-card.tsx:85-94`、`sidebar-modules-card.tsx:194-212`、`notification-tab.tsx:84-90,342`、`profile-header.tsx:52-72` | - | - |
| 18 | profile-insights | 图表 | √ Skeleton | - | × 仅 No data yet 空态 | √ EmptyState | `web/src/features/profile-insights/index.tsx:149,162-175,243,312` | 错误未区分 | P2 |
| 19 | rankings | 排行榜 | √ Skeleton x3+bandwidth | - | √ RankingsError | √ bandwidth 空态 | `index.tsx:62-127`、`bandwidth-section.tsx:51-68` | 覆盖完整 | - |
| 20 | redemption-codes | 码 CRUD/批量/清失效 | √ ConfirmDialog isLoading+Drawer | √ toast.success(含 count) | √ handleServerError | √ emptyTitle+description | `redemptions-primary-buttons.tsx:45-53`、`redemptions-mutate-drawer.tsx:188-222`、`data-table-bulk-actions.tsx:69-81`、`redemptions-table.tsx:166-167` | - | - |
| 21 | security | 改密/绑定/2FA/Passkey/会话/令牌/删号 | √ dialog isLoading/Skeleton | √ toast.success x7+ | √ handleServerError/AuthOperationError | √ 会话 ui/empty | `change-password-dialog.tsx:136-145`、`account-bindings.tsx:202-210`、`passkey-card.tsx:86-105`、`access-token-card.tsx:49-74,184`、`login-sessions-card.tsx:96-127` | - | - |
| 22 | setup | 初始化向导 | √ LoadingState | √ toast.success(System initialized!) | √ handleServerError+isError | √ 非加载分支 | `setup-wizard.tsx:111-132,367-394` | - | P2 |
| 23 | subscriptions | 套餐 CRUD/启停/重置/购买 | √ isPending/isLoading | √ toast.success x8 | √ handleServerError x10 | √ emptyTitle+desc | `subscriptions-mutate-drawer.tsx:128-186`、`toggle-status-dialog.tsx:49-58`、`reset-subscriptions-dialog.tsx:51-62`、`subscription-purchase-dialog.tsx:112-155`、`subscriptions-table.tsx:57-59`、`user-subscription-rate-limit-section.tsx:181-269` | 无删除套餐入口（仅启停/重置） | P2 |
| 24 | system-info | 实例/任务面板 | √ Skeleton+ErrorState | √ 删除/清理 toast | √ handleServerError | √ 任务空态 | `system-tasks-panel.tsx:228-294`、`system-instances-panel.tsx:539-573,583` | 实例查询错误态未独立 | P2 |
| 25 | system-settings | 各类配置保存 | √ 统一 isSaving/isPending | √ 统一 toast.success(Setting updated) | √ 统一 handleServerError+JSON 内联错误 | √ emptyContent 多处 | `hooks/use-update-option.ts:41-59`、`content/announcements-section.tsx:186-278`、`content/api-info-section.tsx:162-313`、`integrations/waffo-settings-section.tsx:104-126,332-333`、`models/model-ratio-visual-editor.tsx:574-603`、`general/quota-settings-section.tsx:173,335` | 混合保存模型（部分草稿+Save、部分 auto-save）toast 语义不一致 | P2 |
| 26 | task-plugins | 上传/安装/激活/启停/删除/干跑/市场源 | √ isPending+表 isLoading+Skeleton | √ toast.success x7 | √ handleServerError x10 | √ emptyTitle+detail Empty | `upload-dialog.tsx:84-114,158-166`、`marketplace-install-dialog.tsx:168-199`、`plugin-detail-sheet.tsx:109-143,232-312`、`plugins-table.tsx:95-140,402-405`、`marketplace-panel.tsx:122-168` | 列表内部分按钮仅禁用无 spinner | P2 |
| 27 | tool-setup | 工具接入 | - | √ CopyButton toast | - | - | `web/src/features/tool-setup/tool-integration-section.tsx` | - | - |
| 28 | usage-logs | 列表/过滤/详情/审计 | √ DataTable isLoading+audit isPending+artifacts Skeleton | √ 复制 toast | √ handleServerError+media onError | √ emptyTitle x3+emptyContent | `usage-logs-table.tsx:168-198`、`audit/audit-log-viewer.tsx:113-115`、`subscription-logs-table.tsx:183-185`、`task-artifacts.tsx:323-363`、`dialogs/user-info-dialog.tsx:63-66`、`details-dialog.tsx:1491-1497` | 部分媒体预览失败仅图标/骨架无文字 | P2 |
| 29 | users | 用户 CRUD/批量/绑定/限速 | √ DataTable isLoading+Processing... | √ toast.success x8 | √ handleServerError x15 | √ emptyTitle No Users Found | `users-table.tsx:209-211`、`data-table-bulk-actions.tsx:146-198,290-324`、`user-quota-dialog.tsx:88-97`、`users-delete-dialog.tsx:43-50`、`users-mutate-drawer.tsx:192-218` | - | - |
| 30 | wallet | 充值/转账/兑换/补单/订阅 | √ Skeleton+loadingKey | √ toast.success(Redirecting/Transfer/Redemption/Order completed) | √ handleServerError x8 | × 计划列表空时隐藏区无空态文案 | `hooks/use-payment.ts:130-153`、`hooks/use-redemption.ts:38-68`、`hooks/use-affiliate.ts:54-80`、`hooks/use-billing-history.ts`、`recharge-form-card.tsx:148-191`、`subscription-plans-card.tsx:143-195,246-256` | 支付失败无 retry 按钮；affiliate 空载直接无内容 | P2 |
| 31 | web-protection | 封禁/解封+保存 | √ setSaving/setIsLoading | √ toast.success x4 | √ handleServerError | × 封禁列表 rows.length===0 无 EmptyState | `web-protection-page.tsx:185-186,274,338-368` | 空列表无文案 | P2 |
