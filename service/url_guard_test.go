package service

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func clearSSRFCache(t *testing.T) {
	t.Helper()
	ssrfCacheMu.Lock()
	ssrfCache = map[string]ssrfCacheEntry{}
	ssrfCacheMu.Unlock()
	t.Cleanup(func() {
		ssrfCacheMu.Lock()
		ssrfCache = map[string]ssrfCacheEntry{}
		ssrfCacheMu.Unlock()
	})
}

func setAllowlistForTest(t *testing.T, entries []string) {
	t.Helper()
	backup := ssrfAllowlisted
	ssrfAllowlisted = map[string]bool{}
	for _, e := range entries {
		ssrfAllowlisted[e] = true
	}
	t.Cleanup(func() { ssrfAllowlisted = backup })
}

// mockResolverForSSRF 替换 net.DefaultResolver，按域名返回固定 IP 列表。
// 完全不依赖网络 DNS。
func mockResolverForSSRF(t *testing.T, answers map[string][]string) {
	t.Helper()
	original := net.DefaultResolver
	net.DefaultResolver = &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			return nil, fmt.Errorf("mock resolver: use canned answers")
		},
	}
	// LookupIPAddr 走 Dial 失败无法给答案，因此直接替换 lookupSSRF 的解析层不现实；
	// 这里通过包装 LookupIPAddr 不可行（结构体方法），改为让 Dial 返回一个自定义 Conn 太重。
	// 简化：mock 用 LookupIPAddr 的 context 短路不可行 —— 采用包内 hook。
	previousHook := ssrfLookupHook
	ssrfLookupHook = func(ctx context.Context, host string) ([]net.IPAddr, error) {
		if records, ok := answers[host]; ok {
			out := make([]net.IPAddr, 0, len(records))
			for _, r := range records {
				out = append(out, net.IPAddr{IP: net.ParseIP(r)})
			}
			return out, nil
		}
		return nil, fmt.Errorf("no such host %s", host)
	}
	t.Cleanup(func() {
		net.DefaultResolver = original
		ssrfLookupHook = previousHook
	})
}

func TestValidateChannelURL(t *testing.T) {
	clearSSRFCache(t)
	mockResolverForSSRF(t, map[string][]string{
		"api.openai.com":   {"93.184.216.34"},
		"api.example.com":  {"93.184.216.34"},
		"rebind.evil.test": {"93.184.216.34", "10.1.2.3"}, // rebinding：公网+私网双记录
		"inner.evil.test":  {"10.1.2.3"},
		"v6.evil.test":     {"2001:db8::1", "fc00::1"},
		"nodns.evil.test":  nil,
	})

	testCases := []struct {
		name      string
		rawURL    string
		wantError bool
	}{
		// scheme 校验
		{name: "https public domain ok", rawURL: "https://api.openai.com/v1", wantError: false},
		{name: "http scheme allowed", rawURL: "http://api.example.com/v1", wantError: false},
		{name: "file scheme rejected", rawURL: "file:///etc/passwd", wantError: true},
		{name: "gopher scheme rejected", rawURL: "gopher://evil.example", wantError: true},
		{name: "invalid url rejected", rawURL: "://broken", wantError: true},
		{name: "empty host rejected", rawURL: "https:///path", wantError: true},
		// 字面 IPv4
		{name: "literal loopback rejected", rawURL: "http://127.0.0.1:3000", wantError: true},
		{name: "literal rfc1918 rejected", rawURL: "http://10.1.2.3/v1", wantError: true},
		{name: "literal 172.16 rejected", rawURL: "http://172.16.0.9/v1", wantError: true},
		{name: "literal 192.168 rejected", rawURL: "http://192.168.1.1/v1", wantError: true},
		{name: "literal metadata rejected", rawURL: "http://169.254.169.254/latest/meta-data", wantError: true},
		{name: "literal 0.x rejected", rawURL: "http://0.0.0.0/v1", wantError: true},
		// 字面 IPv6
		{name: "literal ipv6 loopback rejected", rawURL: "http://[::1]:3000", wantError: true},
		{name: "literal ipv6 ULA rejected", rawURL: "http://[fc00::1]/v1", wantError: true},
		{name: "literal ipv6 link-local rejected", rawURL: "http://[fe80::1]/v1", wantError: true},
		// DNS 二次解析
		{name: "dns rebind (private record) rejected", rawURL: "http://rebind.evil.test/v1", wantError: true},
		{name: "dns all private rejected", rawURL: "http://inner.evil.test/v1", wantError: true},
		{name: "dns ipv6 ULA rejected", rawURL: "http://v6.evil.test/v1", wantError: true},
		{name: "dns failure rejected", rawURL: "http://nodns.evil.test/v1", wantError: true},
		// allowlist：域名可放行，字面私网 IP 不可放行
		{name: "allowlisted domain skips dns", rawURL: "https://self-hosted.internal.example/v1", wantError: false},
		{name: "allowlisted cannot unblock literal private ip", rawURL: "http://127.0.0.1:3000", wantError: true},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			setAllowlistForTest(t, []string{"internal.example"})
			err := ValidateChannelURL(tc.rawURL)
			if tc.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateChannelURLLocalhostResolution(t *testing.T) {
	clearSSRFCache(t)
	mockResolverForSSRF(t, map[string][]string{
		"localhost": {"127.0.0.1", "::1"},
	})
	// localhost 解析到环回 → 拒绝（真实解析路径验证）。
	err := ValidateChannelURL("http://localhost:3000/v1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ssrf guard")
}

func TestValidateChannelURLAllowlistSuffix(t *testing.T) {
	clearSSRFCache(t)
	setAllowlistForTest(t, []string{"example.com"})
	// example.com 的子域命中 allowlist → 跳过 DNS（无论解析结果）。
	assert.NoError(t, ValidateChannelURL("https://api.sub.example.com/v1"))
	// 其它域名不受 allowlist 影响。
	mockResolverForSSRF(t, map[string][]string{
		"localhost": {"127.0.0.1"},
	})
	assert.Error(t, ValidateChannelURL("http://localhost:1"))
}

func TestValidateChannelURLCache(t *testing.T) {
	clearSSRFCache(t)
	// 手动写入 deny 缓存条目 → 命中缓存直接拒绝（不触发解析）。
	ssrfCacheMu.Lock()
	ssrfCache["cached-deny.example"] = ssrfCacheEntry{allowed: false, reason: "unit-test deny", expiresAt: time.Now().Add(time.Minute)}
	ssrfCacheMu.Unlock()
	err := ValidateChannelURL("http://cached-deny.example/v1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unit-test deny")
}

// TestSSRFGuardDisabledSwitch T1：SSRF_GUARD_DISABLED / SetSSRFGuardDisabled
// 仅测试/基准用；默认关闭时私网字面 IP 仍被拒（生产安全不变）。
func TestSSRFGuardDisabledSwitch(t *testing.T) {
	prev := ssrfGuardSkipped
	t.Cleanup(func() { ssrfGuardSkipped = prev })

	ssrfGuardSkipped = false
	err := ValidateChannelURL("http://127.0.0.1:18080/v1")
	require.Error(t, err, "default must reject loopback literal IP")

	SetSSRFGuardDisabled(true)
	require.NoError(t, ValidateChannelURL("http://127.0.0.1:18080/v1"), "disabled switch must allow loopback for local bench")

	SetSSRFGuardDisabled(false)
	err = ValidateChannelURL("http://127.0.0.1:18080/v1")
	require.Error(t, err, "re-enable must reject loopback again")
}
