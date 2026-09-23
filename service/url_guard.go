/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

// B1-2 渠道 base_url SSRF 二次解析（N15 ai-scanner/guardian 证据）。
// 保存时校验不够：DNS rebinding 可在「保存合法、使用时解析到内网」窗口内生效，
// 因此在「即将发起上游请求」时再次校验目标地址。
//
// 私网段清单硬编码为不可配置（纵深防御）；URL_GUARD_ALLOWLIST 环境变量
// 仅允许追加公网域名（跳过 DNS 二次校验，用于反代自建上游/解析抖动场景），
// 私网段永远不可通过环境变量放行。

package service

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// ssrfBlockedCIDRs 私网/环回/链路本地/云元数据段，硬编码不可配置。
var ssrfBlockedCIDRs = func() []*net.IPNet {
	cidrs := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"127.0.0.0/8",
		"169.254.0.0/16", // 链路本地 + 云元数据 169.254.169.254
		"0.0.0.0/8",
		"::1/128",
		"fc00::/7", // IPv6 ULA
		"fe80::/10",
	}
	list := make([]*net.IPNet, 0, len(cidrs))
	for _, c := range cidrs {
		_, parsed, err := net.ParseCIDR(c)
		if err == nil {
			list = append(list, parsed)
		}
	}
	return list
}()

const (
	ssrfLookupTimeout = 3 * time.Second
	// ssrfCacheTTL DNS 判定缓存：同一 host 的校验结果短暂复用，避免每请求解析。
	ssrfCacheTTL = 5 * time.Minute
)

type ssrfCacheEntry struct {
	allowed   bool
	reason    string
	expiresAt time.Time
}

var (
	ssrfCacheMu     sync.RWMutex
	ssrfCache       = map[string]ssrfCacheEntry{}
	ssrfAllowlisted map[string]bool
)

func init() {
	ssrfAllowlisted = map[string]bool{}
	for _, host := range strings.Split(os.Getenv("URL_GUARD_ALLOWLIST"), ",") {
		host = strings.ToLower(strings.TrimSpace(host))
		if host != "" {
			ssrfAllowlisted[host] = true
		}
	}
	// SSRF_GUARD_DISABLED 仅供本地基准/CI 测试（默认 false，生产安全不变）。
	// 注意：这只会跳过整体守卫，私网段 allowlist 仍然不可配置（纵深防御）。
	if strings.EqualFold(os.Getenv("SSRF_GUARD_DISABLED"), "true") {
		ssrfGuardSkipped = true
	}
}

// lookupSSRF 解析 host 全部 A/AAAA 记录，任一命中私网段即拒绝。
func lookupSSRFCached(host string) (bool, string) {
	ssrfCacheMu.RLock()
	if entry, ok := ssrfCache[host]; ok && time.Now().Before(entry.expiresAt) {
		ssrfCacheMu.RUnlock()
		return entry.allowed, entry.reason
	}
	ssrfCacheMu.RUnlock()

	allowed, reason := lookupSSRF(host)
	ssrfCacheMu.Lock()
	ssrfCache[host] = ssrfCacheEntry{allowed: allowed, reason: reason, expiresAt: time.Now().Add(ssrfCacheTTL)}
	ssrfCacheMu.Unlock()
	return allowed, reason
}

// ssrfLookupHook 可替换的解析层（测试注入固定应答；生产为 nil 走真实 DNS）。
var ssrfLookupHook func(ctx context.Context, host string) ([]net.IPAddr, error)

// lookupSSRF 解析 host 全部 A/AAAA 记录，任一命中私网段即拒绝。
func lookupSSRF(host string) (bool, string) {
	ctx, cancel := context.WithTimeout(context.Background(), ssrfLookupTimeout)
	defer cancel()
	var (
		ips []net.IPAddr
		err error
	)
	if ssrfLookupHook != nil {
		ips, err = ssrfLookupHook(ctx, host)
	} else {
		ips, err = net.DefaultResolver.LookupIPAddr(ctx, host)
	}
	if err != nil {
		// DNS 解析失败不能证明安全：拒绝请求并归因。
		return false, fmt.Sprintf("dns lookup failed: %v", err)
	}
	if len(ips) == 0 {
		return false, "dns lookup returned no address"
	}
	for _, ip := range ips {
		for _, cidr := range ssrfBlockedCIDRs {
			if cidr.Contains(ip.IP) {
				return false, fmt.Sprintf("address %s falls into blocked range %s", ip.IP, cidr)
			}
		}
	}
	return true, ""
}

// ssrfGuardSkipped 供测试环境（httptest 环回上游）整体跳过守卫。
var ssrfGuardSkipped bool

// SetSSRFGuardDisabled 测试专用：跳过 ValidateChannelURL 的全部检查。
func SetSSRFGuardDisabled(disabled bool) {
	ssrfGuardSkipped = disabled
}

// ValidateChannelURL 在「即将发起上游请求」时调用（非仅保存时）：
// 拒绝非 http/https scheme 与解析到私网/环回/链路本地/元数据地址的目标。
// allowlist 中的 host 跳过 DNS 二次校验（仍校验 scheme 与字面 IP）。
func ValidateChannelURL(rawURL string) error {
	if ssrfGuardSkipped {
		return nil
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid upstream url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("upstream scheme %q is not allowed", u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("upstream url has empty host")
	}

	// 字面 IP 直接判定，不查 allowlist、不做 DNS。
	if ip := net.ParseIP(host); ip != nil {
		return validateLiteralIP(ip)
	}

	if isAllowlistedHost(host) {
		return nil
	}

	allowed, reason := lookupSSRFCached(host)
	if !allowed {
		return fmt.Errorf("upstream host %q rejected by ssrf guard: %s", host, reason)
	}
	return nil
}

// validateLiteralIP 对字面 IP 的判定（allowlist 不能放行私网字面 IP）。
func validateLiteralIP(ip net.IP) error {
	for _, cidr := range ssrfBlockedCIDRs {
		if cidr.Contains(ip) {
			return fmt.Errorf("upstream address %s falls into blocked range %s", ip, cidr)
		}
	}
	return nil
}

// isAllowlistedHost 支持通配尾缀（.example.com）匹配。
func isAllowlistedHost(host string) bool {
	host = strings.ToLower(host)
	if ssrfAllowlisted[host] {
		return true
	}
	// 逐级去掉首段后按父域匹配：a.b.example.com → b.example.com → example.com。
	for i := 0; i < len(host); i++ {
		if host[i] != '.' {
			continue
		}
		if parent := host[i+1:]; parent != "" && ssrfAllowlisted[parent] {
			return true
		}
	}
	return false
}
