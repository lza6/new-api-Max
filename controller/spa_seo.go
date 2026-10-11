package controller

import (
	"html"
	"regexp"
	"strings"

	"github.com/lza6/new-api-Max/common"
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

// spaDefaultMeta 是未在路由表中命中时的兜底。
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

// spaMetaByPath 在 seoPublicPaths 里查路由元数据。
func spaMetaByPath(path string) (spaRouteMeta, bool) {
	normalized := strings.TrimRight(path, "/")
	if normalized == "" {
		normalized = "/"
	}
	for _, p := range seoPublicPaths {
		if p.Path == normalized {
			return p.Meta, true
		}
	}
	return spaRouteMeta{}, false
}

// spaInternalLinksHTML 生成内容块里的站内链接。
//
// 这是修「5 页不可达 / 内部深度 0」的关键：不执行 JS 的爬虫只有拿到真实的
// `<a href>` 才能发现这些页面。链接文案优先用短的 NavLabel，而不是 "click here"
// 之类的泛化锚文本。
func spaInternalLinksHTML(currentPath, siteName string) string {
	var b strings.Builder
	b.WriteString("<nav aria-label=\"Public pages\"><ul>")
	for _, p := range seoPublicPaths {
		if strings.TrimRight(p.Path, "/") == strings.TrimRight(currentPath, "/") {
			continue // 不给自己加自链接
		}
		b.WriteString("<li><a href=\"")
		b.WriteString(html.EscapeString(p.Path))
		b.WriteString("\">")
		b.WriteString(html.EscapeString(spaLinkLabel(p, siteName)))
		b.WriteString("</a></li>")
	}
	b.WriteString("</ul></nav>")
	return b.String()
}

// spaLinkLabel 取一个路由的链接文案。
//
// 注意：NavLabel/H1 里可能含 `{site}` 占位 —— **必须在这里展开**，
// 否则占位符会原样出现在页面上（单测已锁死这条）。
func spaLinkLabel(p seoPublicPath, siteName string) string {
	if p.Meta.NavLabel != "" {
		return spaExpandSite(p.Meta.NavLabel, siteName)
	}
	if p.Meta.H1 != "" {
		return spaExpandSite(p.Meta.H1, siteName)
	}
	return p.Path
}

// spaOutboundLinksHTML 生成官方文档外链。
//
// 目的有二：① 对应"没有指向外部来源的链接"这一信号；② 对所聚合的上游提供方
// 做正确的归属指引，读者可自行核对协议细节。链接均指向提供方**官方**文档站。
func spaOutboundLinksHTML() string {
	links := []struct{ href, label string }{
		{"https://platform.openai.com/docs/api-reference", "OpenAI API reference"},
		{"https://docs.anthropic.com/en/api/overview", "Anthropic (Claude) API overview"},
		{"https://ai.google.dev/gemini-api/docs", "Google Gemini API docs"},
	}
	var b strings.Builder
	b.WriteString("<p>Upstream protocol references: ")
	for i, l := range links {
		if i > 0 {
			b.WriteString(" · ")
		}
		b.WriteString("<a href=\"")
		b.WriteString(html.EscapeString(l.href))
		b.WriteString("\" rel=\"noopener\" target=\"_blank\">")
		b.WriteString(html.EscapeString(l.label))
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
)

// BuildSPAIndexHTML 把路由级 SEO 元数据与一份准确的内容块注入 SPA 壳。
//
// 入参 indexPage 是 `go:embed` 进来的原始 `web/dist/index.html`；
// 返回可直接写回客户端的 HTML 字节。
//
// 设计上**每次都重新构造**而不做缓存：文档只有约 4 KB，几次正则替换是微秒级，
// 换来的是"站点名/域名改了立即生效"，不必维护缓存失效。
func BuildSPAIndexHTML(indexPage []byte, requestPath string) []byte {
	page := string(indexPage)
	base := spaSiteBaseForSEO()
	siteName := strings.TrimSpace(common.SystemName)
	if siteName == "" {
		siteName = "New API"
	}

	meta, ok := spaMetaByPath(requestPath)
	if !ok {
		meta = spaDefaultMeta
	}
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
	page = reSPAOGURL.ReplaceAllString(page, `<meta property="og:url" content="`+esc(canonical)+`" />`)
	page = reSPATwTitle.ReplaceAllString(page, `<meta name="twitter:title" content="`+esc(title)+`" />`)
	page = reSPATwDesc.ReplaceAllString(page, `<meta name="twitter:description" content="`+esc(description)+`" />`)

	// ---- 内容块注入 ----
	block := spaBuildContentBlock(h1, summary, caveat, requestPath, siteName)
	if loc := reSPABodyOpenTag.FindStringIndex(page); loc != nil {
		page = page[:loc[1]] + block + page[loc[1]:]
	}

	return []byte(page)
}

// spaBuildContentBlock 构造给爬虫看的内容块。
//
// 放在 `#root` **之前**、`<body>` 之后：React 挂载进 `#root` 不会碰到它，
// 再由 `spaSEORemoverScript` 在挂载完成后移除 —— 用户看不到，爬虫看得到。
func spaBuildContentBlock(h1, summary, caveat, requestPath, siteName string) string {
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
	b.WriteString(spaInternalLinksHTML(requestPath, siteName))
	b.WriteString(spaOutboundLinksHTML())
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
