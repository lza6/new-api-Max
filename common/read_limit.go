package common

import (
	"errors"
	"fmt"
	"io"
)

// 出站响应读取的上限工具 —— Batch-9 / G2。
//
// 背景：仓库里有大量裸 `io.ReadAll(resp.Body)`（上游任务响应、OAuth 响应、
// 渠道账单、Midjourney 查询等）。上游是我们**不完全信任**的一方：一次异常或
// 恶意响应就能让单进程把内存吃穿（本机生产只有 4GB）。这里给一个统一收口，
// 避免每处各写一遍 `LimitReader` 算术。
//
// 与 `common/body_storage.go` 的 `CreateBodyStorageFromReader` 同源思路
// （`maxBytes+1` 探界），但那个是**入站请求体**落盘/落内存；本函数是**出站响应**
// 的纯内存读取，不涉及磁盘。
//
// 纪律：超限必须返回**明确错误**，绝不静默截断 —— 截断的上游 JSON 会变成
// 难排查的解析错误，比直接报「响应过大」糟得多。

// 常用出站响应上限（按上下文取值；调用方也可传自定义值）。
const (
	// MaxUpstreamTaskResponseBytes 上游任务轮询/查询响应上限（8 MB）。
	// 视频/图像任务结果是 URL 或小 JSON，8 MB 已有极大余量。
	MaxUpstreamTaskResponseBytes int64 = 8 << 20
	// MaxOAuthResponseBytes OAuth/OIDC token 与 userinfo 响应上限（64 KB）。
	// 这些端点只返回小 JSON；按 OIDC 规范 id_token 可达数 KB，64 KB 足够。
	MaxOAuthResponseBytes int64 = 64 << 10
	// MaxUpstreamErrorResponseBytes 上游错误体上限（1 MB）。
	// 错误体用于日志与透传，超过 1 MB 只会污染日志。
	MaxUpstreamErrorResponseBytes int64 = 1 << 20
	// MaxUpstreamModelsResponseBytes `/v1/models` 等元数据端点上限（8 MB）。
	MaxUpstreamModelsResponseBytes int64 = 8 << 20
)

// ErrReadAllLimitExceeded 表示出站响应超过调用方给定的上限。
var ErrReadAllLimitExceeded = errors.New("upstream response exceeds the configured read limit")

// ReadAllLimited 读取 r 的全部内容，但**最多** maxBytes 字节。
//
// 实现要点：先按 `maxBytes+1` 限流，读到 `maxBytes+1` 即说明超限；这样刚好等于
// 上限的响应能完整通过，只有真正超出才报错（边界不差一）。
//
// maxBytes <= 0 视为调用方配置错误：返回错误而不是「无限读」——静默退化成无界
// 读取正是本次要修的问题。
func ReadAllLimited(r io.Reader, maxBytes int64) ([]byte, error) {
	if r == nil {
		return nil, fmt.Errorf("ReadAllLimited: nil reader")
	}
	if maxBytes <= 0 {
		return nil, fmt.Errorf("ReadAllLimited: non-positive limit %d", maxBytes)
	}
	data, err := io.ReadAll(io.LimitReader(r, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("%w: limit=%d bytes", ErrReadAllLimitExceeded, maxBytes)
	}
	return data, nil
}
