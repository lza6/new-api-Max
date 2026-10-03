package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/model"
	"github.com/lza6/new-api-Max/setting/system_setting"
)

// GetSitemapXml 必须使用当前实例的域名（ServerAddress），否则副站会生成指向
// 主站的 loc，导致 Search Console 判为跨站地图而整份失效。
func TestGetSitemapXmlUsesInstanceDomain(t *testing.T) {
	prev := system_setting.ServerAddress
	t.Cleanup(func() { system_setting.ServerAddress = prev })

	for _, base := range []string{"https://freeapi.example.com", "https://japi.example.com/"} {
		system_setting.ServerAddress = base

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		GetSitemapXml(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		body := w.Body.String()
		trimmed := strings.TrimRight(base, "/")
		if !strings.Contains(body, "<loc>"+trimmed+"/</loc>") {
			t.Fatalf("sitemap missing instance-domain loc for %q:\n%s", base, body)
		}
		if !strings.Contains(body, "<loc>"+trimmed+"/pricing</loc>") {
			t.Fatalf("sitemap missing /pricing for %q", base)
		}
		// 尾斜杠必须被规范掉：不能出现 "//pricing"。
		if strings.Contains(body, trimmed+"//pricing") {
			t.Fatalf("sitemap has doubled slash for %q", base)
		}
	}
}

// robots.txt 必须指向本站 sitemap，并屏蔽登录后页面。
func TestGetRobotsTxtUsesInstanceDomainAndDisallowsAuth(t *testing.T) {
	prev := system_setting.ServerAddress
	t.Cleanup(func() { system_setting.ServerAddress = prev })

	system_setting.ServerAddress = "https://japi.example.com"
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	GetRobotsTxt(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Sitemap: https://japi.example.com/sitemap.xml") {
		t.Fatalf("robots missing instance sitemap: %s", body)
	}
	for _, disallowed := range []string{"/dashboard", "/keys", "/wallet", "/system-settings", "/sign-in"} {
		if !strings.Contains(body, "Disallow: "+disallowed+"\n") {
			t.Fatalf("robots missing Disallow for %s:\n%s", disallowed, body)
		}
	}
}

// GetSiteOverview 聚合口径：仅统计消费日志（type=2），带宽 = 请求+响应字节，
// tokens = prompt+completion，quota 求和。测试关闭 Redis 走无缓存路径。
func TestGetSiteOverviewAggregatesConsumeLogs(t *testing.T) {
	prevRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = prevRedis })

	// 清理并写入可控样本：两条消费 + 一条非消费（不应计入）。
	if err := model.LOG_DB.Where("1 = 1").Delete(&model.Log{}).Error; err != nil {
		t.Fatalf("clean logs: %v", err)
	}
	seed := []model.Log{
		{Type: model.LogTypeConsume, RequestBytes: 100, ResponseBytes: 200, PromptTokens: 10, CompletionTokens: 20, Quota: 5},
		{Type: model.LogTypeConsume, RequestBytes: 400, ResponseBytes: 300, PromptTokens: 30, CompletionTokens: 40, Quota: 7},
		{Type: model.LogTypeError, RequestBytes: 999, ResponseBytes: 999, PromptTokens: 999, CompletionTokens: 999, Quota: 999},
	}
	for i := range seed {
		if err := model.LOG_DB.Create(&seed[i]).Error; err != nil {
			t.Fatalf("seed log: %v", err)
		}
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	GetSiteOverview(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := w.Body.String()
	for _, want := range []string{
		`"total_requests":2`,
		`"total_bytes":1000`,
		`"total_tokens":100`,
		`"total_quota":12`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("aggregate %s missing in: %s", want, body)
		}
	}
}
