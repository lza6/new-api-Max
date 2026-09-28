package service

import (
	"testing"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/relaykit/dto"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 默认档位，与 common.QuotaWarnThresholds 保持一致；单测显式声明以隔离全局配置漂移。
var testWarnThresholds = []int{1000, 500, 100}

// TestDecideQuotaWarnMultiTierOrder 验证多档触发顺序：从大到小，剩余额度跨过哪一档就触发哪一档。
func TestDecideQuotaWarnMultiTierOrder(t *testing.T) {
	tests := []struct {
		name          string
		remaining     int64
		alreadyWarned []int
		wantNotify    []int
		wantWarned    []int
	}{
		{
			name:       "一次性跨过全部三档，按 1000/500/100 顺序触发",
			remaining:  50,
			wantNotify: []int{1000, 500, 100},
			wantWarned: []int{1000, 500, 100},
		},
		{
			name:       "剩余低于 1000 与 500，仍高于 100",
			remaining:  400,
			wantNotify: []int{1000, 500},
			wantWarned: []int{1000, 500},
		},
		{
			name:       "仅跨过 1000 档",
			remaining:  600,
			wantNotify: []int{1000},
			wantWarned: []int{1000},
		},
		{
			name:       "剩余充足不触发",
			remaining:  1500,
			wantNotify: nil,
			wantWarned: nil,
		},
		{
			name:       "档位边界：remaining 等于档值不触发该档",
			remaining:  100,
			wantNotify: []int{1000, 500},
			wantWarned: []int{1000, 500},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			notify, warned := decideQuotaWarn(tt.remaining, 0, testWarnThresholds, tt.alreadyWarned)
			assert.Equal(t, tt.wantNotify, notify, "触发档位顺序必须从大到小")
			assert.Equal(t, tt.wantWarned, warned, "已记录档位要与触发档位一致")
		})
	}
}

// TestDecideQuotaWarnNoDuplicate 验证防重复：同一档位已记录后不再重复触发。
func TestDecideQuotaWarnNoDuplicate(t *testing.T) {
	// 1000 档已提醒过，下降过程中只触发新跨过的 500 档。
	notify, warned := decideQuotaWarn(400, 0, testWarnThresholds, []int{1000})
	assert.Equal(t, []int{500}, notify)
	assert.Equal(t, []int{1000, 500}, warned)

	// 1000/500 均已记录，再降也只维持台账，不重复发。
	notify2, warned2 := decideQuotaWarn(300, 0, testWarnThresholds, []int{1000, 500})
	assert.Nil(t, notify2)
	assert.Equal(t, []int{1000, 500}, warned2)
}

// TestDecideQuotaWarnResetOnRecharge 验证充值/额度增长重置：回升到已记录档位以上后
// 剔除该档，下次再降到该档可再次提醒。
func TestDecideQuotaWarnResetOnRecharge(t *testing.T) {
	// 充值后剩余回升到 3000，三个档位全部剔除，且不再发提醒。
	notify, warned := decideQuotaWarn(3000, 0, testWarnThresholds, []int{1000, 500, 100})
	assert.Nil(t, notify)
	assert.Empty(t, warned)

	// 消耗再次降到 400：1000 与 500 档可重新触发。
	notify2, warned2 := decideQuotaWarn(400, 0, testWarnThresholds, warned)
	assert.Equal(t, []int{1000, 500}, notify2)
	assert.Equal(t, []int{1000, 500}, warned2)

	// 部分回升：仅 500 档被剔除，1000 档保留（未回到 1000 以上）。
	notify3, warned3 := decideQuotaWarn(900, 0, testWarnThresholds, []int{1000, 500})
	assert.Nil(t, notify3)
	assert.Equal(t, []int{1000}, warned3)
}

// TestDecideQuotaWarnExplicitThresholdSingle 验证用户显式单档兼容：
// 显式 QuotaWarningThreshold 作为最高优先级单档，不记录档位，也不受多档默认值影响。
func TestDecideQuotaWarnExplicitThresholdSingle(t *testing.T) {
	// 低于显式阈值 1000 → 触发单档，且不写入台账（newWarned 与入参一致）。
	notify, warned := decideQuotaWarn(900, 1000, testWarnThresholds, nil)
	assert.Equal(t, []int{1000}, notify)
	assert.Empty(t, warned)

	// 高于显式阈值 → 不触发。
	notify2, warned2 := decideQuotaWarn(1200, 1000, testWarnThresholds, nil)
	assert.Nil(t, notify2)
	assert.Empty(t, warned2)

	// 显式阈值 800 高于剩余 900 吗？不 —— 不触发；即使 900 已低于多档 1000 档，
	// 显式单档仍不被多档默认影响（多档仅在未显式设置时启用）。
	notify3, warned3 := decideQuotaWarn(900, 800, testWarnThresholds, nil)
	assert.Nil(t, notify3)
	assert.Empty(t, warned3)

	// 显式阈值跨过但后续回升到阈值以上：仍可再次触发（不记录档位，行为与存量一致）。
	notify4, warned4 := decideQuotaWarn(1500, 1000, testWarnThresholds, nil)
	assert.Nil(t, notify4)
	assert.Empty(t, warned4)
	notify5, _ := decideQuotaWarn(900, 1000, testWarnThresholds, nil)
	assert.Equal(t, []int{1000}, notify5)
}

