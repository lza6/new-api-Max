package controller

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/http"
	"os"
	"strings"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/constant"
	"github.com/lza6/new-api-Max/i18n"
	"github.com/lza6/new-api-Max/logger"
	"github.com/lza6/new-api-Max/middleware"
	"github.com/lza6/new-api-Max/model"
	"github.com/lza6/new-api-Max/oauth"
	"github.com/lza6/new-api-Max/service"
	"github.com/lza6/new-api-Max/setting"
	"github.com/lza6/new-api-Max/setting/console_setting"
	"github.com/lza6/new-api-Max/setting/operation_setting"
	"github.com/lza6/new-api-Max/setting/system_setting"

	"github.com/gin-gonic/gin"
)

func TestStatus(c *gin.Context) {
	err := model.PingDB()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"message": "数据库连接失败",
		})
		return
	}
	// 获取HTTP统计信息
	httpStats := middleware.GetStats()
	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"message":    "Server is running",
		"http_stats": httpStats,
	})
	return
}

func GetStatus(c *gin.Context) {

	cs := console_setting.GetConsoleSetting()
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()

	passkeySetting := system_setting.GetPasskeySettings()
	legalSetting := system_setting.GetLegalSettings()

	data := gin.H{
		"version":                     common.Version,
		"start_time":                  common.StartTime,
		"email_verification":          common.EmailVerificationEnabled,
		"github_oauth":                common.GitHubOAuthEnabled,
		"github_client_id":            common.GitHubClientId,
		"discord_oauth":               system_setting.GetDiscordSettings().Enabled,
		"discord_client_id":           system_setting.GetDiscordSettings().ClientId,
		"linuxdo_oauth":               common.LinuxDOOAuthEnabled,
		"linuxdo_client_id":           common.LinuxDOClientId,
		"linuxdo_minimum_trust_level": common.LinuxDOMinimumTrustLevel,
		"telegram_oauth":              common.TelegramOAuthEnabled,
		"telegram_oauth_configured":   oauth.TelegramConfigurationError() == nil,
		"telegram_bot_name":           common.TelegramBotName,
		"theme":                       "default",
		"system_name":                 common.SystemName,
		"logo":                        common.Logo,
		"footer_html":                 common.Footer,
		"wechat_qrcode":               common.WeChatAccountQRCodeImageURL,
		"wechat_login":                common.WeChatAuthEnabled,
		"server_address":              system_setting.ServerAddress,
		"turnstile_check":             common.TurnstileCheckEnabled,
		"turnstile_site_key":          common.TurnstileSiteKey,
		"docs_link":                   operation_setting.GetGeneralSetting().DocsLink,
		"quota_per_unit":              common.QuotaPerUnit,
		// 兼容旧前端：保留 display_in_currency，同时提供新的 quota_display_type
		"display_in_currency":           operation_setting.IsCurrencyDisplay(),
		"quota_display_type":            operation_setting.GetQuotaDisplayType(),
		"custom_currency_symbol":        operation_setting.GetGeneralSetting().CustomCurrencySymbol,
		"custom_currency_exchange_rate": operation_setting.GetGeneralSetting().CustomCurrencyExchangeRate,
		"enable_batch_update":           common.BatchUpdateEnabled,
		"enable_drawing":                common.DrawingEnabled,
		"enable_task":                   common.TaskEnabled,
		"enable_data_export":            common.DataExportEnabled,
		"data_export_default_time":      common.DataExportDefaultTime,
		"default_collapse_sidebar":      common.DefaultCollapseSidebar,
		"mj_notify_enabled":             setting.MjNotifyEnabled,
		"chats":                         setting.Chats,
		"demo_site_enabled":             operation_setting.DemoSiteEnabled,
		"self_use_mode_enabled":         operation_setting.SelfUseModeEnabled,
		"register_enabled":              common.RegisterEnabled,
		"password_login_enabled":        common.PasswordLoginEnabled,
		"password_register_enabled":     common.PasswordRegisterEnabled,
		"default_use_auto_group":        setting.DefaultUseAutoGroup,

		"password_login_encryption_enabled": common.PasswordLoginEncryptionEnabledValue(),

		// T8 记忆注入是否在本部署开启（前端据此决定是否显示「我的记忆」配置项；
		// 未开启时保存无效，不应给用户一个看起来能用的开关）。
		"memory_injection_enabled": service.MemoryInjectionEnabled(),

		"usd_exchange_rate": operation_setting.USDExchangeRate,
		"price":             operation_setting.Price,
		"stripe_unit_price": setting.StripeUnitPrice,

		// 面板启用开关
		"api_info_enabled":      cs.ApiInfoEnabled,
		"uptime_kuma_enabled":   cs.UptimeKumaEnabled,
		"announcements_enabled": cs.AnnouncementsEnabled,
		"faq_enabled":           cs.FAQEnabled,

		// 模块管理配置
		"HeaderNavModules":    common.OptionMap["HeaderNavModules"],
		"SidebarModulesAdmin": common.OptionMap["SidebarModulesAdmin"],

		"oidc_enabled":                system_setting.GetOIDCSettings().Enabled,
		"oidc_client_id":              system_setting.GetOIDCSettings().ClientId,
		"oidc_authorization_endpoint": system_setting.GetOIDCSettings().AuthorizationEndpoint,
		"oidc_display_name":           system_setting.GetOIDCSettings().GetEffectiveDisplayName(),
		"passkey_login":               passkeySetting.Enabled,
		"passkey_display_name":        passkeySetting.RPDisplayName,
		"passkey_rp_id":               passkeySetting.RPID,
		"passkey_origins":             passkeySetting.Origins,
		"passkey_allow_insecure":      passkeySetting.AllowInsecureOrigin,
		"passkey_user_verification":   passkeySetting.UserVerification,
		"passkey_attachment":          passkeySetting.AttachmentPreference,
		"setup":                       constant.Setup,
		"user_agreement_enabled":      legalSetting.UserAgreement != "",
		"privacy_policy_enabled":      legalSetting.PrivacyPolicy != "",
		"checkin_enabled":             operation_setting.GetCheckinSetting().Enabled,
		// T6 全局并发桶水位（开关/当前并发/排队数/上限），供前端展示。
		"global_concurrency": middleware.GetGlobalConcurrencyStats(),
	}

	// 根据启用状态注入可选内容
	if cs.ApiInfoEnabled {
		data["api_info"] = console_setting.GetApiInfo()
	}
	if cs.AnnouncementsEnabled {
		data["announcements"] = console_setting.GetAnnouncements()
	}
	if cs.FAQEnabled {
		data["faq"] = console_setting.GetFAQ()
	}

	// Add enabled custom OAuth providers
	customProviders := oauth.GetEnabledCustomProviders()
	if len(customProviders) > 0 {
		type CustomOAuthInfo struct {
			Id                    int    `json:"id"`
			Name                  string `json:"name"`
			Slug                  string `json:"slug"`
			Icon                  string `json:"icon"`
			ClientId              string `json:"client_id"`
			AuthorizationEndpoint string `json:"authorization_endpoint"`
			Scopes                string `json:"scopes"`
		}
		providersInfo := make([]CustomOAuthInfo, 0, len(customProviders))
		for _, p := range customProviders {
			config := p.GetConfig()
			providersInfo = append(providersInfo, CustomOAuthInfo{
				Id:                    config.Id,
				Name:                  config.Name,
				Slug:                  config.Slug,
				Icon:                  config.Icon,
				ClientId:              config.ClientId,
				AuthorizationEndpoint: config.AuthorizationEndpoint,
				Scopes:                config.Scopes,
			})
		}
		data["custom_oauth_providers"] = providersInfo
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    data,
	})
	return
}

