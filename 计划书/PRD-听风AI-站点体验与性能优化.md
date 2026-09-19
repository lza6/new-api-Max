# PRD-听风AI-站点体验与性能优化（Landing Vibe 重设计 + 首屏性能优化）

- 版本：v1.0（草稿，待评审）
- 日期：2026-09-20
- 归属切片：P0/P1（自更大路线图切出，聚焦「听风AI」对外站点 Landing 与首屏体验）
- 关联文档：`计划书/前端体验与响应式优化计划.md`（方案基线）、`计划书/下一步改进指南.md`（后端批次 B1-1 等，排除项来源）、`计划书/8x-前端体验专项审计.md`、`web/AGENTS.md`
- 文档性质：仅产品需求规格，不含实现代码；落地必须遵循仓库 `AGENTS.md`（前端复用组件、i18n、性能/安全/数据库门禁）
- 交付约束：只写本文档一个文件；不改任何代码/配置；不执行 git 操作；文件 UTF-8 编码

> 证据口径：本文「已核对」结论均来自本次对仓库代码/文档的只读核查；无法在本机一次性确认的数值统一标注 **（待验证）**。

## 1. 引言

「听风AI」是基于 new-api 网关（Go + React 19）部署的对外 AI API 网关站点。本 PRD 定义其 **Landing 首页 Vibe 重设计 + 首屏性能优化** 的 P0/P1 切片：以 clay/cream 暖色系统一品牌视觉，用滚动叙事与伪 3D 悬停提升首页质感；同时把首屏 JS 负载与 Core Web Vitals 压到目标区间，补齐 Service Worker 离线壳、SEO 分享信息、i18n 全语言与 WCAG 2.1 AA 无障碍基线。

本切片只动**前端展示层与构建产物策略**，不触碰后端转发、计费、数据库、部署与网关可靠性逻辑（见 §4 排除项）。

## 2. 问题陈述（现状痛点）

| # | 痛点 | 证据（已核对/待验证） | 影响 |
|---|---|---|---|
| P1 | **首包 JS 超重** | 已核对：`计划书/前端体验与响应式优化计划.md` 实测 `index.js 4.2MB raw / 1.2MB gzip`，async 最大 6.8MB raw，低带宽 SPA 冷载实测 40s+；任务描述为 4.1MB，口径差异 **（待验证：需重构建实测）** | LCP 主瓶颈；弱网/低端设备接近不可用 |
| P2 | **无 Service Worker** | 已核对：`web/src`、`web/index.html` 全量零 `serviceWorker/workbox/registerSW` 引用；`web/package.json` 无相关依赖 | 无离线降级；二次访问仍全量下载，弱网体验无缓存利用 |
| P3 | **Hero 蓝色系与品牌 clay 不一致** | 已核对：`web/src/features/home/components/hero-terminal-demo.tsx` 含 9 处 `blue/sky/violet` 硬编码强调色 + 硬编码 hex `#0b0f17`（终端底色）；`components/sections/hero.tsx` CC Switch 兜底徽章 `bg-blue-500/10 text-blue-600 dark:bg-blue-400/10 dark:text-blue-400`（蓝系 5 处）；`features.tsx` 5 处、`cta.tsx` 1 处蓝系；而 clay/cream 调色板已存在于 `web/src/lib/theme-customization.ts`/`web/src/styles/theme-presets.css`（anthropic 预设 `#faf9f5`/`#d97757`） | 首屏主视觉与品牌声量脱节；落地页无统一 token 消费，换肤/暗色一致性差 |
| P4 | **大屏（1440+）无内容约束** | 已核对：hero/how-it-works/cta 等 `max-w-6xl`，无 `2xl` 增强；`前端体验与响应式优化计划.md` 断点抽样 sm×140 / md×8 / lg×46 / xl×0 | 大屏留白失衡、内容拉长 |
| P5 | **无视觉回归基线** | 已核对：`web/package.json` 无 playwright；无 VRT 工具/基线目录；`web/src/features/home` 无任何 `*.test.*` 文件 | 重设计无回归护栏，改版易破坏既有观感 |
| P6 | **SEO/分享信息缺失** | 已核对：`web/index.html` 仅 title/description（`new-api-Max`）+ `theme-color #fff`；全仓 `og:/twitter:/robots/sitemap/canonical` 零命中；`web/public` 无 `robots.txt`/`sitemap.xml`；umami/GA 为注释占位 | 社交分享无卡片；搜索引擎对公开页索引缺失或错误 |
| P7 | **终端演示常驻加载** | 已核对：`HeroTerminalDemo` 常驻渲染 + 4.5s 轮播（已有 `prefers-reduced-motion` 降级）；未做视口内懒加载 | 首屏承担的 JS/动画工作与「演示」价值不成比例 |
| P8 | **无硬编码色/性能门禁** | 已核对：无 CI 脚本对落地页硬编码色计数；`rsbuild.config.ts` 已有 vendor 分包（react/ui-primitives/tanstack）与生产路由级 `autoCodeSplitting: isProd`；重库（shiki/vchart/recharts/katex/codemirror）实际拆分情况 **（待验证：需构建产物分析）** | 「清零/达标」无客观度量，回归无门禁 |
## 3. 目标（可量化）

