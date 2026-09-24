package relay

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"context"
	"errors"
	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/constant"
	"github.com/lza6/new-api-Max/logger"
	relaycommon "github.com/lza6/new-api-Max/relay/common"
	relayconstant "github.com/lza6/new-api-Max/relay/constant"
	"github.com/lza6/new-api-Max/relay/helper"
	"github.com/lza6/new-api-Max/relaykit/dto"
	kitreasoning "github.com/lza6/new-api-Max/relaykit/relayconvert/reasoning"
	"github.com/lza6/new-api-Max/relaykit/types"
	"github.com/lza6/new-api-Max/service"
	"github.com/lza6/new-api-Max/setting/model_setting"
	"github.com/lza6/new-api-Max/setting/ratio_setting"
	"github.com/lza6/new-api-Max/setting/relay_setting"
	"github.com/samber/lo"
	"time"

	"github.com/gin-gonic/gin"
)

func TextHelper(c *gin.Context, info *relaycommon.RelayInfo) (newAPIError *types.NewAPIError) {
	info.InitChannelMeta(c)

	textReq, ok := info.Request.(*dto.GeneralOpenAIRequest)
	if !ok {
		return types.NewErrorWithStatusCode(fmt.Errorf("invalid request type, expected dto.GeneralOpenAIRequest, got %T", info.Request), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}

	request, err := common.DeepCopy(textReq)
	if err != nil {
		return types.NewError(fmt.Errorf("failed to copy request to GeneralOpenAIRequest: %w", err), types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}

	if request.WebSearchOptions != nil {
		c.Set("chat_completion_web_search_context_size", request.WebSearchOptions.SearchContextSize)
	}

	err = helper.ModelMappedHelper(c, info, request)
	if err != nil {
		return types.NewError(err, types.ErrorCodeChannelModelMappedError, types.ErrOptionWithSkipRetry())
	}
	if err := helper.ApplyReasoningModelSuffix(c, info, request); err != nil {
		return newConvertRequestFailedError(c, info, err)
	}

	includeUsage := true
	// 判断用户是否需要返回使用情况
	if request.StreamOptions != nil {
		includeUsage = request.StreamOptions.IncludeUsage
	}

	// 如果不支持StreamOptions，将StreamOptions设置为nil
	if !info.SupportStreamOptions || !lo.FromPtrOr(request.Stream, false) {
		request.StreamOptions = nil
	} else {
		// 如果支持StreamOptions，且请求中没有设置StreamOptions，根据配置文件设置StreamOptions
		if constant.ForceStreamOption {
			request.StreamOptions = &dto.StreamOptions{
				IncludeUsage: true,
			}
		}
	}

	info.ShouldIncludeUsage = includeUsage

	adaptor := GetAdaptor(info.ApiType)
	if adaptor == nil {
		return types.NewError(fmt.Errorf("invalid api type: %d", info.ApiType), types.ErrorCodeInvalidApiType, types.ErrOptionWithSkipRetry())
	}
	adaptor.Init(info)

	passThroughGlobal := model_setting.GetGlobalSettings().PassThroughRequestEnabled
	if info.RelayMode == relayconstant.RelayModeChatCompletions &&
		!passThroughGlobal &&
		!info.ChannelSetting.PassThroughBodyEnabled &&
		service.ShouldChatCompletionsUseResponsesGlobal(info.ChannelId, info.ChannelType, info.OriginModelName) {
		applySystemPromptIfNeeded(c, info, request)
		usage, newApiErr := textRequestViaResponses(c, info, adaptor, request)
		if newApiErr != nil {
			return newApiErr
		}

		var containAudioTokens = usage.CompletionTokenDetails.AudioTokens > 0 || usage.PromptTokensDetails.AudioTokens > 0
		var containsAudioRatios = ratio_setting.ContainsAudioRatio(info.OriginModelName) || ratio_setting.ContainsAudioCompletionRatio(info.OriginModelName)

		if containAudioTokens && containsAudioRatios {
			service.PostAudioConsumeQuota(c, info, usage, "")
		} else {
			service.PostTextConsumeQuota(c, info, usage, nil)
		}
		return nil
	}

	var requestBody io.Reader

	if passThroughGlobal || info.ChannelSetting.PassThroughBodyEnabled {
		storage, err := common.GetBodyStorage(c)
		if err != nil {
			return types.NewErrorWithStatusCode(err, types.ErrorCodeReadRequestBodyFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
		}
		if common.DebugEnabled {
			if debugBytes, bErr := storage.Bytes(); bErr == nil {
				logger.LogDebug(c, "requestBody: %s", debugBytes)
			}
		}
		// [fix-defensive] OpenAI 透传路径：归一非标准 reasoning_effort（on/true
		// -> 剔除、off/false -> none）。透传体原样转发，但非法 effort 会触发上游
		// 400 "field ReasoningEffort invalid"（channel 38 第三方中转实锤）。
		requestBody = sanitizeOpenAIPassThroughReasoningEffort(common.NewReplayableBodyReader(storage), info.RelayMode)
	} else {
		convertedRequest, err := adaptor.ConvertOpenAIRequest(c, info, request)
		if err != nil {
			return newConvertRequestFailedError(c, info, err)
		}
		relaycommon.AppendRequestConversionFromRequest(info, convertedRequest)

		if req, ok := convertedRequest.(*dto.GeneralOpenAIRequest); ok {
			applySystemPromptIfNeeded(c, info, req)
		}

		jsonData, err := common.Marshal(convertedRequest)
		if err != nil {
			return types.NewError(err, types.ErrorCodeJsonMarshalFailed, types.ErrOptionWithSkipRetry())
		}

		// remove disabled fields for OpenAI API
		jsonData, err = relaycommon.RemoveDisabledFields(jsonData, info.ChannelOtherSettings, info.ChannelSetting.PassThroughBodyEnabled)
		if err != nil {
			return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}

		// apply param override
		if len(info.ParamOverride) > 0 {
			jsonData, err = relaycommon.ApplyParamOverrideWithRelayInfo(jsonData, info)
			if err != nil {
				return newAPIErrorFromParamOverride(err)
			}
		}

		logger.LogDebug(c, "text request body: %s", jsonData)

		body, closer, err := relaycommon.NewOutboundJSONBody(jsonData)
		if err != nil {
			return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}
		defer closer.Close()
		jsonData = nil
		requestBody = body
	}

	var httpResp *http.Response
	var resp any
	// 非流式请求：给上游首字节（响应头）加可配置超时，超时返回 504 并提示改用流式，
	// 不 skip retry（可换渠道 failover）。流式请求仍由 stream_scanner 的首字超时负责。
	if !info.IsStream && relay_setting.GetNonStreamFirstByteTimeout() > 0 {
		ctx, cancel := context.WithTimeout(c.Request.Context(), time.Duration(relay_setting.GetNonStreamFirstByteTimeout())*time.Second)
		originalRequest := c.Request
		c.Request = c.Request.WithContext(ctx)
		resp, err = adaptor.DoRequest(c, info, requestBody)
		// [修复防御] 首字节已拿到（resp 非 nil 即响应头已到达）：立即解除 deadline，
		// 避免响应体读取被「首字节超时」误杀（上游已返回头但 body 慢/长时被错判 504）。
		// 后续 body 读取只受 http.Client.Timeout（RelayTimeout 总时长）约束。
		if resp != nil {
			cancel()
		}
		c.Request = originalRequest
		defer cancel()
	} else {
		resp, err = adaptor.DoRequest(c, info, requestBody)
	}
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return types.NewOpenAIError(fmt.Errorf("upstream first-byte timeout after %ds (use stream=true for realtime progress)", relay_setting.GetNonStreamFirstByteTimeout()), types.ErrorCodeDoRequestFailed, http.StatusGatewayTimeout)
		}
		return types.NewOpenAIError(err, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
	}

	statusCodeMappingStr := c.GetString("status_code_mapping")

	if resp != nil {
		httpResp = resp.(*http.Response)
		info.IsStream = info.IsStream || strings.HasPrefix(httpResp.Header.Get("Content-Type"), "text/event-stream")
		if httpResp.StatusCode == http.StatusOK {
			// 上游已返回首个响应（流式首个 chunk 已由 stream_scanner 设置，此处兜底非流式）
			info.SetFirstResponseTime()
		}
		if httpResp.StatusCode != http.StatusOK {
			newApiErr := service.RelayErrorHandler(c.Request.Context(), httpResp, false)
			// reset status code 重置状态码
			service.ResetStatusCode(newApiErr, statusCodeMappingStr)
			return newApiErr
		}
	}

	usage, newApiErr := adaptor.DoResponse(c, httpResp, info)
	if newApiErr != nil {
		// reset status code 重置状态码
		service.ResetStatusCode(newApiErr, statusCodeMappingStr)
		return newApiErr
	}

	var containAudioTokens = usage.(*dto.Usage).CompletionTokenDetails.AudioTokens > 0 || usage.(*dto.Usage).PromptTokensDetails.AudioTokens > 0
	var containsAudioRatios = ratio_setting.ContainsAudioRatio(info.OriginModelName) || ratio_setting.ContainsAudioCompletionRatio(info.OriginModelName)

	if containAudioTokens && containsAudioRatios {
		service.PostAudioConsumeQuota(c, info, usage.(*dto.Usage), "")
	} else {
		service.PostTextConsumeQuota(c, info, usage.(*dto.Usage), nil)
	}
	return nil
}

// sanitizeOpenAIPassThroughReasoningEffort 透传模式的 reasoning_effort 归一：
// 解析 JSON 后把顶层 reasoning_effort 交给 sanitizeReasoningEffortForPassthrough
// （on/true->剔除、off/false->none、合法值保留、未知值剔除）；非 JSON/解析失败
// 原样透传（fail-open，不破坏透传语义）。
func sanitizeOpenAIPassThroughReasoningEffort(body io.Reader, relayMode int) io.Reader {
	if relayMode != relayconstant.RelayModeChatCompletions {
		return body
	}
	raw, err := io.ReadAll(body)
	if err != nil {
		return body
	}
	var obj map[string]any
	if err := common.Unmarshal(raw, &obj); err != nil || obj == nil {
		return newBytesReader(raw)
	}
	// [fix-defensive] 归一非标准 reasoning_effort：字符串（on/off/未知值）与
	// bool/number（true/1、false/0）都统一交给 SanitizeEffort；minimal/max 虽
	// 是通用合法枚举，但 channel 38 第三方中转只接受 low/medium/high/xhigh/none，
	// 透传=镜像转发给单一上游，遇不认的枚举必然 400，故透传场景一并剔除（fail-open，
	// 上游用默认）。规避 Phase 1 推演：非 string 类型（bool true）原样透传导致上游
	// 400 "field ReasoningEffort invalid" 的连锁反应。
	sanitizeReasoningKey := func(key string) {
		value, exists := obj[key]
		if !exists {
			return
		}
		var normalized string
		switch v := value.(type) {
		case string:
			normalized = kitreasoning.SanitizeEffort(v)
			// 透传=镜像转发给单一上游：minimal/max 虽为通用合法枚举，但
			// channel 38 第三方中转只接受 low/medium/high/xhigh/none，
			// 保留必 400，故透传场景剔除（上游用默认，不猜语义）。
			if normalized == string(kitreasoning.EffortMinimal) || normalized == string(kitreasoning.EffortMax) {
				normalized = ""
			}
		case bool:
			// true -> 开启思考的通用表达（剔除）；false -> 显式关闭（none）
			if v {
				normalized = ""
			} else {
				normalized = string(kitreasoning.EffortNone)
			}
		case float64:
			// 1/0 数字表达；其他数字视为未知（剔除，避免上游 400）
			if v == 1 {
				normalized = ""
			} else if v == 0 {
				normalized = string(kitreasoning.EffortNone)
			} else {
				normalized = ""
			}
		default:
			// 数组/对象等无法映射的形态：剔除，透传语义由上游默认决定
			normalized = ""
		}
		if normalized == "" {
			delete(obj, key)
		} else {
			obj[key] = normalized
		}
	}
	sanitizeReasoningKey("reasoning_effort")
	sanitizeReasoningKey("ReasoningEffort")
	cleaned, err := common.Marshal(obj)
	if err != nil {
		return newBytesReader(raw)
	}
	return newBytesReader(cleaned)
}
