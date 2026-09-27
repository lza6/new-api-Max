package common

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// §4.1.2 共享出站连接池（非 SSRF 路径）。
//
// 背景：中继渠道适配器（ollama/ali/kilwa 等）此前每请求新建 `&http.Client{}`，
// 走 Go 默认 Transport（MaxIdleConnsPerHost=2）——高并发同上游时 keep-alive 复用
// 不足，造成连接抖动/端口占用。连接池在 Transport 上，http.Client 只是配置壳。
//
// 本文件提供单一共享调优 Transport + 带超时的 client 工厂：
//   - 所有外呼共享同一连接池（更高 MaxIdleConnsPerHost），消除连接抖动；
//   - 各调用方仍可指定自己的 Timeout（如 ollama 大模型 30min/60min），行为不变；
//   - TLS 校验随 TLS_INSECURE_SKIP_VERIFY 与中继口径一致；
//   - 非 SSRF 路径专用：URL 由网关/渠道配置控制；任意用户可控 URL 走 SSRF 客户端。
//
// 构造时机（P1-1 修复）：渠道适配器的**包级 client var 初始化**在程序加载即会触发
// `GetOutboundTransport()`——早于 main() 的 godotenv.Load 与 InitEnv，因此不能依赖
// 包内 `TLSInsecureSkipVerify` 变量或包 var 顺序（都会在 .env 加载前读到 false）。
// 修复采用「惰性 RoundTripper」：真实 Transport 在**首个外呼（RoundTrip）**时才构建，
// 此时 .env/进程 env 均已就绪；os.Getenv 时序无关地正确读取 TLS_INSECURE_SKIP_VERIFY。
// 连接池共享、并发安全（sync.Once）、TLS 行为与 service/http_client.go 运行期构建最终一致。

// lazyOutboundTransport 延迟构建真实 Transport 的 RoundTripper（首个 RoundTrip 时构建）。
type lazyOutboundTransport struct {
	once sync.Once
	tr   *http.Transport
}

func (l *lazyOutboundTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	l.once.Do(func() { l.tr = buildOutboundTransport() })
	return l.tr.RoundTrip(req)
}

func (l *lazyOutboundTransport) ensureBuilt() *http.Transport {
	l.once.Do(func() { l.tr = buildOutboundTransport() })
	return l.tr
}

var sharedOutbound = &lazyOutboundTransport{}

func buildOutboundTransport() *http.Transport {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   32, // 高于 DefaultTransport(2)：高并发同上游 keep-alive 复用
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: time.Second,
	}
	// 直读 env：无论何时构建（首个外呼，晚于 .env/进程 env 就绪），均正确反映开关。
	if GetEnvOrDefaultBool("TLS_INSECURE_SKIP_VERIFY", false) {
		transport.TLSClientConfig = InsecureTLSConfig
	}
	return transport
}

// GetOutboundTransport 返回共享出站连接池（惰性构建的 RoundTripper，只读勿改写）。
func GetOutboundTransport() http.RoundTripper {
	return sharedOutbound
}

// GetOutboundTransportBuilt 强制构建并返回真实 Transport（测试/观测用）。
func GetOutboundTransportBuilt() *http.Transport {
	return sharedOutbound.ensureBuilt()
}

// NewOutboundClient 返回使用共享连接池 + 指定超时的出站 client。
// http.Client 是轻量配置壳，可多次调用；连接池在共享 Transport 中。
// timeout<=0 表示不设超时（与既有实现保持一致，调用方自行权衡）。
func NewOutboundClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, Transport: GetOutboundTransport()}
}