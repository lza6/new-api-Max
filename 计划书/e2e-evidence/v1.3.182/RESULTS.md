# v1.3.182 视觉批次 1 — E2E 证据

**执行时间**：2026-10-22（本地时间 11:36–11:45）
**环境**：本地后端 `new-api-e2e.exe`（SQLite `tmp-e2e/v182.db`，PORT=3000）+ 前端 `rsbuild dev`（PORT=5173，proxy → 3000）
**脚本**：`web/e2e-local/v13182-visual-local.cjs`
**执行命令**：

```bash
BASE=http://127.0.0.1:5173 node e2e-local/v13182-visual-local.cjs
```

## 结果

| 项 | 结果 |
|---|---|
| 管理员登录（root） | 成功，落地 `/dashboard/overview` |
| 截图 | 10 张（`dashboard-overview` / `dashboard-models` / `keys` × 1440/768/320 亮色 + `dashboard-overview` 1440 暗色） |
| axe（WCAG 2.1 A/AA，真实页面注入） | **0 违规** |
| console error | 1 条（登录前的 `401 Unauthorized`，良性） |
| 溢出/触控 | 320px 无横向滚动；读数条降为 2 列并使用分隔线 |

## 浏览器内求值（令牌与排版实测）

```json
{
  "--radius": ".625rem",
  "--surface-1": "color-mix(in oklch, #0a0a0a 2%, #fff)",
  "--rule": "color-mix(in oklch, #0a0a0a 9%, transparent)",
  "--numeric-font": "\"JetBrains Mono Variable\", ui-monospace, …",
  "metric values": [
    { "text": "$0", "font": "JetBrains Mono Variable", "variant": "tabular-nums" }
  ],
  "rounded-2xl count on /dashboard/*": 0,
  "surface class count (/dashboard/overview)": 9,
  "body font": "\"Public Sans\", sans-serif"
}
```

## 截图清单

| 文件 | 说明 |
|---|---|
| `dashboard-overview-1440-light.png` | 概览首屏（试点页） |
| `focus-summary-strip.png` | 用量概览读数条（单色 sparkline、等宽数字、中性右栏） |
| `dashboard-models-1440-light.png` | 模型调用分析 |
| `focus-models-strip.png` | 5 格读数条（无粉彩图标底） |
| `dashboard-overview-1440-dark.png` | 暗色（表面派生令牌成立） |
| `*-768-*` / `*-320-*` | 平板 / 手机断点 |
| `keys-1440-light.png` | 圆角收敛后的控件观感（10px 控件 / 14px 容器） |

## 未取得 / 边界

1. **生产环境截图未取**（本机直连该域名的 TLS 会被中间设备 RST，见记忆库
   `tingfengai-domain-rst-20261010.md`）。部署后的生产验证改用服务器侧
   `curl`（见 `workflow_status.md`），不做本地浏览器直连。
2. 本地实例为空库，数值多为 0；本轮验证目标是**排版与层级**而非数据形态，
   真实数据形态下的读数条需在下一批（V4/V6 之后）补测。
3. 暗色仅覆盖 `dashboard-overview` 一页；其余页面暗色留待 V9 全量回归。
