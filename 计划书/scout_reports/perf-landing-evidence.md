# perf-landing-evidence — 听风AI 站点体验与性能优化（Landing Vibe + 首屏性能）证据简报

- 采集时间: 2026-09-20（本地仓库只读采集；未改动代码、未做 git 写操作）
- 仓库: C:\Users\Administrator.DESKTOP-EGNE9ND\Desktop\公益服务网关\new-api
- git: 分支 main，ahead upstream/main 130、behind 0；porcelain 未跟踪项仅 `.codex/` 与 `计划书/前端体验与响应式优化计划.md`

## 1. Landing 现状（web/src/features/home）

全部文件与行数（Get-Content 实测）：
- C:\Users\Administrator.DESKTOP-EGNE9ND\Desktop\公益服务网关\new-api\web\src\features\home\index.tsx — 133 行
- C:\Users\Administrator.DESKTOP-EGNE9ND\Desktop\公益服务网关\new-api\web\src\features\home\api.ts — 39 行
- C:\Users\Administrator.DESKTOP-EGNE9ND\Desktop\公益服务网关\new-api\web\src\features\home\constants.ts — 147 行
- C:\Users\Administrator.DESKTOP-EGNE9ND\Desktop\公益服务网关\new-api\web\src\features\home\types.ts — 39 行
- C:\Users\Administrator.DESKTOP-EGNE9ND\Desktop\公益服务网关\new-api\web\src\features\home\hooks\use-home-page-content.ts — 82 行
- C:\Users\Administrator.DESKTOP-EGNE9ND\Desktop\公益服务网关\new-api\web\src\features\home\hooks\index.ts — 19 行
- C:\Users\Administrator.DESKTOP-EGNE9ND\Desktop\公益服务网关\new-api\web\src\features\home\lib\icon-mapper.tsx — 59 行
- C:\Users\Administrator.DESKTOP-EGNE9ND\Desktop\公益服务网关\new-api\web\src\features\home\components\index.ts — 23 行
- C:\Users\Administrator.DESKTOP-EGNE9ND\Desktop\公益服务网关\new-api\web\src\features\home\components\hero-terminal-demo.tsx — 548 行
- C:\Users\Administrator.DESKTOP-EGNE9ND\Desktop\公益服务网关\new-api\web\src\features\home\components\connection-line.tsx — 39 行
- C:\Users\Administrator.DESKTOP-EGNE9ND\Desktop\公益服务网关\new-api\web\src\features\home\components\feature-item.tsx — 47 行
- C:\Users\Administrator.DESKTOP-EGNE9ND\Desktop\公益服务网关\new-api\web\src\features\home\components\gateway-card.tsx — 75 行
- C:\Users\Administrator.DESKTOP-EGNE9ND\Desktop\公益服务网关\new-api\web\src\features\home\components\hero-buttons.tsx — 53 行
- C:\Users\Administrator.DESKTOP-EGNE9ND\Desktop\公益服务网关\new-api\web\src\features\home\components\icon-card.tsx — 47 行
- C:\Users\Administrator.DESKTOP-EGNE9ND\Desktop\公益服务网关\new-api\web\src\features\home\components\scrolling-icons.tsx — 63 行
- C:\Users\Administrator.DESKTOP-EGNE9ND\Desktop\公益服务网关\new-api\web\src\features\home\components\stat-item.tsx — 60 行
- C:\Users\Administrator.DESKTOP-EGNE9ND\Desktop\公益服务网关\new-api\web\src\features\home\components\sections\hero.tsx — 245 行
- C:\Users\Administrator.DESKTOP-EGNE9ND\Desktop\公益服务网关\new-api\web\src\features\home\components\sections\features.tsx — 241 行
- C:\Users\Administrator.DESKTOP-EGNE9ND\Desktop\公益服务网关\new-api\web\src\features\home\components\sections\stats.tsx — 128 行
- C:\Users\Administrator.DESKTOP-EGNE9ND\Desktop\公益服务网关\new-api\web\src\features\home\components\sections\how-it-works.tsx — 90 行
- C:\Users\Administrator.DESKTOP-EGNE9ND\Desktop\公益服务网关\new-api\web\src\features\home\components\sections\cta.tsx — 84 行

