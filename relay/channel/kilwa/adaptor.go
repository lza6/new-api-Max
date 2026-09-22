/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
package kilwa

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/logger"
	relaycommon "github.com/lza6/new-api-Max/relay/common"
	"github.com/lza6/new-api-Max/relay/helper"
	"github.com/lza6/new-api-Max/relaykit/dto"
	"github.com/lza6/new-api-Max/relaykit/relayconvert"
	"github.com/lza6/new-api-Max/relaykit/types"
	"github.com/lza6/new-api-Max/service"

	"github.com/gin-gonic/gin"
)

const ChannelName = "Kilwa"

var ModelList = []string{"kilwa-grok", "kilwa-claude"}

const (
	kilwaPath       = "/kilwa-grok"
	kilwaPathClaude = "/kilwa-claude"
	kilwaTimeout    = 120 * time.Second
)

// kilwaRequest is the normalized upstream payload produced by every request
// converter. It is marshaled by the relay core and consumed by DoRequest.
type kilwaRequest struct {
	Prompt string `json:"prompt"`
}

// kilwaResponse mirrors the upstream JSON body.
type kilwaResponse struct {
	Status  string `json:"status"`
	Model   string `json:"model"`
	Reply   string `json:"reply"`
	Owner   string `json:"owner"`
	Channel string `json:"channel"`
	Message string `json:"message,omitempty"`
}

type Adaptor struct {
	reply string
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	path := kilwaPath
	if info.UpstreamModelName == "kilwa-claude" {
		path = kilwaPathClaude
	}
	return relaycommon.GetFullRequestURL(info.ChannelBaseUrl, path, info.ChannelType), nil
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	return nil // KILWA is a public GET endpoint, no auth header
}

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	prompt, err := extractPrompt(request)
	if err != nil {
		return nil, err
	}
	return &kilwaRequest{Prompt: prompt}, nil
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ClaudeRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	openAIRequest, err := relayconvert.ClaudeMessagesRequestToOpenAIChat(*request, info)
	if err != nil {
		return nil, err
	}
	prompt, err := extractPrompt(openAIRequest)
	if err != nil {
		return nil, err
	}
	return &kilwaRequest{Prompt: prompt}, nil
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	openAIRequest, err := relayconvert.ResponsesRequestToChatCompletionsRequest(&request)
	if err != nil {
		return nil, err
	}
	prompt, err := extractPrompt(openAIRequest)
	if err != nil {
		return nil, err
	}
	return &kilwaRequest{Prompt: prompt}, nil
}

func (a *Adaptor) ConvertGeminiRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeminiChatRequest) (any, error) {
	return nil, errors.New("endpoint not supported")
}

func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return nil, errors.New("endpoint not supported")
}

func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, errors.New("endpoint not supported")
}

func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	return nil, errors.New("endpoint not supported")
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	return nil, errors.New("endpoint not supported")
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	body, err := io.ReadAll(requestBody)
	if err != nil {
		return nil, err
	}
	// 兼容多种被转发 body：本适配器转换产物（kilwaRequest）、原始 OpenAI/
	// Claude/Responses 请求（passthrough 或协议桥接时），统一提取提示词。
	logger.LogWarn(c, "kilwa request body: %.300s", string(body))
	var kr kilwaRequest
	if err := common.Unmarshal(body, &kr); err == nil && strings.TrimSpace(kr.Prompt) != "" {
		// use kilwaRequest
	} else if prompt, err := extractPromptFromRaw(body); err == nil {
		kr.Prompt = prompt
	} else {
		return nil, fmt.Errorf("kilwa: no prompt in request body (%v)", err)
	}

	fullURL, err := a.GetRequestURL(info)
	if err != nil {
		return nil, err
	}
	parsed, err := url.Parse(fullURL)
	if err != nil {
		return nil, err
	}
	query := parsed.Query()
	query.Set("text", kr.Prompt)
	parsed.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: kilwaTimeout}
	upstreamResp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer upstreamResp.Body.Close()

	if upstreamResp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(upstreamResp.Body, 4096))
		logger.LogWarn(c, "kilwa upstream status %d: %s", upstreamResp.StatusCode, string(raw))
		return nil, fmt.Errorf("kilwa upstream returned status %d", upstreamResp.StatusCode)
	}

	raw, err := io.ReadAll(io.LimitReader(upstreamResp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	var krResp kilwaResponse
	if err := common.Unmarshal(raw, &krResp); err != nil {
		return nil, fmt.Errorf("invalid kilwa response: %w", err)
	}
	if krResp.Status != "success" {
		message := krResp.Message
		if message == "" {
			message = "kilwa upstream error"
		}
		return nil, errors.New(message)
	}
	if strings.TrimSpace(krResp.Reply) == "" {
		return nil, errors.New("kilwa returned an empty reply")
	}
	a.reply = krResp.Reply
	logger.LogDebug(c, "kilwa reply: %.120s", a.reply)
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader("")),
	}, nil
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	model := info.UpstreamModelName
	if model == "" {
		model = "kilwa-grok"
	}
	created := time.Now().Unix()
	id := fmt.Sprintf("chatcmpl-%d", created)

	oai := &dto.OpenAITextResponse{
		Id:      id,
		Model:   model,
		Object:  "chat.completion",
		Created: created,
		Choices: []dto.OpenAITextResponseChoice{{
			Index: 0,
			Message: dto.Message{
				Role:    "assistant",
				Content: a.reply,
			},
			FinishReason: "stop",
		}},
		Usage: dto.Usage{},
	}

	usage = &dto.Usage{}
	switch info.RelayFormat {
	case types.RelayFormatClaude:
		return usage, a.doClaudeResponse(c, info, oai, id, model, created)
	case types.RelayFormatOpenAIResponses:
		return usage, a.doResponsesResponse(c, info, oai, id, model, created)
	default:
		return usage, a.doOpenAIResponse(c, info, oai, id, model, created)
	}
}