func GetNotice(c *gin.Context) {
	common.OptionMapRWMutex.RLock()
	notice := common.OptionMap["Notice"]
	common.OptionMapRWMutex.RUnlock()
	serveRevalidatedJSON(c, notice)
}

func GetAbout(c *gin.Context) {
	common.OptionMapRWMutex.RLock()
	about := common.OptionMap["About"]
	common.OptionMapRWMutex.RUnlock()
	serveRevalidatedJSON(c, about)
}

func GetUserAgreement(c *gin.Context) {
	serveRevalidatedJSON(c, system_setting.GetLegalSettings().UserAgreement)
}

func GetPrivacyPolicy(c *gin.Context) {
	serveRevalidatedJSON(c, system_setting.GetLegalSettings().PrivacyPolicy)
}

func GetMidjourney(c *gin.Context) {
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    common.OptionMap["Midjourney"],
	})
	return
}

func GetHomePageContent(c *gin.Context) {
	common.OptionMapRWMutex.RLock()
	homePageContent := common.OptionMap["HomePageContent"]
	common.OptionMapRWMutex.RUnlock()
	serveRevalidatedJSON(c, homePageContent)
}

// seoPublicPath 是一个可被收录的公开路由及其**服务端渲染元数据**。
//
// 为什么带 Meta：本前端是纯 CSR，`index.html` 是所有路由共用的一份壳，
// 不执行 JS 的爬虫看不到任何路由级内容（实测 6 个页面返回完全相同的字节、
// 零个 `<h1>`、且把 canonical 一律写死为首页）。因此路由级 meta 与一份
// 准确的内容摘要必须在**服务端**注入 —— 见 `controller/spa_seo.go`。
//
// `Title` / `Summary` / `H1` / `Caveat` 里可用 `{site}` 占位，替换为运行时站点名。
// **内容必须与页面真实呈现一致**（注入块在 React 挂载后会被移除，写不实内容即构成 cloaking）。
type seoPublicPath struct {
	Path     string
	Priority string
	Meta     spaRouteMeta
}

