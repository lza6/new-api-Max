package controller

import (
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/model"
	"github.com/lza6/new-api-Max/setting/system_setting"
)

// Batch-12 / SEO-1：SPA 服务端注入的契约测试。
//
// 这些断言锁的是一个**实测过的缺陷**：修复前 6 个公开页返回完全相同的
// 3982 字节空壳 —— 零个 `<h1>`，且 `canonical` 一律硬编码为 `/`
// （等于让 /pricing、/about 主动声明"我的正版是首页"）。

// 复用壳的最小样本：只要包含被替换的那些标签即可，不必是完整 dist 产物。
const spaFixtureHTML = `<!doctype html>
<html lang="en">
<head>
<title>Old</title>
<meta name="title" content="Old" />
<meta
  name="description"
  content="Old desc"
/>
<link rel="canonical" href="/" />
<meta property="og:title" content="Old" />
<meta
  property="og:description"
  content="Old og desc"
/>
<meta property="og:url" content="/" />
<meta name="twitter:title" content="Old" />
<meta
  name="twitter:description"
  content="Old tw desc"
/>
</head>
<body>
<div id="root"></div>
</body>
</html>`

// buildWithLang 用一个真实的 gin 上下文构造页面（注入逻辑从 c 取路径与 Accept-Language）。
func buildWithLang(t *testing.T, path, acceptLanguage string) string {
	t.Helper()
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest("GET", path, nil)
	if acceptLanguage != "" {
		c.Request.Header.Set("Accept-Language", acceptLanguage)
	}
	return string(BuildSPAIndexHTML(c, []byte(spaFixtureHTML)))
}

// buildFixture 以**英文**语言偏好构造页面（多数结构性断言与语言无关）。
func buildFixture(t *testing.T, path string) string {
	t.Helper()
	return buildWithLang(t, path, "en-US,en;q=0.9")
}

// buildFixtureLang 指定 Accept-Language 构造页面。
func buildFixtureLang(t *testing.T, path, acceptLanguage string) string {
	t.Helper()
	return buildWithLang(t, path, acceptLanguage)
}

func TestSPASEOInjectsPerRouteTitleAndCanonical(t *testing.T) {
	previousBase := system_setting.ServerAddress
	system_setting.ServerAddress = "https://example.test"
	t.Cleanup(func() { system_setting.ServerAddress = previousBase })

	seenCanonical := map[string]string{}
	seenTitle := map[string]string{}
	for _, p := range seoPublicPaths {
		page := buildFixture(t, p.Path)

		canon := regexp.MustCompile(`<link rel="canonical" href="([^"]*)"`).FindStringSubmatch(page)
		require.Len(t, canon, 2, "%s 必须注入 canonical", p.Path)
		// canonical 必须是**绝对**地址且指向本页 —— 这是本次修复的核心。
		want := "https://example.test" + p.Path
		if p.Path == "/" {
			want = "https://example.test/"
		}
		assert.Equal(t, want, canon[1], "%s 的 canonical 必须指向自己", p.Path)

		title := regexp.MustCompile(`<title>([^<]*)</title>`).FindStringSubmatch(page)
		require.Len(t, title, 2)
		assert.NotEqual(t, "Old", title[1], "%s 必须换成路由级标题", p.Path)

		seenCanonical[p.Path] = canon[1]
		seenTitle[p.Path] = title[1]
	}

	// 不同页面的 canonical / title 必须**互不相同** —— 否则又回到"全是同一份"。
	assert.Len(t, distinctValues(seenCanonical), len(seoPublicPaths), "各页 canonical 必须唯一")
	assert.Len(t, distinctValues(seenTitle), len(seoPublicPaths), "各页 title 必须唯一")
}

