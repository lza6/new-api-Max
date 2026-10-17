package dto

import (
	"net/http"

	"github.com/lza6/new-api-Max/relaykit/types"
)

// OpenAIOpsRequest 是 /v1/files、/v1/fine-tunes、/v1/batches 的原始请求。
//
// 这些端点不共享 chat/completions 的 JSON 结构：files 上传是 multipart，
// batches 只带 input_file_id，fine-tunes 的 model 在 JSON body 里。因此这里
// 只提取**路由所需的模型名**，不把请求体物化到内存（大文件上传会溢写磁盘）。
//
// 模型名来源（优先级）：`X-New-API-Model` 请求头 → JSON body 的 `model`
// → query `model`。没有模型时由分发中间件回退到运维配置的
// `OPENAI_OPS_DEFAULT_MODEL`；仍为空则明确拒绝（网关按模型选渠道，不能瞎猜）。
type OpenAIOpsRequest struct {
	BaseRequest
	Model string
}

func (r *OpenAIOpsRequest) GetTokenCountMeta() *types.TokenCountMeta {
	return &types.TokenCountMeta{TokenType: types.TokenTypeTokenizer}
}

func (r *OpenAIOpsRequest) IsStream(_ *http.Request) bool { return false }

func (r *OpenAIOpsRequest) SetModelName(modelName string) { r.Model = modelName }
