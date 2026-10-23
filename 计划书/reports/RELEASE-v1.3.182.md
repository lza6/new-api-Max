# Release v1.3.182 — 视觉批次 1：静默仪表令牌层 + 概览读数条

- **提交**：`366f909e4`（tag `v1.3.182`）
- **基线**：`v1.3.181`
- **施工契约**：`计划书/视觉识别体系-反AI通用美学界面改造指南.md`（V0–V3）
- **部署**：2026-10-22 蓝绿上线，生产 `x-new-api-version: v1.3.182`，healthy，0 个 5xx

## 变更

1. **令牌层（V1）**：`--surface-1/2/3`、`--rule`、`--rule-strong`、`--grid-line/step`、
   `--accent-quiet*`、`--motion-fast/base/slow`、`--ease-quiet`、`--font-numeric`；全部由
   `--foreground/--background/--primary` 经 `color-mix(in oklch)` 派生 → 10 套颜色预设与亮/暗主题自动成立。
2. **圆角收敛（D2）**：`--radius` 1rem → 0.625rem；控件 ~10px、容器 ~14px、药丸 full。
3. **等宽数字（D3）**：自托管 `@fontsource-variable/jetbrains-mono`（latin 40.4 kB），只用于数字/ID/密钥。
4. **`Card.tone`（V2）**：`default`（原样）/ `panel` / `plain` / `inset`，用表面明度 + 规则线表达层级。
5. **dashboard 试点（V2/V3）**：读数条化、粉彩图标底退场、单色 sparkline、彩虹渐变改中性面、
   `rounded-2xl` 归零；新增 `Metric/MetricLabel/MetricValue` 读数排版件。
6. **死配置清理（D5）**：删除 `config/fonts.ts` 与 `context/font-provider.tsx`。

## 验证摘要

| 项 | 结果 |
|---|---|
| typecheck / oxlint（改动文件） / build / i18n / 术语 / 格式 | PASS |
| 定向 vitest（dashboard + ui + metric） | 54 + 5 PASS |
| 真实浏览器 E2E（本地后端，管理员登录） | 10 图，axe **0 违规** |
| 浏览器内实测 | `--radius=.625rem`；metric 值 `JetBrains Mono Variable` + `tabular-nums`；dashboard `rounded-2xl`=0；body 仍 `Public Sans` |
| 生产 | 200 / healthy / 0 5xx / 产物含新令牌与字体 / RestartCount=0 |
| 已知噪声 | `task-event-stream.test.tsx`、`delete-plan-dialog.test.tsx` 在整批并行下偶发超时，单跑全 PASS（见 `test_ledger.md`） |

## 回滚

```bash
ssh root@154.37.208.171 'cd /opt/new-api && ./deploy-zero-downtime.sh v1.3.181'
```

## 未完成（下一批）

V4 表格视觉统一 · V5 状态色令牌化存量清理 · V6 图表单色阶 · V7 三态视觉 ·
V8 动效规范 · V9–V11 页面覆盖 · V12 视觉基线脚本 · V13 `ink` 结构差异套。

---

# v1.3.185（视觉批次 2：V4/V5/V6 + V7）

- **提交**：`f7afe2ac6`（V4–V6）；`127749a52`（V7）
- **部署**：2026-10-22 蓝绿，生产 `x-new-api-version: v1.3.185`，healthy，0 个 5xx
- **回滚**：`./deploy-zero-downtime.sh v1.3.184`
- **新增**：表格微标签（11px + tracking）、data-table 外框 `surface-1 + border-rule`、
  `Alert` 的 success/warning/info 令牌变体、`lib/chart-palette.ts`（canvas 安全单色序列阶）、
  空态 `surface-grid` 工程网格 + `EmptyMedia` aria-hidden
- **验证**：typecheck / oxlint / build PASS；定向 vitest 81/81 + chart-palette 4/4 + empty-state 6/6；
  浏览器实测 canvas 颜色管线（`--primary` → `#146fa8` → `ctx.fillStyle`）与表头 11px
- **未完成**：V4b 数字列右对齐、V5 存量 `bg-*-50` 清扫、V6b `--chart-N` 令牌统一、V8–V13

---

# v1.3.186（视觉批次 3：V8 + V12 + V13）

- **提交**：`620e574df`（tag `v1.3.186`）
- **部署**：2026-10-22 蓝绿 3001，`x-new-api-version: v1.3.186`，healthy，0 ERROR / 0 panic / 0 restart
- **回滚**：`./deploy-zero-downtime.sh v1.3.185`
- **新增**：`.transition-quiet`（`transition-all` 41 处清零）、`ink` 石墨结构差异套预设
  （半径/密度提示 + 用户显式选择优先）、`e2e-local/visual-baseline.cjs` 可重跑视觉基线
- **修复**：`anthropic` 预设白字 on 陶土 `#e37756` 仅 **2.9:1** → primary 改 `oklch(0.57 0.128 38)`，
  亮/暗 axe 复测 0 违规；11 预设 × 2 页 color-contrast 全 0
- **验证**：typecheck / build PASS；vitest 163/163；基线 18 图；生产五项健康检查通过
- **事故**：清理临时 worktree 时 `rmdir /S` 顺 junction 删掉 `node_modules/.bin`，已 `bun install`
  恢复并重跑全部验证；GitHub HTTPS 推送短时中断 10 次后恢复
