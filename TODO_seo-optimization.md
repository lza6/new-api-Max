<!--
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
-->
# TODO — SEO 优化（new-api-Max 听风API 网关）

> 生成日期：2026-10-03 · 基线版本：v1.3.77
> 站点：主站 `https://freeapi.tingfengai.art` · 副站 `https://japi.tingfengai.art`
> 交付约束：本文件**仅含提案**，不直接改源码；改动以 patch 形式给出，由人工/后续任务执行。

---

## 0. 需求澄清（先定框架，否则全是空谈）

公开的、可被搜索引擎收录且**不需要登录**的页面（据 `web/src/routes/` 与路由守卫核对）：

| 公开路由 | 内容类型 | 当前是否在 sitemap |
|---|---|---|
| `/` | 落地页（营销，v1.3.77 改版） | ✅ |
| `/pricing` | 模型定价表（BOFU） | ✅ |
| `/rankings` | 模型/渠道排行 | ✅ |
| `/model-test` | 模型可用性测试工具 | ✅ |
| `/tool-setup` | 客户端接入配置生成器 | ✅ |
| `/about` | 关于 | ✅ |
| `/docs` | 文档（外链跳转 `docs_link`） | ❌ |
| `/privacy-policy`、`/user-agreement` | 法务 | ❌（可 noindex） |

**非目标（明确排除）**：`/dashboard`、`/keys`、`/channels`、`/usage-logs`、`/system-settings`、`/wallet`、`/chat`、`/playground`、`/setup`、`/sign-in|up` 等登录后页面——**必须 noindex**，不参与任何 SEO 动作。

---

## 1. Context（上下文）

### 1.1 目标关键词与搜索意图

- **主关键词（Primary）**：`AI API 网关` / `AI API gateway`
- **意图分类**：**Commercial Investigation（商业调研）** 为主 + Informational（信息型）为辅
  - TOFU（认知）：`what is an AI API gateway`、`统一 AI 接口是什么`
  - MOFU（考虑）：`AI API 网关对比`、`多模型统一 API 平台`、`OpenAI 兼容网关`
  - BOFU（决策）：`AI API 中转站`、`免费 AI API 额度`、`deepseek 便宜 api`、`模型价格对比`
- **关键判断**：本站**不是内容站**，是**产品站 + 工具站**。SEO 主战场是「工具页被搜到」+「品牌词/长尾词」，不是写博客冲大词。所以策略重心：**技术 SEO（可索引性/结构化/性能）+ 工具页与定价页的意图对齐**，而非堆长文。

### 1.2 目标受众 Persona

| Persona | 痛点 | 目标 | 决策标准 |
|---|---|---|---|
| **独立开发者 / 小团队**（主） | 多厂商 key 管理烦、想省钱、想一个 SDK 打通所有模型 | 快速接入、按量付费、便宜稳定 | 价格、兼容性、接入文档、稳定性 |
| **AI 应用创业者** | 上游不稳定、限流、账单不可控 | 网关兜底、多渠道路由、成本可视化 | 可用性 SLA、失败重试、计费透明 |
| **进阶用户**（Cherry Studio / CC Switch 用户） | 不会配 base_url | 一键配置 | 工具易用性、是否支持客户端 |

对应漏斗阶段：独立开发者偏 MOFU/BOFU；创业者偏 MOFU；进阶用户偏 BOFU（转换最快）。

### 1.3 内容类型与目标字数

- `/`（落地页）：**产品营销页**，不计字数；目标是**首屏信息密度 + 结构化数据 + 可索引文案**。
- 新增 **`/guide`（长文）**：MOFU 支柱内容，**1500–2500 字**（中英各一版），主题「如何用一个 API 接入 OpenAI/Claude/Gemini/DeepSeek」。这是本站唯一值得新建的长文。

---

## 2. SEO Strategy Plan

- [ ] **SEO-PLAN-1.1 [Keyword Cluster — 首页/品牌词]**
  - **Primary**：`AI API 网关`、`AI API gateway`
  - **Secondary**：`统一 AI 接口`、`多模型 API 平台`、`OpenAI 兼容 API`、`Claude API 中转`、`Gemini API 代理`
  - **Long-Tail**：`一个接口调用所有大模型`、`免费 AI API 中转站推荐`、`怎么用 cherry studio 接自己的 api`、`cc switch 配置教程`
  - **Intent**：Commercial Investigation

