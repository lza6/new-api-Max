package operation_setting

import (
	"maps"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/lza6/new-api-Max/setting/config"
)

type PaymentSetting struct {
	AmountOptions  []int           `json:"amount_options"`
	AmountDiscount map[int]float64 `json:"amount_discount"` // 充值金额对应的折扣，例如 100 元 0.9 表示 100 元充值享受 9 折优惠

	ComplianceConfirmed    bool   `json:"compliance_confirmed"`
	ComplianceTermsVersion string `json:"compliance_terms_version"`
	ComplianceConfirmedAt  int64  `json:"compliance_confirmed_at"`
	ComplianceConfirmedBy  int    `json:"compliance_confirmed_by"`
	ComplianceConfirmedIP  string `json:"compliance_confirmed_ip"`

	// 兑换码（额度卡）兑换功能开关；与充值总开关相互独立。关闭后用户无法兑换额度卡，管理员仍可生成
	RedemptionEnabled bool `json:"redemption_enabled"`
	// 在线充值总开关；只控制在线支付与订阅入口（兑换码不受它影响），关闭后用户无法在线充值
	TopUpEnabled bool `json:"topup_enabled"`
}

const CurrentComplianceTermsVersion = "v1"

// 默认配置
var paymentSetting = PaymentSetting{
	AmountOptions:     []int{10, 20, 50, 100, 200, 500},
	AmountDiscount:    map[int]float64{},
	RedemptionEnabled: true,
	TopUpEnabled:      true,
}

// paymentSettingMu 保护 paymentSetting 主副本（含 map/slice 字段）。所有写入
// （配置热更新反射写入、UpdatePaymentSetting）持写锁，写完后发布新快照。
var paymentSettingMu sync.RWMutex

// paymentSettingSnapshot 已发布的不可变快照。充值/合规热路径只 Load() 后只读，
// 永不触碰正被 60s 周期重载（model.SyncOptions → updateConfigFromMap 反射就地写
// map/slice）的主副本 —— 消除 map 并发读写导致的不可 recover fatal。
var paymentSettingSnapshot atomic.Pointer[PaymentSetting]

// publishPaymentSettingSnapshotLocked 在持 paymentSettingMu 前提下深拷贝主副本并发布。
// map/slice 必须克隆，否则快照与主副本共享底层存储，后续写入仍会与读者竞争。
func publishPaymentSettingSnapshotLocked() {
	snap := paymentSetting
	snap.AmountOptions = slices.Clone(paymentSetting.AmountOptions)
	snap.AmountDiscount = maps.Clone(paymentSetting.AmountDiscount)
	paymentSettingSnapshot.Store(&snap)
}

// loadPaymentSetting 返回当前不可变快照（无锁）。首次快照发布前兜底返回主副本指针。
func loadPaymentSetting() *PaymentSetting {
	if s := paymentSettingSnapshot.Load(); s != nil {
		return s
	}
	return &paymentSetting
}

// UpdatePaymentSetting 在写锁内修改主副本并发布新快照（供运行时变更与测试使用）。
func UpdatePaymentSetting(fn func(*PaymentSetting)) {
	paymentSettingMu.Lock()
	defer paymentSettingMu.Unlock()
	fn(&paymentSetting)
	publishPaymentSettingSnapshotLocked()
}

// BeforeConfigWrite / AfterConfigWrite 实现 config.configWriteHook：热更新反射就地
// 写入期间持写锁，写完后发布新快照，使读侧只见一致快照。
func (p *PaymentSetting) BeforeConfigWrite() { paymentSettingMu.Lock() }
func (p *PaymentSetting) AfterConfigWrite() {
	publishPaymentSettingSnapshotLocked()
	paymentSettingMu.Unlock()
}

// LockConfigRead / UnlockConfigRead 实现 config.configReadGuard：反射式读取主副本
// （ExportAllConfigs/SaveToDB）持读锁，与热更新写入互斥。
func (p *PaymentSetting) LockConfigRead()   { paymentSettingMu.RLock() }
func (p *PaymentSetting) UnlockConfigRead() { paymentSettingMu.RUnlock() }

func init() {
	// 注册到全局配置管理器
	config.GlobalConfig.Register("payment_setting", &paymentSetting)
	paymentSettingMu.Lock()
	publishPaymentSettingSnapshotLocked()
	paymentSettingMu.Unlock()
}

// GetPaymentSetting 返回当前不可变快照。只读；修改请用 UpdatePaymentSetting。
func GetPaymentSetting() *PaymentSetting {
	return loadPaymentSetting()
}

func IsPaymentComplianceConfirmed() bool {
	s := loadPaymentSetting()
	return s.ComplianceConfirmed &&
		s.ComplianceTermsVersion == CurrentComplianceTermsVersion
}

// IsRedemptionEnabled 兑换码兑换是否开放（需同时满足合规确认与开关；与充值总开关相互独立）
func IsRedemptionEnabled() bool {
	if !IsPaymentComplianceConfirmed() {
		return false
	}
	return loadPaymentSetting().RedemptionEnabled
}

// IsTopUpEnabled 在线充值是否开放（需同时满足合规确认与开关；只管在线支付与订阅，不管兑换码）
func IsTopUpEnabled() bool {
	if !IsPaymentComplianceConfirmed() {
		return false
	}
	return loadPaymentSetting().TopUpEnabled
}
