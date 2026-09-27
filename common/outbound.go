package common

import (
	"net"
	"net/http"
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
//   - 保持流式不透传缓冲、直连；TLS 校验随 TLS_INSECURE_SKIP_VERIFY 与中继口径一致。
//   - 非 SSRF 路径专用：URL 由网关/渠道配置控制；任意用户可控 URL 走 SSRF 客户端。
//
// 构造时机（P1-1 修复）：本 Transport 由渠道适配器的**包级 client var 初始化**触发构建，
// 先于 main()/InitEnv()，因此不能依赖包内 `TLSInsecureSkipVerify` 变量（其在 init.go 的
// init() 里才从 env 赋值）——builder 直接读取 env（os.Getenv 时序无关），保证与
// service/http_client.go 的 newRelayHTTPTransport（运行期构建）最终口径一致。

var sharedOutboundTransport = buildOutboundTransport()

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
	// 直读 env：无论本包/调用方何时初始化，均正确反映 TLS_INSECURE_SKIP_VERIFY。
	if GetEnvOrDefaultBool("TLS_INSECURE_SKIP_VERIFY", false) {
		transport.TLSClientConfig = InsecureTLSConfig
	}
	return transport
}

// GetOutboundTransport 返回共享出站连接池 Transport（只读，勿修改字段）。
func GetOutboundTransport() *http.Transport {
	return sharedOutboundTransport
}

// NewOutboundClient 返回使用共享连接池 + 指定超时的出站 client。
// http.Client 是轻量配置壳，可多次调用；连接池在共享 Transport 中。
// timeout<=0 表示不设超时（与既有实现保持一致，调用方自行权衡）。
func NewOutboundClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, Transport: GetOutboundTransport()}
}