func TestSPASEOUnknownPathKeepsItsOwnCanonical(t *testing.T) {
	previousBase := system_setting.ServerAddress
	system_setting.ServerAddress = "https://example.test"
	t.Cleanup(func() { system_setting.ServerAddress = previousBase })

	page := buildFixture(t, "/some/unknown/route")
	canon := regexp.MustCompile(`<link rel="canonical" href="([^"]*)"`).FindStringSubmatch(page)
	require.Len(t, canon, 2)
	assert.Equal(t, "https://example.test/some/unknown/route", canon[1],
		"未在路由表里的路径也必须 canonical 到自己，绝不能落回首页（那正是原缺陷）")
	assert.NotEqual(t, "https://example.test/", canon[1])
}

func TestSPASEOInjectsCrawlableContentBlock(t *testing.T) {
	page := buildFixture(t, "/pricing")

	assert.Contains(t, page, `id="spa-seo-content"`, "必须注入内容块")

	h1 := regexp.MustCompile(`<h1>([^<]*)</h1>`).FindStringSubmatch(page)
	require.Len(t, h1, 2, "必须有一个真实 <h1>（爬虫据此判断页面主题）")
	assert.Equal(t, "Model pricing", h1[1])

	// 站内链接：必须是真的 <a href>，且**不包含自链接**，条数 = 公开路由数 - 1。
	internal := regexp.MustCompile(`<nav aria-label="Public pages">.*?</nav>`).FindString(page)
	require.NotEmpty(t, internal, "必须有站内链接块（否则爬虫仍无法发现这些页面）")
	hrefs := regexp.MustCompile(`href="([^"]*)"`).FindAllStringSubmatch(internal, -1)
	assert.Len(t, hrefs, len(seoPublicPaths)-1, "站内链接应为「公开路由数 - 1」（排除自链接）")
	for _, h := range hrefs {
		assert.NotContains(t, h[1], "/pricing", "不应给当前页加自链接")
	}

	// 外链：指向上游提供方的官方文档（真实存在的地址，用于归属指引）。
	assert.Contains(t, page, "platform.openai.com", "应有指向官方文档的外链")
	assert.Contains(t, page, `rel="noopener"`, "外链必须带 rel=noopener")

	// 内容块必须在 #root **之前**：React 挂载进 #root 才不会碰到它。
	assert.Less(t, strings.Index(page, `id="spa-seo-content"`), strings.Index(page, `id="root"`),
		"内容块必须位于 #root 之前")

	// 移除脚本必须在场：否则用户会一直看到这段文字。
	assert.Contains(t, page, "spa-seo-content", "必须带移除脚本")
	assert.Contains(t, page, "MutationObserver", "移除时机必须等 React 真正渲染（否则会闪空白）")
	assert.Contains(t, page, "spa-seo-pricing", "移除脚本必须覆盖价目表 id")
}

func TestSPASEOExpandsSitePlaceholder(t *testing.T) {
	previousName := common.SystemName
	common.SystemName = "Example Gateway"
	t.Cleanup(func() { common.SystemName = previousName })

	page := buildFixture(t, "/")
	assert.NotContains(t, page, "{site}", "占位符必须被替换，不能漏到页面上")
	assert.Contains(t, page, "Example Gateway")
}

func TestSPASEOHandlesEmptySystemName(t *testing.T) {
	previousName := common.SystemName
	common.SystemName = ""
	t.Cleanup(func() { common.SystemName = previousName })

	page := buildFixture(t, "/about")
	assert.NotContains(t, page, "{site}")
	assert.Contains(t, page, "About this gateway", "站点名为空时也要产出可读内容")
}

func TestSPASEODoesNotTouchNonSEOTags(t *testing.T) {
	page := buildFixture(t, "/")
	// 结构性标签必须原样保留，否则 React 挂载点会丢。
	assert.Contains(t, page, `<div id="root"></div>`)
	assert.Contains(t, page, "<!doctype html>")
	assert.Contains(t, page, "</html>")
}

func distinctValues(m map[string]string) map[string]struct{} {
	out := make(map[string]struct{}, len(m))
	for _, v := range m {
		out[v] = struct{}{}
	}
	return out
}

