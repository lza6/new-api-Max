package service

import (
	"context"
	"errors"
	"time"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/model"
)

// P2-2 支付 webhook 事件归一：易支付充值回调 → 事件总线 → 处理器做账本更新。
//
// 幂等设计（验收：同一 webhook 推送一次、重放一次账本一致）：
//   - 事件 ID 固定为 "epay-topup:" + trade_no，总线持久化 (event_id, handler)
//     联合唯一：重复投递在总线层即被拒绝；
//   - 总线成功 → 已处理（alreadyDone=true）；
//   - 总线记录存在但非成功（failed/dead/pending，如上次处理中崩溃）→
//     兜底直接走 model.RechargeEpay（trade_no 行锁 + 状态校验的账本级幂等）；
//   - 首次处理失败 → 返回错误，webhook 回 "fail"，由支付网关稍后重推。
//
// maxRetries=0：webhook 请求内不重试（避免阻塞回调响应），重试交给网关重推。

const (
	// EventTypeEpayTopupSuccess 易支付充值成功事件类型。
	EventTypeEpayTopupSuccess = "epay.topup.success"
)

var (
	paymentEventBus = NewEventBus(0, time.Second)
)

func init() {
	paymentEventBus.SetDeliveryStore(NewEventDeliveryStore())
	paymentEventBus.Register(EventTypeEpayTopupSuccess, handleEpayTopupSuccess)
}

// epayTopupPayload 充值事件载荷（仅非敏感字段，不落密钥/签名）。
type epayTopupPayload struct {
	TradeNo      string `json:"trade_no"`
	ActualMethod string `json:"actual_payment_method,omitempty"`
	ClientIP     string `json:"client_ip,omitempty"`
}

func handleEpayTopupSuccess(ctx context.Context, ev Event) error {
	payload, err := decodeEpayTopupPayload(ev.Payload)
	if err != nil {
		return err
	}
	_, err = model.RechargeEpay(payload.TradeNo, payload.ActualMethod, payload.ClientIP)
	return err
}

func decodeEpayTopupPayload(raw any) (epayTopupPayload, error) {
	data, err := common.Marshal(raw)
	if err != nil {
		return epayTopupPayload{}, err
	}
	var payload epayTopupPayload
	if err := common.Unmarshal(data, &payload); err != nil {
		return epayTopupPayload{}, err
	}
	return payload, nil
}

// DispatchEpayTopupEvent 把易支付充值回调归一为事件投递。
// alreadyDone=true 表示该 trade_no 已入账（幂等命中），调用方无需再处理。
func DispatchEpayTopupEvent(ctx context.Context, tradeNo, actualPaymentMethod, clientIP string) (bool, error) {
	ev := Event{
		ID:   "epay-topup:" + tradeNo,
		Type: EventTypeEpayTopupSuccess,
		Payload: epayTopupPayload{
			TradeNo:      tradeNo,
			ActualMethod: actualPaymentMethod,
			ClientIP:     clientIP,
		},
	}
	err := paymentEventBus.Publish(ctx, ev)
	if err == nil {
		return false, nil
	}
	if errors.Is(err, ErrEventBusDup) {
		if state, ok := paymentEventBus.State(ev.ID); ok && state == EventStateSuccess {
			return true, nil
		}
		// 已投递但非成功：账本级幂等兜底（行锁 + 状态校验）。
		return model.RechargeEpay(tradeNo, actualPaymentMethod, clientIP)
	}
	return false, err
}