| 指标 | 目标 | 基线（已核对/待验证） |
|---|---|---|
| 首包 gzip（index + 首屏关键路由） | **< 1.5MB** | 1.2MB index gzip（计划文档）；async 最大 6.8MB raw **（待验证重测）** |
| LCP | **< 2.5s**（P75） | 无基线 **（待验证：需 Lighthouse/真实设备基线）** |
| INP | **< 100ms**（P75） | 无基线 **（待验证）** |
| CLS | **< 0.1** | 无基线 **（待验证）** |
| 弱网二次访问 | 应用壳秒开；离线可展示降级页 | 现状无 SW，不可用 |
| 落地页硬编码色（hex + `blue-*`/`sky-*`/`violet-*` 等） | **归零**（`rg` 统计 = 0，全走语义 token） | 当前 home 目录 20 处蓝系/hex（已核对） |
| 动画帧率 | 60fps（仅 transform/opacity） | 现状存在部分布局动画 **（待验证全量审计）** |
| i18n | 7 语言 `i18n:sync` **零漂移** | 7 语言文件存在 **（漂移现状待验证）** |
| 无障碍 | axe 关键页 **无 critical/serious**；对比度 AA | 无自动化基线 **（待验证）** |
| SEO | OG/Twitter 卡片正确；robots/sitemap 上线 | 现状零 SEO（已核对） |
| 回归 | `bun run build` + vitest + typecheck 全绿；自定义首页三路径（iframe/html/md）无回归 | 既有质量门存在（已核对 `web/package.json`） |

## 4. 范围

### 4.1 包含项（In Scope）

- **Landing 首页 Vibe 重设计**：clay/cream 暖色系品牌统一（消费既有 anthropic 预设 token，不新造色板）；滚动叙事（sticky/渐进揭示，reduced-motion 降级）；伪 3D 悬停（仅 transform/opacity，`(hover:hover)` 启用，触屏无粘滞）；终端演示**视口内懒加载** + 交互增强。
- **性能批 1（主包瘦身）**：重依赖（shiki/vchart/recharts/katex/codemirror 等）按需/异步 chunk 复核；图标 tree-shaking lint 防 `import * as Icons`；字体子集复核（Lora/Public Sans）；CLS 占位（`min-h`/宽高比）。
- **性能批 2（离线与交互）**：Service Worker 应用壳 precache（版本化缓存名、`self.skipWaiting + clients.claim`、`/api` `/v1` 不缓存、发布可更新可回收）；图片 `srcset` + `loading=lazy`；长任务/INP 与事件去重；`prefers-reduced-motion` 产品级校验。
- **SEO 分享卡 + 检索**：OG/Twitter 分享卡（`og:image`、`og:title`、`og:description`、`og:url`、`twitter:card`、`canonical`、语义化 `theme-color`）；`robots.txt` + `sitemap.xml`（公开路由白名单）。
- **i18n 全语言**：所有新增 Landing 文案入 7 语言文件（en/zh/zh-TW/fr/ja/ru/vi），`i18n:sync` 零漂移。
- **无障碍**：WCAG 2.1 AA（对比度/焦点/ARIA/键盘/动效降级），axe 关键页扫描。