// Batch-12 / SEO-2：注入内容的**语言**必须跟随访问者，而不是写死英文。
//
// 本站面向中文用户，而前端的 meta 都是运行时由 JS 写的 —— 服务端注入若不跟随语言，
// 等于把中文站推荐给爬虫当英文站。
func TestSPASEOLanguageFollowsAcceptLanguage(t *testing.T) {
	cases := []struct {
		header     string
		wantLang   string
		wantInPage string
		note       string
	}{
		{"zh-CN,zh;q=0.9,en;q=0.8", "zh-CN", "模型价格", "中文偏好"},
		{"zh", "zh-CN", "模型价格", "只有 zh"},
		{"en-US,en;q=0.9", "en", "Model pricing", "英文偏好"},
		{"fr-FR,fr;q=0.9", "zh-CN", "模型价格", "非中非英 → 默认中文"},
		{"", "zh-CN", "模型价格", "无头 → 默认中文"},
		{"*", "zh-CN", "模型价格", "通配 → 默认中文"},
		{"ja-JP,ja;q=0.9,zh-CN;q=0.8", "zh-CN", "模型价格", "首选日文但含中文 → 命中中文"},
		{"ja-JP", "zh-CN", "模型价格", "只有日文 → 默认中文"},
	}
	for _, tc := range cases {
		page := buildFixtureLang(t, "/pricing", tc.header)
		lang := regexp.MustCompile(`<html lang="([^"]*)"`).FindStringSubmatch(page)
		require.Len(t, lang, 2, "Accept-Language=%q", tc.header)
		assert.Equal(t, tc.wantLang, lang[1], "%s：<html lang> 必须与内容语言一致", tc.note)
		assert.Contains(t, page, tc.wantInPage, "%s：内容语言错误", tc.note)
	}
}

func TestSPASEOSetsOGLocale(t *testing.T) {
	zhPage := buildFixtureLang(t, "/", "zh-CN")
	assert.Contains(t, zhPage, `<meta property="og:locale" content="zh_CN" />`)

	enPage := buildFixtureLang(t, "/", "en-US")
	assert.Contains(t, enPage, `<meta property="og:locale" content="en_US" />`)
}

// og:url 必须是**结构完整**的 meta 标签，且指向本页的绝对地址。
//
// 这条是给一个真实事故补的回归锁：og:url 的替换串曾被写成 `X content="…"`（漏了
// 开头的 `<meta property="og:url"`），于是生产页面上的 og:url 整条标签被替换成
// 一段非法文本 `X content="https://…" />`。当时的断言只检查 og:locale 是否存在，
// 所以 CI 全绿而线上是坏的 —— 断言"某标签存在"不等于断言"标签仍然合法"。
func TestSPASEOSetsWellFormedOGURL(t *testing.T) {
	previousBase := system_setting.ServerAddress
	system_setting.ServerAddress = "https://example.test"
	t.Cleanup(func() { system_setting.ServerAddress = previousBase })

	for _, path := range []string{"/", "/pricing", "/about"} {
		page := buildFixture(t, path)

		want := "https://example.test" + path
		if path == "/" {
			want = "https://example.test/"
		}
		assert.Contains(t, page,
			`<meta property="og:url" content="`+want+`" />`,
			"%s 的 og:url 必须是完整的 meta 标签并指向本页", path)

		// 反向锁：替换串写坏时会留下 `X content=`。这条断言让它无从遁形。
		assert.NotContains(t, page, `X content=`, "og:url 替换串写坏了（出现 X content=）")

		// og:url 与 canonical 必须一致，否则分享卡片与收录指向会分叉。
		canon := regexp.MustCompile(`<link rel="canonical" href="([^"]*)"`).FindStringSubmatch(page)
		require.Len(t, canon, 2)
		assert.Equal(t, canon[1], want)
	}
}

