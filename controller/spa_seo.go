package controller

import (
	"html"
	"regexp"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/model"
	"github.com/lza6/new-api-Max/service"
)

// SPA 的服务端 SEO 注入（Batch-12 / SEO-1）。
//
// ## 为什么需要
//
// 本前端是**纯客户端渲染**：`web/dist/index.html` 是一个壳，所有路由共用同一份
// 字节。实测（2026-10-11）6 个公开页返回的都是**完全相同的 3982 字节**，其中：
//   - `<title>` 全站同一个（路由级 meta 由 JS 运行时写，爬虫看不到）；
//   - **`<h1>` 数量为 0**；
//   - **`<link rel="canonical" href="/">` 是写死的** —— 等于让 `/pricing`、`/about`
//     等子页主动声明"我的正版是首页"，会直接压制它们被收录；
//   - 壳里没有任何可跟随的 `<a href>`，所以外部爬虫看到「内部深度 0、5 页不可达」。
//
// 这不是"内容不够好"，而是**爬虫根本看不到内容**。因此修法不是堆文案，而是
// 在服务端把路由级 meta 与一份**准确**的内容块注入进壳：
//   - **meta 注入**让搜索引擎（含执行 JS 的 Google，meta 以原始 HTML 为准）拿到
//     正确的 title/description/canonical/OG；
//   - **内容块**（`<h1>` + 摘要 + 站内链接 + 官方文档外链）给不执行 JS 的爬虫
//     提供可索引的真实内容与可跟随的链接；
//   - 内容块在 React 挂载后被移除，**用户看不到**它 —— 因此它必须与页面真实内容
//     一致（否则构成 cloaking）。这里只写"这个页面是什么"，不写任何未经证实的数字。
//
// ## 边界
//
// - 只对 `seoPublicPaths` 里的公开路由注入；登录后页面与 API 路径不受影响。
// - 未知路径走默认 meta（不再有"所有子页都 canonical 到首页"这种错误声明）。
// - 不新增任何文件、不引入模板引擎：全是对已嵌入壳的定点替换。

// spaRouteMeta 是一个公开路由的服务端渲染元数据。
type spaRouteMeta struct {
	// Title 页面标题；`{site}` 会被替换为运行时站点名。
	Title string
	// Description 供 meta/OG 使用的一句话描述（准确、无夸大）。
	Description string
	// H1 内容块的可见一级标题。
	H1 string
	// Summary 内容块的正文摘要。**必须描述页面真实内容**，不得填充关键词。
	Summary string
	// Caveat 一条诚实的边界/限制说明（对应"helpful content 缺少 caveat"的信号）。
	Caveat string
	// NavLabel 站内导航里的短文案（不含 {site} 展开后的长标题，避免导航条过长）。
	// 为空时回退到 H1。
	NavLabel string
}

// spaDefaultMeta 是未在路由表中命中时的兜底（英文）。
//
// 关键：它的 canonical 由**实际请求路径**决定（见 BuildSPAIndexHTML），
// 而不是写死 "/"，这样未知路径也不会错误地把权重指向首页。
var spaDefaultMeta = spaRouteMeta{
	Title:       "{site}",
	Description: "AI API gateway exposing OpenAI-, Claude- and Gemini-compatible endpoints over one unified base URL.",
	H1:          "{site}",
	Summary:     "{site} is an AI API gateway. The pages indexed here are the public ones: model pricing, usage rankings, the model test console and client setup guidance.",
	Caveat:      "Availability and model coverage depend on the upstream channels the operator has configured for this deployment.",
}

// spaDefaultMetaZh 是兜底文案的中文版（与英文版同义）。
var spaDefaultMetaZh = spaRouteMeta{
	Title:       "{site}",
	Description: "AI API 网关：用同一个基础地址提供 OpenAI、Claude、Gemini 兼容接口。",
	H1:          "{site}",
	Summary:     "{site} 是一个 AI API 网关。可被收录的公开页包括：模型价格、用量排行、模型测试台与客户端配置。",
	Caveat:      "可用的模型与可用性取决于本部署所配置的上游渠道。",
}

// spaResolveLanguage 决定注入内容使用哪种语言。
//
// 规则（按顺序）：解析 `Accept-Language` 的偏好顺序 ——
//  1. 第一个能识别的语言是中文 → zh
//  2. 第一个能识别的语言是英文 → en
//  3. **其它语言或无法识别 → zh**（本站主要受众是中文用户，默认中文）
//
// 刻意只做"整站一种语言"而不是按 URL 分语言：本站 7 种语言是**同一 URL 的运行时切换**，
// 没有独立的语言化 URL。给同一 URL 输出多语言内容会与 canonical 语义冲突
// （也必须**不做** hreflang —— 那会变成指向自身）。
func spaResolveLanguage(acceptLanguage string) string {
	for _, tag := range spaAcceptLanguageOrder(acceptLanguage) {
		switch {
		case strings.HasPrefix(tag, "zh"):
			return "zh"
		case strings.HasPrefix(tag, "en"):
			return "en"
		}
	}
	return "zh"
}