// seoPublicPaths 列出可被搜索引擎收录的公开路由（不含登录后页面）。
// 与 web/src/routes 下的公开路由保持一致；新增公开页时同步此表。
//
// 这张表是 **sitemap、robots 与 SPA 服务端注入**三者的唯一真相源 ——
// 不允许在别处再抄一份路由清单（抄一份就会分叉一份）。
var seoPublicPaths = []seoPublicPath{
	{
		Path: "/", Priority: "1.0",
		Meta: spaRouteMeta{
			Title:       "{site} — one API endpoint for many AI models",
			Description: "An AI API gateway: OpenAI-, Claude- and Gemini-compatible endpoints behind one base URL, with a per-model price list and usage rankings.",
			H1:          "{site}: one API endpoint for many AI models",
			NavLabel:    "Overview",
			Summary:     "{site} is an AI API gateway. It exposes OpenAI-compatible, Claude-compatible and Gemini-compatible endpoints from a single base URL, so an existing SDK only needs its base URL and key changed. This page is the entry point; the public pages below cover model pricing, usage rankings, the model test console and client setup.",
			Caveat:      "Model coverage and availability depend on the upstream channels configured for this deployment, and may change over time.",
		},
	},
	{
		Path: "/pricing", Priority: "0.9",
		Meta: spaRouteMeta{
			Title:       "Model pricing — {site}",
			Description: "Per-model price list for this gateway: what one call to each model costs, with the billing mode shown for every model.",
			H1:          "Model pricing",
			NavLabel:    "Model pricing",
			Summary:     "This page lists the models available on this gateway together with the price applied to each one. Each entry shows the billing mode in effect for that model and the resulting cost, so you can compare models before sending a request. Prices shown here are the ones this deployment actually applies.",
			Caveat:      "Prices are per this deployment's configuration and can be changed by its operator at any time; the price in effect when a request is handled is the one charged.",
		},
	},
	{
		Path: "/rankings", Priority: "0.7",
		Meta: spaRouteMeta{
			Title:       "Usage rankings — {site}",
			Description: "Model and usage rankings aggregated from this gateway's request statistics.",
			H1:          "Usage rankings",
			NavLabel:    "Usage rankings",
			Summary:     "This page aggregates request statistics collected by the gateway and ranks models by usage. It is a view over this deployment's own traffic, not a third-party benchmark.",
			Caveat:      "Rankings reflect only traffic that passed through this deployment; they are not an independent evaluation of model quality.",
		},
	},
	{
		Path: "/model-test", Priority: "0.6",
		Meta: spaRouteMeta{
			Title:       "Model test console — {site}",
			Description: "Send a test request to a model through this gateway and inspect the response.",
			H1:          "Model test console",
			NavLabel:    "Model test console",
			Summary:     "This page sends a request through the gateway to a chosen model and shows the reply, which is useful for checking that a key, a group or an upstream channel works before wiring it into an application.",
			Caveat:      "Test requests are real requests: they consume quota and are subject to the same routing and rate limits as application traffic.",
		},
	},
	{
		Path: "/tool-setup", Priority: "0.6",
		Meta: spaRouteMeta{
			Title:       "Client setup — {site}",
			Description: "How to point common AI clients and SDKs at this gateway's base URL.",
			H1:          "Client setup",
			NavLabel:    "Client setup",
			Summary:     "This page shows how to point common desktop clients and SDKs at this gateway, including the base URL to use and the environment variables or configuration keys that carry it.",
			Caveat:      "Client versions and configuration screens change; if an option name differs in your version, the equivalent setting is the one that overrides the API base URL.",
		},
	},
	{
		Path: "/about", Priority: "0.5",
		Meta: spaRouteMeta{
			Title:       "About — {site}",
			Description: "What this gateway is, what it depends on, and what it does not do.",
			H1:          "About this gateway",
			NavLabel:    "About",
			Summary:     "This page describes what {site} is and how it is operated. It is an API gateway: it forwards requests to upstream model providers that the operator configures, applies the deployment's billing and access rules, and returns the provider's response.",
			Caveat:      "As a gateway, this deployment does not train or host the models it serves; correctness and availability of a model's output remain the upstream provider's responsibility.",
		},
	},
	{
		// G7 公开状态页。文案只描述**页面上确实存在**的内容（与前端 /status 页同源），
		// 且刻意不写任何数字：页面上那些数字来自运行时探测与日志统计。
		Path: "/status", Priority: "0.4",
		Meta: spaRouteMeta{
			Title:       "Service status — {site}",
			Description: "What this gateway can report about itself: dependency reachability, requests recorded in the last 24 hours, and background task activity.",
			H1:          "Service status",
			NavLabel:    "Service status",
			Summary:     "This page reports what this deployment observes about itself: the reachability of its database, log database and cache; the requests and error entries recorded over the last 24 hours together with the resulting success share (shown only when error logging is enabled); the most recent heartbeat of each background task; and how long the process has been running. Every figure is measured on this instance; it is not a third-party monitoring service.",
			Caveat:      "These are measurements of the gateway itself, not an uptime guarantee or a service-level commitment: availability history is not sampled or retained here, and the figures refresh at intervals rather than in real time.",
		},
	},
}