### 4.2 排除项（Out of Scope，明确不在本文档）

| 排除项 | 归属计划 | 说明 |
|---|---|---|
| B1-1 流式首包缓冲 fallover（`relay.stream_fallover`） | `计划书/下一步改进指南.md` 后端批次 | 后端转发可靠性（含首包超时/缓冲上限），非前端 |
| SQL/数据库优化（迁移、索引、三库兼容验证） | 其他后端批次 | 涉及 GORM/迁移门禁，另行立项 |
| Docker/部署加固（compose、镜像、网关层 TLS） | 其他部署批次 | 基础设施，非站点前端 |
| 管理台全量响应式改造（侧边栏/表格/底部 Tab） | `前端体验与响应式优化计划.md`（更大范围） | 本切片只覆盖 Landing 首屏；管理台另立 PRD |
| 后端计费/渠道/任务/日志业务逻辑 | 既有批次 | 与本 PRD 无耦合，仅做回归冒烟 |
| 自定义首页的内容层改造（iframe sandbox 策略、RichContent 渲染器本身） | —— | 只做回归验证，不改机制（见 FR-15） |
| 网关 API 鉴权/速率限制/安全策略调整 | 后端安全批次 | 前端 CSP/消毒仅为配套建议（见 NFR-4） |
## 5. 功能需求

### FR-01 Hero 主视觉 Vibe 重设计（clay/cream 统一）
**描述**：重设首页 Hero 区视觉与文案层级，主色/背景/卡片统一消费 clay/cream 语义 token（对齐 `theme-presets.css` anthropic 预设 `#faf9f5`/`#d97757` 家族），移除蓝系强调；保留品牌归属署名（new-api / QuantumNous 版权与引导不允许删除/替换）。
**验收要点**：
- Hero 区零硬编码 hex/蓝系类名（`rg` 计数归零）；
- light/dark 双模式均以 clay/cream 为基底且对比度 AA；
- 桌面（≥1024）与移动（≤640）两断点截图无溢出、无拉伸。

### FR-02 滚动叙事
**描述**：首页按「问题 → 能力 → 三步上手 → 行动召唤」滚动推进，采用 sticky/渐进揭示（复用 `AnimateInView` 体系），叙事不阻塞首屏渲染。
**验收要点**：
- 叙事动画全部 `transform/opacity` 实现，无 layout 动画；
- `prefers-reduced-motion: reduce` 下动画退化为静态展示、内容完整可读；
- 键盘『跳过到主内容』可越过动画区直达正文。

### FR-03 伪 3D 悬停
**描述**：能力卡片/终端演示支持轻量伪 3D 悬停（tilt 上限 ≤8°，仅 transform），仅 `(hover:hover)` 设备启用，触屏无粘滞。
**验收要点**：
- 触屏设备 hover 不触发、不粘滞；
- 悬停角度/缩放有上限，无布局位移（CLS 无增量）；
- 键盘焦点态与悬停态可区分（`focus-visible` 2px 环）。

### FR-04 终端演示懒加载与交互增强
**描述**：`HeroTerminalDemo` 改为视口内懒加载（IntersectionObserver + 独立 chunk），展示真实 API 调用链（headers/method/endpoint/usage/latency）并保留 reduced-motion 静态降级。
**验收要点**：
- 首屏未进入视口前演示 chunk 不下载/不执行（DevTools 网络面板验证）；
- 轮播/打字动画受 `prefers-reduced-motion` 控制；
- 演示文案全部走 i18n。

### FR-05 落地页硬编码色清零
**描述**：home 全目录硬编码色（hex、`blue-*`/`sky-*`/`violet-*` 等非语义类）迁移为语义 token/`@theme` 变量；终端语法高亮如需保留色相差异，用 token 化 accent 语义色表达并满足 AA。
**验收要点**：
- `rg -n --no-heading "#[0-9a-fA-F]{3,8}\b|blue-[0-9]|sky-[0-9]|violet-[0-9]" web/src/features/home` 结果为 0；
- 明暗两模式下同样本截图对比无「跑色」。