// spaAcceptLanguageOrder 把 `Accept-Language` 头拆成按偏好排序的语言标签（小写）。
// 只取 `;q=` 之前的部分；不做完整 q 值排序 —— 头里的书写顺序本身就是优先级顺序，
// 且浏览器极少发出乱序 q 值。
func spaAcceptLanguageOrder(header string) []string {
	if strings.TrimSpace(header) == "" {
		return nil
	}
	parts := strings.Split(header, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		tag, _, _ := strings.Cut(p, ";")
		tag = strings.ToLower(strings.TrimSpace(tag))
		if tag == "" || tag == "*" {
			continue
		}
		out = append(out, tag)
	}
	return out
}

// spaHTMLLang 把内部语言代码映射到 `<html lang>` 取值。
func spaHTMLLang(language string) string {
	if language == "en" {
		return "en"
	}
	return "zh-CN"
}

// spaRouteMetaFor 按语言与路径取路由文案（缺中文时回退英文，避免出现空标题）。
func spaRouteMetaFor(path, language string) spaRouteMeta {
	normalized := strings.TrimRight(path, "/")
	if normalized == "" {
		normalized = "/"
	}
	if language == "zh" {
		if meta, ok := spaRouteMetaZh[normalized]; ok {
			return meta
		}
		return spaDefaultMetaZh
	}
	for _, p := range seoPublicPaths {
		if p.Path == normalized {
			return p.Meta
		}
	}
	return spaDefaultMeta
}

// spaSiteBaseForSEO 返回用于 canonical/OG 的站点根地址（无尾斜杠）。
func spaSiteBaseForSEO() string { return seoSiteBase() }