// 中文页的站内导航与外链标签也必须是中文 —— 只翻译标题会显得半成品。
func TestSPASEOLocalizesNavAndOutboundLabels(t *testing.T) {
	page := buildFixtureLang(t, "/pricing", "zh-CN")
	assert.Contains(t, page, ">首页</a>", "站内导航应为中文")
	assert.Contains(t, page, "用量排行")
	assert.Contains(t, page, "上游协议参考", "外链引导语应为中文")
	assert.Contains(t, page, "OpenAI API 参考", "外链标签应为中文")
	// 外链地址不翻译
	assert.Contains(t, page, "https://platform.openai.com/docs/api-reference")

	enPage := buildFixtureLang(t, "/pricing", "en-US")
	assert.Contains(t, enPage, ">Overview</a>")
	assert.Contains(t, enPage, "Upstream protocol references")
}

// 中文表必须覆盖全部公开路由，否则会静默回退英文（页面语言混用）。
func TestSPASEOLanguageTablesCoverAllPublicRoutes(t *testing.T) {
	for _, p := range seoPublicPaths {
		meta, ok := spaRouteMetaZh[p.Path]
		require.Truef(t, ok, "公开路由 %s 缺少中文文案 —— 会在中文站点上回退成英文", p.Path)
		assert.NotEmpty(t, meta.Title, "%s 中文 title 不能为空", p.Path)
		assert.NotEmpty(t, meta.Description, "%s 中文 description 不能为空", p.Path)
		assert.NotEmpty(t, meta.H1, "%s 中文 H1 不能为空", p.Path)
		assert.NotEmpty(t, meta.Summary, "%s 中文摘要不能为空", p.Path)
		assert.NotEmpty(t, meta.Caveat, "%s 中文边界说明不能为空", p.Path)
	}
	// 兜底也要有中文版
	assert.NotEmpty(t, spaDefaultMetaZh.Title)
	assert.NotEmpty(t, spaDefaultMetaZh.Summary)
}

// Batch-12 / SEO-3：价目表的两个纯函数。
//
// 关键纪律：**只渲染能准确渲染的字段**。页面上「一次调用 ¥X」是前端按计费表达式/
// 阶梯算出来的，Go 侧没有等价实现 —— 公开页面上的价格算错比不显示价格更糟，
// 因此表达式/阶梯计费的模型只标注计费方式，**不猜数字**。
func TestSPAPricingLabels(t *testing.T) {
	cases := []struct {
		name      string
		pricing   model.Pricing
		wantModeZ string
		wantValZ  string
		wantModeE string
		wantValE  string
	}{
		{
			name:      "按次计费：给出固定价",
			pricing:   model.Pricing{QuotaType: 1, ModelPrice: 0.04},
			wantModeZ: "按次计费", wantValZ: "$0.04 / 次",
			wantModeE: "Per request", wantValE: "$0.04 / request",
		},
		{
			name:      "按 token：给倍率并说明口径，不谎称这是最终价",
			pricing:   model.Pricing{QuotaType: 0, ModelRatio: 2.5},
			wantModeZ: "按 token 计费", wantValZ: "倍率 2.5（实际费用 = 倍率 × 用量）",
			wantModeE: "Per token", wantValE: "ratio 2.5 (actual cost = ratio x usage)",
		},
		{
			name:      "计费表达式：只标方式，不猜数字",
			pricing:   model.Pricing{QuotaType: 0, ModelRatio: 1, BillingExpr: `tier("base", p*2)`},
			wantModeZ: "按用量表达式计费", wantValZ: "见本页交互界面（按用量计算）",
			wantModeE: "Usage-based (billing expression)", wantValE: "see the interactive view (computed from usage)",
		},
		{
			// 判据顺序必须与前端 `getBillingModeLabelKey` 一致：**先判表达式/billing_mode**。
			// 生产数据形态：deepseek-v4.1-flash 与 glm-5.3-flash 都是
			// quota_type=1 + billing_mode=tiered_expr + billing_expr。
			// 前端对这类模型显示「动态计费」，价格列走动态阶梯分支
			// （`model-price-cell.tsx` 的 getDynamicPricingSummary），实际扣费来自
			// 计费表达式（`service/tiered_settle.go`）。因此这里必须标成表达式计费、
			// 且**不得**展示 model_price —— 展示它会让读者以为每次固定收那个价。
			name:      "按次 + 有表达式：必须优先判为表达式计费（与前端一致）",
			pricing:   model.Pricing{QuotaType: 1, ModelPrice: 0.006, BillingMode: "tiered_expr", BillingExpr: `tier("a", p*1)`},
			wantModeZ: "按用量表达式计费", wantValZ: "见本页交互界面（按用量计算）",
			wantModeE: "Usage-based (billing expression)", wantValE: "see the interactive view (computed from usage)",
		},
		{
			name:      "按 token + billing_mode=fixed：给倍率，不当作表达式计费",
			pricing:   model.Pricing{QuotaType: 0, ModelRatio: 3, BillingMode: "fixed"},
			wantModeZ: "按 token 计费", wantValZ: "倍率 3（实际费用 = 倍率 × 用量）",
			wantModeE: "Per token", wantValE: "ratio 3 (actual cost = ratio x usage)",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.wantModeZ, spaPricingModeLabel(tc.pricing, true))
			assert.Equal(t, tc.wantValZ, spaPricingValueLabel(tc.pricing, true))
			assert.Equal(t, tc.wantModeE, spaPricingModeLabel(tc.pricing, false))
			assert.Equal(t, tc.wantValE, spaPricingValueLabel(tc.pricing, false))
		})
	}
}