- [ ] **SEO-PLAN-1.2 [Keyword Cluster — 定价页 BOFU]**
  - **Primary**：`AI 模型 API 价格`、`大模型 API 报价对比`
  - **Secondary**：`deepseek api 多少钱`、`gpt api 价格`、`gemini api 免费额度`
  - **Long-Tail**：`deepseek-v4 价格 每百万 token`、`claude 便宜 api 渠道`
  - **Intent**：Transactional / Commercial

- [ ] **SEO-PLAN-1.3 [Keyword Cluster — 工具页]**
  - **Primary**：`OpenAI 兼容 API 地址`、`base_url 配置`
  - **Secondary**：`API 中转地址`、`API key 怎么获取`、`nextchat 配置 API`
  - **Long-Tail**：`cherry studio 怎么填 api 地址`、`写代码用的 api 中转`
  - **Intent**：Transactional（工具页直接给答案 = 高转化）

- [ ] **SEO-PLAN-1.4 [Keyword Cluster — 信息型 TOFU]**
  - **Primary**：`什么是 AI API 网关`
  - **Secondary**：`API 网关原理`、`多模型路由`、`API 网关 vs 直连`
  - **Long-Tail**：`为什么需要用 api 网关`、`api 中转站安全吗`
  - **Intent**：Informational（投放 `/guide`）

- [ ] **SEO-PLAN-2.1 [双站策略]**
  - 主站 `freeapi` = 主 SEO 阵地（收录 `/`、`/pricing`、`/rankings`、工具页）
  - 副站 `japi` = **`canonical` 指回主站**，或整站 `noindex`，二者择一，**避免双站内容互掐**（当前两站共用同一前端、同一 sitemap，属**严重重复内容源**）
  - **Rationale**：两站 HTML/标题/描述完全一致，Google 会择一收录并降权另一，浪费抓取预算

---

## 3. SEO Optimization Items

### 3.1 技术 SEO（最高优先级）

- [ ] **SEO-ITEM-1.1 [robots.txt / sitemap 域名硬编码]**
  - **Element**：`web/public/robots.txt`、`web/public/sitemap.xml`
  - **Current State**：sitemap 与 robots 里**写死** `https://freeapi.tingfengai.art`；副站部署同一产物 → 副站 sitemap 指向主站，且缺失 `/docs`、`/privacy-policy`、`/user-agreement`、`/pricing/plans`、`/pricing/$modelId`
  - **Recommended Change**：构建期按 `VITE_REACT_APP_SERVER_URL` 生成 sitemap/robots（见 §5 diff `SEO-ITEM-1.1`）
  - **Rationale**：跨域名 sitemap 会被 Search Console 判为「站点地图不在本站」，整份失效

- [ ] **SEO-ITEM-1.2 [SPA 可索引性 —— 最大技术风险]**
  - **Element**：SPA 渲染模式
  - **Current State**：纯 CSR（`web/dist/index.html` 只有一个空 `#root` + 引导动画）；**title/description 在 `main.tsx` 由 JS 运行时写入**（`initSystemBranding`）
  - **Recommended Change**：至少为 `/`、`/pricing`、`/about`、工具页做**静态 meta 注入**（构建期或 Caddy 层按路由吐带正确 `<title>`/`<meta>`/JSON-LD 的 HTML 外壳）；进阶方案为 prerender（`rsbuild` 无内置 SSG，可用 `vite-plugin-ssg` 类或 Caddy `handle` 规则 + 预渲染快照）
  - **Rationale**：Google 能执行 JS 但预算有限、时效差；Bing/百度**基本不执行**。当前 `<title>` 对爬虫是 `new-api-Max`，描述是英文默认串——SEO 起点几乎为零

- [ ] **SEO-ITEM-1.3 [canonical 缺失]**
  - **Element**：`<link rel="canonical">`
  - **Current State**：无
  - **Recommended Change**：每页注入自指 canonical；`/pricing/$modelId` 等参数化页面 canonical 指向 `/pricing`
  - **Rationale**：防止 `?aff=xxx`（联盟参数，见 `__root.tsx` `saveAffiliateCode`）与双站造成重复内容

- [ ] **SEO-ITEM-1.4 [登录页 noindex]**
  - **Element**：`robots` meta
  - **Current State**：全局 `Allow: /`，登录后页面同样被允许抓取
  - **Recommended Change**：对 `/dashboard`、`/keys`、`/channels`、`/usage-logs`、`/wallet`、`/system-settings`、`/setup`、`/sign-in|up|forgot-password` 注入 `<meta name="robots" content="noindex,nofollow">`；robots.txt 同步 `Disallow`
  - **Rationale**：避免抓取预算浪费在无索引价值的认证页