// spaRouteMetaZh 是同一批公开路由的**中文**文案（zh-CN / zh-TW 共用）。
//
// 为什么需要：本站面向中文用户，而前端的 `<title>`/description 与路由级 meta 都是
// 运行时由 JS 写入的 —— 不执行 JS 的爬虫只会看到壳里的英文默认值。服务端注入必须
// 跟随**访问者的语言**（`Accept-Language`），否则等于把中文站推荐给爬虫当英文站。
//
// 优先级与默认：能识别出中文 → 中文；明确是英文 → 英文；**其它/无法识别 → 中文**
// （本站主要受众是中文用户）。见 `spaResolveLanguage`。
//
// 文案纪律：与英文表同源同义，只描述"这页是什么"，**不写任何未经证实的数字或评价**。
var spaRouteMetaZh = map[string]spaRouteMeta{
	"/": {
		Title:       "{site} — 一个接口调用多种 AI 模型",
		Description: "AI API 网关：用同一个基础地址提供 OpenAI、Claude、Gemini 兼容接口，并给出按模型的价目表与用量排行。",
		H1:          "{site}：一个接口调用多种 AI 模型",
		NavLabel:    "首页",
		Summary:     "{site} 是一个 AI API 网关。它用同一个基础地址同时提供 OpenAI、Claude、Gemini 兼容接口，已有的 SDK 通常只需改基础地址与密钥即可接入。本页是入口，下面的公开页分别介绍模型价格、用量排行、模型测试台与客户端配置。",
		Caveat:      "可用的模型与可用性取决于本部署所配置的上游渠道，并可能随时间变化。",
	},
	"/pricing": {
		Title:       "模型价格 — {site}",
		Description: "本站按模型的价目表：每个模型一次调用的费用，以及该模型当前适用的计费方式。",
		H1:          "模型价格",
		NavLabel:    "模型价格",
		Summary:     "本页列出本站已启用的模型及其对应价格。每条记录给出该模型当前适用的计费方式与实际费用，便于在发请求之前比较模型。此处显示的价格就是本部署实际执行的价格。",
		Caveat:      "价格由本部署的配置决定，运营方可能调整；计费以请求被处理时生效的价格为准。",
	},
	"/rankings": {
		Title:       "用量排行 — {site}",
		Description: "基于本站请求统计聚合出的模型与用量排行。",
		H1:          "用量排行",
		NavLabel:    "用量排行",
		Summary:     "本页把网关收集到的请求统计聚合起来，按用量对模型排序。它反映的是本部署自身的流量，不是第三方评测。",
		Caveat:      "排行只包含经过本部署的流量，不构成对模型质量的独立评价。",
	},
	"/model-test": {
		Title:       "模型测试台 — {site}",
		Description: "通过本站向模型发一次测试请求并查看返回结果。",
		H1:          "模型测试台",
		NavLabel:    "模型测试台",
		Summary:     "本页通过网关向所选模型发一次请求并显示回复，适合在把密钥、分组或上游渠道接入应用之前，先确认它们能正常工作。",
		Caveat:      "测试请求是真实请求：会消耗额度，并受与应用流量相同的路由与限流规则约束。",
	},
	"/tool-setup": {
		Title:       "客户端配置 — {site}",
		Description: "如何把常见 AI 客户端与 SDK 指向本站的基础地址。",
		H1:          "客户端配置",
		NavLabel:    "客户端配置",
		Summary:     "本页说明如何把常见的桌面客户端与 SDK 指向本站，包括应使用的基础地址，以及承载该地址的环境变量或配置项名称。",
		Caveat:      "客户端版本与配置界面会变化；若你所用版本中的选项名称不同，对应的就是那个覆盖 API 基础地址的设置项。",
	},
	"/about": {
		Title:       "关于 — {site}",
		Description: "本站是什么、依赖什么，以及它不做什么。",
		H1:          "关于本站",
		NavLabel:    "关于",
		Summary:     "本页说明 {site} 是什么以及如何运营。它是一个 API 网关：把请求转发给运营方配置的上游模型服务商，套用本部署的计费与访问规则，再把服务商的响应返回给你。",
		Caveat:      "作为网关，本部署不训练也不托管它所服务的模型；模型输出的正确性与可用性仍由上游服务商负责。",
	},
	"/status": {
		Title:       "服务状态 — {site}",
		Description: "本站对自身的可观测结果：各依赖组件是否可达、近 24 小时记录的请求量，以及后台任务活动。",
		H1:          "服务状态",
		NavLabel:    "服务状态",
		Summary:     "本页展示本部署对自身的观测结果：主库、日志库与缓存的探测状态；近 24 小时记录的请求量与错误条数及其得出的成功比例（仅在开启错误日志时给出）；各后台任务的最近一次心跳；以及进程已运行时长。所有数字均由本实例自身测得，不是第三方监控服务。",
		Caveat:      "这些是对网关自身的测量，不构成可用性承诺或服务等级保证：本站不采样、也不保留可用性历史，数字按固定间隔刷新而非实时。",
	},
	"/user-agreement": {
		Title:       "用户协议 — {site}",
		Description: "本部署运营方配置的用户协议。",
		H1:          "用户协议",
		NavLabel:    "用户协议",
		Summary:     "本页展示本部署运营方配置的用户协议；正文由运营方提供。",
		Caveat:      "若运营方尚未配置该文档，页面可能为空。",
	},
	"/privacy-policy": {
		Title:       "隐私政策 — {site}",
		Description: "本部署运营方配置的隐私政策。",
		H1:          "隐私政策",
		NavLabel:    "隐私政策",
		Summary:     "本页展示本部署运营方配置的隐私政策；正文由运营方提供。",
		Caveat:      "若运营方尚未配置该文档，页面可能为空。",
	},
}