// 价目表只在 /pricing 出现，且模型为空时**不输出空表**（空表比没有更糟）。
func TestSPAPricingTableOnlyOnPricingRoute(t *testing.T) {
	for _, path := range []string{"/", "/about", "/rankings"} {
		page := buildFixtureLang(t, path, "zh-CN")
		assert.NotContains(t, page, `id="spa-seo-pricing"`, "%s 不应有价目表", path)
	}
}

// 可见的附加内容（价目表）必须在 `#spa-seo-content` **容器内部**。
//
// 锁一个真实生产缺陷：价目表原先被拼在整块之后（`</div>` 之外），而移除脚本
// 只处理根容器 —— 表格因此**永远不会被移除**，用户在生产 `/pricing` 页顶部
// 一直看到这张爬虫用的表；又因为 SPA 客户端路由切换不重载文档，它还会跟着
// 走到其它页面上（用户实测截图确认）。
func TestSPASEOVisibleExtrasStayInsideContainer(t *testing.T) {
	// 直接驱动内容块构造函数，验证它对 extraHTML 的容器边界契约。
	page := spaBuildContentBlock(
		"Pricing", "summary", "caveat", "/pricing", "Example", "en",
		`<table id="spa-seo-pricing"><tr><td>model</td></tr></table>`,
	)

	contentStart := strings.Index(page, `id="spa-seo-content"`)
	require.GreaterOrEqual(t, contentStart, 0, "必须输出内容块")
	contentEnd := strings.Index(page[contentStart:], "</div>")
	require.Greater(t, contentEnd, 0, "内容块必须有闭合的 </div>")
	contentEnd += contentStart

	tableStart := strings.Index(page, `id="spa-seo-pricing"`)
	require.GreaterOrEqual(t, tableStart, 0, "附加内容必须被输出")
	assert.Less(t, tableStart, contentEnd,
		"附加内容必须在 #spa-seo-content 容器内部，否则移除脚本删不掉它")

	// 移除脚本必须在容器之后（否则它自己会被删掉）。
	scriptStart := strings.Index(page, "<script>")
	require.GreaterOrEqual(t, scriptStart, 0)
	assert.Greater(t, scriptStart, contentEnd, "移除脚本必须在容器闭合之后")
}

func TestSPATrimFloat(t *testing.T) {
	assert.Equal(t, "2.5", trimFloat(2.5))
	assert.Equal(t, "0.04", trimFloat(0.04))
	assert.Equal(t, "3", trimFloat(3))
	assert.Equal(t, "-1", trimFloat(-1))
}