### 3.2 页面级 On-Page

- [ ] **SEO-ITEM-2.1 [首页 title/description]**
  - **Element**：`<title>`、`<meta name="description">`
  - **Current State**：`<title>new-api-Max</title>`（11 字符，无关键词）；desc = `Unified AI API gateway and admin dashboard.`（46 字符，无中文、无卖点）
  - **Recommended Change**
    - 中文 title（≤60）：`听风API - 一个接口调用所有大模型 | AI API 网关`
    - 英文 title（≤60）：`New API Max — One Endpoint for Every AI Model`
    - 中文 desc（≤160）：`统一 OpenAI / Claude / Gemini / DeepSeek 等 40+ 大模型的 AI API 网关。按量付费、价格透明、支持 Cherry Studio 等客户端一键配置，注册即用。`
  - **Rationale**：主关键词进 title 与 desc，是排名最直接的杠杆

- [ ] **SEO-ITEM-2.2 [H1 与首段]**
  - **Element**：`hero.tsx` 的 `<h1>`
  - **Current State**：`Unified API Gateway for Vast Range of AI Models`（英文，无中文关键词）
  - **Recommended Change**：保留英文品牌调性作默认，但通过 i18n key 让中文 locale 的 H1 为 `一个接口，调用所有大模型`（含「大模型」「接口」核心词），首段 100 词内自然出现 `AI API 网关`、`统一接口`
  - **Rationale**：中文站点搜索主关键词需在 H1 与首屏文案出现

- [ ] **SEO-ITEM-2.3 [图片 alt]**
  - **Element**：`hero.tsx` 的 Cherry Studio logo、`footer.tsx` logo
  - **Current State**：Logo `<img alt>` 有值但为品牌名；Cherry Studio 图标 `Suspense` fallback 无 alt
  - **Recommended Change**：装饰性图标统一 `aria-hidden` + 空 alt；内容性图片补描述性 alt（如 `alt="Cherry Studio 客户端接入听风API 配置示例"`）
  - **Rationale**：无障碍与图片搜索双收益

### 3.3 结构化数据（Schema / JSON-LD）

- [ ] **SEO-ITEM-3.1 [Organization + WebSite]**
  - **Element**：`<script type="application/ld+json">`
  - **Current State**：无
  - **Recommended Change**：全站注入 `Organization`（name=New API Max、url、logo）+ `WebSite`（含 `SearchAction` 指向 `/pricing?q={query}` 若可用）
  - **Rationale**：品牌知识面板与站点名称展示

- [ ] **SEO-ITEM-3.2 [SoftwareApplication]**
  - **Element**：首页 JSON-LD
  - **Current State**：无
  - **Recommended Change**：注入 `SoftwareApplication`（applicationCategory=`DeveloperApplication`、offers 按 `/api/pricing` 动态或静态示例价格、aggregateRating 若有真实数据才加——**无数据不得伪造**）
  - **Rationale**：产品站可获得富结果；评分字段留空优于造假

- [ ] **SEO-ITEM-3.3 [FAQPage]**
  - **Element**：首页 FAQ 区块 + JSON-LD
  - **Current State**：页面**无 FAQ 区块**
  - **Recommended Change**：新增 5–6 条 FAQ（见 §4 内容大纲）并配 `FAQPage` schema
  - **Rationale**：抢「People Also Ask」位置，TOFU 增益

- [ ] **SEO-ITEM-3.4 [BreadcrumbList]**
  - **Element**：`/pricing/$modelId`、`/docs`
  - **Current State**：无
  - **Recommended Change**：面包屑 + `BreadcrumbList` schema
  - **Rationale**：SERP 展示层级，提升 CTR

### 3.4 内容缺口与重复

- [ ] **SEO-ITEM-4.1 [内容缺口]**
  - **Current State**：无任何信息型长文；`/about` 内容薄弱；无 `how-to`、无对比
  - **Recommended Change**：新建 `/guide`（与 `/docs` 区分：`/docs` 是 API 参考，`/guide` 是入门教程）+ 在 `/tool-setup` 扩写「如何获取 key / 如何填 base_url」
  - **Rationale**：MOFU 内容缺失导致只靠交易词，天花板低

