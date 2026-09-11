package operation_setting

import "github.com/lza6/new-api-Max/setting/config"

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

func init() {
	// 注册到全局配置管理器
	config.GlobalConfig.Register("payment_setting", &paymentSetting)
}

func GetPaymentSetting() *PaymentSetting {
	return &paymentSetting
}

func IsPaymentComplianceConfirmed() bool {
	return paymentSetting.ComplianceConfirmed &&
		paymentSetting.ComplianceTermsVersion == CurrentComplianceTermsVersion
}

// IsRedemptionEnabled 兑换码兑换是否开放（需同时满足合规确认与开关；与充值总开关相互独立）
func IsRedemptionEnabled() bool {
	return IsPaymentComplianceConfirmed() && paymentSetting.RedemptionEnabled
}

// IsTopUpEnabled 在线充值是否开放（需同时满足合规确认与开关；只管在线支付与订阅，不管兑换码）
func IsTopUpEnabled() bool {
	return IsPaymentComplianceConfirmed() && paymentSetting.TopUpEnabled
}
