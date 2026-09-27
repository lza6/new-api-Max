package common

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOutboundSharedTransport 锁定 §4.1.2：所有 NewOutboundClient 与
// GetOutboundTransport 必须引用同一 Transport 单例（连接池共享），
// 且调优值满足高并发 keep-alive 语义（宽松下界）。
func TestOutboundSharedTransport(t *testing.T) {
	a := NewOutboundClient(0)
	b := NewOutboundClient(30 * time.Second)
	c := GetOutboundTransport()

	require.NotNil(t, a.Transport)
	require.NotNil(t, b.Transport)
	require.NotNil(t, c)
	// 同一指针 = 同一连接池（这是"复用"的构造性证明）。
	assert.Same(t, c, a.Transport, "跨 client 应共享同一 Transport（连接池）")
	assert.Same(t, c, b.Transport)

	// 调优下界：MaxIdleConnsPerHost 应高于 Go 默认 2，支撑同上游高并发 keep-alive。
	assert.GreaterOrEqual(t, c.MaxIdleConnsPerHost, 8)
	assert.GreaterOrEqual(t, c.MaxIdleConns, 32)
}

// TestOutboundTransportHonorsTLSEnv 锁定 §4.1.2 P1-1 修复：builder 直接读取
// TLS_INSECURE_SKIP_VERIFY 环境变量（os.Getenv 时序无关），不受包级 var/init 顺序
// 影响。测试用 t.Setenv + 直接调 builder，确定性覆盖 true/false 两分支。
func TestOutboundTransportHonorsTLSEnv(t *testing.T) {
	t.Setenv("TLS_INSECURE_SKIP_VERIFY", "true")
	tr := buildOutboundTransport()
	require.NotNil(t, tr.TLSClientConfig)
	assert.Same(t, InsecureTLSConfig, tr.TLSClientConfig)

	t.Setenv("TLS_INSECURE_SKIP_VERIFY", "false")
	tr2 := buildOutboundTransport()
	assert.Nil(t, tr2.TLSClientConfig, "未开启 TLS_INSECURE_SKIP_VERIFY 时必须保持默认校验")
}