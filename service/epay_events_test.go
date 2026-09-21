package service

import (
	"context"
	"testing"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedEpayTopup 创建一笔待入账的易支付充值订单（service 测试库）。
func seedEpayTopup(t *testing.T, userID int, tradeNo string) {
	t.Helper()
	topUp := &model.TopUp{
		UserId:          userID,
		Amount:          2,
		Money:           10.0,
		TradeNo:         tradeNo,
		PaymentMethod:   "alipay",
		PaymentProvider: model.PaymentProviderEpay,
		CreateTime:      common.GetTimestamp(),
		Status:          common.TopUpStatusPending,
	}
	require.NoError(t, model.DB.Create(topUp).Error)
}

func quotaOfUser(t *testing.T, userID int) int {
	t.Helper()
	var user model.User
	require.NoError(t, model.DB.First(&user, userID).Error)
	return user.Quota
}

// TestDispatchEpayTopupEventOnceAndReplayConsistent 验收：真实 webhook 推送
// 一次、重放一次，账本一致（不二次入账）。
func TestDispatchEpayTopupEventOnceAndReplayConsistent(t *testing.T) {
	truncate(t)
	oldQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500000
	t.Cleanup(func() { common.QuotaPerUnit = oldQuotaPerUnit })

	user := &model.User{
		Id:       901,
		Username: "epay-bus-once",
		Quota:    0,
		Status:   common.UserStatusEnabled,
	}
	require.NoError(t, model.DB.Create(user).Error)
	seedEpayTopup(t, user.Id, "EPAYBUS-ONCE")

	alreadyDone, err := DispatchEpayTopupEvent(context.Background(), "EPAYBUS-ONCE", "alipay", "127.0.0.1")
	require.NoError(t, err)
	assert.False(t, alreadyDone)
	assert.Equal(t, 2*500000, quotaOfUser(t, user.Id))

	// 重放：总线幂等命中 success → alreadyDone=true，账本不再变化。
	alreadyDone, err = DispatchEpayTopupEvent(context.Background(), "EPAYBUS-ONCE", "alipay", "127.0.0.1")
	require.NoError(t, err)
	assert.True(t, alreadyDone)
	assert.Equal(t, 2*500000, quotaOfUser(t, user.Id))

	var delivery model.EventDelivery
	require.NoError(t, model.DB.Where("event_id = ?", "epay-topup:EPAYBUS-ONCE").First(&delivery).Error)
	assert.Equal(t, "success", delivery.State)
	assert.Equal(t, "epay.topup.success", delivery.EventType)
}

// TestDispatchEpayTopupEventRecoversAfterFailedDelivery 验收：首次投递失败
// （订单不存在 → webhook 回 fail）→ 补单后重放 → 兜底账本幂等入账；
// 第三次重放 alreadyDone=true 不重复入账。
func TestDispatchEpayTopupEventRecoversAfterFailedDelivery(t *testing.T) {
	truncate(t)
	oldQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500000
	t.Cleanup(func() { common.QuotaPerUnit = oldQuotaPerUnit })

	// 首次投递：订单尚不存在 → 处理失败（webhook 将回 "fail"）。
	_, err := DispatchEpayTopupEvent(context.Background(), "EPAYBUS-RECOVER", "alipay", "127.0.0.1")
	require.ErrorIs(t, err, model.ErrTopUpNotFound)

	// 补单后重放：投递记录存在但非 success → 兜底 RechargeEpay 入账。
	user := &model.User{
		Id:       902,
		Username: "epay-bus-recover",
		Quota:    0,
		Status:   common.UserStatusEnabled,
	}
	require.NoError(t, model.DB.Create(user).Error)
	seedEpayTopup(t, user.Id, "EPAYBUS-RECOVER")

	alreadyDone, err := DispatchEpayTopupEvent(context.Background(), "EPAYBUS-RECOVER", "alipay", "127.0.0.1")
	require.NoError(t, err)
	assert.False(t, alreadyDone)
	assert.Equal(t, 2*500000, quotaOfUser(t, user.Id))

	// 第三次重放：账本幂等命中，不再入账。
	alreadyDone, err = DispatchEpayTopupEvent(context.Background(), "EPAYBUS-RECOVER", "alipay", "127.0.0.1")
	require.NoError(t, err)
	assert.True(t, alreadyDone)
	assert.Equal(t, 2*500000, quotaOfUser(t, user.Id))
}
