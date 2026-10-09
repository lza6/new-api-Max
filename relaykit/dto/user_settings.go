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
	// T13 Skills 注入 v1：用户启用中的技能会被拼进 system 前缀。
	Skills []UserSkill `json:"skills,omitempty"`
	// T13 Agent 预设：游乐场可一键套用的对话配置快照。
	AgentPresets []AgentPreset `json:"agent_presets,omitempty"`
}

// MaxMemoryInjectionRunes 记忆注入文本的最大字符数（按 Unicode 码点计）。
// 记忆文本会进入每一次上游请求的 system 前缀，必须限长，否则用户可提交超大
// 文本把成本转嫁到每一次调用上。2000 字符足以表达稳定的偏好/背景设定。
const MaxMemoryInjectionRunes = 2000

// UserSkill T13 Skills 注入 v1：用户自定义的「技能/提示片段」。
//
// v1 只做**注入**——把启用中的技能按名称拼进 system 前缀，让模型知道该按什么
// 方式作答（对齐 litellm 的 Skills 注入 Hook）；不包含沙箱执行（那属于 v2，
// 需要独立的执行隔离与权限模型）。
//
// 存储：UserSetting 的 JSON 列内新增字段，无需迁移（三库均为 text/json 列）。
type UserSkill struct {
	Id   string `json:"id"`
	Name string `json:"name"`
	// Prompt 该技能的具体指令（例如「回答先用一句话给结论，再展开依据」）。
	Prompt string `json:"prompt"`
	// Enabled 仅启用中的技能会被注入。
	Enabled bool `json:"enabled"`
}

// AgentPreset T13 per-agent 能力下沉 v1：可复用的「对话配置快照」。
//
// 对齐《参考的结果计划指南》§4 的落地形态 {模型/参数/系统提示/...}，
// 前端游乐场按预设一键切换，把「助手」的配置与聊天记录解耦。
// 存储同上：UserSetting JSON 列内新增字段，无需迁移。
type AgentPreset struct {
	Id    string `json:"id"`
	Name  string `json:"name"`
	Model string `json:"model,omitempty"`
	Group string `json:"group,omitempty"`
	// SystemPrompt 该预设的角色设定。
	SystemPrompt string `json:"system_prompt,omitempty"`
	// Temperature / MaxTokens 用指针以区分「未设置」与「显式 0」。
	Temperature *float64 `json:"temperature,omitempty"`
	MaxTokens   *uint    `json:"max_tokens,omitempty"`
	// ReasoningEffort 思考程度（如 low/medium/high），空=不干预。
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
}

// 上界常量：这些字段都会进入每一次上游请求，必须限长限数，防止用户把
// 成本转嫁到每一次调用上（与 MaxMemoryInjectionRunes 同一理由）。
const (
	MaxUserSkills        = 20
	MaxSkillNameRunes    = 64
	MaxSkillPromptRunes  = 2000
	MaxAgentPresets      = 20
	MaxPresetNameRunes   = 64
	MaxPresetSystemRunes = 8000
)

var (
	NotifyTypeEmail   = "email"   // Email 邮件
	NotifyTypeWebhook = "webhook" // Webhook
	NotifyTypeBark    = "bark"    // Bark 推送
	NotifyTypeGotify  = "gotify"  // Gotify 推送
)
