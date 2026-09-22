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
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lza6/new-api-Max/common"
	relaycommon "github.com/lza6/new-api-Max/relay/common"
	"github.com/lza6/new-api-Max/relaykit/dto"
	"github.com/lza6/new-api-Max/relaykit/relayconvert"
	"github.com/lza6/new-api-Max/relaykit/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gin-gonic/gin"
)

func newRelayInfo(baseURL string, format types.RelayFormat) *relaycommon.RelayInfo {
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}
	info.ChannelBaseUrl = baseURL
	info.ChannelType = 62
	info.UpstreamModelName = "kilwa-grok"
	info.RelayFormat = format
	return info
}

func newUpstreamServer(t *testing.T, reply string, wantStatus string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/kilwa-grok", r.URL.Path)
		require.Equal(t, "ping", r.URL.Query().Get("text"))
		payload := `{"status":` + wantStatus + `,"model":"🌌 KILWA GROK","reply":` + quote(reply) + `}`
		if wantStatus == `"error"` {
			payload = `{"status":"error","message":"boom"}`
		}
		_, _ = io.WriteString(w, payload)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func quote(s string) string {
	b, err := common.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func TestExtractPromptUsesLastUserMessage(t *testing.T) {
	req := &dto.GeneralOpenAIRequest{
		Messages: []dto.Message{
			{Role: "system", Content: "ignore me"},
			{Role: "user", Content: "first"},
			{Role: "assistant", Content: "second"},
			{Role: "user", Content: "final prompt"},
		},
	}
	prompt, err := extractPrompt(req)
	require.NoError(t, err)
	assert.Equal(t, "final prompt", prompt)
}

func TestExtractPromptFallsBackToAnyContent(t *testing.T) {
	req := &dto.GeneralOpenAIRequest{
		Messages: []dto.Message{{Role: "assistant", Content: "hello world"}},
	}
	prompt, err := extractPrompt(req)
	require.NoError(t, err)
	assert.Equal(t, "hello world", prompt)
}

func TestConvertOpenAIRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	converted, err := (&Adaptor{}).ConvertOpenAIRequest(c, newRelayInfo("", types.RelayFormatOpenAI), &dto.GeneralOpenAIRequest{
		Messages: []dto.Message{{Role: "user", Content: "ping"}},
	})
	require.NoError(t, err)
	kr, ok := converted.(*kilwaRequest)
	require.True(t, ok)
	assert.Equal(t, "ping", kr.Prompt)
}

func TestConvertClaudeRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	maxTokens := uint(128)
	converted, err := (&Adaptor{}).ConvertClaudeRequest(c, newRelayInfo("", types.RelayFormatClaude), &dto.ClaudeRequest{
		Model:     "kilwa-grok",
		MaxTokens: &maxTokens,
		Messages: []dto.ClaudeMessage{
			{Role: "user", Content: "ping"},
		},
	})
	require.NoError(t, err)
	kr, ok := converted.(*kilwaRequest)
	require.True(t, ok)
	assert.Contains(t, kr.Prompt, "ping")
}

func TestConvertResponsesRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	responsesReq := dto.OpenAIResponsesRequest{
		Model: "kilwa-grok",
		Input: json.RawMessage(`[{"role":"user","content":"ping"}]`),
	}
	converted, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(c, newRelayInfo("", types.RelayFormatOpenAIResponses), responsesReq)
	require.NoError(t, err)
	kr, ok := converted.(*kilwaRequest)
	require.True(t, ok)
	assert.Equal(t, "ping", kr.Prompt)
}

func TestDoRequestBuildsGETAndStoresReply(t *testing.T) {
	srv := newUpstreamServer(t, "hello back", `"success"`)
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil)
	raw, err := common.Marshal(&kilwaRequest{Prompt: "ping"})
	require.NoError(t, err)
	a := &Adaptor{}
	result, err := a.DoRequest(c, newRelayInfo(srv.URL, types.RelayFormatOpenAI), bytes.NewReader(raw))
	require.NoError(t, err)
	resp, ok := result.(*http.Response)
	require.True(t, ok)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "hello back", a.reply)
}

func TestDoRequestRejectsUpstreamFailure(t *testing.T) {
	srv := newUpstreamServer(t, "", `"error"`)
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil)
	raw, err := common.Marshal(&kilwaRequest{Prompt: "ping"})
	require.NoError(t, err)
	_, err = (&Adaptor{}).DoRequest(c, newRelayInfo(srv.URL, types.RelayFormatOpenAI), bytes.NewReader(raw))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom")
}

func TestDoResponseOpenAIJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	usage, apiErr := (&Adaptor{reply: "hi"}).DoResponse(c, &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(""))}, newRelayInfo("", types.RelayFormatOpenAI))
	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"chat.completion"`)
	assert.Contains(t, w.Body.String(), `"content":"hi"`)
}

func TestResponsesRequestBridgeProducesPrompt(t *testing.T) {
	req := &dto.OpenAIResponsesRequest{
		Model: "m",
		Input: json.RawMessage(`[{"role":"user","content":"ping"}]`),
	}
	chat, err := relayconvert.ResponsesRequestToChatCompletionsRequest(req)
	require.NoError(t, err)
	require.NotEmpty(t, chat.Messages)
	text, err := extractPrompt(chat)
	require.NoError(t, err)
	assert.Equal(t, "ping", text)
}

func TestDoResponseClaudeStreamFullEventSequence(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	info := newRelayInfo("", types.RelayFormatClaude)
	info.IsStream = true
	a := &Adaptor{reply: "hi"}
	oai := &dto.OpenAITextResponse{Id: "chatcmpl-1", Model: "kilwa-grok", Object: "chat.completion", Created: int64(1), Choices: []dto.OpenAITextResponseChoice{{Index: 0, Message: dto.Message{Role: "assistant", Content: "hi"}, FinishReason: "stop"}}}
	err := a.doClaudeResponse(c, info, oai, "chatcmpl-1", "kilwa-grok", 1)
	require.Nil(t, err)
	body := w.Body.String()
	for _, ev := range []string{"message_start", "content_block_start", "content_block_delta", "content_block_stop", "message_delta", "message_stop"} {
		assert.Contains(t, body, "event: "+ev, "missing claude stream event %s", ev)
	}
}

func TestGetRequestURLSelectsPathByModel(t *testing.T) {
	gin.SetMode(gin.TestMode)

	grok := newRelayInfo("https://up.example", types.RelayFormatOpenAI)
	grok.UpstreamModelName = "kilwa-grok"
	url, err := (&Adaptor{}).GetRequestURL(grok)
	require.NoError(t, err)
	assert.Contains(t, url, "/kilwa-grok")

	claude := newRelayInfo("https://up.example", types.RelayFormatOpenAI)
	claude.UpstreamModelName = "kilwa-claude"
	url, err = (&Adaptor{}).GetRequestURL(claude)
	require.NoError(t, err)
	assert.Contains(t, url, "/kilwa-claude")
	assert.NotContains(t, url, "/kilwa-grok")

	unknown := newRelayInfo("https://up.example", types.RelayFormatOpenAI)
	unknown.UpstreamModelName = "kilwa-other"
	url, err = (&Adaptor{}).GetRequestURL(unknown)
	require.NoError(t, err)
	assert.Contains(t, url, "/kilwa-grok")
}
