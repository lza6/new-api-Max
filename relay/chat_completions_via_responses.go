package relay

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/constant"
	"github.com/lza6/new-api-Max/relay/channel"
	openaichannel "github.com/lza6/new-api-Max/relay/channel/openai"
	relaycommon "github.com/lza6/new-api-Max/relay/common"
	relayconstant "github.com/lza6/new-api-Max/relay/constant"
	"github.com/lza6/new-api-Max/relaykit/dto"
	"github.com/lza6/new-api-Max/relaykit/types"
	"github.com/lza6/new-api-Max/service"

	"github.com/gin-gonic/gin"
)

func applySystemPromptIfNeeded(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) {
	if info == nil || request == nil {
		return
	}
	if info.ChannelSetting.SystemPrompt == "" {
		return
	}

	systemRole := request.GetSystemRoleName()

	// A Kimi K3 dynamic tool loading message ({"role":"system","tools":[...]})
	// declares tools rather than a system prompt and must never receive content.
	containSystemPrompt := false
	for _, message := range request.Messages {
		if message.Role == systemRole && len(message.Tools) == 0 {
			containSystemPrompt = true
			break
		}
	}
	if !containSystemPrompt {
		systemMessage := dto.Message{
			Role:    systemRole,
			Content: info.ChannelSetting.SystemPrompt,
		}
		request.Messages = append([]dto.Message{systemMessage}, request.Messages...)
		return
	}

	if !info.ChannelSetting.SystemPromptOverride {
		return
	}

	common.SetContextKey(c, constant.ContextKeySystemPromptOverride, true)
	for i, message := range request.Messages {
		if message.Role != systemRole || len(message.Tools) > 0 {
			continue
		}
		if message.IsStringContent() {
			request.Messages[i].SetStringContent(info.ChannelSetting.SystemPrompt + "\n" + message.StringContent())
			return
		}
		contents := message.ParseContent()
		contents = append([]dto.MediaContent{
			{
				Type: dto.ContentTypeText,
				Text: info.ChannelSetting.SystemPrompt,
			},
		}, contents...)
		request.Messages[i].Content = contents
		return
	}
}

// injectSystemPromptIntoPassThroughBody 在**请求体透传**路径下把渠道级系统提示词
// 注入到原始 JSON body，其余字段原样保留。
//
// 背景（生产 bug）：渠道同时配置 system_prompt + pass_through_body_enabled 时，
// 旧实现走透传分支直接转发原始 body，**完全跳过系统提示词注入** → 用户反馈
// 「渠道内置提示词不生效」（channel 50 实测）。非透传分支已注入，透传分支必须补齐。
//
// 实现：读取原始 body → 解析为 GeneralOpenAIRequest（仅取 messages，其余字段丢弃）——
// 不，透传语义要求**保留所有原始字段**，因此这里只对 messages 做 JSON 级改写：
// 解析顶层 JSON 为 map，改 messages 数组后重新序列化。解析/改写失败则返回错误，
// 由调用方回退到原始透传体（不阻断请求）。
func injectSystemPromptIntoPassThroughBody(storage common.BodyStorage, info *relaycommon.RelayInfo) (io.Reader, error) {
	raw, err := storage.Bytes()
	if err != nil {
		return nil, err
	}
	var body map[string]any
	if err := common.Unmarshal(raw, &body); err != nil {
		return nil, err
	}
	msgs, _ := body["messages"].([]any)
	// 拆成 GeneralOpenAIRequest 复用注入逻辑（只处理 messages）。
	req := &dto.GeneralOpenAIRequest{}
	for i := range msgs {
		if m, ok := msgs[i].(map[string]any); ok {
			var msg dto.Message
			if err := common.Unmarshal(mustJSON(m), &msg); err == nil {
				req.Messages = append(req.Messages, msg)
			}
		}
	}
	applySystemPromptIntoMessages(info, req)

	// 回写 messages（保留 body 其余所有字段）。
	outMsgs := make([]any, 0, len(req.Messages))
	for i := range req.Messages {
		var m map[string]any
		if err := common.Unmarshal(mustJSON(req.Messages[i]), &m); err == nil {
			outMsgs = append(outMsgs, m)
		}
	}
	body["messages"] = outMsgs
	rewritten, err := common.Marshal(body)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(rewritten), nil
}

// applySystemPromptIntoMessages 把渠道 system prompt 注入到 messages（前插/合并）。
// 与 applySystemPromptIfNeeded 的 messages 语义一致，但不依赖 *gin.Context（透传路径
// 无需 SetContextKey 的覆盖标记，因为透传不改写其它字段）。
func applySystemPromptIntoMessages(info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) {
	if info == nil || request == nil || info.ChannelSetting.SystemPrompt == "" {
		return
	}
	systemRole := request.GetSystemRoleName()
	for i := range request.Messages {
		if request.Messages[i].Role == systemRole && len(request.Messages[i].Tools) == 0 {
			if info.ChannelSetting.SystemPromptOverride {
				if request.Messages[i].IsStringContent() {
					existing := request.Messages[i].StringContent()
					if existing == "" {
						request.Messages[i].SetStringContent(info.ChannelSetting.SystemPrompt)
					} else {
						request.Messages[i].SetStringContent(info.ChannelSetting.SystemPrompt + "\n" + existing)
					}
					return
				}
			}
			// 无 override：已有 system 时保持原样（与原 applySystemPromptIfNeeded 一致）。
			return
		}
	}
	// 无 system message：头部插入。
	request.Messages = append([]dto.Message{{
		Role:    systemRole,
		Content: info.ChannelSetting.SystemPrompt,
	}}, request.Messages...)
}