hero.tsx 蓝色系品牌色位置：
- web\src\features\home\components\sections\hero.tsx:107 — `text-blue-600` / `border-blue-500/20` / `bg-blue-500/5` / `dark:text-blue-400`（顶部 badge）
- hero.tsx:111 — `bg-blue-400`（animate-ping 光环）
- hero.tsx:112 — `bg-blue-500 dark:bg-blue-400`（状态点）
- hero.tsx:123 — `from-blue-400 via-violet-400 to-purple-500`（标题渐变文字）
- hero.tsx:219 — `bg-blue-500/10` + `text-blue-600 dark:text-blue-400`（图标块）

theme-customization.ts 品牌色注释：
- C:\Users\Administrator.DESKTOP-EGNE9ND\Desktop\公益服务网关\new-api\web\src\lib\theme-customization.ts:33-34 — “Inspired by Anthropic's official brand language: warm cream canvas (#faf9f5) paired with clay/coral (#d97757) as the single accent.”
## 2. 性能证据（web/dist 生产产物，bytes 实测）

web\dist\static\js\ 顶层：
- index.a86c829ec5.js — 4,295,965
- 9298.7629f8b113.js — 932,345
- vendor-ui-primitives.3ac43eace1.js — 344,568
- lib-react.064bab1680.js — 189,031
- vendor-tanstack.41b48bbfab.js — 178,357
- 该目录 .js 合计 5,940,266（不含 LICENSE.txt）

web\dist\static\js\async\（共 116 个 .js，合计 51,485,053 B）Top10：
1. 9926.350cbf223b.js — 6,827,320
2. 1522.80d09b18dd.js — 5,470,182
3. 9197.6b3bf7a7e7.js — 5,371,624
4. 3898.ebb33f5002.js — 5,088,125
5. 8496.d136c3a3a9.js — 3,164,321
6. 5143.003cfdc535.js — 2,147,792
7. 240.8dd4101a83.js — 2,119,826
8. 9243.b5c19ac00c.js — 2,066,096
9. 3239.33fd36c243.js — 1,961,944
10. 4151.99268a3b97.js — 1,664,491

大库归属（对产物内容标记扫描 rg -a -l）：
- shiki + codemirror → async/3898.ebb33f5002.js（5.09MB）；codemirror 另见 async/7124.65a9269f85.js
- visactor/vchart → async/9243.b5c19ac00c.js（2.07MB）
- recharts → async/9045.376a7c2509.js（398,029B）+ async/9637.e56d43278d.js（11,313B）
- hugeicons → static/js/9298.7629f8b113.js（932,345B）
- Top1-3/5（9926、1522、9197、8496）minified 产物无以上库名标记，归属：未验证

web\rsbuild.config.ts：
- L6 — import { pluginTailwindcss } from '@rsbuild/plugin-tailwindcss'
- L27 — plugins: [pluginReact(), pluginTailwindcss({ optimize: false })]
- L29-54 — splitChunks：L30 preset: 'default'；L31-52 cacheGroups：vendor-react（L32-38）、vendor-ui-primitives（L39-45）、vendor-tanstack（L46-52），均 priority 0 + enforce true
- L92-97 — tanstackRouter 插件；L96 — autoCodeSplitting: isProd（dev 关 / prod 开）
- L86 — removeConsole: isProd ['log']；L87 — buildCache: false

web\package.json 关键依赖版本：
- @codemirror/lang-javascript ^6.2.5（L28）、lang-markdown ^6.5.0（L29）、language ^6.12.4（L30）、state ^6.7.0（L31）、view ^6.43.4（L32）
- @hugeicons/react ^1.1.9（L37）
- @visactor/react-vchart ^2.1.2（L44）、@visactor/vchart ^2.1.2（L45）
- motion ^12.42.2（L60）
- recharts 3.9.1（L73）
- shiki ^4.3.0（L74）
- tailwindcss ^4.3.2（L79）、@rsbuild/plugin-tailwindcss ^2.0.3（L91）

@hugeicons/react 引用：web\src 下 30 个文件引用（rg -l 计数 = 30；rg -o 总命中 = 30）。

React.lazy 用法：
- C:\Users\Administrator.DESKTOP-EGNE9ND\Desktop\公益服务网关\new-api\web\src\features\dashboard\index.tsx — L80、86、92、98、104、110（LogStatCards / ModelCharts / ConsumptionDistributionChart / PerformanceOverview / UserCharts / FlowCharts）
- C:\Users\Administrator.DESKTOP-EGNE9ND\Desktop\公益服务网关\new-api\web\src\features\channels\components\dialogs\status-code-risk-dialog.tsx — L29（Markdown lazy）
## 3. index.html / public / serviceWorker