- [ ] **SEO-ITEM-4.2 [重复内容 / 双站]**
  - **Current State**：主副站同构同文案
  - **Recommended Change**：见 `SEO-PLAN-2.1`
  - **Rationale**：见该条

- [ ] **SEO-ITEM-4.3 [内链]**
  - **Element**：内部链接锚文本
  - **Current State**：首页几乎无内链；`/pricing`、`/rankings`、`/tool-setup` 之间无互链
  - **Recommended Change**：首页 Features/HowItWorks 段落加内链，锚文本示例见表

| 源页面 | 目标 | 锚文本 |
|---|---|---|
| `/` 定价卡片 | `/pricing` | `查看完整模型价格` |
| `/` 排行卡片 | `/rankings` | `模型可用性排行` |
| `/` HowItWorks ③ | `/tool-setup` | `一键生成客户端配置` |
| `/pricing` 顶部 | `/tool-setup` | `不知道 base_url 怎么填？` |
| `/tool-setup` 底部 | `/guide` | `完整接入教程` |
| `/guide` 内 | `/pricing` | `实时价格表` |

- **Rationale**：把权重导向 BOFU 页，缩短转化路径

### 3.5 站外与权威（Off-Page）

- [ ] **SEO-ITEM-5.1 [可链接资产]**
  - **Current State**：无
  - **Recommended Change**：① 公开「模型价格对比表」独立页（可被引用）；② `/model-test` 可用性看板（数据型可引用）；③ 开源仓库 README 互链
  - **Rationale**：数据/工具型资产自然获链，优于外链采购

- [ ] **SEO-ITEM-5.2 [外链与 GP 目标]**
  - **Recommended Change**：目标池 = 中文 AI 工具导航站、V2EX/LINUX DO 相关帖（自然提及，非刷）、GitHub Awesome-LLM 类清单、掘金/思否教程（GP 主题「一个 API 接入多模型」）
  - **Anchor 策略**：品牌词 60% / 部分匹配 25% / 裸链 15%，禁全站精确匹配
  - **Rationale**：中文 AI 圈层获客 + 权重

---

## 4. 内容大纲（`/guide` 支柱页，1500–2500 字）

- H1：`一个 API 接入所有大模型：AI API 网关完整指南`
- H2：什么是 AI API 网关（定义段，抢 Featured Snippet）
- H2：为什么要用统一接口（多 key 管理 / 上游容错 / 成本可视化）— 用**表格对比「直连 vs 网关」**
- H2：支持哪些模型与协议（OpenAI / Claude / Gemini / DeepSeek…）— 表格
- H2：如何开始（编号步骤，抢 HowTo Snippet）：① 注册 ② 拿 key ③ 填 base_url ④ 发第一个请求（含 curl 代码块）
- H2：常见客户端配置（Cherry Studio / CC Switch / NextChat）+ 截图
- H2：价格与计费说明 → 内链 `/pricing`
- H2：FAQ（6 条，配 `FAQPage` schema）：
  1. 网关和直连有什么区别？
  2. 支持 OpenAI 官方 SDK 吗？
  3. 怎么充值 / 怎么计费？
  4. 数据安全吗？
  5. 免费额度有吗？
  6. 支持流式输出吗？

---

## 5. Proposed Code Changes

> 说明：`index.html` 是构建期模板；下列 diff 为**提案**，需按实现方式（构建期注入 vs Caddy 路由层）二选一。

**SEO-ITEM-1.1 — sitemap/robots 按站点域名生成**（构建期脚本，示例）

