package operation_setting

import (
	"strings"

	"github.com/lza6/new-api-Max/setting/config"
)

// WebhookSetting 通用 Webhook 事件通知配置（热更新，注册名 "webhook"）。
// T15-B：仅做「事件通知 + 幂等签名」，不参与结算/退款（任务结算语义仍以轮询为准）。
type WebhookSetting struct {
	// Enabled 总开关（默认关，以避免未配置时产生任何外呼）。
	Enabled bool `json:"enabled"`
	// URL 接收回调的 HTTPS 地址（SSRF 防护：仅 https，且拒绝私网/环回/云元数据段）。
	URL string `json:"url"`
	// Secret 用于 HMAC-SHA256 签名，请求头 X-New-API-Webhook-Signature: sha256=<hex>。
	Secret string `json:"secret"`
	// Events 订阅的事件类型白名单（空=不通知任何事件）。
	Events []string `json:"events"`
}

const (
	// WebhookEventEpayTopupSuccess 充值到账通知。
	WebhookEventEpayTopupSuccess = "epay.topup.success"
	// WebhookEventEpaySubscriptionSuccess 订阅成功通知。
	WebhookEventEpaySubscriptionSuccess = "epay.subscription.success"
	// WebhookEventTaskSettled 任务结算通知（批次任务到达终态且结算完成）。
	WebhookEventTaskSettled = "task.settled"
)

var webhookSetting = WebhookSetting{Enabled: false}

func init() {
	config.GlobalConfig.Register("webhook", &webhookSetting)
}

func GetWebhookSetting() *WebhookSetting {
	return &webhookSetting
}

// IsWebhookEventSubscribed 事件类型是否在订阅白名单内（空白条目丢弃）。
func IsWebhookEventSubscribed(eventType string) bool {
	for _, it := range strings.FieldsFunc(strings.Join(webhookSetting.Events, " "), func(r rune) bool {
		return r == ' ' || r == ',' || r == '\n' || r == '\t'
	}) {
		if strings.TrimSpace(it) == eventType {
			return true
		}
	}
	return false
}