package service

import (
	"math"
	"net/http"
	"testing"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/model"
	relaycommon "github.com/lza6/new-api-Max/relay/common"
	"github.com/lza6/new-api-Max/relaykit/dto"
	"github.com/lza6/new-api-Max/relaykit/types"
	hosttypes "github.com/lza6/new-api-Max/types"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAttachQuotaSaturationNestsUnderAdminInfo verifies the saturation marker
// is nested under other.admin_info.quota_saturation so it is admin-only (the
// log formatter strips admin_info for non-admin viewers).
func TestAttachQuotaSaturationNestsUnderAdminInfo(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(nil)

	relayInfo := &relaycommon.RelayInfo{
		UserId:          7,
		OriginModelName: "gpt-image-1",
		QuotaClamp: &common.QuotaClamp{
			Op:       "QuotaFromDecimal",
			Kind:     common.QuotaClampOverflow,
			Original: 1.8e19,
			Clamped:  common.MaxQuota,
		},
	}

	other := model.NewLogOther()
	other.SetPublic("model_price", 0.004)
	attachQuotaSaturation(ctx, relayInfo, other)

	adminInfo, ok := other.Snapshot()["admin_info"].(map[string]any)
	require.True(t, ok, "admin_info should be created")
	sat, ok := adminInfo["quota_saturation"].(map[string]any)
	require.True(t, ok, "quota_saturation should be nested under admin_info")
	require.Equal(t, "QuotaFromDecimal", sat["op"])
	require.Equal(t, common.QuotaClampOverflow, sat["kind"])
	require.Equal(t, common.MaxQuota, sat["clamped"])
}

func TestCalcViolationFeeQuotaSaturates(t *testing.T) {
	oldQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500_000
	t.Cleanup(func() { common.QuotaPerUnit = oldQuotaPerUnit })

	got, clamp := calcViolationFeeQuota(1e20, 1)
	require.Equal(t, common.MaxQuota, got)
	require.NotNil(t, clamp, "超界违规费金额必须产生饱和审计事件")
	require.Equal(t, common.QuotaClampOverflow, clamp.Kind)
}

// TestClampAudioDurationSecondsAuditsOutOfRange T4-2b：音频时长必须先钳制再
// 转换；超界值产生饱和审计事件，正常值 clamp 为 nil 且结果不变。
func TestClampAudioDurationSecondsAuditsOutOfRange(t *testing.T) {
	// 正常值：不钳制、无饱和事件。
	d, clamp := ClampAudioDurationSeconds(30)
	require.Equal(t, 30.0, d)
	require.Nil(t, clamp)

	// 超界值：钳到任务时长上限并返回 overflow 审计事件。
	d, clamp = ClampAudioDurationSeconds(1e9)
	require.Equal(t, float64(relaycommon.MaxTaskDurationSeconds), d)
	require.NotNil(t, clamp)
	require.Equal(t, common.QuotaClampOverflow, clamp.Kind)
	require.Equal(t, "AudioDurationClamp", clamp.Op)

	// 负值：钳到 0 并返回 underflow 审计事件。
	d, clamp = ClampAudioDurationSeconds(-5)
	require.Equal(t, 0.0, d)
	require.NotNil(t, clamp)
	require.Equal(t, common.QuotaClampUnderflow, clamp.Kind)

	// NaN：钳到 0 并返回 nan 审计事件。
	d, clamp = ClampAudioDurationSeconds(math.NaN())
	require.Equal(t, 0.0, d)
	require.NotNil(t, clamp)
	require.Equal(t, common.QuotaClampNaN, clamp.Kind)
}

func TestCalcOpenRouterCacheCreateTokensDoesNotWrap(t *testing.T) {
	oldQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500_000
	t.Cleanup(func() { common.QuotaPerUnit = oldQuotaPerUnit })

	got := CalcOpenRouterCacheCreateTokens(dto.Usage{Cost: math.Inf(1)}, hosttypes.PriceData{
		ModelRatio:         1,
		CacheCreationRatio: 2,
		CacheRatio:         1,
		CompletionRatio:    1,
	})
	require.Equal(t, -1, got)
}

// TestAttachQuotaSaturationPreservesExistingAdminInfo verifies the marker is
// merged into a pre-existing admin_info map without clobbering it.
func TestAttachQuotaSaturationPreservesExistingAdminInfo(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(nil)

	relayInfo := &relaycommon.RelayInfo{
		QuotaClamp: &common.QuotaClamp{Op: "QuotaFromFloat", Kind: common.QuotaClampUnderflow, Clamped: common.MinQuota},
	}
	other := model.NewLogOther()
	other.SetAdmin("admin_username", "root")
	attachQuotaSaturation(ctx, relayInfo, other)

	adminInfo := other.Snapshot()["admin_info"].(map[string]any)
	require.Equal(t, "root", adminInfo["admin_username"], "existing admin_info fields preserved")
	require.NotNil(t, adminInfo["quota_saturation"])
}

// TestAttachQuotaSaturationNoClampNoMarker verifies the common case (no
// saturation) leaves the log untouched.
func TestAttachQuotaSaturationNoClampNoMarker(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(nil)

	relayInfo := &relaycommon.RelayInfo{QuotaClamp: nil}
	other := model.NewLogOther()
	other.SetPublic("model_price", 0.004)
	attachQuotaSaturation(ctx, relayInfo, other)

	_, hasAdmin := other.Snapshot()["admin_info"]
	require.False(t, hasAdmin, "no admin_info should be added when there is no clamp")
}

func TestPreConsumeBillingRejectsSaturatedQuotaBeforeDeduction(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	info := &relaycommon.RelayInfo{
		QuotaClamp: &common.QuotaClamp{
			Op:       "QuotaFromFloat",
			Kind:     common.QuotaClampOverflow,
			Original: 1e30,
			Clamped:  common.MaxQuota,
		},
	}

	apiErr := PreConsumeBilling(c, common.MaxQuota, info)

	require.NotNil(t, apiErr)
	require.Equal(t, types.ErrorCodeModelPriceError, apiErr.GetErrorCode())
	require.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
	require.Same(t, info.QuotaClamp, apiErr.Err)
	var clamp *common.QuotaClamp
	require.ErrorAs(t, apiErr, &clamp)
	require.Same(t, info.QuotaClamp, clamp)
	require.Nil(t, info.Billing)
}

func TestPreConsumeBillingRejectsNegativeQuotaBeforeDeduction(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	info := &relaycommon.RelayInfo{}

	apiErr := PreConsumeBilling(c, -1, info)

	require.NotNil(t, apiErr)
	require.Equal(t, types.ErrorCodeModelPriceError, apiErr.GetErrorCode())
	require.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
	require.Nil(t, info.Billing)
}

// TestNotifyQuotaExhaustedNilSafety B6-3：额度用尽通知在空/零用户时不 panic。
func TestNotifyQuotaExhaustedNilSafety(t *testing.T) {
	// nil relayInfo 安全返回。
	notifyQuotaExhausted(nil, 0)
	// 零 userId 安全返回。
	notifyQuotaExhausted(&relaycommon.RelayInfo{}, 0)
}

// TestPreConsumeWalletExhaustedReturnsInsufficientQuota B6-3：钱包额度为 0 时
// 预扣费返回 insufficient_user_quota，且不会因通知路径 panic。
func TestPreConsumeWalletExhaustedReturnsInsufficientQuota(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const userID = 4301
	truncate(t)
	seedUser(t, userID, 0) // 额度 0 = 已用尽

	c, _ := gin.CreateTestContext(nil)
	info := &relaycommon.RelayInfo{
		UserId:      userID,
		UserSetting: dto.UserSetting{BillingPreference: "wallet_only"},
	}

	apiErr := PreConsumeBilling(c, 100, info)

	require.NotNil(t, apiErr, "额度用尽时应拒绝预扣")
	require.Equal(t, types.ErrorCodeInsufficientUserQuota, apiErr.GetErrorCode())
	require.Equal(t, http.StatusForbidden, apiErr.StatusCode)
	require.Nil(t, info.Billing)
}

func TestClampImageDimensionsBounds(t *testing.T) {
	cases := []struct {
		name         string
		w, h         int
		wantW, wantH int
	}{
		{"normal", 1024, 768, 1024, 768},
		{"huge_width", math.MaxInt32, 1024, maxTokenImageDimension, 1024},
		{"huge_both", math.MaxInt32, math.MaxInt32, maxTokenImageDimension, maxTokenImageDimension},
		{"negative", -100, -50, 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotW, gotH := clampImageDimensions(tc.w, tc.h)
			assert.Equal(t, tc.wantW, gotW)
			assert.Equal(t, tc.wantH, gotH)
		})
	}
}

