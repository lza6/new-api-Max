package helper

import (
	"fmt"
	"io"

	"github.com/lza6/new-api-Max/constant"
)

// maxUpstreamResponseBytes 上游响应体读取上限（与请求体 128MB 对齐，可经
// MAX_REQUEST_BODY_MB 调整）。防止恶意/异常上游返回超大 body 导致 OOM。
func maxUpstreamResponseBytes() int64 {
	mb := constant.MaxRequestBodyMB
	if mb <= 0 {
		mb = 128
	}
	// int64 溢出防御：上限 8MB 封顶，实际场景正常响应远小于此。
	const ceiling = int64(512 * 1024 * 1024)
	limit := int64(mb) * 1024 * 1024
	if limit <= 0 || limit > ceiling {
		return ceiling
	}
	return limit
}

// ReadLimitedUpstreamBody 用 io.LimitReader 读取上游响应体，超限返回错误，
// 避免非流式 handler 无界 io.ReadAll 造成内存暴涨。
func ReadLimitedUpstreamBody(body io.Reader) ([]byte, error) {
	if body == nil {
		return nil, fmt.Errorf("upstream body is nil")
	}
	limit := maxUpstreamResponseBytes()
	// 多读 1 字节以检测超限。
	data, err := io.ReadAll(io.LimitReader(body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("upstream response body exceeds limit of %d bytes", limit)
	}
	return data, nil
}