func mustJSON(v any) []byte {
	b, _ := common.Marshal(v)
	return b
}

func textRequestViaResponses(c *gin.Context, info *relaycommon.RelayInfo, adaptor channel.Adaptor, request any) (*dto.Usage, *types.NewAPIError) {
	paramOverrideApplied := false
	if chatRequest, ok := request.(*dto.GeneralOpenAIRequest); ok {
		chatJSON, err := common.Marshal(chatRequest)
		if err != nil {
			return nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}

		chatJSON, err = relaycommon.RemoveDisabledFields(chatJSON, info.ChannelOtherSettings, info.ChannelSetting.PassThroughBodyEnabled)
		if err != nil {
			return nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}

		if len(info.ParamOverride) > 0 {
			chatJSON, err = relaycommon.ApplyParamOverrideWithRelayInfo(chatJSON, info)
			if err != nil {
				return nil, newAPIErrorFromParamOverride(err)
			}
			paramOverrideApplied = true
		}

		var overriddenChatReq dto.GeneralOpenAIRequest
		if err := common.Unmarshal(chatJSON, &overriddenChatReq); err != nil {
			return nil, types.NewError(err, types.ErrorCodeChannelParamOverrideInvalid, types.ErrOptionWithSkipRetry())
		}
		request = &overriddenChatReq
	}

	result, err := service.ConvertRequest(c, info, types.RelayFormatOpenAIResponses, request)
	if err != nil {
		return nil, types.NewErrorWithStatusCode(err, types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	responsesReq, ok := result.Value.(*dto.OpenAIResponsesRequest)
	if !ok {
		return nil, types.NewError(fmt.Errorf("expected OpenAI responses request, got %T", result.Value), types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}
	return relayResponsesRequest(c, info, adaptor, responsesReq, paramOverrideApplied)
}

func relayResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, adaptor channel.Adaptor, responsesReq *dto.OpenAIResponsesRequest, paramOverrideApplied bool) (*dto.Usage, *types.NewAPIError) {
	savedRelayMode := info.RelayMode
	savedRequestURLPath := info.RequestURLPath
	defer func() {
		info.RelayMode = savedRelayMode
		info.RequestURLPath = savedRequestURLPath
	}()

	info.RelayMode = relayconstant.RelayModeResponses
	info.RequestURLPath = "/v1/responses"

	convertedRequest, err := adaptor.ConvertOpenAIResponsesRequest(c, info, *responsesReq)
	if err != nil {
		return nil, newConvertRequestFailedError(c, info, err)
	}
	relaycommon.AppendRequestConversionFromRequest(info, convertedRequest)

	jsonData, err := common.Marshal(convertedRequest)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}

	jsonData, err = relaycommon.RemoveDisabledFields(jsonData, info.ChannelOtherSettings, info.ChannelSetting.PassThroughBodyEnabled)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}
	if !paramOverrideApplied && len(info.ParamOverride) > 0 {
		jsonData, err = relaycommon.ApplyParamOverrideWithRelayInfo(jsonData, info)
		if err != nil {
			return nil, newAPIErrorFromParamOverride(err)
		}
	}

	body, closer, err := relaycommon.NewOutboundJSONBody(jsonData)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}
	defer closer.Close()
	jsonData = nil
	var requestBody io.Reader = body

	var httpResp *http.Response
	resp, err := adaptor.DoRequest(c, info, requestBody)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
	}
	if resp == nil {
		return nil, types.NewOpenAIError(nil, types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}

	statusCodeMappingStr := c.GetString("status_code_mapping")

	httpResp = resp.(*http.Response)
	clientStream := info.IsStream
	upstreamStream := isResponsesEventStreamContentType(httpResp.Header.Get("Content-Type"))
	info.IsStream = clientStream || upstreamStream
	if httpResp.StatusCode != http.StatusOK {
		newApiErr := service.RelayErrorHandler(c.Request.Context(), httpResp, false)
		service.ResetStatusCode(newApiErr, statusCodeMappingStr)
		return nil, newApiErr
	}

	if upstreamStream && clientStream {
		usage, newApiErr := openaichannel.OaiResponsesToChatStreamHandler(c, info, httpResp)
		if newApiErr != nil {
			service.ResetStatusCode(newApiErr, statusCodeMappingStr)
			return nil, newApiErr
		}
		return usage, nil
	}
	if upstreamStream {
		info.IsStream = false
		usage, newApiErr := openaichannel.OaiResponsesToChatBufferedStreamHandler(c, info, httpResp)
		if newApiErr != nil {
			service.ResetStatusCode(newApiErr, statusCodeMappingStr)
			return nil, newApiErr
		}
		return usage, nil
	}

	usage, newApiErr := openaichannel.OaiResponsesToChatHandler(c, info, httpResp)
	if newApiErr != nil {
		service.ResetStatusCode(newApiErr, statusCodeMappingStr)
		return nil, newApiErr
	}
	return usage, nil
}

func isResponsesEventStreamContentType(contentType string) bool {
	return strings.Contains(strings.ToLower(contentType), "text/event-stream")
}