// spaExternalDocLinks 是内容块里的上游官方文档外链（两种语言共用，地址不翻译）。
var spaExternalDocLinks = []struct{ Href, LabelEn, LabelZh string }{
	{"https://platform.openai.com/docs/api-reference", "OpenAI API reference", "OpenAI API 参考"},
	{"https://docs.anthropic.com/en/api/overview", "Anthropic (Claude) API overview", "Anthropic（Claude）API 概览"},
	{"https://ai.google.dev/gemini-api/docs", "Google Gemini API docs", "Google Gemini API 文档"},
}

// seoSiteBase 返回当前实例的站点根地址（无尾斜杠）。优先取系统设置的
// ServerAddress——它按站点各自配置（主站/副站不同），因此同一份二进制在
// 两台机器上会生成各自域名的 robots/sitemap，避免跨站指向。
// 回退顺序：ServerAddress → FRONTEND_BASE_URL 环境变量 → 默认 localhost。
func seoSiteBase() string {
	base := strings.TrimSpace(system_setting.ServerAddress)
	if base == "" {
		base = strings.TrimSpace(os.Getenv("FRONTEND_BASE_URL"))
	}
	if base == "" {
		base = "http://localhost:3000"
	}
	return strings.TrimRight(base, "/")
}

// GetRobotsTxt 动态生成 robots.txt：站点域名与 Disallow 规则随实例变化。
// 由于前端静态资源经 NoRoute 提供、无显式文件路由，此处显式注册优先命中。
func GetRobotsTxt(c *gin.Context) {
	base := seoSiteBase()
	body := "User-agent: *\n" +
		"Allow: /\n" +
		"Disallow: /dashboard\n" +
		"Disallow: /console\n" +
		"Disallow: /keys\n" +
		"Disallow: /channels\n" +
		"Disallow: /usage-logs\n" +
		"Disallow: /wallet\n" +
		"Disallow: /system-settings\n" +
		"Disallow: /setup\n" +
		"Disallow: /sign-in\n" +
		"Disallow: /sign-up\n" +
		"Disallow: /oauth\n" +
		"Sitemap: " + base + "/sitemap.xml\n"
	c.Header("Cache-Control", "public, max-age=3600")
	c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(body))
}

