package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/model"
)

// B2-3 通用 Webhook 多端点子系统。
//
// 架构（复用既有底座，不重复造轮子）：
//   - 配置：model.WebhookEndpoint 表（多端点、每端点独立密钥/事件订阅/开关）。
//   - 投递：EventBus（service.event_bus.go）——幂等（event_id+handler 联合唯一，
//     持久化在 event_deliveries 表，跨重启/跨实例只处理一次）+ 指数退避重试 +
//     状态机 pending→success|failed(retry)→dead。
//   - 每个「事件 × 端点」是一条独立投递：event_id = "<原始事件ID>:<endpointID>"，
//     事件类型固定为 webhookDeliveryEventType；handler 名 = 事件类型。因此同一事件
//     发给 N 个端点互不干扰，每个端点各自的成功/失败/重试/终态独立（幂等键含端点 ID）。
//
// 与 operation_setting.WebhookSetting（单 URL 全局配置，T15-B）并存：两者可同时生效。
// 单 URL 走旧路径（NotifyWebhooks）；本子系统走多端点表。

const (
	// webhookDeliveryEventType 多端点投递在事件总线中的固定事件类型（路由 + 幂等键第二列）。
	webhookDeliveryEventType = "webhook.delivery"
	// webhookDeliveryMaxRetries 单端点投递最大重试次数（超过进 dead）。
	webhookDeliveryMaxRetries = 4
	// webhookDeliveryBaseDelay 退避基数（EventBus 内逐次翻倍，上限 30s）。
	webhookDeliveryBaseDelay = time.Second
)

// webhookDeliveryPayload 投递任务的运行时数据（不落库；仅进程内事件载荷）。
// Secret 仅在内存中流转，不写入 event_deliveries（该表只记 event_id/handler/state）。
type webhookDeliveryPayload struct {
	EndpointID int64  `json:"endpoint_id"`
	URL        string `json:"url"`
	Secret     string `json:"secret"`
	EventType  string `json:"event_type"`
	EventID    string `json:"event_id"`
	Data       any    `json:"data"`
}

// webhookEndpointEventBus 多端点投递总线。持久化 store 保证幂等。
var webhookEndpointEventBus = NewEventBus(webhookDeliveryMaxRetries, webhookDeliveryBaseDelay)

func init() {
	webhookEndpointEventBus.SetDeliveryStore(NewEventDeliveryStore())
	webhookEndpointEventBus.Register(webhookDeliveryEventType, handleWebhookDelivery)
}

// handleWebhookDelivery 单条「事件 × 端点」的投递处理器：组装信封 + HMAC 签名发送。
func handleWebhookDelivery(_ context.Context, ev Event) error {
	data, err := common.Marshal(ev.Payload)
	if err != nil {
		return err
	}
	var p webhookDeliveryPayload
	if err := common.Unmarshal(data, &p); err != nil {
		return err
	}
	body, err := common.Marshal(map[string]any{
		"event_type": p.EventType,
		"event_id":   p.EventID,
		"payload":    p.Data,
		"timestamp":  time.Now().Unix(),
	})
	if err != nil {
		return err
	}
	return SendSignedEventWebhook(p.URL, p.Secret, body)
}

// ParseWebhookEndpointEvents 解析端点的订阅事件列表（JSON 数组字符串；容错逗号/空白分隔）。
func ParseWebhookEndpointEvents(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var events []string
	if err := common.UnmarshalJsonStr(raw, &events); err == nil {
		return normalizeEventList(events)
	}
	return normalizeEventList(strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\n' || r == '\t'
	}))
}