C:\Users\Administrator.DESKTOP-EGNE9ND\Desktop\公益服务网关\new-api\web\index.html（25 行，逐行实测）：
- L2 — <html lang="en">
- L5 — link rel="icon" href="/logo.png"
- L6 — meta viewport
- L7 — meta google notranslate
- L10 — <title>new-api-Max</title>；L11 — meta title；L12-15 — meta description "Unified AI API gateway and admin dashboard."
- L17 — meta theme-color content="#fff"
- preconnect / preload / OG / JSON-LD / canonical：无（全文仅上述标签）

web\public\ 文件清单（无 robots.txt、无 sitemap.xml）：
favicon.ico、logo.png、model-test.html、pay-apple.png、pay-card.png、pay-google.png、waffo-logo-dark.svg、waffo-logo-light.svg

serviceWorker：
- C:\Users\Administrator.DESKTOP-EGNE9ND\Desktop\公益服务网关\new-api\web\src\main.tsx — 118 行；rg 无 serviceWorker / registerSW / workbox 命中 → 无注册（线上行为：未验证）

## 4. i18n

locales（C:\Users\Administrator.DESKTOP-EGNE9ND\Desktop\公益服务网关\new-api\web\src\i18n\locales\，7 个）：
en.json（469,612B）、fr.json（528,765B）、ja.json（563,988B）、ru.json（676,968B）、vi.json（544,159B）、zh-TW.json（455,783B）、zh.json（455,017B）

同步脚本：
- web\package.json L23 — "i18n:sync": "node scripts/sync-i18n.mjs"
## 5. 已有计划/证据（计划书/）

- C:\Users\Administrator.DESKTOP-EGNE9ND\Desktop\公益服务网关\new-api\计划书\8x-前端体验专项审计.md — 存在（2,853B），已被 git 跟踪（git ls-files 命中，非未跟踪）
- C:\Users\Administrator.DESKTOP-EGNE9ND\Desktop\公益服务网关\new-api\计划书\前端体验与响应式优化计划.md — 存在（10,056B），git 未跟踪（porcelain 显示 ??）
- 其他 md：参考的结果计划指南.md、存量违反台账.md、上游首字慢与400根因诊断.md、下一步改进指南.md、线上版本卡v1.2.3根因审计.md、B2-3-方案.md、B4-1-任务事件流SSE方案.md、B5-2-计费现代化方案.md、N24-分析报告.md、v1.2 规划草案.md、workflow_status.md

计划书\e2e-evidence\ 截图/证据清单：
- b13_web_protection_sidebar.png
- b31_after_login_dashboard.png、b31_insights_page.png、b31_profile_insights_entry.png
- b41_task_event_stream.png
- b55_tool_setup_page.png
- b6-2\badkey.png、b6-2\content.png、b6-2\overquota.png、b6-2\ratelimit.png、b6-2\updown.png（+ b6-2\report.json）
- p1-p2\p1-checkin.png、p1-p2\p2-profile.png
- t2-t5\t4-dashboard.png、t2-t5\t4-timeline.png、t2-t5\t6-ui-switch.png（+ t2-t5\验收报告.md、t2-t5\全局并发桶T6验收.md）
- user-source\users-source.png（+ user-source\README.md）
- 真实E2E验收报告.md

## 6. git 状态

- 当前分支：main；上游：upstream/main；ahead 130 / behind 0（git rev-list 实测：@{u}..HEAD = 130、HEAD..@{u} = 0）
- git status --porcelain 未跟踪项（仅两条）：?? .codex/ ；?? 计划书/前端体验与响应式优化计划.md
- 8x-前端体验专项审计.md 为已跟踪文件（非未跟踪）

## 未验证项汇总

- 异步 Top 大包 9926 / 1522 / 9197 / 8496 的库归属（minified 无库名标记）
- robots.txt / sitemap 线上可访问性（本地 web\public\ 无对应文件）
- 线上真实首屏 LCP / 传输体积（本次仅本地静态产物，未做线上实测）
- serviceWorker 线上行为