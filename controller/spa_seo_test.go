package controller

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lza6/new-api-Max/common"
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

func buildFixture(t *testing.T, path string) string {
	t.Helper()
	return string(BuildSPAIndexHTML([]byte(spaFixtureHTML), path))
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
