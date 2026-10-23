# v1.3.186 视觉批次 3 — E2E 证据

**执行时间**：2026-10-22 13:20–13:30（本地）
**环境**：本地后端 `new-api-e2e.exe`（SQLite `tmp-e2e/v186.db`，PORT=3000）+ `rsbuild dev`（PORT=5173）
**脚本**：`web/e2e-local/visual-baseline.cjs`（本轮新增，V12）

## 运行命令

```bash
BASE=http://127.0.0.1:5173 USER_PASS=*** \
  PAGES="/dashboard/overview:dashboard-overview,/keys:keys" \
  PRESETS="default,anthropic,ink" WIDTHS="1440,320" \
  node e2e-local/visual-baseline.cjs
```

## 结果

| 项 | 结果 |
|---|---|
| 截图 | 18 张（2 页 × 3 预设 × {1440,320} 亮色 + 1440 暗色） |
| axe（WCAG 2.1 A/AA） | default 0 / ink 0；**anthropic 修复前 1 节点** → 已修（见下） |
| console error | 1 条（登录前 401，良性） |
| `ink` 预设 | 石墨中性 + 黑色主按钮；圆角/密度收至 `sm`；`/keys` 与概览 axe 0 |

## 预设对比度扫描（11 套 × 2 页）

| 预设 | `/keys` | `/dashboard/overview` |
|---|---|---|
| default / simple-large / underground / rose-garden / lake-view / sunset-glow / forest-whisper / ocean-breeze / lavender-dream / ink | 0 | 0 |
| **anthropic** | 修复前 1（白字 on `#e37756` = **2.9:1**）→ 修复后 **0** | 同上 |

修复方式：`--primary: oklch(0.685 0.142 38)` → `oklch(0.57 0.128 38)`（同色相加深，白字达 AA）。
修复后亮/暗各复测一次：`violations=0`（截图 `anthropic-light-after-fix.png` / `anthropic-dark-after-fix.png`）。

## 截图清单

| 文件 | 说明 |
|---|---|
| `dashboard-overview-1440-default-light.png` | 默认预设基线 |
| `dashboard-overview-1440-anthropic-light.png` | 暖奶油 + 加深陶土 |
| `dashboard-overview-1440-ink-light.png` | **ink 结构差异套** |
| `keys-1440-ink-light.png` | 表格 + 空态在 ink 下 |
| `anthropic-{light,dark}-after-fix.png` | 对比度修复后复测 |
| `results.json` | 全部截图元数据 + 每页 axe 结果 |

## 未取得 / 边界

1. 生产浏览器截图未取（本机直连该域名 TLS 被中间设备 RST）；线上验证用服务器侧 curl + 产物核对。
2 . 本地空库，多数读数为 0；真实数据形态留待后续批次。
3. 暗色只覆盖 1440 一档；320 暗色未测。