// TestDecideQuotaWarnSubscriptionSame 验证订阅额度口径与钱包额度走同一多档逻辑
// （订阅剩余 = SubscriptionAmountTotal - UsedAfterPreConsume - PostDelta）。
func TestDecideQuotaWarnSubscriptionSame(t *testing.T) {
	// 订阅剩余 400：触发 1000/500 档（与钱包口径的 decideQuotaWarn 完全一致）。
	total := int64(5000)
	usedAfter := int64(4600)
	remaining := total - usedAfter
	notify, warned := decideQuotaWarn(remaining, 0, testWarnThresholds, nil)
	assert.Equal(t, []int{1000, 500}, notify)
	assert.Equal(t, []int{1000, 500}, warned)

	// 订阅剩余 50：三档全触发。
	remaining2 := total - int64(4950)
	notify2, _ := decideQuotaWarn(remaining2, 0, testWarnThresholds, nil)
	assert.Equal(t, []int{1000, 500, 100}, notify2)

	// 订阅剩余 100（等于 100 档值）：100 档不触发。
	remaining3 := total - int64(4900)
	notify3, warned3 := decideQuotaWarn(remaining3, 0, testWarnThresholds, nil)
	assert.Equal(t, []int{1000, 500}, notify3)
	assert.Equal(t, []int{1000, 500}, warned3)

	// 订阅显式阈值单档同样生效。
	notify4, warned4 := decideQuotaWarn(remaining2, 80, testWarnThresholds, nil)
	assert.Equal(t, []int{80}, notify4)
	assert.Empty(t, warned4)
}

// TestParseQuotaWarnThresholds 验证 QUOTA_WARN_THRESHOLDS 逗号分隔解析：
// 去重、排序（从大到小）、容忍空白，非法输入返回错误由调用方回退默认。
func TestParseQuotaWarnThresholds(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    []int
		wantErr bool
	}{
		{name: "标准逗号分隔", raw: "1000,500,100", want: []int{1000, 500, 100}},
		{name: "容忍空格", raw: "1000, 500, 100", want: []int{1000, 500, 100}},
		{name: "乱序去重", raw: "100,1000,500,1000", want: []int{1000, 500, 100}},
		{name: "空段跳过", raw: "1000,,500", want: []int{1000, 500}},
		{name: "全空回退默认", raw: "", want: nil},
		{name: "非法值报错", raw: "abc", wantErr: true},
		{name: "含非法值报错", raw: "1000,abc", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := common.ParseQuotaWarnThresholds(tt.raw)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestUsingExplicitQuotaWarn P1-1 审查修复的 gating 分支直接单测：
// 显式单档仅在「阈值非零 且 非注册注入默认」时启用；注入默认 80%（default=true）
// 必须走多档；存量用户（无 default 字段）非零阈值走显式单档。
func TestUsingExplicitQuotaWarn(t *testing.T) {
	tests := []struct {
		name  string
		setup dto.UserSetting
		want  bool
	}{
		{
			name:  "零阈值 + 无 default 标记 → 多档（未显式设置）",
			setup: dto.UserSetting{},
			want:  false,
		},
		{
			name: "用户显式设置非零阈值 → 单档最高优先级",
			setup: dto.UserSetting{
				QuotaWarningThreshold: 8000,
			},
			want: true,
		},
		{
			name: "注册注入默认 80%（default=true）→ 多档，不误判显式",
			setup: dto.UserSetting{
				QuotaWarningThreshold:      400000,
				QuotaWarnThresholdsDefault: true,
			},
			want: false,
		},
		{
			name: "存量用户无 default 字段 + 非零阈值 → 显式单档（兼容）",
			setup: dto.UserSetting{
				QuotaWarningThreshold: 1280,
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, usingExplicitQuotaWarn(tt.setup))
		})
	}
}