// absoluteSEOURL 把站内路径拼成绝对 URL（canonical 必须是绝对地址才有意义）。
func absoluteSEOURL(base, path string) string {
	if path == "" {
		path = "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	// 首页保持 `https://host/`；其它路径保持无尾斜杠，避免与 sitemap 里的写法分叉。
	if path == "/" {
		return base + "/"
	}
	return base + strings.TrimRight(path, "/")
}

// spaInternalLinksHTML 生成内容块里的站内链接。
//
// 这是修「5 页不可达 / 内部深度 0」的关键：不执行 JS 的爬虫只有拿到真实的
// `<a href>` 才能发现这些页面。链接文案优先用短的 NavLabel，而不是 "click here"
// 之类的泛化锚文本；语言跟随访问者。
func spaInternalLinksHTML(currentPath, siteName, language string) string {
	var b strings.Builder
	b.WriteString("<nav aria-label=\"Public pages\"><ul>")
	for _, p := range seoPublicPaths {
		if strings.TrimRight(p.Path, "/") == strings.TrimRight(currentPath, "/") {
			continue // 不给自己加自链接
		}
		b.WriteString("<li><a href=\"")
		b.WriteString(html.EscapeString(p.Path))
		b.WriteString("\">")
		b.WriteString(html.EscapeString(spaLinkLabel(p, siteName, language)))
		b.WriteString("</a></li>")
	}
	b.WriteString("</ul></nav>")
	return b.String()
}

// spaLinkLabel 取一个路由的链接文案（按语言）。
//
// 注意：NavLabel/H1 里可能含 `{site}` 占位 —— **必须在这里展开**，
// 否则占位符会原样出现在页面上（单测已锁死这条）。
func spaLinkLabel(p seoPublicPath, siteName, language string) string {
	meta := p.Meta
	if language == "zh" {
		if zh, ok := spaRouteMetaZh[p.Path]; ok {
			meta = zh
		}
	}
	if meta.NavLabel != "" {
		return spaExpandSite(meta.NavLabel, siteName)
	}
	if meta.H1 != "" {
		return spaExpandSite(meta.H1, siteName)
	}
	return p.Path
}

// spaOutboundLinksHTML 生成官方文档外链（标签随语言，地址不翻译）。
//
// 目的有二：① 对应"没有指向外部来源的链接"这一信号；② 对所聚合的上游提供方
// 做正确的归属指引，读者可自行核对协议细节。链接均指向提供方**官方**文档站。
func spaOutboundLinksHTML(language string) string {
	var b strings.Builder
	if language == "zh" {
		b.WriteString("<p>上游协议参考：")
	} else {
		b.WriteString("<p>Upstream protocol references: ")
	}
	for i, l := range spaExternalDocLinks {
		if i > 0 {
			b.WriteString(" · ")
		}
		label := l.LabelEn
		if language == "zh" {
			label = l.LabelZh
		}
		b.WriteString("<a href=\"")
		b.WriteString(html.EscapeString(l.Href))
		b.WriteString("\" rel=\"noopener\" target=\"_blank\">")
		b.WriteString(html.EscapeString(label))
		b.WriteString("</a>")
	}
	b.WriteString("</p>")
	return b.String()
}

// spaSEORemoverScript 在 React 真正渲染出内容后移除 SEO 内容块。
//
// 为什么用 MutationObserver 而不是定时器/首帧：React 何时挂载取决于
// i18n 语言包加载等异步步骤，定时器要么早（内容被删、界面空白）要么晚
// （用户看到一段不属于本页顶部的文字）。观察 `#root` 首次出现子节点才是
// 与框架无关的正确时机；另有 4s 兜底，避免异常时内容块永久滞留。
const spaSEORemoverScript = `<script>(function(){` +
	`var seo=document.getElementById('spa-seo-content');if(!seo)return;` +
	`var root=document.getElementById('root');if(!root){seo.remove();return}` +
	`var done=false;function rm(){if(done)return;if(root.childElementCount>0){done=true;seo.remove();if(obs)obs.disconnect()}}` +
	`var obs=window.MutationObserver?new MutationObserver(rm):null;` +
	`if(obs)obs.observe(root,{childList:true});setTimeout(rm,4000)})();</script>`

// 定点替换用的正则。壳里的属性可能跨行（如 `description` 的 content 独占一行），
// 因此所有 `\s` 都要能跨行匹配 —— Go 的 `\s` 默认包含 `\n`。
var (
	reSPATitle       = regexp.MustCompile(`(?s)<title>.*?</title>`)
	reSPACanonical   = regexp.MustCompile(`(?s)<link\s+rel="canonical"\s+href="[^"]*"\s*/?>`)
	reSPAMetaName    = regexp.MustCompile(`(?s)<meta\s+name="description"\s+content="[^"]*"\s*/?>`)
	reSPAMetaTitle   = regexp.MustCompile(`(?s)<meta\s+name="title"\s+content="[^"]*"\s*/?>`)
	reSPAOGTitle     = regexp.MustCompile(`(?s)<meta\s+property="og:title"\s+content="[^"]*"\s*/?>`)
	reSPAOGDesc      = regexp.MustCompile(`(?s)<meta\s+property="og:description"\s+content="[^"]*"\s*/?>`)
	reSPAOGURL       = regexp.MustCompile(`(?s)<meta\s+property="og:url"\s+content="[^"]*"\s*/?>`)
	reSPATwTitle     = regexp.MustCompile(`(?s)<meta\s+name="twitter:title"\s+content="[^"]*"\s*/?>`)
	reSPATwDesc      = regexp.MustCompile(`(?s)<meta\s+name="twitter:description"\s+content="[^"]*"\s*/?>`)
	reSPABodyOpenTag = regexp.MustCompile(`(?s)<body[^>]*>`)
	reSPAHTMLTag     = regexp.MustCompile(`(?s)<html[^>]*>`)
)

// BuildSPAIndexHTML 把路由级 SEO 元数据与一份准确的内容块注入 SPA 壳。
//
// 入参 indexPage 是 `go:embed` 进来的原始 `web/dist/index.html`；
// 从 `c` 取请求路径（决定路由文案与 canonical）与 `Accept-Language`（决定语言，默认中文），
// 并在公开页 `/pricing` 上附加一张**服务端渲染的价目表**。
//
// 设计上**每次都重新构造**而不做缓存：文档只有约 4 KB，几次正则替换是微秒级，
// 换来的是"站点名/域名/语言/价格改了立即生效"，不必维护缓存失效。
func BuildSPAIndexHTML(c *gin.Context, indexPage []byte) []byte {
	page := string(indexPage)
	requestPath := "/"
	acceptLanguage := ""
	if c != nil && c.Request != nil {
		requestPath = c.Request.URL.Path
		acceptLanguage = c.GetHeader("Accept-Language")
	}
	base := spaSiteBaseForSEO()
	siteName := strings.TrimSpace(common.SystemName)
	if siteName == "" {
		siteName = "New API"
	}
	language := spaResolveLanguage(acceptLanguage)

	meta := spaRouteMetaFor(requestPath, language)
	title := spaExpandSite(meta.Title, siteName)
	summary := spaExpandSite(meta.Summary, siteName)
	h1 := spaExpandSite(meta.H1, siteName)
	caveat := spaExpandSite(meta.Caveat, siteName)
	description := spaExpandSite(meta.Description, siteName)

	canonical := absoluteSEOURL(base, requestPath)
	esc := html.EscapeString

	// ---- meta 替换 ----
	page = reSPATitle.ReplaceAllString(page, "<title>"+esc(title)+"</title>")
	page = reSPAMetaTitle.ReplaceAllString(page, `<meta name="title" content="`+esc(title)+`" />`)
	page = reSPAMetaName.ReplaceAllString(page, `<meta name="description" content="`+esc(description)+`" />`)
	page = reSPACanonical.ReplaceAllString(page, `<link rel="canonical" href="`+esc(canonical)+`" />`)
	page = reSPAOGTitle.ReplaceAllString(page, `<meta property="og:title" content="`+esc(title)+`" />`)
	page = reSPAOGDesc.ReplaceAllString(page, `<meta property="og:description" content="`+esc(description)+`" />`)
	// og:locale 壳里**没有** —— 与 og:url 一起追加（否则要单独插一条无从匹配的正则）。
	page = reSPAOGURL.ReplaceAllString(page,
		`X content="`+esc(canonical)+`" />`+
			"\n    "+`<meta property="og:locale" content="`+esc(spaOGLocale(language))+`" />`)
	page = reSPATwTitle.ReplaceAllString(page, `<meta name="twitter:title" content="`+esc(title)+`" />`)
	page = reSPATwDesc.ReplaceAllString(page, `<meta name="twitter:description" content="`+esc(description)+`" />`)
	// 壳里的 `<html lang="en">` 是写死的：内容语言与 `lang` 不一致会被判为语言标注错误。
	page = reSPAHTMLTag.ReplaceAllString(page, `<html lang="`+esc(spaHTMLLang(language))+`">`)

	// ---- 内容块注入 ----
	block := spaBuildContentBlock(h1, summary, caveat, requestPath, siteName, language)
	// /pricing 额外附一张服务端渲染的价目表：
	// 页面上的价格是 JS 拉取后渲染的，爬虫看不到；这是把「定价透明」变成可索引内容。
	if strings.TrimRight(requestPath, "/") == "/pricing" {
		block += spaPricingTableHTML(c, language)
	}
	if loc := reSPABodyOpenTag.FindStringIndex(page); loc != nil {
		page = page[:loc[1]] + block + page[loc[1]:]
	}

	return []byte(page)
}

// spaPricingModelsForCrawler 取「未登录访客视角」的价目列表 —— 与页面上的
// `/api/pricing` 用同一数据源与同一分组过滤，因此表里的模型集合与用户看到的一致。
func spaPricingModelsForCrawler() []model.Pricing {
	pricing := model.GetPricing()
	// 未登录：没有用户分组 → 用空分组取可用分组集合（与 buildPricingResponse 的
	// `exists == false` 分支等价），得到公开可见的那部分模型。
	usable := service.GetUserUsableGroups("")
	return filterPricingByUsableGroups(pricing, usable)
}

// spaPricingMaxRows 是价目表的行数上限：给爬虫一条可比较的清单，但**不让页面无限增长**
// （模型很多时 HTML 会膨胀，且注入块的主要读者是爬虫而非用户）。
const spaPricingMaxRows = 120

// spaPricingTableHTML 生成服务端渲染的价目表。
//
// **只渲染能准确渲染的字段。** 页面上「一次调用 ¥X」是前端按计费表达式/阶梯算出来的，
// Go 侧没有等价实现 —— 我不会把那段逻辑半吊子地照搬过来：**公开页面上的价格算错
// 比不显示价格更糟**。因此这里：
//   - 按次计费（quota_type=1）→ 直接给出该模型的固定价格；
//   - 按 token 计费 → 给出倍率，并明确说明"实际费用 = 倍率 × 用量"；
//   - 使用计费表达式（阶梯/动态）的模型 → 只标注计费方式，**不猜一个数字**。
func spaPricingTableHTML(c *gin.Context, language string) string {
	_ = c
	pricing := spaPricingModelsForCrawler()
	if len(pricing) == 0 {
		return ""
	}
	zh := language == "zh"
	esc := html.EscapeString

	head := []string{"Model", "Billing mode", "Price"}
	if zh {
		head = []string{"模型", "计费方式", "价格"}
	}

	var b strings.Builder
	b.WriteString(`<table id="spa-seo-pricing"><thead><tr>`)
	for _, h := range head {
		b.WriteString("<th>")
		b.WriteString(esc(h))
		b.WriteString("</th>")
	}
	b.WriteString("</tr></thead><tbody>")
	for i, p := range pricing {
		if i >= spaPricingMaxRows {
			break
		}
		b.WriteString("<tr><td>")
		b.WriteString(esc(p.ModelName))
		b.WriteString("</td><td>")
		b.WriteString(esc(spaPricingModeLabel(p, zh)))
		b.WriteString("</td><td>")
		b.WriteString(esc(spaPricingValueLabel(p, zh)))
		b.WriteString("</td></tr>")
	}
	b.WriteString("</tbody></table>")
	if len(pricing) > spaPricingMaxRows {
		if zh {
			b.WriteString("<p>（表中仅列出前 ")
		} else {
			b.WriteString("<p>(showing the first ")
		}
		b.WriteString(itoaForSEO(spaPricingMaxRows))
		if zh {
			b.WriteString(" 个模型；完整列表见本页交互界面。）</p>")
		} else {
			b.WriteString(" models; the full list is in the interactive view on this page.)</p>")
		}
	}
	return b.String()
}

// spaPricingModeLabel 描述计费方式（与 model.Pricing 的字段语义一一对应）。
func spaPricingModeLabel(p model.Pricing, zh bool) string {
	switch {
	case strings.TrimSpace(p.BillingExpr) != "" || strings.TrimSpace(p.BillingMode) != "" && p.BillingMode != "fixed":
		if zh {
			return "按用量表达式计费"
		}
		return "Usage-based (billing expression)"
	case p.QuotaType == 1:
		if zh {
			return "按次计费"
		}
		return "Per request"
	default:
		if zh {
			return "按 token 计费"
		}
		return "Per token"
	}
}

// spaPricingValueLabel 描述价格。上面注释解释了为什么这里**不给阶梯/表达式模型编数字**。
func spaPricingValueLabel(p model.Pricing, zh bool) string {
	switch {
	case strings.TrimSpace(p.BillingExpr) != "" || (strings.TrimSpace(p.BillingMode) != "" && p.BillingMode != "fixed"):
		if zh {
			return "见本页交互界面（按用量计算）"
		}
		return "see the interactive view (computed from usage)"
	case p.QuotaType == 1:
		return "$" + trimFloat(p.ModelPrice) + " " + map[bool]string{true: "/ 次", false: "/ request"}[zh]
	default:
		if zh {
			return "倍率 " + trimFloat(p.ModelRatio) + "（实际费用 = 倍率 × 用量）"
		}
		return "ratio " + trimFloat(p.ModelRatio) + " (actual cost = ratio x usage)"
	}
}

// trimFloat 去掉浮点数的尾随零（避免 "0.500000" 这种难读的输出）。
func trimFloat(v float64) string {
	s := strconv.FormatFloat(v, 'f', -1, 64)
	return s
}

// itoaForSEO 是给拼 HTML 用的十进制转换。
func itoaForSEO(v int) string { return strconv.Itoa(v) }

// spaOGLocale 返回 og:locale 取值。
func spaOGLocale(language string) string {
	if language == "en" {
		return "en_US"
	}
	return "zh_CN"
}

// spaBuildContentBlock 构造给爬虫看的内容块。
//
// 放在 `#root` **之前**、`<body>` 之后：React 挂载进 `#root` 不会碰到它，
// 再由 `spaSEORemoverScript` 在挂载完成后移除 —— 用户看不到，爬虫看得到。
func spaBuildContentBlock(h1, summary, caveat, requestPath, siteName, language string) string {
	esc := html.EscapeString
	var b strings.Builder
	b.WriteString(`<div id="spa-seo-content">`)
	b.WriteString("<h1>")
	b.WriteString(esc(h1))
	b.WriteString("</h1>")
	b.WriteString("<p>")
	b.WriteString(esc(summary))
	b.WriteString("</p>")
	if caveat != "" {
		b.WriteString("<p>")
		b.WriteString(esc(caveat))
		b.WriteString("</p>")
	}
	b.WriteString(spaInternalLinksHTML(requestPath, siteName, language))
	b.WriteString(spaOutboundLinksHTML(language))
	b.WriteString("</div>")
	b.WriteString(spaSEORemoverScript)
	return b.String()
}

// spaExpandSite 把 `{site}` 占位替换为运行时站点名。
func spaExpandSite(s, siteName string) string {
	if s == "" {
		return ""
	}
	return strings.ReplaceAll(s, "{site}", siteName)
}