// GetSitemapXml 动态生成 sitemap.xml，loc 前缀使用当前实例域名。
//
// **刻意不输出 `<lastmod>`。** 此前它取 `time.Now()`，于是每次抓取时**所有** URL 都
// 写着"今天刚改过" —— 而 Google 明确表示会忽略不可信的 lastmod，持续给出假的
// "最新时间"会让整张 sitemap 的时间戳失去参考价值（连同真实变更一起被忽略）。
// 缺省比错误安全：等有真实的按页变更时间可用时再加回来。
func GetSitemapXml(c *gin.Context) {
	base := seoSiteBase()
	var b strings.Builder
	b.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	b.WriteString("<urlset xmlns=\"http://www.sitemaps.org/schemas/sitemap/0.9\" xmlns:xhtml=\"http://www.w3.org/1999/xhtml\">\n")
	// 只为**服务端确有对应语言内容**的语言生成 URL（见 spaHreflangLanguages）。
	// fr/ru/ja/vi 目前只有前端 UI 翻译，服务端页面文案仍是英文，收录会形成重复内容。
	for _, p := range seoPublicPaths {
		for _, lang := range spaHreflangLanguages {
			b.WriteString("  <url><loc>")
			b.WriteString(html.EscapeString(absoluteSEOURL(base, spaLocalizedPath(lang, p.Path))))
			b.WriteString("</loc>")
			for _, alt := range spaHreflangLanguages {
				b.WriteString(`<xhtml:link rel="alternate" hreflang="`)
				b.WriteString(html.EscapeString(spaHreflangTag(alt)))
				b.WriteString(`" href="`)
				b.WriteString(html.EscapeString(absoluteSEOURL(base, spaLocalizedPath(alt, p.Path))))
				b.WriteString(`"/>`)
			}
			b.WriteString(`<xhtml:link rel="alternate" hreflang="x-default" href="`)
			b.WriteString(html.EscapeString(absoluteSEOURL(base, spaLocalizedPath(spaDefaultHreflangLanguage, p.Path))))
			b.WriteString(`"/>`)
			b.WriteString("<priority>")
			b.WriteString(p.Priority)
			b.WriteString("</priority></url>\n")
		}
	}
	b.WriteString("</urlset>\n")
	c.Header("Cache-Control", "public, max-age=3600")
	c.Data(http.StatusOK, "application/xml; charset=utf-8", []byte(b.String()))
}