### FR-06 主题兼容（light/dark + 既有预设不回归）
**描述**：重设计必须兼容既有主题系统（`theme-provider` + `theme-customization.ts` 预设），不破坏管理台主题切换、不引入第二套色板体系。
**验收要点**：
- 切换 light/dark 及既有预设（至少 default/anthropic）后首页正常渲染；
- 不新增全局样式副作用；原生首屏防闪烁策略保持。

### FR-07 首包/主包瘦身（性能批 1）
**描述**：复核并下沉重依赖（shiki/vchart/recharts/katex/codemirror 等）为按需/异步 chunk；补图标全量导入 lint；复核字体子集（Lora/Public Sans）；关键路由（首页/登录/注册/定价）首屏不携带重型 chunk。
**验收要点**：
- 重构建后首屏 gzip **< 1.5MB**（对比基线记录在案）；
- dist 产物扫描：无超大同步 chunk 进入首页入口；
- `bun run lint` 通过（含图标导入规则）。

### FR-08 Service Worker 离线壳（性能批 2）
**描述**：新增 SW 预缓存应用壳（index.html + 核心 chunk + 静态资源），版本化缓存名 + `skipWaiting`/`clients.claim`；网络不可达展示离线降级页；**永不缓存 `/api`、`/v1`、鉴权响应与用户敏感数据**。
**验收要点**：
- 二次访问（DevTools Offline）可渲染应用壳 + 离线提示；
- 登录态路径（`/dashboard` 等）离线时明确降级而非假成功；
- SW 版本更新后旧缓存清理、新版本生效。

### FR-09 图片/媒体资源优化
**描述**：首页图片（logo、品牌图标、示例图）统一 `srcset` + `loading=lazy`、固定宽高比，消除布局抖动。
**验收要点**：
- 每个 `<img>` 具 width/height 或 aspect 占位（CLS 无增量）；
- 折叠区资源不进首屏请求队列。

### FR-10 INP 与长任务
**描述**：首页/公开页交互去重与长任务拆分（滚动叙事节流、轮播定时器清理、事件处理 debounce）。
**验收要点**：
- 交互路径 INP < 100ms（P75）；
- 组件卸载后无残留定时器（含终端演示轮播）。

### FR-11 SEO 分享卡
**描述**：`index.html`/路由 head 补 OG + Twitter 分享卡（`og:image` 需真实可访问图、标题/描述、`og:url`、`twitter:card`）、`canonical`、语义化 `theme-color`；`title` 保持品牌署名完整性（new-api 归属不得移除）。
**验收要点**：
- 分享链接/调试器渲染标题+描述+图片；
- `og:image` URL 非登录态可公开访问（200）。

### FR-12 robots.txt + sitemap.xml
**描述**：`web/public` 增 `robots.txt`（公开路由白名单，登录/管理路由 `Disallow`）与 `sitemap.xml`（关于/定价/协议等公开页）。
**验收要点**：
- `/robots.txt`、`/sitemap.xml` 200 且与路由树一致；
- 不索引登录/管理/密钥类路由。

### FR-13 i18n 全语言
**描述**：所有新增 Landing 文案入 `web/src/i18n/locales/{en,zh,zh-TW,fr,ja,ru,vi}.json`，`bun run i18n:sync` 无漂移。
**验收要点**：
- `bun run i18n:sync` 后无 key 漂移；
- 抽查 zh/fr/ja 渲染无英文残留、无占位 key 输出。

### FR-14 无障碍（WCAG 2.1 AA）
**描述**：Landing 全量过对比度/焦点/ARIA/键盘/动效降级；图标按钮 `aria-label`、终端演示文本可读、滚动叙事可跳过。
**验收要点**：
- axe-core 扫描 Landing 关键区块无 critical/serious；
- 语义色正文 ≥4.5:1、大文本 ≥3:1；
- 键盘可完成「首页 → 注册 → 首次请求」主流程。

### FR-15 自定义首页覆盖回归
**描述**：既有后台自定义首页（URL iframe / HTML / Markdown 三路径）在重设计与性能优化后保持可用；iframe sandbox 策略与 RichContent 消毒机制不改。
**验收要点**：
- 三路径分别冒烟通过（iframe 需覆盖允许脚本 + 用户激活跳转场景）；
- SW 激活后自定义首页仍正常加载（不被离线缓存误伤）；
- DOM 消毒（dompurify）路径回归：恶意 HTML 不执行。
## 6. 非功能需求

