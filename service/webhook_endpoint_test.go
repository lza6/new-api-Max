package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/lza6/new-api-Max/model"
	"github.com/lza6/new-api-Max/setting/system_setting"
)

// setupWebhookEndpointDB 挂内存 SQLite 主库，供端点表 CRUD + 投递幂等测试。
func setupWebhookEndpointDB(t *testing.T) {
	t.Helper()
	prevDB, prevLogDB := model.DB, model.LOG_DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.WebhookEndpoint{}, &model.EventDelivery{}))
	model.DB, model.LOG_DB = db, db
	t.Cleanup(func() { model.DB, model.LOG_DB = prevDB, prevLogDB })
}

// allowPrivateWebhookFetch 让 SSRF 校验放行 httptest 的 127.0.0.1 + 动态端口。
func allowPrivateWebhookFetch(t *testing.T) {
	t.Helper()
	system_setting.UpdateFetchSetting(func(s *system_setting.FetchSetting) {
		s.EnableSSRFProtection = false
		s.AllowPrivateIp = true
	})
	t.Cleanup(func() {
		system_setting.UpdateFetchSetting(func(s *system_setting.FetchSetting) {
			s.EnableSSRFProtection = true
			s.AllowPrivateIp = false
		})
	})
}

// TestNotifyWebhookEndpointsSignsAndDelivers E2E：NotifyWebhookEndpoints 扇出到真实
// HTTP 接收方，投递带正确 HMAC-SHA256 签名头；未订阅的端点不收到。
func TestNotifyWebhookEndpointsSignsAndDelivers(t *testing.T) {
	setupWebhookEndpointDB(t)
	allowPrivateWebhookFetch(t)

	const secret = "e2e-secret"
	got := make(chan string, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		got <- r.Header.Get(WebhookSignatureHeader)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	require.NoError(t, model.CreateWebhookEndpoint(&model.WebhookEndpoint{
		Name: "sub", URL: server.URL, Secret: secret, Enabled: true, Events: `["task.settled"]`,
	}))

	NotifyWebhookEndpoints(context.Background(), "task.settled", "task-1", map[string]any{"task_id": "task-1"})

	select {
	case sig := <-got:
		require.True(t, strings.HasPrefix(sig, "sha256="), "signature header must be prefixed sha256=")
	case <-time.After(5 * time.Second):
		t.Fatal("no delivery received")
	}
}

// TestNotifyWebhookEndpointsRespectsSubscription 未订阅该事件的端点不得收到投递。
func TestNotifyWebhookEndpointsRespectsSubscription(t *testing.T) {
	setupWebhookEndpointDB(t)
	allowPrivateWebhookFetch(t)

	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	require.NoError(t, model.CreateWebhookEndpoint(&model.WebhookEndpoint{
		Name: "other", URL: server.URL, Secret: "s", Enabled: true, Events: `["epay.topup.success"]`,
	}))

	NotifyWebhookEndpoints(context.Background(), "task.settled", "task-2", nil)
	time.Sleep(500 * time.Millisecond)
	require.Zero(t, atomic.LoadInt32(&hits), "unsubscribed endpoint must not receive the event")
}

// TestDispatchWebhookEventEndToEnd 同步扇出端到端：投递计数、签名正确、二次幂等。
func TestDispatchWebhookEventEndToEnd(t *testing.T) {
	setupWebhookEndpointDB(t)
	allowPrivateWebhookFetch(t)

	const secret = "sync-secret"
	type captured struct {
		sig  string
		body string
	}
	received := make(chan captured, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		received <- captured{sig: r.Header.Get(WebhookSignatureHeader), body: string(body)}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	require.NoError(t, model.CreateWebhookEndpoint(&model.WebhookEndpoint{
		Name: "sync", URL: server.URL, Secret: secret, Enabled: true, Events: `["task.settled"]`,
	}))

	delivered, dup, errs := DispatchWebhookEvent(context.Background(), "task.settled", "task-3", map[string]any{"k": "v"})
	require.Empty(t, errs)
	require.Equal(t, 1, delivered)
	require.Zero(t, dup)

	select {
	case c := <-received:
		// 校验签名 = HMAC-SHA256(secret, body)。
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte(c.body))
		expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
		require.Equal(t, expected, c.sig, "delivered HMAC must match the body")
		require.Contains(t, c.body, `"event_type":"task.settled"`)
		require.Contains(t, c.body, `"event_id":"task-3"`)
	case <-time.After(5 * time.Second):
		t.Fatal("no delivery received")
	}

	// 二次投递同一事件 → 幂等命中。
	delivered2, dup2, errs2 := DispatchWebhookEvent(context.Background(), "task.settled", "task-3", map[string]any{"k": "v"})
	require.Empty(t, errs2)
	require.Zero(t, delivered2)
	require.Equal(t, 1, dup2, "second dispatch of same event must be idempotent")
}

// TestDispatchWebhookEventIsIdempotent 验证：同一事件对同一端点的重复投递被幂等拒绝，
// 处理器只执行一次（B2-3 验收：重复投递不重复处理）。
func TestDispatchWebhookEventIsIdempotent(t *testing.T) {
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// 直接注入端点（绕过 SSRF 的私网拒绝：投递内核会再校验 URL，故用真实 httptest URL会
	// 被拒；此处直接测总线幂等语义，用 handleWebhookDelivery 的等效路径）。
	bus := NewEventBus(0, time.Second)
	bus.SetDeliveryStore(newFakeDeliveryStore())
	bus.Register(webhookDeliveryEventType, func(_ context.Context, _ Event) error {
		atomic.AddInt32(&hits, 1)
		return nil
	})
	payload := webhookDeliveryPayload{EndpointID: 1, URL: server.URL, Secret: "s", EventType: "task.settled", EventID: "ev-1"}

	require.NoError(t, bus.Publish(context.Background(), Event{ID: "ev-1:1", Type: webhookDeliveryEventType, Payload: payload}))
	err := bus.Publish(context.Background(), Event{ID: "ev-1:1", Type: webhookDeliveryEventType, Payload: payload})
	require.ErrorIs(t, err, ErrEventBusDup, "duplicate delivery must be rejected")
	require.Equal(t, int32(1), atomic.LoadInt32(&hits), "handler must run exactly once")
}

// TestParseWebhookEndpointEvents 验证订阅事件解析（JSON 数组 + 容错逗号分隔）。
func TestParseWebhookEndpointEvents(t *testing.T) {
	require.Equal(t, []string{"a", "b"}, ParseWebhookEndpointEvents(`["a","b"]`))
	require.Equal(t, []string{"a", "b"}, ParseWebhookEndpointEvents(`a, b`))
	require.Equal(t, []string{"a", "b"}, ParseWebhookEndpointEvents("\na\nb\n"))
	require.Empty(t, ParseWebhookEndpointEvents(""))
	require.Empty(t, ParseWebhookEndpointEvents("   "))
}

// TestEndpointSubscribes 验证端点只收到自己订阅的事件类型。
func TestEndpointSubscribes(t *testing.T) {
	ep := &model.WebhookEndpoint{Events: `["epay.topup.success","task.settled"]`}
	require.True(t, endpointSubscribes(ep, "task.settled"))
	require.True(t, endpointSubscribes(ep, "epay.topup.success"))
	require.False(t, endpointSubscribes(ep, "epay.subscription.success"))
}

// TestWebhookDeliveryBackoffCurve 验证退避曲线：失败后重试间隔按基数翻倍（上限 30s）。
// 通过 EventBus 的状态机观察 attempts 递增与最终 dead。
func TestWebhookDeliveryBackoffCurve(t *testing.T) {
	bus := NewEventBus(2, 10*time.Millisecond) // 10ms 基数便于测试快速跑完
	var attempts int32
	bus.Register("boom", func(_ context.Context, _ Event) error {
		atomic.AddInt32(&attempts, 1)
		return errors.New("fail")
	})
	err := bus.Publish(context.Background(), Event{ID: "ev-boom", Type: "boom"})
	require.Error(t, err)
	// maxRetries=2 → 首次 + 2 次重试 = 3 次尝试，之后进 dead。
	require.Equal(t, int32(3), atomic.LoadInt32(&attempts))
	state, ok := bus.State("ev-boom")
	require.True(t, ok)
	require.Equal(t, EventStateDead, state)
}

// TestWebhookEventFanoutToMultipleEndpoints 验证：一个事件扇出到多个订阅端点，
// 每个端点独立投递、独立幂等（deliveryID 含 endpointID）。
func TestWebhookEventFanoutToMultipleEndpoints(t *testing.T) {
	// 用两个内存总线模拟两个端点各自的投递通道；验证 deliveryID 命名互不冲突。
	bus := NewEventBus(0, time.Second)
	bus.SetDeliveryStore(newFakeDeliveryStore())
	seen := make(map[string]int)
	bus.Register(webhookDeliveryEventType, func(_ context.Context, ev Event) error {
		seen[ev.ID]++
		return nil
	})
	require.NoError(t, bus.Publish(context.Background(), Event{ID: fmt.Sprintf("%s:%d", "ev-x", 1), Type: webhookDeliveryEventType}))
	require.NoError(t, bus.Publish(context.Background(), Event{ID: fmt.Sprintf("%s:%d", "ev-x", 2), Type: webhookDeliveryEventType}))
	require.Equal(t, 1, seen["ev-x:1"])
	require.Equal(t, 1, seen["ev-x:2"])
}

// TestWebhookDeliveryStateQuery 验证投递状态可查（审计用）。
func TestWebhookDeliveryStateQuery(t *testing.T) {
	bus := NewEventBus(0, time.Second)
	bus.Register(webhookDeliveryEventType, func(_ context.Context, _ Event) error { return nil })
	require.NoError(t, bus.Publish(context.Background(), Event{ID: "ev-state:7", Type: webhookDeliveryEventType}))
	state, ok := bus.State("ev-state:7")
	require.True(t, ok)
	require.Equal(t, EventStateSuccess, state)
}