func SendEmailVerification(c *gin.Context) {
	email, err := service.ValidateAccountEmail(c.Query("email"))
	if err != nil {
		writeSecurityOperationError(c, err)
		return
	}

	// G4（防枚举）：无论邮箱是否已注册，都返回完全一致的统一成功响应。
	// 已注册邮箱不生成、不落库、不发送验证码；未注册邮箱照常发送。
	// 响应体不包含任何区分信息，杜绝通过该接口探测已注册邮箱。
	if !model.IsEmailAlreadyTaken(email) {
		code := common.GenerateVerificationCode(6)
		common.RegisterVerificationCodeWithKey(email, code, common.EmailVerificationPurpose)
		subject := fmt.Sprintf("%s邮箱验证邮件", common.SystemName)
		content := fmt.Sprintf("<p>您好，你正在进行%s邮箱验证。</p>"+
			"<p>您的验证码为: <strong>%s</strong></p>"+
			"<p>验证码 %d 分钟内有效，如果不是本人操作，请忽略。</p>", common.SystemName, code, common.VerificationValidMinutes)
		err = common.SendEmail(subject, email, content)
		if err != nil {
			// 发送失败不向客户端暴露（避免响应差异泄露邮箱状态），只记录日志。
			logger.LogError(c.Request.Context(), fmt.Sprintf("failed to send email verification to %s: %s", email, err.Error()))
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
	return
}

func SendPasswordResetEmail(c *gin.Context) {
	email := model.NormalizeEmail(c.Query("email"))
	if err := common.Validate.Var(email, "required,email"); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if _, err := model.GetUniqueUserByEmail(email); err == nil {
		code := common.GenerateVerificationCode(0)
		common.RegisterVerificationCodeWithKey(email, code, common.PasswordResetPurpose)
		link := fmt.Sprintf("%s/user/reset?email=%s&token=%s", system_setting.ServerAddress, email, code)
		subject := fmt.Sprintf("%s密码重置", common.SystemName)
		content := fmt.Sprintf("<p>您好，你正在进行%s密码重置。</p>"+
			"<p>点击 <a href='%s'>此处</a> 进行密码重置。</p>"+
			"<p>如果链接无法点击，请尝试点击下面的链接或将其复制到浏览器中打开：<br> %s </p>"+
			"<p>重置链接 %d 分钟内有效，如果不是本人操作，请忽略。</p>", common.SystemName, link, link, common.VerificationValidMinutes)
		err := common.SendEmail(subject, email, content)
		if err != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("failed to send password reset email to %s: %s", email, err.Error()))
		}
	} else if err != nil && !errors.Is(err, model.ErrEmailNotFound) {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("skip password reset email for %s: %s", email, err.Error()))
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

type PasswordResetRequest struct {
	Email string `json:"email"`
	Token string `json:"token"`
}

func ResetPassword(c *gin.Context) {
	var req PasswordResetRequest
	err := json.NewDecoder(c.Request.Body).Decode(&req)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	req.Email = model.NormalizeEmail(req.Email)
	if req.Email == "" || req.Token == "" {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if !common.VerifyCodeWithKey(req.Email, req.Token, common.PasswordResetPurpose) {
		common.ApiErrorI18n(c, i18n.MsgUserPasswordResetLinkInvalid)
		return
	}
	// T8/G3 修复：临时密码改用 crypto/rand 高熵随机（GenerateRandomCharsKey 内部
	// 使用 crand.Int，16 字符 62 字符集 ≈ 95 bit 熵），且响应绝不回传明文。
	password, err := common.GenerateRandomCharsKey(16)
	if err != nil {
		common.SysError("failed to generate temporary password: " + err.Error())
		common.ApiError(c, err)
		return
	}
	err = model.ResetUserPasswordByEmail(req.Email, password)
	if err != nil {
		if errors.Is(err, model.ErrEmailNotFound) || errors.Is(err, model.ErrEmailAmbiguous) {
			common.ApiErrorI18n(c, i18n.MsgUserPasswordResetLinkInvalid)
			return
		}
		common.ApiError(c, err)
		return
	}
	common.DeleteKey(req.Email, common.PasswordResetPurpose)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Password has been reset. Please check your email for the temporary password.",
	})
	return
}
