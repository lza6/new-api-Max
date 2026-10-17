package router

import (
	"embed"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gin-contrib/gzip"
	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"
	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/controller"
	"github.com/lza6/new-api-Max/middleware"
	"github.com/lza6/new-api-Max/service"
)

// WebAssets holds the embedded dashboard frontend assets.
type WebAssets struct {
	BuildFS   embed.FS
	IndexPage []byte
}

func SetWebRouter(router *gin.Engine, assets WebAssets, pluginDispatcher gin.HandlerFunc) {
	frontendFS := common.EmbedFolder(assets.BuildFS, "web/dist")

	// SEO：站点感知的 robots.txt / sitemap.xml。必须显式注册（前端静态资源
	// 经下方 NoRoute 提供，无显式文件路由），否则会命中 embed 里写死主站域名的
	// 旧文件，导致副站 sitemap 指向主站。
	router.GET("/robots.txt", controller.GetRobotsTxt)
	router.GET("/sitemap.xml", controller.GetSitemapXml)

	router.NoRoute(
		pluginDispatcher,
		middleware.RouteTag("web"),
		gzip.Gzip(gzip.DefaultCompression),
		middleware.AccessTokenAudit(),
		middleware.GlobalWebRateLimit(),
		// 站点访客统计（Batch-13）：只对**公开页面文档请求**计数，
		// 内存去重（一 IP 一天一次）后批量落库，绝不阻塞请求。见 service/site_visitor_tracker.go。
		// 位置刻意放在 static.Serve **之前**：此时路径尚未被静态资源命中，
		// 且 TrackSiteVisit 自身会过滤掉 API/静态资源，因此不会把资源请求计入访客。
		func(c *gin.Context) {
			service.TrackSiteVisit(c)
			c.Next()
		},
		middleware.Cache(),
		func(c *gin.Context) {
			// 文档路由（无文件扩展名）不缓存：每次部署后浏览器重新获取最新
			// index.html（其引用的静态资源带内容哈希，Caddy 已配 immutable）。
			// 带扩展名的资源（.js/.css/.png 等）保持可缓存，不受影响。
			if filepath.Ext(c.Request.URL.Path) == "" {
				c.Header("Cache-Control", "no-cache")
			}
			c.Next()
		},
		static.Serve("/", frontendFS),
		func(c *gin.Context) {
			if strings.HasPrefix(c.Request.RequestURI, "/v1") || strings.HasPrefix(c.Request.RequestURI, "/api") || strings.HasPrefix(c.Request.RequestURI, "/assets") {
				controller.RelayNotFound(c)
				return
			}
			// 默认语言（zh）不使用 URL 前缀：/zh/... 301 到无前缀地址，
			// 避免同一内容出现两份 canonical。其它语言前缀原样服务。
			if target, ok := controller.DefaultLocaleRedirectTarget(c.Request.URL.Path); ok {
				if c.Request.URL.RawQuery != "" {
					target += "?" + c.Request.URL.RawQuery
				}
				c.Redirect(http.StatusMovedPermanently, target)
				return
			}
			c.Header("Cache-Control", "no-cache")
			// SEO：把路由级 meta（title/description/canonical/OG）与一份**准确**的
			// 内容块注入 SPA 壳，语言跟随访问者的 Accept-Language（默认中文）。
			// 不执行 JS 的爬虫因此能看到每个页面各自的内容与可跟随的站内链接，
			// 而不是 6 份一模一样的空壳、也不是所有子页都声明 canonical 指向首页。
			// 详见 controller/spa_seo.go。
			c.Data(http.StatusOK, "text/html; charset=utf-8",
				controller.BuildSPAIndexHTML(c, assets.IndexPage))
		},
	)
}
