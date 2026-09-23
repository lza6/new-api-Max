# UX 交互反馈盘点表（UX Interaction Ledger）

> 生成：2026-09-24 · 只读审计（代码证据）+ 部分标注「待浏览器实测」。
> 范围：web/src/features 下 20 个最高价值 feature 的关键交互：是否有
> loading / 成功 / 失败 / 空态反馈 + 证据行号 + 缺口优先级。
> 更新纪律：改到相关组件后回填本表；浏览器截图证据入 `计划书/e2e-evidence/`。

## 结论摘要
- **强规范已普遍落地**：`handleServerError` + `getFriendlyErrorMessage`（人话错误映射）、
  sonner toast（成功/失败）、Skeleton/loading 态、空态组件在多 feature 广泛使用。
- **主要缺口（P1）**：部分列表页加载态用「整体 spinner」而非行级 skeleton；弹窗提交按钮
  缺 `disabled` 防双击；移动断点（375px）未逐页实测。
- **主要缺口（P2）**：个别 feature 空态文案未 i18n；键盘焦点顺序在复杂弹窗未逐项验证。

## Feature 盘点

### 1. auth（登录/注册/找回/2FA/Passkey）
- 加载：表单提交中按钮 loading/disabled（forgot-password-form.tsx、login 表单）。
- 成功：toast.success（forgot-password-form.tsx:85「Reset email sent」）。
- 失败：handleServerError + getFriendlyErrorMessage（forgot-password-form.tsx:87）。
- 空态：N/A（表单场景）。
- 缺口：登录失败统一文案防枚举已由后端实现；前端无额外缺口。

### 2. keys（API 密钥 CRUD）
- 加载：表格 Skeleton + 删除/批量删除按钮 loading。
- 成功：toast.success（api-keys-delete-dialog.tsx:51、api-keys-multi-delete-dialog.tsx:56）。
- 失败：handleServerError（api-keys-delete-dialog.tsx:55/58）。
- 空态：api-keys-table.tsx 空态渲染。
- 缺口：无 P0；删除二次确认弹窗已存在。

### 3. channels（渠道管理）
- 加载：渠道列表 Skeleton；健康分/冷却徽章异步。
- 成功：保存渠道 toast。
- 失败：handleServerError。
- 空态：渠道列表空态。
- 缺口：无 P0；健康分路由新增后前端概览卡（T2-4）待补。

### 4. usage-logs（消费日志）
- 加载：日志表格 Skeleton；费用明细面板 loading（CostDetailPanel）。
- 成功：N/A（只读）。
- 失败：handleServerError + 费用明细失败静默隐藏（优雅降级）。
- 空态：日志列表空态。
- 缺口：无 P0。

### 5. wallet（钱包/充值/提现）
- 加载：userLoading / paymentLoading / topupLoading（wallet/index.tsx:65-115）。
- 成功：充值成功 toast。
- 失败：handleServerError。
- 空态：recharge-form-card 空态。
- 缺口：支付中按钮防双击需实测（paymentLoading 已设）。

### 6. task-plugins（任务插件市场）
- 加载：marketplace-panel / install-dialog 加载态。
- 成功：安装成功 toast；完整性校验 UI（plugin-integrity-check）。
- 失败：URL 校验失败提示；handleServerError。
- 空态：插件列表空态。
- 缺口：无 P0。

### 7. web-protection（Web 防护 + 服务器状态）
- 加载：ServerStatsCard 1.5s 轮询，首载无数据返回 null（不渲染）。
- 成功：设置保存 toast。
- 失败：getServerStats catch 静默（undefined）。
- 空态：设置页空态。
- 缺口：P2——状态页首载无数据时显示骨架而非整卡隐藏（ServerStatsCard 若 !s return null）。

### 8. security（账户安全）
- 加载：2FA/Passkey 设置对话框 loading。
- 成功：启用成功 toast。
- 失败：handleServerError。
- 空态：无。

### 9. profile（个人资料）
- 加载：profile-header 加载。
- 成功：保存 toast。
- 失败：handleServerError。
- 空态：无。

### 10. profile-insights（用量洞察）
- 加载：图表 loading。
- 成功：N/A。
- 失败：handleServerError。
- 空态：无数据图表空态。

### 11-20（dashboard/rankings/redemption-codes/models/model-pricing/pricing/subscriptions/system-settings/docs/home）
- 抽查结论：dashboard 有 performance-health-panel + 空态；rankings 列表空态；models 表格 Skeleton；
  system-settings 保存 toast；docs 目录空态。均为既有规范模式，无 P0 缺口。

## 移动端断点（待浏览器实测）
- 关键页：登录、渠道、日志、钱包、任务插件。
- 375/768/1280 断点截图待 Playwright 实测后入 `计划书/e2e-evidence/browser-e2e-ux-*/`。

## a11y（自动化）
- 已有 `button-touch-target.test.tsx`（触摸目标尺寸）。
- 缺口：axe-core 关键页扫描未接入（T7 C3，P2）。

## 缺口优先级汇总
| 优先级 | 缺口 | 状态 |
|---|---|---|
| P1 | 列表页行级 skeleton 未全量覆盖 | 待补（渐进） |
| P1 | 支付/提交按钮防双击 | 待实测确认 |
| P2 | 状态页首载骨架 | 待补 |
| P2 | axe-core a11y 扫描 | 待补（T7 C3） |
| P2 | 移动断点截图证据 | 待 Playwright |