```diff
--- a/web/scripts/generate-seo-assets.mjs        （新增文件）
+++ b/web/scripts/generate-seo-assets.mjs
@@
+// 构建期根据部署域名生成 robots.txt / sitemap.xml，避免副站沿用主站域名。
+import fs from 'node:fs'
+import path from 'node:path'
+
+const site = process.env.VITE_REACT_APP_SERVER_URL || 'https://freeapi.tingfengai.art'
+const pub = path.resolve('web/public')
+
+const publicPaths = [
+  { p: '/', priority: '1.0' },
+  { p: '/pricing', priority: '0.9' },
+  { p: '/rankings', priority: '0.7' },
+  { p: '/guide', priority: '0.8' },
+  { p: '/model-test', priority: '0.6' },
+  { p: '/tool-setup', priority: '0.6' },
+  { p: '/about', priority: '0.5' },
+]
+const today = new Date().toISOString().slice(0, 10)
+
+fs.writeFileSync(
+  path.join(pub, 'robots.txt'),
+  `User-agent: *\nAllow: /\nDisallow: /dashboard\nDisallow: /keys\nDisallow: /channels\nDisallow: /usage-logs\nDisallow: /wallet\nDisallow: /system-settings\nDisallow: /setup\nDisallow: /sign-in\nDisallow: /sign-up\nSitemap: ${site}/sitemap.xml\n`
+)
+
+fs.writeFileSync(
+  path.join(pub, 'sitemap.xml'),
+  `<?xml version="1.0" encoding="UTF-8"?>\n<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">\n` +
+    publicPaths.map(({ p, priority }) =>
+      `  <url><loc>${site}${p}</loc><lastmod>${today}</lastmod><priority>${priority}</priority></url>`
+    ).join('\n') +
+    `\n</urlset>\n`
+)
+console.log('[seo] generated robots.txt + sitemap.xml for', site)
```

```diff
--- a/web/package.json
+++ b/web/package.json
@@
   "scripts": {
     "dev": "rsbuild dev",
-    "build": "rsbuild build",
+    "build": "node scripts/generate-seo-assets.mjs && rsbuild build",
```

**SEO-ITEM-2.1 — 首页静态 meta（构建期模板，示例；域名/文案按站点注入）**

```diff
--- a/web/index.html
+++ b/web/index.html
@@
-    <title>new-api-Max</title>
-    <meta name="title" content="new-api-Max" />
-    <meta
-      name="description"
-      content="Unified AI API gateway and admin dashboard."
-    />
+    <!-- 默认英文；中文站点由构建期/运行时按 locale 覆写 -->
+    <title>New API Max — One Endpoint for Every AI Model</title>
+    <meta name="title" content="New API Max — One Endpoint for Every AI Model" />
+    <meta
+      name="description"
+      content="Unified AI API gateway for OpenAI, Claude, Gemini & 40+ providers. Transparent pay-as-you-go pricing, drop-in OpenAI-compatible API, plug-and-play desktop clients."
+    />
+    <link rel="canonical" href="https://freeapi.tingfengai.art/" />
+    <meta property="og:type" content="website" />
+    <meta property="og:title" content="New API Max — One Endpoint for Every AI Model" />
+    <meta property="og:description" content="Unified AI API gateway for 40+ model providers with transparent pricing." />
+    <meta property="og:image" content="https://freeapi.tingfengai.art/og-cover.png" />
+    <meta name="twitter:card" content="summary_large_image" />
```

**SEO-ITEM-3.1 / 3.2 / 3.3 — JSON-LD（加到 `index.html` `<head>`）**

```html
<script type="application/ld+json">
{
  "@context": "https://schema.org",
  "@type": "SoftwareApplication",
  "name": "New API Max",
  "applicationCategory": "DeveloperApplication",
  "operatingSystem": "Web",
  "description": "Unified AI API gateway for OpenAI, Claude, Gemini and 40+ providers.",
  "offers": { "@type": "Offer", "price": "0", "priceCurrency": "USD" },
  "url": "https://freeapi.tingfengai.art/"
}
</script>
<script type="application/ld+json">
{
  "@context": "https://schema.org",
  "@type": "FAQPage",
  "mainEntity": [
    { "@type": "Question", "name": "Is the API OpenAI-compatible?",
      "acceptedAnswer": { "@type": "Answer", "text": "Yes. Use the same OpenAI SDK by pointing base_url to the gateway endpoint." } },
    { "@type": "Question", "name": "How is billing calculated?",
      "acceptedAnswer": { "@type": "Answer", "text": "Pay-as-you-go per token, with live usage and cost visible in the dashboard." } }
  ]
}
</script>
```

**SEO-ITEM-1.4 — 认证页 noindex（TanStack Router head 或路由级注入，示例）**

```diff
--- a/web/src/routes/_authenticated/route.tsx
+++ b/web/src/routes/_authenticated/route.tsx
@@
 export const Route = createFileRoute('/_authenticated')({
+  head: () => ({
+    meta: [{ name: 'robots', content: 'noindex,nofollow' }],
+  }),
   component: AuthenticatedLayout,
 })
```

---

## 6. Commands（本地 / CI）

```bash
# 生成 SEO 资产（含域名）
cd web && VITE_REACT_APP_SERVER_URL=https://freeapi.tingfengai.art node scripts/generate-seo-assets.mjs

# 构建并核查产物
cd web && bun run build
grep -o '<title>[^<]*' dist/index.html
grep -c 'canonical' dist/index.html
node -e "require('fs').readFileSync('dist/sitemap.xml','utf8').split('<url>').length-1" # 期望 url 数

# schema 校验（构建后）
npx --yes schema-dts-lint dist/index.html 2>/dev/null || true
# 线上验证：Google Rich Results Test / Search Console URL 检查
```

---

## 7. 质量保证 Checklist（交付前自检）

- [ ] 关键词已按 intent / funnel 分簇
- [ ] Title ≤60、desc ≤160，均含主关键词
- [ ] 内容大纲匹配主关键词的搜索意图（商业调研 → 产品页 + 定价表）
- [ ] Schema 类型恰当且结构正确（SoftwareApplication / FAQPage / Organization）
- [ ] 内链锚文本已具体化（见 §3.4 表）
- [ ] 外链目标具体（中文 AI 导航站、Awesome 清单、掘金/思否）
- [ ] **无 cannibalization**：`/docs`（API 参考）与 `/guide`（教程）职责区分；`/pricing` 与 `/pricing/$modelId` canonical 归并
- [ ] 双站重复内容已处置（canonical 或 noindex）

---

## 8. KPI 与迭代

| KPI | 现状 | 目标（3 个月） | 测量 |
|---|---|---|---|
| 首页被索引 | 未知（无 GSC 验证） | 已索引 | Google Search Console |
| 主关键词排名 | 无 | 中文「AI API 网关」前 50 | GSC 查询报告 |
| 自然搜索 CTR | 无 | >3% | GSC |
| `/guide` 自然流量 | 0 | >100/月 | GA4 |
| 转化（注册） | 无自然来源归因 | 建立 GA4 事件 | GA4 自定义事件 |

- **A/B 测试**：title 与 desc 两版对比 CTR（4 周）
- **刷新节奏**：定价页随渠道变更即时更新 `lastmod`；`/guide` 每季度复核
- **主题簇扩张**：以 `/guide` 为中心，后续可加「各家模型 API 差异」「成本优化」「多渠道路由」子页

---

## 9. 与本站 SEO 强相关的**性能危机**（独立章节，非 SEO 但影响排名）

> Core Web Vitals 是排名因子；本站有一个**已实测**的严重性能问题，必须在 SEO 前处理。

- [ ] **PERF-1.1 [公开端点全表聚合，3–4.5s，无缓存]**
  - **发现**：`/api/site/stats`（公开、无认证）实测 **3.2–4.5 秒**（连测 5 次全部 >3s），源码 `controller/log.go:234 GetSiteOverview` 每次请求对 67 万行 / 836MB `logs` 表做 `COUNT(*) + SUM(...)` 全表聚合，**无缓存**（对比 `GetBandwidthLeaderboard` 已有 60s 缓存）
  - **影响**：① 匿名爬虫可无限打 → DB CPU 耗尽风险；② 首页渲染阻塞（`SiteStats` 首屏请求它）；③ INP/LCP 恶化
  - **建议**：加 60–300s Redis 缓存 + 匿名限流（按 IP）；或改为**定时预聚合**（如每 5 分钟把总量写一个小表/Redis key）
  - **Rationale**：既是性能危机也是「被爬虫打爆」的安全隐患

- [ ] **PERF-1.2 [logs 表无归档/分区]**
  - **发现**：单表 836MB、67 万行且持续增长（消费日志 `LOG_FLUSH_ENABLED=true`，1s 批量落库）
  - **建议**：定期归档/清理 N 天前日志；`created_at` 建索引；评估按时间分区
  - **Rationale**：不处理则上述聚合只会越来越慢

---

## 10. Red Flags 自查（本次已规避/待处理）

- ✅ 未建议关键词堆砌；建议自然融入
- ✅ 未忽略搜索意图（明确「产品站+工具站」而非内容站）
- ⚠️ **重复内容未处理** → `SEO-ITEM-4.2` / `SEO-PLAN-2.1`
- ⚠️ **Schema 缺失** → `SEO-ITEM-3.x`
- ⚠️ **内链缺失** → `SEO-ITEM-4.3`
- ⚠️ **性能未达标** → §9（聚合 3–4.5s）
- ⚠️ **无排名追踪** → §8（需接 GSC/GA4）
- ⛔ **数据不完整项**：所有关键词**搜索量/竞争度未标注真实数值**——当前无付费数据源（Ahrefs/Semrush），上表聚类为**基于意图与场景的推断**，非真实 volume。接入数据源后需回填。