func normalizeEventList(in []string) []string {
	out := make([]string, 0, len(in))
	for _, it := range in {
		if s := strings.TrimSpace(it); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// NotifyWebhookEndpoints 向所有订阅该事件的已启用端点异步扇出（fire-and-forget，
// 与旧 NotifyWebhooks 语义一致：调用方业务不受投递结果影响）。
func NotifyWebhookEndpoints(ctx context.Context, eventType, eventID string, payload any) {
	endpoints, err := model.ListEnabledWebhookEndpoints()
	if err != nil {
		common.SysError("webhook endpoints: list failed: " + err.Error())
		return
	}
	for _, endpoint := range endpoints {
		if !endpointSubscribes(endpoint, eventType) {
			continue
		}
		if err := validateWebhookURL(endpoint.URL); err != nil {
			common.SysError(fmt.Sprintf("webhook endpoint %d: %v", endpoint.Id, err))
			continue
		}
		ep := endpoint
		gopool.Go(func() {
			dispatchWebhookEndpoint(context.Background(), ep, eventType, eventID, payload)
		})
	}
}

// DispatchWebhookEvent 同步扇出（测试与显式调用）；返回 已投递/幂等命中/错误 计数。
func DispatchWebhookEvent(ctx context.Context, eventType, eventID string, payload any) (delivered, alreadyDone int, errs []error) {
	if eventID == "" {
		eventID = common.NewRequestId()
	}
	endpoints, err := model.ListEnabledWebhookEndpoints()
	if err != nil {
		return 0, 0, []error{fmt.Errorf("list webhook endpoints: %w", err)}
	}
	for _, endpoint := range endpoints {
		if !endpointSubscribes(endpoint, eventType) {
			continue
		}
		if err := validateWebhookURL(endpoint.URL); err != nil {
			errs = append(errs, fmt.Errorf("endpoint %d: %w", endpoint.Id, err))
			continue
		}
		d, dup, dErr := dispatchWebhookEndpointSync(ctx, endpoint, eventType, eventID, payload)
		delivered += d
		if dup {
			alreadyDone++
		}
		if dErr != nil {
			errs = append(errs, dErr)
		}
	}
	return delivered, alreadyDone, errs
}

// dispatchWebhookEndpoint 异步路径：内部同步执行（含退避重试）。
func dispatchWebhookEndpoint(ctx context.Context, endpoint *model.WebhookEndpoint, eventType, eventID string, payload any) {
	_, _, err := dispatchWebhookEndpointSync(ctx, endpoint, eventType, eventID, payload)
	if err != nil {
		common.SysError(fmt.Sprintf("webhook endpoint %d delivery failed: %v", endpoint.Id, err))
	}
}

// dispatchWebhookEndpointSync 投递到单个端点，返回 (投递数, 是否幂等命中, 错误)。
func dispatchWebhookEndpointSync(ctx context.Context, endpoint *model.WebhookEndpoint, eventType, eventID string, payload any) (int, bool, error) {
	deliveryID := fmt.Sprintf("%s:%d", eventID, endpoint.Id)
	ev := Event{
		ID:   deliveryID,
		Type: webhookDeliveryEventType,
		Payload: webhookDeliveryPayload{
			EndpointID: endpoint.Id,
			URL:        endpoint.URL,
			Secret:     endpoint.Secret,
			EventType:  eventType,
			EventID:    eventID,
			Data:       payload,
		},
	}
	err := webhookEndpointEventBus.Publish(ctx, ev)
	switch {
	case err == nil:
		return 1, false, nil
	case errors.Is(err, ErrEventBusDup):
		return 0, true, nil
	default:
		return 0, false, fmt.Errorf("endpoint %d: %w", endpoint.Id, err)
	}
}

// WebhookDeliveryState 返回某「事件 × 端点」的投递状态（审计/管理端查询用）。
func WebhookDeliveryState(eventID string, endpointID int64) (EventState, bool) {
	return webhookEndpointEventBus.State(fmt.Sprintf("%s:%d", eventID, endpointID))
}

func endpointSubscribes(endpoint *model.WebhookEndpoint, eventType string) bool {
	for _, it := range ParseWebhookEndpointEvents(endpoint.Events) {
		if it == eventType {
			return true
		}
	}
	return false
}