### NFR-1 性能
- 首屏 gzip < 1.5MB；LCP < 2.5s（P75）；INP < 100ms（P75）；CLS < 0.1（**均待验证基线后以重构建实测为准**）。
- 动画仅 transform/opacity；`content-visibility` 等只用于长列表，不引入未验证风险。
- 弱网（Fast 3G）冷载可交互 ≤ 10s（**待验证**）；二次访问（SW 生效）壳渲染 ≤ 2s。

### NFR-2 无障碍（WCAG 2.1 AA）
- 对比度、焦点可见（`focus-visible` 2px 高对比环）、ARIA 标注、键盘全流程可达、`prefers-reduced-motion` 降级、44×44px 触摸目标（触屏设备）。

### NFR-3 国际化
- 7 语言（en/zh/zh-TW/fr/ja/ru/vi）key 齐全，`bun run i18n:sync` 零漂移；禁止硬编码用户可见文案；`lang`/`dir` 属性正确。

### NFR-4 安全
- **DOM 消毒**：任何渲染后台/用户内容（自定义首页 HTML、Markdown）必须走既有 `dompurify` 消毒（`web/package.json` 已含 dompurify 3.4.13），本切片不改动该机制但必须回归。
- **iframe sandbox**：自定义首页 iframe 保持现有最小化 sandbox 集，不新增权限 token。
- **CSP（建议）**：重设计与 SW 引入的资源加载模式应支持后续叠加 CSP（script-src/connect-src 白名单），不在文档阶段强上；具体值实现期按部署环境评估。
- **SW 边界**：绝不缓存 `/api`、`/v1`、登录态响应与用户数据；SW 更新须版本化可回收。
- 审计日志/新增埋点不含敏感信息（与既有 `admin_info` 脱敏规范一致）。

### NFR-5 可维护性
- token 单一来源（`@theme`/语义类），落地页禁止散落硬编码色值；
- 复用 `web/src/components/ui/*` 与既有业务组件，禁止重复造轮子（遵 `web/AGENTS.md` 复用优先）；
- 构建可复现：`bun install && bun run build` 在干净环境可复现产物，记录基线哈希；
- 性能/色值门禁以脚本形式入库（脚本本身属实施期产物，不在本文档）。

## 7. 用户故事

1. **新访客（移动弱网）**：作为首次访问者，3 分钟内从首页理解产品 → 注册 → 生成密钥 → 发出首次请求并看到真实返回；弱网下首屏不白屏、不卡死。
2. **管理员（大屏）**：作为管理员，在 1440px 显示器上浏览 Landing 与公开页，内容不横向拉伸、卡片密度合理、视觉与品牌一致。
3. **老用户（二次访问/离线）**：作为已访问过的用户，弱网或离线时应用壳秒开并明确提示离线，不会出现登录态假成功或无限 loading。
4. **暗色/动效敏感用户**：作为暗色主题或 `prefers-reduced-motion` 用户，首页在 dark 下对比度 AA、动画降级为静态但内容完整。
5. **键盘/读屏用户**：作为仅用键盘与读屏的用户，可从首页完整走通注册与首次请求主流程。
6. **分享传播者**：作为将首页链接分享到 IM/社交平台的人，卡片展示正确标题/描述/图片（OG/Twitter）。
7. **运维/SEO 关注者**：作为站长，`/robots.txt` 与 `/sitemap.xml` 正确引导公开页索引，管理/密钥路由不被收录。

## 8. 验收标准（映射真实 E2E）