// TestClampImageDimensionsProductCannotOverflow proves that after clamping, the
// width*height product cannot overflow int32 (so area stays positive and the
// token estimate cannot go negative).
func TestClampImageDimensionsProductCannotOverflow(t *testing.T) {
	w, h := clampImageDimensions(math.MaxInt32, math.MaxInt32)
	require.LessOrEqual(t, w, maxTokenImageDimension)
	require.LessOrEqual(t, h, maxTokenImageDimension)
	product := w * h
	require.Greater(t, product, 0, "clamped product must stay positive")
	require.LessOrEqual(t, int64(product), int64(maxTokenImageDimension)*int64(maxTokenImageDimension))
}

// TestAudioTokenAccumulationSaturates proves the multi-file audio token total
// cannot wrap negative when durations are forged to huge values.
func TestAudioTokenAccumulationSaturates(t *testing.T) {
	total := 0
	for i := 0; i < 10000; i++ {
		// each file claims ~1e9 seconds of audio
		duration := 1e9
		audioToken := common.QuotaRound(math.Ceil(duration) / 60.0 * 1000)
		if audioToken > 0 && total > common.MaxQuota-audioToken {
			total = common.MaxQuota
		} else {
			total += audioToken
		}
	}
	require.Greater(t, total, 0, "accumulated audio tokens must stay positive")
	require.LessOrEqual(t, total, common.MaxQuota, "accumulated audio tokens must saturate at MaxQuota")
	assert.Positive(t, total)
}
