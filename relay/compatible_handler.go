package relay

import (
	"bytes"
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
		// [fix] 透传路径也必须注入渠道级系统提示词：否则「渠道已配 system_prompt +
		// 开启请求体透传」时系统提示词被静默丢弃（用户反馈「渠道内置提示词不生效」，
		// channel 50 pass_through_body_enabled=true 实测）。注入是对请求体做一次
		// JSON 就地改写（把 system message 前插/合并），再原样透传其余字段。
		if info.ChannelSetting.SystemPrompt != "" {
			if injected, iErr := injectSystemPromptIntoPassThroughBody(storage, info); iErr != nil {
				logger.LogError(c, "failed to inject channel system prompt into pass-through body: "+iErr.Error())
				// 注入失败：退回原始透传体（不阻断请求，仅告警）。
				requestBody = sanitizeOpenAIPassThroughReasoningEffort(common.NewReplayableBodyReader(storage), info.RelayMode)
			} else {
				requestBody = injected
			}
		} else {
			// [fix-defensive] OpenAI 透传路径：归一非标准 reasoning_effort（on/true
			// -> 剔除、off/false -> none）。透传体原样转发，但非法 effort 会触发上游
			// 400 "field ReasoningEffort invalid"（channel 38 第三方中转实锤）。
			requestBody = sanitizeOpenAIPassThroughReasoningEffort(common.NewReplayableBodyReader(storage), info.RelayMode)
		}
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
		resp, err = doRequestWithResponseCache(c, info, adaptor, requestBody)
		// [修复防御] 首字节已拿到（resp 非 nil 即响应头已到达）：立即解除 deadline，
		// 避免响应体读取被「首字节超时」误杀（上游已返回头但 body 慢/长时被错判 504）。
		// 后续 body 读取只受 http.Client.Timeout（RelayTimeout 总时长）约束。
		if resp != nil {
			cancel()
		}
		c.Request = originalRequest
		defer cancel()
	} else {
		resp, err = doRequestWithResponseCache(c, info, adaptor, requestBody)
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
	// [性能] 大 prompt 透传优化：绝大多数请求体不含 reasoning_effort。先做
	// 流式探测（可回放 body 无需整体入内存），不含键时**原样返回**——既不
	// ReadAll 几十 MB，也不 Unmarshal 成 map 再 Marshal 回来。语义与「解析后
	// 发现无键」完全一致（不改变任何字段）。
	if replayable, ok := body.(common.ReplayableBody); ok {
		hasKey, scanErr := replayableContainsReasoningEffortKey(replayable)
		if scanErr == nil && !hasKey {
			return body
		}
	}
	raw, err := io.ReadAll(body)
	if err != nil {
		return body
	}
	if !bytesContainsReasoningEffortKey(raw) {
		return newBytesReader(raw)
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

// bytesContainsReasoningEffortKey 字节级探测请求体是否可能含 reasoning_effort
// 键（大小写两种拼写）。仅用于「快速跳过无关键的大 body」，因此只需无假阴性：
// 命中任何候选拼写即返回 true（宁可多走一次解析，也不能漏掉真实键）。
func bytesContainsReasoningEffortKey(raw []byte) bool {
	return bytes.Contains(raw, []byte("reasoning_effort")) ||
		bytes.Contains(raw, []byte("ReasoningEffort"))
}

// replayableContainsReasoningEffortKey 流式探测可回放请求体是否含 reasoning_effort
// 键，全程只持有固定大小窗口，不把大 body 整体读入内存。使用独立 reader，
// 不扰动原 body 的游标。扫描失败返回 error，调用方回退到常规路径。
func replayableContainsReasoningEffortKey(body common.ReplayableBody) (bool, error) {
	reader, err := body.NewReader()
	if err != nil {
		return false, err
	}
	defer reader.Close()

	// 键最长 16 字节（"ReasoningEffort"），窗口留 2 倍余量避免跨块边界漏配。
	const window = 32
	needleLower := []byte("reasoning_effort")
	needleUpper := []byte("ReasoningEffort")
	carry := make([]byte, 0, window)
	chunk := make([]byte, window)

	for {
		n, readErr := reader.Read(chunk)
		if n > 0 {
			// 拼上一块的尾部窗口，避免键被块边界截断而漏配。
			windowBytes := make([]byte, 0, len(carry)+n)
			windowBytes = append(windowBytes, carry...)
			windowBytes = append(windowBytes, chunk[:n]...)
			if bytes.Contains(windowBytes, needleLower) || bytes.Contains(windowBytes, needleUpper) {
				return true, nil
			}
			if len(windowBytes) > window {
				carry = append(carry[:0], windowBytes[len(windowBytes)-window:]...)
			} else {
				carry = append(carry[:0], windowBytes...)
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return false, nil
			}
			return false, readErr
		}
	}
}