| # | 验收项 | 真实 E2E 验证方式 | 通过标准 |
|---|---|---|---|
| A1 | Landing 重设计 | Playwright/浏览器 375/768/1024/1440 × light/dark 截图（存入 `计划书/e2e-evidence`） | 四档无溢出、无截断、品牌色一致；与基线对比记录差异 |
| A2 | 硬编码色清零 | `rg -n --no-heading "#[0-9a-fA-F]{3,8}\b|blue-[0-9]|sky-[0-9]|violet-[0-9]" web/src/features/home` | 计数 = 0 |
| A3 | 构建与单测 | `bun run build` + `bun run test`（vitest）+ `bun run typecheck` + `bun run lint` | 全绿；新增行为有对应回归测试 |
| A4 | i18n | `bun run i18n:sync` | 无漂移；7 语言抽查渲染无英文残留 |
| A5 | 性能 | 重构建前后 Lighthouse（LCP/INP/CLS）+ dist gzip 统计对比表 | 首包 gzip < 1.5MB；LCP < 2.5s；CLS < 0.1；INP < 100ms（数值以实测记录为准，不达标则更新文档并列出差距） |
| A6 | SW | DevTools 二次加载 + Offline 模式 | 壳渲染 + 离线降级页；`/api` `/v1` 无缓存命中；更新后旧缓存清理 |
| A7 | 自定义首页回归 | 后台配置 iframe URL / HTML / Markdown 三种内容各冒烟一次 | 三路径正常；SW 激活后仍正常；恶意 HTML 不执行 |
| A8 | 无障碍 | axe-core 扫描 Landing 关键区块 | 无 critical/serious；对比度 AA 抽查 |
| A9 | SEO | 分享调试器/抓取 OG 标签；`/robots.txt` `/sitemap.xml` 200 | 卡片信息正确；索引白名单与路由一致 |
| A10 | 品牌治理 | `rg -i "new-api|quantumnous" web`（署名/版权/标识相关） | 无删除、无替换、无弱化 |

## 9. 风险与开放问题

1. **性能数据口径**：4.1MB（任务描述）vs 4.2MB（计划文档实测）需以重构建产物为准；LCP/INP/CLS 当前无基线（**待验证**），本机是否具备可复现 Lighthouse（Chrome）环境需确认。
2. **主包瘦身边界**：shiki/vchart/recharts/katex/codemirror 等重库实际拆分状态未知（**待验证**）；若某库被多路由共享，单纯移出首屏可能增大 async 总包，需以「首屏指标」而非「总包」为验收口径。
3. **SW 与登录态边界**：SW 缓存应用壳不得缓存鉴权/动态数据；离线时 `/dashboard` 等登录路由的降级语义需产品拍板（提示离线 vs 渲染壳），避免「假登录成功」。
4. **SW 撤回成本**：一旦发布难以即时撤回，需版本化缓存名 + 更新通道，发布前在小流量验证。
5. **SEO 对登录型网关的天花板**：核心价值在登录后的控制台，公开页有限；robots/sitemap 只覆盖公开白名单，预期收益（自然流量/分享转化）需管理预期，不建议做内容农场式 SEO。
6. **clay 语法高亮对比度**：终端演示的多色语法高亮在 clay 浅底上达标 AA 有难度；若无法达标，接受「语义色 + 非纯颜色通道」方案并记录豁免（类似既有 A-F 徽章双通道先例）。
7. **视觉回归基线缺失**：当前无 Playwright/VRT；引入视觉回归依赖与 CI 运行环境（**待验证**本机/CI 支持度），实施期需与基础设施决策合并评审。
8. **自定义首页覆盖**：管理员配置自定义首页（iframe/HTML/Markdown）会完全替代默认 Landing；性能指标与 SW 承诺在该路径下不成立，验收需区分「默认 Landing」与「自定义首页」两种状态。
9. **i18n 漂移现状**：`bun run i18n:sync` 当前状态（是否有存量漂移）**待验证**；若有存量漂移需先清理再要求零漂移门禁。

## 治理与合规声明

- 本 PRD 及其后续实施**不得移除、替换或弱化** nеw-арi 与 QuantumNous 的任何品牌标识、版权头、元数据或署名（README/许可证头/页面标题/页脚/关于页/图标等），与仓库 `AGENTS.md` 项目治理一致。
- 本 PRD 不包含 UI 实现代码；所有新增用户可见文案在实施期必须以 i18n key 形式加入 7 语言文件（本文档仅给出文案要求）。
- 实施/验收遵循仓库门禁：复用组件优先、`bun` 工具链、OWASP 认证安全要求（涉及登录/注册变更时）、三库兼容与计费不变量（本切片不触及，但回归需覆盖）。