package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/setting/operation_setting"
)

// Webhook 通知：包含两类。
// 1) 用户级通知（NotifyTypeWebhook，user_notify.go）：用户自配 URL + secret，同步发送 dto.Notify。
// 2) 全局事件 Webhook（T15-B）：operation_setting 配置，订阅业务事件，异步签名通知。
// 两者共用 SendWebhookNotify 作为「SSRF 校验 + HMAC 签名」的发送内核。

// WebhookSignatureHeader 请求头：X-New-API-Webhook-Signature: sha256=<hex hmac>。
const WebhookSignatureHeader = "X-New-API-Webhook-Signature"

const (
	webhookTimeout  = 10 * time.Second
	webhookMaxRetry = 3
	webhookDedupWin = 60 * time.Second
	webhookDedupCap = 1000
)

var webhookDedup = struct {
	mu sync.Mutex
	m  map[string]time.Time
}{m: make(map[string]time.Time)}

// SendWebhookNotify 向用户配置的 webhook URL 发送签名 POST。
// 响应体作为 body 发送，签名覆盖 body 全量字节；失败返回 error（由调用方决定重试/记录）。
// 任意用户可控 URL 必须走 SSRF 校验（ValidateSSRFProtectedFetchURL）。
func SendWebhookNotify(webhookURL string, webhookSecret string, body []byte) error {
	if webhookURL == "" {
		return errors.New("webhook url is empty")
	}
	if err := ValidateSSRFProtectedFetchURL(webhookURL); err != nil {
		return fmt.Errorf("webhook url rejected: %w", err)
	}
	sig := webhookHMAC(webhookSecret, body)
	ctx, cancel := context.WithTimeout(context.Background(), webhookTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("webhook build request failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(WebhookSignatureHeader, "sha256="+sig)
	client := GetSSRFProtectedHTTPClient()
	if client == nil {
		client = &http.Client{Timeout: webhookTimeout}
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("webhook request returned status %d", resp.StatusCode)
	}
	return nil
}

// NotifyWebhooks 向全局配置的 webhook URL 发送事件通知。
// 未启用 / 未订阅 / URL 非法（非 https 或 SSRF 拒绝）时静默跳过（仅日志）。
// 返回前不发起网络请求（异步派发），调用方业务不受影响。
func NotifyWebhooks(ctx context.Context, eventType, eventID string, payload any) {
	setting := operation_setting.GetWebhookSetting()
	if !setting.Enabled || setting.URL == "" || setting.Secret == "" {
		return
	}
	if !operation_setting.IsWebhookEventSubscribed(eventType) {
		return
	}
	if err := validateWebhookURL(setting.URL); err != nil {
		common.SysError("webhook: " + err.Error())
		return
	}
	key := eventType + ":" + eventID
	if !acquireWebhookDedup(key) {
		return
	}
	gopool.Go(func() {
		dispatchWebhook(setting.URL, setting.Secret, eventType, eventID, payload)
	})
}

// validateWebhookURL 校验全局 webhook URL：仅允许 http/https，且复用 SSRF 校验
// （私网/环回/云元数据拒绝）。https 为强烈推荐（防窃听/防重放），但不强制——
// 安全边界在 SSRF 守卫（拒绝私网目标）与 HMAC 签名（完整性）。
func validateWebhookURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("webhook: invalid url: %w", err)
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return errors.New("webhook: url must be http(s)")
	}
	if err := ValidateSSRFProtectedFetchURL(raw); err != nil {
		return fmt.Errorf("webhook: %w", err)
	}
	return nil
}

// acquireWebhookDedup 去重窗口内同一 (eventType, eventID) 只允许一次通知。
func acquireWebhookDedup(key string) bool {
	webhookDedup.mu.Lock()
	defer webhookDedup.mu.Unlock()
	now := time.Now()
	if t, ok := webhookDedup.m[key]; ok && now.Sub(t) < webhookDedupWin {
		return false
	}
	webhookDedup.m[key] = now
	if len(webhookDedup.m) > webhookDedupCap {
		for k, t := range webhookDedup.m {
			if now.Sub(t) >= webhookDedupWin {
				delete(webhookDedup.m, k)
			}
		}
	}
	return true
}

// dispatchWebhook 组装事件信封并发送，最多重试 webhookMaxRetry 次。
func dispatchWebhook(rawURL, secret, eventType, eventID string, payload any) {
	body, err := common.Marshal(map[string]any{
		"event_type": eventType,
		"event_id":   eventID,
		"payload":    payload,
		"timestamp":  time.Now().Unix(),
	})
	if err != nil {
		common.SysError("webhook: marshal payload failed: " + err.Error())
		return
	}
	for attempt := 1; attempt <= webhookMaxRetry; attempt++ {
		err := SendWebhookNotify(rawURL, secret, body)
		if err == nil {
			return
		}
		common.SysError(fmt.Sprintf("webhook: notify %s event_id=%s attempt=%d failed: %v", eventType, eventID, attempt, err))
		if attempt < webhookMaxRetry {
			time.Sleep(time.Duration(attempt) * time.Second)
		}
	}
}

func webhookHMAC(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}