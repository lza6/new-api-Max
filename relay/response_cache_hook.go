package relay

import (
	"bytes"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/lza6/new-api-Max/relay/channel"
	relaycommon "github.com/lza6/new-api-Max/relay/common"
	"github.com/lza6/new-api-Max/service"
)

// 网关响应缓存的**取数钩子**（Batch-9 / G3）。
//
// 为什么包在 `adaptor.DoRequest` 外层、而不是在 controller 里直接回写响应并自行计费：
//
//	下游（`adaptor.DoResponse` → usage 提取 → `PostTextConsumeQuota` → 消费日志）
//	依赖 relay handler 逐层填充的 relayInfo 状态。**绕过 handler 直接调用计费会 nil 解引用**
//	（本轮真实 E2E 实际抓到这个 panic），而且消费日志会缺失 —— 管理员会看到
//	「上游调用量下降，但日志里什么都没有」，反而更难排查。
//
// 伪造一个 200 响应喂给既有链路，等于让缓存命中「长得和真实响应一模一样」：
// 计费、日志、健康分、中继自检全部原样运行，不需要任何特判分支。

// httpResponseFromCache 用缓存字节构造一个等价的 200 响应。
// 只设置最小必要字段：Content-Type 与 Body —— 下游 DoResponse 按客户端格式解析字节，
// 不依赖上游头的其余部分（缓存命中时本来也没有真实上游头可透传）。
func httpResponseFromCache(body []byte) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(body)),
	}
}

// doRequestWithResponseCache 先查网关响应缓存；命中则返回伪造响应，否则走真实上游。
//
// 未启用 / 模型不在白名单 / 流式 / 未命中 → 一律原样落到 `adaptor.DoRequest`，
// 缓存失效绝不影响正常链路。
func doRequestWithResponseCache(c *gin.Context, info *relaycommon.RelayInfo, adaptor channel.Adaptor, requestBody io.Reader) (any, error) {
	if body, ok := service.ResponseCacheServeHit(c, info); ok {
		service.MarkResponseCacheServed(c)
		return httpResponseFromCache(body), nil
	}
	return adaptor.DoRequest(c, info, requestBody)
}
