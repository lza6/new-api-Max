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

	// 兑换码（额度卡）兑换功能开关；关闭后用户无法兑换额度卡，管理员仍可生成
	RedemptionEnabled bool `json:"redemption_enabled"`
	// 充值功能总开关；关闭后所有充值入口（兑换码/在线支付/订阅）对用户停用
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

// IsRedemptionEnabled 兑换码兑换是否开放（需同时满足合规确认与开关）
func IsRedemptionEnabled() bool {
	return IsPaymentComplianceConfirmed() && paymentSetting.RedemptionEnabled
}

// IsTopUpEnabled 充值功能是否开放（需同时满足合规确认与开关）
func IsTopUpEnabled() bool {
	return IsPaymentComplianceConfirmed() && paymentSetting.TopUpEnabled
}
