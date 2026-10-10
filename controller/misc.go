package controller

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

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

// seoPublicPaths 列出可被搜索引擎收录的公开路由（不含登录后页面）。
// 与 web/src/routes 下的公开路由保持一致；新增公开页时同步此表。
var seoPublicPaths = []struct {
	Path     string
	Priority string
}{
	{"/", "1.0"},
	{"/pricing", "0.9"},
	{"/rankings", "0.7"},
	{"/model-test", "0.6"},
	{"/tool-setup", "0.6"},
	{"/about", "0.5"},
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
func GetSitemapXml(c *gin.Context) {
	base := seoSiteBase()
	lastmod := time.Now().Format("2006-01-02")
	var b strings.Builder
	b.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	b.WriteString("<urlset xmlns=\"http://www.sitemaps.org/schemas/sitemap/0.9\">\n")
	for _, p := range seoPublicPaths {
		b.WriteString("  <url><loc>")
		b.WriteString(base)
		b.WriteString(p.Path)
		b.WriteString("</loc><lastmod>")
		b.WriteString(lastmod)
		b.WriteString("</lastmod><priority>")
		b.WriteString(p.Priority)
		b.WriteString("</priority></url>\n")
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
