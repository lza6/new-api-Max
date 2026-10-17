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
	//
	// 渲染时它会被提升为一个**显式小节**（`<h2>限制与边界</h2>` + 本字段原文）。
	// 内容不得因提升为小节而扩充 —— 仍然是这一句话。
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
	// 服务端注入的页面文案目前只有英文与简体中文两份。fr/ru/ja/vi 前缀由前端
	// 运行时切换 UI 语言，但服务端内容仍是英文；声明 lang=fr 会误导搜索引擎，
	// 因此这些前缀统一按内容语言 en 标注，并在 BuildSPAIndexHTML 中对它们加 noindex。
	switch strings.ToLower(language) {
	case "en", "fr", "ru", "ja", "vi":
		return "en"
	default:
		return "zh-CN"
	}
}

// spaPathLocales 是可以在 URL 路径前缀中出现的语言（默认语言 zh 不放前缀）。
// key 为小写路径段，value 为内部语言代码。
var spaPathLocales = map[string]string{
	"en":    "en",
	"fr":    "fr",
	"ru":    "ru",
	"ja":    "ja",
	"vi":    "vi",
	"zh-tw": "zh-TW",
}

// DefaultLocaleRedirectTarget 把默认语言 zh 的前缀地址 301 到无前缀地址。
//
// 默认语言不使用前缀（`/pricing` 就是 zh-CN 的 canonical），因此 `/zh/pricing`
// 必须重定向，否则同一内容会出现两份可索引 URL。其它语言前缀不重定向。
func DefaultLocaleRedirectTarget(path string) (string, bool) {
	if path == "/zh" {
		return "/", true
	}
	if strings.HasPrefix(path, "/zh/") {
		target := strings.TrimPrefix(path, "/zh")
		if target == "" {
			return "/", true
		}
		// 防御开放重定向：`/zh//evil.com`、`/zh/%2F%2Fevil.com` 会得到
		// 协议相对地址 `//evil.com`。只允许站内单斜杠路径，否则回到首页。
		if !strings.HasPrefix(target, "/") || strings.HasPrefix(target, "//") || strings.HasPrefix(target, `/\`) {
			return "/", true
		}
		return target, true
	}
	return "", false
}

// spaParseLocalePrefix 从请求路径中剥离语言前缀。
//
// 例：`/en/pricing` → ("en", "/pricing", true)；`/zh-TW/pricing` → ("zh-TW", "/pricing", true)；
// `/pricing` → ("", "/pricing", false)。默认语言 zh 不生成前缀，`/zh/...` 由路由层 301
// 到无前缀地址，避免同一内容出现两份 canonical。
func spaParseLocalePrefix(path string) (language, rest string, ok bool) {
	if !strings.HasPrefix(path, "/") {
		return "", path, false
	}
	trimmed := strings.TrimPrefix(path, "/")
	if trimmed == "" {
		return "", "/", false
	}
	segment, tail, found := strings.Cut(trimmed, "/")
	lang, exists := spaPathLocales[strings.ToLower(segment)]
	if !exists {
		return "", path, false
	}
	if !found || tail == "" {
		return lang, "/", true
	}
	return lang, "/" + tail, true
}

// spaPathLocaleTag 返回某语言在 URL 前缀中使用的写法（默认语言返回空串）。
func spaPathLocaleTag(language string) string {
	switch language {
	case "en", "fr", "ru", "ja", "vi":
		return language
	case "zh-TW":
		return "zh-TW"
	default:
		return "" // zh-CN：无前缀
	}
}

// spaLocalizedPath 给站内路径加上语言前缀（默认语言 zh 不加）。
func spaLocalizedPath(language, path string) string {
	if path == "" {
		path = "/"
	}
	tag := spaPathLocaleTag(language)
	if tag == "" {
		return path
	}
	if path == "/" {
		return "/" + tag
	}
	return "/" + tag + path
}

// spaHreflangTag 返回 hreflang 使用的 BCP-47 标签。
func spaHreflangTag(language string) string {
	switch language {
	case "en":
		return "en"
	case "zh-TW":
		return "zh-TW"
	default:
		return "zh-CN"
	}
}

// spaHreflangLanguages 是**服务端确有对应语言内容**、可以诚实声明 hreflang 的语言。
//
// 前端 UI 支持 7 种语言，但服务端注入的页面文案目前只有英文与中文两份；
// 对 fr/ru/ja/vi 声明 hreflang 会让搜索引擎把英文内容标成法语/俄语等，
// 属于错误标注。因此这四门语言仍可通过 `/fr/...` 访问（UI 会切换），
// 但在补齐服务端翻译前**不声明** hreflang。
var spaHreflangLanguages = []string{"zh", "en"}

const spaDefaultHreflangLanguage = "zh"

// spaIsChinese 报告该语言是否使用中文文案（简繁共用同一份服务端文案）。
func spaIsChinese(language string) bool {
	switch strings.ToLower(language) {
	case "zh", "zh-tw", "zhtw", "zh-cn", "zhcn":
		return true
	default:
		return false
	}
}

// spaRouteMetaFor 按语言与路径取路由文案（缺中文时回退英文，避免出现空标题）。
func spaRouteMetaFor(path, language string) spaRouteMeta {
	normalized := strings.TrimRight(path, "/")
	if normalized == "" {
		normalized = "/"
	}
	if spaIsChinese(language) {
		if meta, ok := spaRouteMetaZh[normalized]; ok {
			return meta
		}
		return spaDefaultMetaZh
	}
	// 政策页正文来自后台配置，默认不进 sitemap；但页面本身是公开可访问的，
	// 仍应给出与页面标题一致的 meta（否则爬虫看到的是站点兜底标题）。
	switch normalized {
	case "/user-agreement":
		return spaRouteMeta{
			Title:       "User Agreement — {site}",
			Description: "The user agreement configured by the operator of this deployment.",
			H1:          "User Agreement",
			NavLabel:    "User Agreement",
			Summary:     "This page shows the user agreement configured for this deployment. Its content is provided by the operator.",
			Caveat:      "If the operator has not configured this document, the page may be empty.",
		}
	case "/privacy-policy":
		return spaRouteMeta{
			Title:       "Privacy Policy — {site}",
			Description: "The privacy policy configured by the operator of this deployment.",
			H1:          "Privacy Policy",
			NavLabel:    "Privacy Policy",
			Summary:     "This page shows the privacy policy configured for this deployment. Its content is provided by the operator.",
			Caveat:      "If the operator has not configured this document, the page may be empty.",
		}
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

// spaPolicyNavLinks 是内容块站内导航里额外给出的「用户协议 / 隐私政策」链接。
//
// 为什么**不**把它们放进 `seoPublicPaths`：这两个前端路由确实存在
// （`web/src/routes/user-agreement.tsx`、`web/src/routes/privacy-policy.tsx`），
// 但它们的正文来自后端配置项 `setting/system_setting/legal.go`，其默认值是**空字符串** ——
// 纳入 `seoPublicPaths` 会让 sitemap.xml 收录两个可能为空的页面。而在站内导航里给出
// 可跟随的 `<a href>` 既不承诺页面有内容，又让爬虫能发现、用户能点。
//
// 文案取自页面自己的可见标题（`t('User Agreement')` / `t('Privacy Policy')`），
// 与 `web/src/components/layout/components/footer.tsx` 的页脚链接一致 —— 不用
// "Terms of Service" 这类页面标题里并不存在的说法。
var spaPolicyNavLinks = []struct{ Path, LabelEn, LabelZh string }{
	{"/user-agreement", "User Agreement", "用户协议"},
	{"/privacy-policy", "Privacy Policy", "隐私政策"},
}

// spaInternalLinksHTML 生成内容块里的站内链接。
//
// 这是修「5 页不可达 / 内部深度 0」的关键：不执行 JS 的爬虫只有拿到真实的
// `<a href>` 才能发现这些页面。链接文案优先用短的 NavLabel，而不是 "click here"
// 之类的泛化锚文本；语言跟随访问者。
func spaInternalLinksHTML(currentPath, siteName, language, prefixLanguage string) string {
	current := strings.TrimRight(currentPath, "/")
	var b strings.Builder
	b.WriteString("<nav aria-label=\"Public pages\"><ul>")
	for _, p := range seoPublicPaths {
		if strings.TrimRight(p.Path, "/") == current {
			continue // 不给自己加自链接
		}
		b.WriteString("<li><a href=\"")
		b.WriteString(html.EscapeString(spaLocalizedPath(prefixLanguage, p.Path)))
		b.WriteString("\">")
		b.WriteString(html.EscapeString(spaLinkLabel(p, siteName, language)))
		b.WriteString("</a></li>")
	}
	// 用户协议与隐私政策：不参与 sitemap，但必须在导航里可达（见 spaPolicyNavLinks 注释）。
	for _, l := range spaPolicyNavLinks {
		if l.Path == current {
			continue
		}
		label := l.LabelEn
		if spaIsChinese(language) {
			label = l.LabelZh
		}
		b.WriteString("<li><a href=\"")
		b.WriteString(html.EscapeString(spaLocalizedPath(prefixLanguage, l.Path)))
		b.WriteString("\">")
		b.WriteString(html.EscapeString(label))
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
	if spaIsChinese(language) {
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
	if spaIsChinese(language) {
		b.WriteString("<p>上游协议参考：")
	} else {
		b.WriteString("<p>Upstream protocol references: ")
	}
	for i, l := range spaExternalDocLinks {
		if i > 0 {
			b.WriteString(" · ")
		}
		label := l.LabelEn
		if spaIsChinese(language) {
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
//
// 按 id 兜底清理所有 SEO 注入元素：即使未来某个可见附加块被拼在根容器外，
// 也不会在 SPA 路由切换后残留在页面上。
const spaSEORemoverScript = `<script>(function(){` +
	`var ids=['spa-seo-content','spa-seo-pricing'];` +
	`function rmAll(){for(var i=0;i<ids.length;i++){var e=document.getElementById(ids[i]);if(e&&e.parentNode)e.parentNode.removeChild(e)}}` +
	`var seo=document.getElementById('spa-seo-content');if(!seo){rmAll();return}` +
	`var root=document.getElementById('root');if(!root){rmAll();return}` +
	`var done=false;function rm(){if(done)return;if(root.childElementCount>0){done=true;rmAll();if(obs)obs.disconnect()}}` +
	`var obs=window.MutationObserver?new MutationObserver(rm):null;` +
	`if(obs)obs.observe(root,{childList:true});setTimeout(rm,4000)})();</script>`

// 定点替换用的正则。壳里的属性可能跨行（如 `description` 的 content 独占一行），
// 因此所有 `\s` 都要能跨行匹配 —— Go 的 `\s` 默认包含 `\n`。
var (
	// 只匹配**行首**的 `<title>`：index.html 的注释里可能出现字面 `<title>`，
	// 从注释内开始匹配会把注释尾部吃掉，导致整段 head 被浏览器当注释吞掉
	// （真实浏览器 E2E 曾观测到 canonical/meta 消失）。
	reSPATitle       = regexp.MustCompile(`(?m)^[ \t]*<title>.*?</title>`)
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
	// 语言前缀优先于 Accept-Language：`/en/pricing` 必须输出英文内容与英文 canonical。
	// 默认语言 zh 不带前缀；`/zh/...` 由路由层 301 到无前缀地址。
	// prefixLanguage 只用于**生成链接**：无前缀请求（默认语言或按 Accept-Language
	// 协商内容）保持无前缀 canonical，避免把现有 URL 全部改写成 /en。
	language := ""
	prefixLanguage := ""
	hasLocalePrefix := false
	if localeLanguage, rest, ok := spaParseLocalePrefix(requestPath); ok {
		language = localeLanguage
		prefixLanguage = localeLanguage
		requestPath = rest
		hasLocalePrefix = true
	} else {
		language = spaResolveLanguage(acceptLanguage)
	}
	base := spaSiteBaseForSEO()
	siteName := strings.TrimSpace(common.SystemName)
	if siteName == "" {
		siteName = "New API"
	}

	meta := spaRouteMetaFor(requestPath, language)
	title := spaExpandSite(meta.Title, siteName)
	summary := spaExpandSite(meta.Summary, siteName)
	h1 := spaExpandSite(meta.H1, siteName)
	caveat := spaExpandSite(meta.Caveat, siteName)
	description := spaExpandSite(meta.Description, siteName)

	canonical := absoluteSEOURL(base, spaLocalizedPath(prefixLanguage, requestPath))
	esc := html.EscapeString

	// ---- meta 替换 ----
	// ReplaceAllString 会把替换串里的 $1/${name} 当捕获组展开；URL/标题里
	// 出现 $ 时必须转义为 $$，否则 canonical/meta 会被静默改写。
	safe := func(v string) string { return strings.ReplaceAll(v, "$", "$$") }
	page = reSPATitle.ReplaceAllString(page, "<title>"+safe(esc(title))+"</title>")
	page = reSPAMetaTitle.ReplaceAllString(page, `<meta name="title" content="`+safe(esc(title))+`" />`)
	page = reSPAMetaName.ReplaceAllString(page, `<meta name="description" content="`+safe(esc(description))+`" />`)
	page = reSPACanonical.ReplaceAllString(page, `<link rel="canonical" href="`+safe(esc(canonical))+`" />`)
	page = reSPAOGTitle.ReplaceAllString(page, `<meta property="og:title" content="`+safe(esc(title))+`" />`)
	page = reSPAOGDesc.ReplaceAllString(page, `<meta property="og:description" content="`+safe(esc(description))+`" />`)
	// og:locale 壳里**没有** —— 与 og:url 一起追加（否则要单独插一条无从匹配的正则）。
	page = reSPAOGURL.ReplaceAllString(page,
		`<meta property="og:url" content="`+safe(esc(canonical))+`" />`+
			"\n    "+`<meta property="og:locale" content="`+safe(esc(spaOGLocale(language)))+`" />`)
	page = reSPATwTitle.ReplaceAllString(page, `<meta name="twitter:title" content="`+safe(esc(title))+`" />`)
	page = reSPATwDesc.ReplaceAllString(page, `<meta name="twitter:description" content="`+safe(esc(description))+`" />`)
	// 壳里的 `<html lang="en">` 是写死的：内容语言与 `lang` 不一致会被判为语言标注错误。
	page = reSPAHTMLTag.ReplaceAllString(page, `<html lang="`+esc(spaHTMLLang(language))+`">`)

	// hreflang：只对**服务端确有对应语言内容**的语言声明（见 spaHreflangLanguages）。
	// 同一条内容的不同语言 URL 必须互相列出，并给出 x-default。
	if spaShouldAdvertiseHreflang(requestPath) {
		page = strings.Replace(page, "</head>",
			spaHreflangLinks(base, requestPath)+"\n  </head>", 1)
	}

	// TODO SEO-ITEM-1.4：登录/注册/找回密码等入口页不参与索引。
	// 这些页面没有对搜索者有用的公开内容，被收录只会挤占站点配额并与登录页重复。
	if spaNoIndexPath(requestPath) || (hasLocalePrefix && !spaHasServerTranslation(language)) {
		page = strings.Replace(page, "</head>", `<meta name="robots" content="noindex, nofollow" />`+"\n  </head>", 1)
	}

	// ---- 内容块注入 ----
	// 可见的附加内容必须放进 `#spa-seo-content`，否则移除脚本不会删掉它。
	extraHTML := ""
	if strings.TrimRight(requestPath, "/") == "/pricing" {
		extraHTML = spaPricingTableHTML(c, language)
	}
	block := spaBuildContentBlock(h1, summary, caveat, requestPath, siteName, language, prefixLanguage, extraHTML)
	// 结构化数据（JSON-LD）刻意留在容器**外**：它是 `<script>`、不渲染给用户，
	// 而移除脚本会在 React 挂载后删掉整个 `#spa-seo-content` —— 放进容器里会连它一起删掉。
	block += spaJSONLDScript(meta, requestPath, siteName, language, prefixLanguage, description, !spaCustomHomeContentConfigured())
	if loc := reSPABodyOpenTag.FindStringIndex(page); loc != nil {
		page = page[:loc[1]] + block + page[loc[1]:]
	}

	return []byte(page)
}

// spaHasServerTranslation 报告服务端是否有该语言的页面文案。
// 只有 zh-CN 与 en 两份；其余语言前缀仍是英文内容，必须 noindex。
func spaHasServerTranslation(language string) bool {
	switch language {
	case "zh", "en":
		return true
	default:
		return false
	}
}

// spaShouldAdvertiseHreflang 报告该路径是否应输出 hreflang 链接。
// 只覆盖公开内容页与两份政策页；认证入口页是 noindex，不参与。
func spaShouldAdvertiseHreflang(path string) bool {
	if spaNoIndexPath(path) {
		return false
	}
	if spaIsPublicPath(path) {
		return true
	}
	return path == "/user-agreement" || path == "/privacy-policy"
}

// spaHreflangLinks 生成 `<link rel="alternate" hreflang=...>` 组（含 x-default）。
func spaHreflangLinks(base, path string) string {
	var b strings.Builder
	for _, lang := range spaHreflangLanguages {
		b.WriteString(`<link rel="alternate" hreflang="`)
		b.WriteString(html.EscapeString(spaHreflangTag(lang)))
		b.WriteString(`" href="`)
		b.WriteString(html.EscapeString(absoluteSEOURL(base, spaLocalizedPath(lang, path))))
		b.WriteString(`" />`)
		b.WriteString("\n    ")
	}
	b.WriteString(`<link rel="alternate" hreflang="x-default" href="`)
	b.WriteString(html.EscapeString(absoluteSEOURL(base, spaLocalizedPath(spaDefaultHreflangLanguage, path))))
	b.WriteString(`" />`)
	return b.String()
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
	zh := spaIsChinese(language)
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

// spaPricingModeLabel 描述计费方式。
//
// **判据顺序必须与前端 `getBillingModeLabelKey` 一致：先判 billing_mode/表达式，
// 再判 quota_type。**（`web/src/features/pricing/lib/billing-mode.ts`）
// 前端对「billing_mode==='tiered_expr' 且有 billing_expr」的模型显示
// 「动态计费」，价格列也走动态阶梯分支（`model-price-cell.tsx` 的
// `getDynamicPricingSummary`）。这类模型即便 `quota_type=1`，**实际扣费也来自
// 计费表达式**（见 `service/tiered_settle.go` 的 BillingMode 判定），
// 因此展示 `model_price` 会误导读者以为每次固定收那个价。
func spaPricingModeLabel(p model.Pricing, zh bool) string {
	// 任务型计费是前端 `getBillingModeLabelKey` 的**最高优先级**分支
	// （`hasTaskUsageSchema` = billing_usage_schema 非空），必须先判，
	// 否则这类模型会落到下面的分支而与前端的 badge 不一致。
	if len(p.BillingUsageSchema) > 0 {
		if zh {
			return "任务计费"
		}
		return "Task billing"
	}
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

// spaPricingValueLabel 描述价格。上面注释解释了为什么这里**不给阶梯/表达式模型编数字**：
// 表达式可能按上下文长度/缓存分档计费，Go 侧没有等价实现，报一个数就是误导。
func spaPricingValueLabel(p model.Pricing, zh bool) string {
	// 任务型计费按用量单位（$/秒、$/张…）结算，Go 侧同样没有等价实现。
	if len(p.BillingUsageSchema) > 0 {
		if zh {
			return "见本页交互界面（按用量单位计费）"
		}
		return "see the interactive view (billed per usage unit)"
	}
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
	switch strings.ToLower(language) {
	case "en":
		return "en_US"
	case "fr":
		return "fr_FR"
	case "ru":
		return "ru_RU"
	case "ja":
		return "ja_JP"
	case "vi":
		return "vi_VN"
	case "zh-tw", "zhtw":
		return "zh_TW"
	default:
		return "zh_CN"
	}
}

// spaBuildContentBlock 构造给爬虫看的内容块。
//
// 放在 `#root` **之前**、`<body>` 之后：React 挂载进 `#root` 不会碰到它，
// 再由 `spaSEORemoverScript` 在挂载完成后移除。
//
// **关于用户可见性的事实（必须如实描述，避免"宣称与事实不一致"）**：
// 容器**没有任何 CSS 隐藏**。因此在「文档送达 → React 首次渲染出子节点」
// 这段窗口内，这段内容**对用户是可见的**（`/pricing` 上还包含最多 120 行价目表）。
// 它是**服务端渲染的降级内容**，描述与 React 渲染的是同一件事 ——
// 这正是它不构成 cloaking 的前提。
//
// 刻意**不**用 CSS 隐藏它：Googlebot 会应用 CSS，用 `display:none` /
// `visibility:hidden` 把内容藏起来只让爬虫读到，恰恰构成 cloaking。
// 所以这里选择"短暂可见"而非"隐藏"。
//
// 结构：`<h1>` → 摘要 → 限制与边界小节 → `extraHTML`（可见的附加内容，如价目表）→
// 站内导航 → 上游官方文档外链。
//
// `extraHTML` 是**可见**的附加内容（目前只有 /pricing 的价目表），必须写在
// `</div>` 之前：移除脚本删的是整个 `#spa-seo-content`，落在容器外的可见内容
// 不会被删掉，会一直留在页面上（生产事故，见 spaSEORemoverScript 注释）。
func spaBuildContentBlock(h1, summary, caveat, requestPath, siteName, language, prefixLanguage, extraHTML string) string {
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
		b.WriteString("<h2>")
		b.WriteString(spaLimitationHeading(language))
		b.WriteString("</h2><p>")
		b.WriteString(esc(caveat))
		b.WriteString("</p>")
	}
	b.WriteString(spaInternalLinksHTML(requestPath, siteName, language, prefixLanguage))
	b.WriteString(spaOutboundLinksHTML(language))
	b.WriteString(extraHTML)
	b.WriteString("</div>")
	b.WriteString(spaSEORemoverScript)
	return b.String()
}

// spaLimitationHeading 是「限制与边界」小节的标题。
//
// 把现有的 `Meta.Caveat`（本来就是准确的边界说明）提升为一个显式小节标题，
// 使正文里出现 limitation/caveat 类语义锚点，便于读者与机器识别页面边界。
// **内容仍然只是 Caveat 原文，一个字都不新增。**
func spaLimitationHeading(language string) string {
	if spaIsChinese(language) {
		return "限制与边界"
	}
	return "Limitations"
}

// spaExpandSite 把 `{site}` 占位替换为运行时站点名。
func spaExpandSite(s, siteName string) string {
	if s == "" {
		return ""
	}
	return strings.ReplaceAll(s, "{site}", siteName)
}

// ---- 结构化数据（Batch-13 / GEO）----
//
// 为什么在**服务端**注入：首页 FAQ 的 `FAQPage` 目前由前端在运行时用 JS 追加
// （`web/src/features/home/components/sections/faq.tsx` 的 `landing-faq-jsonld`），
// 不执行 JS 的爬虫看不到；其余路由则完全没有结构化数据。
//
// 纪律：**只声明页面上真实存在、且我们能证明的事实**。因此：
//   - 首页 → `WebSite`（站点身份）；`/about` → `AboutPage`；其余 → `WebPage`。
//   - 首页 FAQ → 与页面上**用户可见**的 FAQ accordion 逐条同文案
//     （见 spaFAQItems*，来源是 faq.tsx + i18n 语言包，另有测试锁住不漂移）。
//   - **不写** `aggregateRating` / `review` / `author` —— 我们既没有评分或评价数据，
//     也不是模型作者；声明它们等于向搜索引擎提供不实信息。
//
// 位置：`<script>` 本身不渲染给用户，因此放在 `#spa-seo-content` **容器外**，
// 不会被 `spaSEORemoverScript` 在 React 挂载后一并删除。

// spaJSONLDContext 是所有结构化数据共用的 @context。
const spaJSONLDContext = "https://schema.org"

// spaNoIndexPath 报告该路径是否应禁止搜索引擎索引。
//
// 只覆盖「没有公开内容、被收录只会造成重复/低质页面」的入口页：
// 登录、注册、找回/重置密码、OTP、OAuth 回调与首次安装向导。
// 公开内容页（首页/定价/榜单等）**绝不**加 noindex。
func spaNoIndexPath(path string) bool {
	clean := strings.TrimRight(path, "/")
	if clean == "" {
		return false
	}
	switch clean {
	case "/sign-in", "/sign-up", "/login", "/register",
		"/forgot-password", "/reset", "/otp", "/oauth",
		"/user/reset", "/setup":
		return true
	}
	return strings.HasPrefix(clean, "/oauth/")
}

// spaRouteSchemaType 返回路由对应的 schema.org 页面类型。
//
// 首页是站点入口，用 `WebSite` 正确。**刻意不加 `SearchAction`** —— 本站没有
// 站点级搜索端点，声明它等于挂一个不存在的功能。
func spaRouteSchemaType(path string) string {
	switch strings.TrimRight(path, "/") {
	case "":
		return "WebSite"
	case "/about":
		return "AboutPage"
	default:
		return "WebPage"
	}
}

// spaJSONLDScript 生成页面底部的 JSON-LD（首页会多出一条 FAQPage）。
//
// `includeFAQ` 为真时才写入 FAQPage —— 调用方必须先确认首页**确实**渲染了那个
// FAQ accordion（见 spaCustomHomeContentConfigured）。
//
// 序列化失败（理论上不会）时跳过该条：宁可少一段结构化数据，也不把坏 JSON 送上页面。
func spaJSONLDScript(meta spaRouteMeta, requestPath, siteName, language, prefixLanguage, description string, includeFAQ bool) string {
	base := spaSiteBaseForSEO()
	schemaType := spaRouteSchemaType(requestPath)

	var graph map[string]any
	if schemaType == "WebSite" {
		// 首页声明的是**站点身份**（站点名 + 站点地址），不是某个页面的标题。
		graph = map[string]any{
			"@context":   spaJSONLDContext,
			"@type":      schemaType,
			"name":       siteName,
			"url":        absoluteSEOURL(base, spaLocalizedPath(prefixLanguage, "/")),
			"inLanguage": spaHTMLLang(language),
		}
	} else {
		graph = map[string]any{
			"@context":    spaJSONLDContext,
			"@type":       schemaType,
			"name":        spaExpandSite(meta.Title, siteName),
			"description": description,
			"url":         absoluteSEOURL(base, spaLocalizedPath(prefixLanguage, requestPath)),
			"inLanguage":  spaHTMLLang(language),
		}
	}

	var b strings.Builder
	writeScript := func(v any) {
		raw, err := common.Marshal(v)
		if err != nil {
			return
		}
		b.WriteString(`<script type="application/ld+json">`)
		b.Write(raw)
		b.WriteString("</script>")
	}
	writeScript(graph)
	if schemaType == "WebSite" {
		// TODO SEO-ITEM-3.1：站点身份（Organization）与 WebSite 并列声明。
		// 只写我们**确实拥有**的事实：站点名与站点地址，不编造作者、地址或资质。
		writeScript(map[string]any{
			"@context": spaJSONLDContext,
			"@type":    "Organization",
			"name":     siteName,
			"url":      absoluteSEOURL(base, spaLocalizedPath(prefixLanguage, "/")),
		})
	}
	if schemaType != "WebSite" && spaIsPublicPath(requestPath) {
		// TODO SEO-ITEM-3.4：公开内容页给出一条真实可跟随的层级路径（首页 → 当前页）。
		// 只在确实存在于 sitemap 的公开页生成，未知路径不伪造面包屑。
		writeScript(map[string]any{
			"@context": spaJSONLDContext,
			"@type":    "BreadcrumbList",
			"itemListElement": []any{
				map[string]any{
					"@type":    "ListItem",
					"position": 1,
					"name":     siteName,
					"item":     absoluteSEOURL(base, spaLocalizedPath(prefixLanguage, "/")),
				},
				map[string]any{
					"@type":    "ListItem",
					"position": 2,
					"name":     spaExpandSite(meta.Title, siteName),
					"item":     absoluteSEOURL(base, spaLocalizedPath(prefixLanguage, requestPath)),
				},
			},
		})
	}
	if schemaType == "WebSite" && includeFAQ {
		writeScript(spaFAQJSONLD(language))
	}
	return b.String()
}

// spaIsPublicPath 报告路径是否属于 sitemap 收录的公开内容页。
// 面包屑等结构化数据只在真实公开页上生成，避免为任意 SPA 路径编造层级。
func spaIsPublicPath(path string) bool {
	clean := strings.TrimRight(path, "/")
	if clean == "" {
		return true // 首页
	}
	for _, p := range seoPublicPaths {
		if strings.TrimRight(p.Path, "/") == clean {
			return true
		}
	}
	return false
}

// spaCustomHomeContentConfigured 报告本部署是否配置了自定义首页内容。
//
// 配置后首页渲染的是那段内容（`web/src/features/home/index.tsx` 在 `content` 非空时
// 提前返回，不再渲染 `<FAQ />`），页面上**根本没有** FAQ accordion —— 此时注入
// `FAQPage` 就是在描述页面上不存在的内容。因此 FAQ 结构化数据只在本部署使用
// 默认落地页时注入。判断与前端同源：同一个 `HomePageContent` 选项。
func spaCustomHomeContentConfigured() bool {
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	return strings.TrimSpace(common.OptionMap["HomePageContent"]) != ""
}

// spaFAQItemsEN / spaFAQItemsZH 是首页 FAQ accordion 的问答对，**逐字复制**自页面文案。
//
// 来源（必须保持一致，否则再修改前端文案时此处的服务端 JSON-LD 会与用户所见漂移）：
//   - 英文：`web/src/features/home/components/sections/faq.tsx` 的 `FAQ_ITEMS` 原文；
//   - 中文：`web/src/i18n/locales/zh.json` 中同一批键对应的值。
//
// 之所以在 Go 侧复录一份而不是运行时读取前端语言包：服务端注入必须**跟随访问者语言**，
// 而语言包是构建产物。测试 TestSPASEOHomeFAQStructuredData 锁住条数与首条问句。
var spaFAQItemsEN = [][2]string{
	{
		"What is an AI API gateway?",
		"An AI API gateway is a single endpoint that routes your requests to many upstream model providers. Instead of integrating OpenAI, Claude, Gemini, and others separately, you call one OpenAI-compatible API and the gateway handles authentication, protocol translation, load balancing, and billing.",
	},
	{
		"Which AI models and providers are supported?",
		"It aggregates 40+ upstream providers, including OpenAI, Anthropic Claude, Google Gemini, Azure OpenAI, AWS Bedrock, DeepSeek, and Qwen, behind one unified protocol. New channels can be added as providers ship.",
	},
	{
		"Is it OpenAI API compatible?",
		"Yes. Requests and responses follow the OpenAI-compatible format, so existing SDKs and tools work by changing only the base URL and API key. Native Claude and Gemini endpoints are also exposed for clients that need them.",
	},
	{
		"How is usage billed?",
		"Billing is pay-as-you-go and transparent: every request records prompt and completion tokens, latency, and cost. You can review usage in real time, set quotas and rate limits per user or token, and see the cost of each request before you commit.",
	},
	{
		"Can I self-host the gateway?",
		"Yes. The gateway is self-hostable and open source, with SQLite, MySQL, or PostgreSQL for the primary database and optional Redis and ClickHouse for caching and logs. You keep control of your keys, data, and deployment.",
	},
	{
		"How does it handle reliability and failover?",
		"Requests are routed to the healthiest upstream based on live channel status. Unhealthy channels are taken out of rotation automatically, and load balancing spreads traffic across the providers you configure so a single outage does not take you down.",
	},
}

// spaFAQItemsZH 是同一批问答的中文文案（取自 zh.json，与页面实际渲染一致）。
var spaFAQItemsZH = [][2]string{
	{
		"什么是 AI API 网关？",
		"AI API 网关是一个统一入口，把你的请求路由到众多上游模型服务商。你无需分别对接 OpenAI、Claude、Gemini 等，只需调用一个兼容 OpenAI 的 API，网关自动处理鉴权、协议转换、负载均衡与计费。",
	},
	{
		"支持哪些 AI 模型和服务商？",
		"它把 40+ 上游服务商聚合到一套统一协议之下，包括 OpenAI、Anthropic Claude、Google Gemini、Azure OpenAI、AWS Bedrock、DeepSeek 和 Qwen。随服务商上线可持续新增渠道。",
	},
	{
		"兼容 OpenAI API 吗？",
		"兼容。请求与响应遵循 OpenAI 兼容格式，现有 SDK 和工具只需修改 base URL 与 API Key 即可使用。同时提供原生 Claude 与 Gemini 端点，供有需要的客户端调用。",
	},
	{
		"如何计费？",
		"按量计费、透明可查：每次请求都会记录 prompt 与 completion tokens、延迟和费用。你可以实时查看用量，按用户或令牌设置额度与速率限制，并在提交前了解每次请求的费用。",
	},
	{
		"可以自行部署吗？",
		"可以。网关支持自部署且开源，主数据库可用 SQLite、MySQL 或 PostgreSQL，缓存与日志可选 Redis 和 ClickHouse。密钥、数据与部署都由你掌控。",
	},
	{
		"如何保障可靠性与故障转移？",
		"请求会根据渠道实时状态路由到最健康的上游。异常渠道会被自动移出轮询，负载均衡在你配置的服务商之间分摊流量，单点故障不会导致服务中断。",
	},
}

// spaFAQJSONLD 生成首页的 FAQPage 结构化数据（与页面上的 FAQ accordion 同文案）。
func spaFAQJSONLD(language string) map[string]any {
	items := spaFAQItemsEN
	if spaIsChinese(language) {
		items = spaFAQItemsZH
	}
	entities := make([]map[string]any, 0, len(items))
	for _, qa := range items {
		entities = append(entities, map[string]any{
			"@type": "Question",
			"name":  qa[0],
			"acceptedAnswer": map[string]any{
				"@type": "Answer",
				"text":  qa[1],
			},
		})
	}
	return map[string]any{
		"@context":   spaJSONLDContext,
		"@type":      "FAQPage",
		"mainEntity": entities,
	}
}
