package common

import "net"

func IsIP(s string) bool {
	ip := net.ParseIP(s)
	return ip != nil
}

func ParseIP(s string) net.IP {
	return net.ParseIP(s)
}

// IsPrivateIP 判定 IP 是否为私有/保留/特殊用途地址（**SSRF 语义**：该地址不可作为
// 出站请求目标）。§4.11.5：补齐 CGNAT（100.64.0.0/10）与链路本地/保留段，与
// ssrf_protection.go 的 isPrivateIP 保持同一集合（100.64/10 + 127/8 + 0/8 + 169.254/16
// + RFC1918），避免两处判定不一致导致的 SSRF 绕过。
//
// 注意：本函数是**「不该连过去」**的语义，**不是**「可以信任」。CGNAT 段（运营商
// 大内网 / VPN / 隧道出口）既不可作为出站目标、也**绝不可**当作可信来源——两者切勿混用。
// 判定「请求来源是否可信」请用 IsTrustedSourceIP。
func IsPrivateIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified() || ip.IsMulticast() || ip.IsInterfaceLocalMulticast() {
		return true
	}

	private := []net.IPNet{
		{IP: net.IPv4(10, 0, 0, 0), Mask: net.CIDRMask(8, 32)},     // 10.0.0.0/8
		{IP: net.IPv4(100, 64, 0, 0), Mask: net.CIDRMask(10, 32)},  // 100.64.0.0/10 (CGNAT)
		{IP: net.IPv4(172, 16, 0, 0), Mask: net.CIDRMask(12, 32)},  // 172.16.0.0/12
		{IP: net.IPv4(192, 168, 0, 0), Mask: net.CIDRMask(16, 32)}, // 192.168.0.0/16
		{IP: net.IPv4(169, 254, 0, 0), Mask: net.CIDRMask(16, 32)}, // 169.254.0.0/16 (link-local)
		{IP: net.IPv4(127, 0, 0, 0), Mask: net.CIDRMask(8, 32)},    // 127.0.0.0/8
		{IP: net.IPv4(0, 0, 0, 0), Mask: net.CIDRMask(8, 32)},      // 0.0.0.0/8
	}

	for _, privateNet := range private {
		if privateNet.Contains(ip) {
			return true
		}
	}
	// IPv6 私有段：::1/128 已由 IsLoopback 覆盖；fc00::/7 (ULA)、fe80::/10 由
	// IsLinkLocalUnicast 覆盖。此处补 IPv4-mapped 的 ULA 判定。
	if v4 := ip.To4(); v4 == nil {
		if ip.IsPrivate() {
			return true
		}
	}
	return false
}

// IsTrustedSourceIP 判定请求来源是否「内部/可信」，用于 Web 防护信任来源豁免。
// **仅**环回、链路本地、以及本机/容器所在私网（RFC1918：10/8、172.16/12、192.168/16）为信任；
// **不含 CGNAT 100.64.0.0/10**——该段是运营商大内网 / VPN / 隧道出口，属**外部**来源，
// 若当信任来源会豁免限流与自动封禁（安全反向扩大，见审查 C2）。
//
// 与 IsPrivateIP 的语义区别：IsPrivateIP = 「不该连过去」(SSRF)；
// IsTrustedSourceIP = 「可以信任它的来源」(Web 防护豁免)。两者不是同一集合。
func IsTrustedSourceIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}
	trusted := []net.IPNet{
		{IP: net.IPv4(10, 0, 0, 0), Mask: net.CIDRMask(8, 32)},     // 10.0.0.0/8
		{IP: net.IPv4(172, 16, 0, 0), Mask: net.CIDRMask(12, 32)},  // 172.16.0.0/12（含 Docker 网关 172.18.x）
		{IP: net.IPv4(192, 168, 0, 0), Mask: net.CIDRMask(16, 32)}, // 192.168.0.0/16
	}
	for _, n := range trusted {
		if n.Contains(ip) {
			return true
		}
	}
	// IPv6 ULA fc00::/7 视为内部。
	if ip.To4() == nil && ip.IsPrivate() {
		return true
	}
	return false
}

func IsIpInCIDRList(ip net.IP, cidrList []string) bool {
	for _, cidr := range cidrList {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			// 尝试作为单个IP处理
			if whitelistIP := net.ParseIP(cidr); whitelistIP != nil {
				if ip.Equal(whitelistIP) {
					return true
				}
			}
			continue
		}

		if network.Contains(ip) {
			return true
		}
	}
	return false
}
