package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/lza6/new-api-Max/setting/operation_setting"
	"github.com/lza6/new-api-Max/setting/system_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func disableSSRFProtection(t *testing.T) {
	t.Helper()
	fetchSetting := system_setting.GetFetchSetting()
	original := *fetchSetting
	t.Cleanup(func() { *fetchSetting = original })
	fetchSetting.EnableSSRFProtection = false
}

func hmacHex(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func TestSendSignedEventWebhookSignsAndPosts(t *testing.T) {
	disableSSRFProtection(t)
	received := make(chan []byte, 1)
	var signature string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		signature = r.Header.Get(WebhookSignatureHeader)
		body, _ := io.ReadAll(r.Body)
		received <- body
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	body := []byte(`{"event_type":"epay.topup.success","event_id":"T1"}`)
	require.NoError(t, SendSignedEventWebhook(srv.URL, "webhook-secret-abc", body))
	select {
	case got := <-received:
		assert.Equal(t, body, got)
	case <-time.After(3 * time.Second):
		t.Fatal("webhook request was not sent")
	}
	assert.Equal(t, "sha256="+hmacHex("webhook-secret-abc", body), signature)
}

func TestSendSignedEventWebhookRejectsPrivateURLUnderSSRF(t *testing.T) {
	configureSSRFTestFetchSetting(t)
	err := SendSignedEventWebhook("http://127.0.0.1:1/notify", "secret", []byte(`{}`))
	require.Error(t, err)
}

func TestSendSignedEventWebhookFailsOnNon2xx(t *testing.T) {
	disableSSRFProtection(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	err := SendSignedEventWebhook(srv.URL, "secret", []byte(`{}`))
	require.Error(t, err)
}

func TestValidateWebhookURLRequiresHTTPSScheme(t *testing.T) {
	configureSSRFTestFetchSetting(t)
	require.Error(t, validateWebhookURL("ftp://example.com/hook"))
	require.Error(t, validateWebhookURL("file:///etc/passwd"))
	require.NoError(t, validateWebhookURL("https://example.com/hook"))
}

func TestNotifyWebhooksDisabledOrUnsubscribedDoesNotDispatch(t *testing.T) {
	disableSSRFProtection(t)
	setting := operation_setting.GetWebhookSetting()
	original := *setting
	t.Cleanup(func() { *setting = original })

	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// 未启用：任何事件都不外呼。
	*setting = operation_setting.WebhookSetting{Enabled: false, URL: srv.URL, Secret: "s", Events: []string{operation_setting.WebhookEventEpayTopupSuccess}}
	NotifyWebhooks(t.Context(), operation_setting.WebhookEventEpayTopupSuccess, "T1", map[string]any{"x": 1})
	time.Sleep(200 * time.Millisecond)
	assert.Zero(t, calls)

	// 启用但事件未订阅：不外呼。
	*setting = operation_setting.WebhookSetting{Enabled: true, URL: srv.URL, Secret: "s", Events: []string{operation_setting.WebhookEventTaskSettled}}
	NotifyWebhooks(t.Context(), operation_setting.WebhookEventEpayTopupSuccess, "T2", map[string]any{"x": 1})
	time.Sleep(200 * time.Millisecond)
	assert.Zero(t, calls)
}

func TestNotifyWebhooksDispatchesSubscribedEventWithDedup(t *testing.T) {
	disableSSRFProtection(t)
	setting := operation_setting.GetWebhookSetting()
	original := *setting
	t.Cleanup(func() { *setting = original })

	received := make(chan struct{}, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- struct{}{}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	*setting = operation_setting.WebhookSetting{
		Enabled: true,
		URL:     srv.URL,
		Secret:  "s",
		Events:  []string{operation_setting.WebhookEventEpayTopupSuccess},
	}
	NotifyWebhooks(t.Context(), operation_setting.WebhookEventEpayTopupSuccess, "T1", map[string]any{"x": 1})
	// 重复通知同一事件：去重窗口内只外呼一次。
	NotifyWebhooks(t.Context(), operation_setting.WebhookEventEpayTopupSuccess, "T1", map[string]any{"x": 1})

	select {
	case <-received:
		select {
		case <-received:
			t.Fatal("dedup window should have collapsed duplicate notification")
		case <-time.After(300 * time.Millisecond):
		}
	case <-time.After(3 * time.Second):
		t.Fatal("subscribed webhook event was not dispatched")
	}
}

func TestWebhookEventSubscriptionParsing(t *testing.T) {
	original := *operation_setting.GetWebhookSetting()
	t.Cleanup(func() { operation_setting.ReplaceWebhookSetting(original) })
	operation_setting.UpdateWebhookSetting(func(setting *operation_setting.WebhookSetting) {
		setting.Events = []string{"epay.topup.success, epay.subscription.success", "task.settled"}
	})
	assert.True(t, operation_setting.IsWebhookEventSubscribed("epay.topup.success"))
	assert.True(t, operation_setting.IsWebhookEventSubscribed("task.settled"))
	assert.False(t, operation_setting.IsWebhookEventSubscribed("unknown.event"))
	operation_setting.UpdateWebhookSetting(func(setting *operation_setting.WebhookSetting) {
		setting.Events = nil
	})
}