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
			c.Header("Cache-Control", "no-cache")
			// SEO：把路由级 meta（title/description/canonical/OG）与一份**准确**的
			// 内容块注入 SPA 壳。不执行 JS 的爬虫因此能看到每个页面各自的内容与
			// 可跟随的站内链接，而不是 6 份一模一样的空壳、也不是所有子页都
			// 声明 canonical 指向首页。详见 controller/spa_seo.go。
			c.Data(http.StatusOK, "text/html; charset=utf-8",
				controller.BuildSPAIndexHTML(assets.IndexPage, c.Request.URL.Path))
		},
	)
}
