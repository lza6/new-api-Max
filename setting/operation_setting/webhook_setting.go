package operation_setting

import (
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/lza6/new-api-Max/setting/config"
)

// WebhookSetting 通用 Webhook 事件通知配置（热更新，注册名 "webhook"）。
// T15-B：仅做「事件通知 + 幂等签名」，不参与结算/退款（任务结算语义仍以轮询为准）。
type WebhookSetting struct {
	// Enabled 总开关（默认关，以避免未配置时产生任何外呼）。
	Enabled bool `json:"enabled"`
	// URL 接收回调的地址。允许 http/https（https 强烈推荐）；SSRF 防护拒绝
	// 私网/环回/云元数据段（保存与发送时双重校验）。
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

// webhookSettingMu 保护 webhookSetting 主副本（Events 为 slice）。
var webhookSettingMu sync.RWMutex

// webhookSettingSnapshot 已发布的不可变快照。事件通知判定走快照，避免与周期
// 热更新（反射就地写 slice）或管理端替换竞争。
var webhookSettingSnapshot atomic.Pointer[WebhookSetting]

// publishWebhookSettingSnapshotLocked 在持 webhookSettingMu 前提下深拷贝主副本并发布。
func publishWebhookSettingSnapshotLocked() {
	snap := webhookSetting
	snap.Events = slices.Clone(webhookSetting.Events)
	webhookSettingSnapshot.Store(&snap)
}

// loadWebhookSetting 返回当前不可变快照（无锁）。首次快照发布前兜底返回主副本指针。
func loadWebhookSetting() *WebhookSetting {
	if s := webhookSettingSnapshot.Load(); s != nil {
		return s
	}
	return &webhookSetting
}

// BeforeConfigWrite / AfterConfigWrite 实现 config.configWriteHook。
func (w *WebhookSetting) BeforeConfigWrite() { webhookSettingMu.Lock() }
func (w *WebhookSetting) AfterConfigWrite() {
	publishWebhookSettingSnapshotLocked()
	webhookSettingMu.Unlock()
}

// LockConfigRead / UnlockConfigRead 实现 config.configReadGuard。
func (w *WebhookSetting) LockConfigRead()   { webhookSettingMu.RLock() }
func (w *WebhookSetting) UnlockConfigRead() { webhookSettingMu.RUnlock() }

func init() {
	config.GlobalConfig.Register("webhook", &webhookSetting)
	webhookSettingMu.Lock()
	publishWebhookSettingSnapshotLocked()
	webhookSettingMu.Unlock()
}

// GetWebhookSetting 返回当前不可变快照。只读，勿直接改写返回对象。
func GetWebhookSetting() *WebhookSetting {
	return loadWebhookSetting()
}

// SnapshotWebhookSetting 返回当前配置的深拷贝，用于「副本组装→校验→提交」，
// 避免控制层校验失败时污染运行时配置。
func SnapshotWebhookSetting() WebhookSetting {
	return *loadWebhookSetting()
}

// ReplaceWebhookSetting 用已校验的副本原子替换运行时配置（Events 深拷贝）。
func ReplaceWebhookSetting(s WebhookSetting) {
	webhookSettingMu.Lock()
	defer webhookSettingMu.Unlock()
	s.Events = slices.Clone(s.Events)
	webhookSetting = s
	publishWebhookSettingSnapshotLocked()
}

// UpdateWebhookSetting 在写锁内修改主副本并发布新快照（供运行时变更与测试使用）。
func UpdateWebhookSetting(fn func(*WebhookSetting)) {
	webhookSettingMu.Lock()
	defer webhookSettingMu.Unlock()
	fn(&webhookSetting)
	publishWebhookSettingSnapshotLocked()
}

// IsWebhookEventSubscribed 事件类型是否在订阅白名单内（空白条目丢弃）。
func IsWebhookEventSubscribed(eventType string) bool {
	for _, it := range strings.FieldsFunc(strings.Join(loadWebhookSetting().Events, " "), func(r rune) bool {
		return r == ' ' || r == ',' || r == '\n' || r == '\t'
	}) {
		if strings.TrimSpace(it) == eventType {
			return true
		}
	}
	return false
}