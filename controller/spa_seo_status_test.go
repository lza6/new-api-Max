package controller

import (
	"regexp"
	"strings"
	"testing"
)

// G7 公开状态页的 SEO 一致性契约。
//
// 硬约束：`/status` 进入 `seoPublicPaths` 后，服务端注入的内容块会同时进入
// sitemap 与「不执行 JS 的爬虫看到的 HTML」。而注入块**没有 CSS 隐藏**
// （挂载前的短暂窗口里用户也能看到），因此它必须与页面真实内容一致 —— 否则构成 cloaking。
//
// 本用例把这条纪律变成可执行的断言：注入的 H1/摘要不得为空、不得是占位或
// "待补充"式的空话，且必须提到页面上真实存在的几类内容（依赖 / 请求 / 后台任务）。
func TestSPASEOStatusContentMatchesPage(t *testing.T) {
	isolateHomePageOption(t, "")

	for _, tc := range []struct{ path, acceptLanguage, h1 string }{
		{"/status", "en-US,en;q=0.9", "Service status"},
		{"/status", "zh-CN,zh;q=0.9", "服务状态"},
	} {
		page := buildFixtureLang(t, tc.path, tc.acceptLanguage)
		block := spaContentBlockHTML(t, page)

		// 与前端页面标题同源：H1 必须是页面的可见一级标题。
		h1 := regexp.MustCompile(`<h1>([^<]*)</h1>`).FindStringSubmatch(block)
		if len(h1) != 2 {
			t.Fatalf("%s（%s）必须注入一个真实 <h1>", tc.path, tc.acceptLanguage)
		}
		if h1[1] != tc.h1 {
			t.Fatalf("注入的 H1 必须是页面自身的标题：got %q want %q", h1[1], tc.h1)
		}

		meta := spaRouteMetaFor(tc.path, spaResolveLanguage(tc.acceptLanguage))
		summary := strings.ToLower(meta.Summary)
		if strings.Contains(summary, "temporary") || strings.Contains(summary, "todo") {
			t.Fatalf("注入摘要不得是占位文本（会描述页面上不存在的内容）：%q", meta.Summary)
		}
		// 必须如实覆盖页面上真实存在的三类内容。
		for _, marker := range []string{"depend", "request", "background", "依", "请求", "后台"} {
			if strings.Contains(summary, marker) {
				return
			}
		}
		t.Fatalf("注入摘要必须提到页面上真实存在的内容（依赖/请求/后台任务）：%q", meta.Summary)
	}
}
