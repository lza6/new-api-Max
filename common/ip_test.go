package common

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
)

// §4.11.5：IsPrivateIP 必须覆盖 CGNAT 与常见保留段，与 ssrf_protection 判定一致。
func TestIsPrivateIPCoversCGNATAndReserved(t *testing.T) {
	private := []string{
		"10.0.0.1", "10.255.255.255",
		"100.64.0.1", "100.127.255.255", // CGNAT 100.64.0.0/10
		"172.16.0.1", "172.31.255.255",
		"192.168.0.1", "192.168.255.255",
		"169.254.1.1", // link-local
		"127.0.0.1", "0.0.0.0",
		"::1",
	}
	for _, s := range private {
		assert.Truef(t, IsPrivateIP(net.ParseIP(s)), "%s must be private", s)
	}

	public := []string{
		"100.128.0.1", // 紧邻 CGNAT 上界之外
		"8.8.8.8", "1.1.1.1", "103.233.252.213", "172.32.0.1", "192.169.0.1",
	}
	for _, s := range public {
		assert.Falsef(t, IsPrivateIP(net.ParseIP(s)), "%s must be public", s)
	}
}

// §审查 C2：信任来源不得包含 CGNAT（运营商大内网/VPN 出口属外部来源）。
func TestIsTrustedSourceIPExcludesCGNAT(t *testing.T) {
	trusted := []string{"127.0.0.1", "10.0.0.1", "172.18.0.1", "192.168.1.1", "169.254.1.1", "::1"}
	for _, s := range trusted {
		assert.Truef(t, IsTrustedSourceIP(net.ParseIP(s)), "%s must be a trusted source", s)
	}
	// CGNAT 必须是「私有目标」(IsPrivateIP) 但「非信任来源」(IsTrustedSourceIP)。
	assert.True(t, IsPrivateIP(net.ParseIP("100.64.0.1")), "CGNAT is a private SSRF target")
	assert.False(t, IsTrustedSourceIP(net.ParseIP("100.64.0.1")), "CGNAT must NOT be a trusted source")
	assert.False(t, IsTrustedSourceIP(net.ParseIP("100.127.255.255")), "CGNAT must NOT be a trusted source")
	assert.False(t, IsTrustedSourceIP(net.ParseIP("8.8.8.8")))
}
