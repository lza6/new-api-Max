package dto

type UserSetting struct {
	NotifyType                       string  `json:"notify_type,omitempty"`                          // QuotaWarningType 额度预警类型
	QuotaWarningThreshold            float64 `json:"quota_warning_threshold,omitempty"`              // QuotaWarningThreshold 额度预警阈值
	QuotaWarnedLevels                []int   `json:"quota_warned_levels,omitempty"`                  // QuotaWarnedLevels 已触发额度预警档位记录（内部台账，非用户可配置）
	QuotaWarnThresholdsDefault       bool    `json:"quota_warn_thresholds_default,omitempty"`        // 是否注册注入的默认 80% 阈值（true=默认走多档；false=用户显式设置，单档最高优先级）
	WebhookUrl                       string  `json:"webhook_url,omitempty"`                          // WebhookUrl webhook地址
	WebhookSecret                    string  `json:"webhook_secret,omitempty"`                       // WebhookSecret webhook密钥
	NotificationEmail                string  `json:"notification_email,omitempty"`                   // NotificationEmail 通知邮箱地址
	BarkUrl                          string  `json:"bark_url,omitempty"`                             // BarkUrl Bark推送URL
	GotifyUrl                        string  `json:"gotify_url,omitempty"`                           // GotifyUrl Gotify服务器地址
	GotifyToken                      string  `json:"gotify_token,omitempty"`                         // GotifyToken Gotify应用令牌
	GotifyPriority                   int     `json:"gotify_priority"`                                // GotifyPriority Gotify消息优先级
	UpstreamModelUpdateNotifyEnabled bool    `json:"upstream_model_update_notify_enabled,omitempty"` // 是否接收上游模型更新定时检测通知（仅管理员）
	AcceptUnsetRatioModel            bool    `json:"accept_unset_model_ratio_model,omitempty"`       // AcceptUnsetRatioModel 是否接受未设置价格的模型
	RecordIpLog                      bool    `json:"record_ip_log,omitempty"`                        // 是否记录请求和错误日志IP
	SidebarModules                   string  `json:"sidebar_modules,omitempty"`                      // SidebarModules 左侧边栏模块配置
	BillingPreference                string  `json:"billing_preference,omitempty"`                   // BillingPreference 扣费策略（订阅/钱包）
	Language                         string  `json:"language,omitempty"`                             // Language 用户语言偏好 (zh, en)
	// §4.7.1 轻量用户记忆（规则+KV，非向量库）：记录用户最近一次成功调用的模型名，
	// 供前端在新建会话/Playground 时回填默认模型，减少重复选择。JSON 列内新增字段，
	// 无需迁移；空值表示尚未记录（前端回退到站点默认）。
	LastUsedModel string `json:"last_used_model,omitempty"`
	// T8 用户记忆注入：一段用户自定义的「记忆/偏好」文本，在开启
	// MEMORY_INJECTION_ENABLED 的部署里被注入到发往上游的 system prompt 之前，
	// 用于个性化。默认空（不注入）。JSON 列内新增字段，无需迁移。
	MemoryInjection string `json:"memory_injection,omitempty"`
}

var (
	NotifyTypeEmail   = "email"   // Email 邮件
	NotifyTypeWebhook = "webhook" // Webhook
	NotifyTypeBark    = "bark"    // Bark 推送
	NotifyTypeGotify  = "gotify"  // Gotify 推送
)