func (a *Adaptor) doOpenAIResponse(c *gin.Context, info *relaycommon.RelayInfo, oai *dto.OpenAITextResponse, id, model string, created int64) *types.NewAPIError {
	if !info.IsStream {
		c.JSON(http.StatusOK, oai)
		return nil
	}
	helper.SetEventStreamHeaders(c)
	roleChunk := &dto.ChatCompletionsStreamResponse{
		Id:      id,
		Object:  "chat.completion.chunk",
		Created: created,
		Model:   model,
		Choices: []dto.ChatCompletionsStreamResponseChoice{{
			Index: 0,
			Delta: dto.ChatCompletionsStreamResponseChoiceDelta{Role: "assistant"},
		}},
	}
	if err := helper.ObjectData(c, roleChunk); err != nil {
		return types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	stop := "stop"
	content := a.reply
	contentChunk := &dto.ChatCompletionsStreamResponse{
		Id:      id,
		Object:  "chat.completion.chunk",
		Created: created,
		Model:   model,
		Choices: []dto.ChatCompletionsStreamResponseChoice{{
			Index:        0,
			Delta:        dto.ChatCompletionsStreamResponseChoiceDelta{Content: &content},
			FinishReason: &stop,
		}},
	}
	if err := helper.ObjectData(c, contentChunk); err != nil {
		return types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	helper.Done(c)
	return nil
}

func (a *Adaptor) doClaudeResponse(c *gin.Context, info *relaycommon.RelayInfo, oai *dto.OpenAITextResponse, id, model string, created int64) *types.NewAPIError {
	if !info.IsStream {
		claudeResp := service.ResponseOpenAI2Claude(oai, info)
		c.JSON(http.StatusOK, claudeResp)
		return nil
	}
	// 上游为单次回复，无法透传增量流：手动构造完整 Anthropic SSE 事件序列
	// （message_start -> content_block_start -> content_block_delta ->
	//  content_block_stop -> message_delta -> message_stop），保证 Claude Code
	// 等客户端正常收尾。
	helper.SetEventStreamHeaders(c)
	index := 0
	stopReason := "end_turn"
	empty := ""
	msgID := id
	if !strings.HasPrefix(msgID, "msg_") {
		msgID = "msg_" + strings.TrimPrefix(id, "chatcmpl-")
	}
	startMessage := &dto.ClaudeMediaMessage{
		Id:    msgID,
		Model: model,
		Type:  "message",
		Role:  "assistant",
		Usage: &dto.ClaudeUsage{InputTokens: 0, OutputTokens: 0},
	}
	startMessage.SetContent(make([]any, 0))
	events := []dto.ClaudeResponse{
		{Type: "message_start", Message: startMessage},
		{Type: "content_block_start", Index: &index, ContentBlock: &dto.ClaudeMediaMessage{Type: "text", Text: &empty}},
		{Type: "content_block_delta", Index: &index, Delta: &dto.ClaudeMediaMessage{Type: "text_delta", Text: &a.reply}},
		{Type: "content_block_stop", Index: &index},
		{Type: "message_delta", Delta: &dto.ClaudeMediaMessage{Type: "message_delta", StopReason: &stopReason}, Usage: &dto.ClaudeUsage{OutputTokens: 0}},
		{Type: "message_stop"},
	}
	for _, ev := range events {
		if err := helper.ClaudeData(c, ev); err != nil {
			return types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
		}
	}
	return nil
}

func (a *Adaptor) doResponsesResponse(c *gin.Context, info *relaycommon.RelayInfo, oai *dto.OpenAITextResponse, id, model string, created int64) *types.NewAPIError {
	if !info.IsStream {
		responsesResp, _, err := relayconvert.ChatCompletionsResponseToResponsesResponse(oai, id)
		if err != nil {
			return types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
		}
		c.JSON(http.StatusOK, responsesResp)
		return nil
	}
	helper.SetEventStreamHeaders(c)
	stop := "stop"
	content := a.reply
	chunk := &dto.ChatCompletionsStreamResponse{
		Id:      id,
		Object:  "chat.completion.chunk",
		Created: created,
		Model:   model,
		Choices: []dto.ChatCompletionsStreamResponseChoice{{
			Index:        0,
			Delta:        dto.ChatCompletionsStreamResponseChoiceDelta{Content: &content},
			FinishReason: &stop,
		}},
	}
	state := relayconvert.NewChatToResponsesStreamState(id, model)
	events, err := relayconvert.ChatCompletionsStreamChunkToResponsesEvents(chunk, state)
	if err != nil {
		return types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	events = append(events, relayconvert.FinalizeChatCompletionsStreamToResponses(state)...)
	for _, event := range events {
		data, err := common.Marshal(event.Payload)
		if err != nil {
			return types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
		}
		if err := helper.ResponseChunkData(c, dto.ResponsesStreamResponse{Type: event.Type}, string(data)); err != nil {
			return types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
		}
	}
	return nil
}

func (a *Adaptor) GetModelList() []string {
	return ModelList
}

func (a *Adaptor) GetChannelName() string {
	return ChannelName
}

// extractPromptFromRaw handles raw request bodies that bypass Convert*Request
// (global/channel passthrough or protocol bridges): OpenAI chat, Claude, Responses.
func extractPromptFromRaw(body []byte) (string, error) {
	var openAI dto.GeneralOpenAIRequest
	if err := common.Unmarshal(body, &openAI); err == nil && len(openAI.Messages) > 0 {
		return extractPrompt(&openAI)
	}
	var claudeReq dto.ClaudeRequest
	if err := common.Unmarshal(body, &claudeReq); err == nil && len(claudeReq.Messages) > 0 {
		for i := len(claudeReq.Messages) - 1; i >= 0; i-- {
			message := claudeReq.Messages[i]
			if message.Role != "user" {
				continue
			}
			text := claudeMessageText(message.Content)
			if text != "" {
				return text, nil
			}
		}
	}
	var responsesReq dto.OpenAIResponsesRequest
	if err := common.Unmarshal(body, &responsesReq); err == nil {
		chat, convErr := relayconvert.ResponsesRequestToChatCompletionsRequest(&responsesReq)
		if convErr == nil && len(chat.Messages) > 0 {
			return extractPrompt(chat)
		}
	}
	return "", errors.New("unsupported request body shape")
}

func claudeMessageText(content any) string {
	switch v := content.(type) {
	case string:
		return v
	case []any:
		var builder strings.Builder
		for _, item := range v {
			if block, ok := item.(map[string]any); ok {
				if block["type"] == "text" {
					if text, ok := block["text"].(string); ok {
						builder.WriteString(text)
					}
				}
			}
		}
		return builder.String()
	}
	return ""
}

// extractPrompt picks the last non-empty user text message as the upstream prompt.
func extractPrompt(request *dto.GeneralOpenAIRequest) (string, error) {
	if request == nil {
		return "", errors.New("request is nil")
	}
	if len(request.Messages) == 0 {
		return "", errors.New("no messages in request")
	}
	var fallback string
	for i := len(request.Messages) - 1; i >= 0; i-- {
		message := request.Messages[i]
		text := message.StringContent()
		if text == "" {
			continue
		}
		if message.Role == "user" {
			return text, nil
		}
		if fallback == "" {
			fallback = text
		}
	}
	if fallback != "" {
		return fallback, nil
	}
	return "", errors.New("empty prompt")
}
