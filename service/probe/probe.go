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

// Package probe B4-1 渠道验真探测题库引擎（思想迁自 casbin-gateway
// probe_case_builtin，代码按本项目风格重写）。
// 目标：把「渠道可信度」从人工猜测量化为 A-F 分。每个用例向渠道发起
// 真实（最便宜档）OpenAI 兼容 chat 请求并校验响应证据。
//
// 探测成本控制（指南默认值）：每渠道每日 1 轮；单轮 ≤10 次调用；
// 探测用最小 max_tokens；仅管理员手动「立即探测」可超频。
package probe

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/lza6/new-api-Max/common"
)

// CaseResult 单个用例结果。
type CaseResult struct {
	Name     string  `json:"name"`
	Passed   bool    `json:"passed"`
	Score    float64 `json:"score"`
	Weight   float64 `json:"weight"`
	Evidence string  `json:"evidence"`
	Error    string  `json:"error,omitempty"`
}

// ProbeCase 统一用例接口。
type ProbeCase interface {
	Name() string
	Weight() float64
	Run(ctx context.Context, ch *ProbeTarget) CaseResult
}

// ProbeTarget 探测目标（从渠道行解出，含凭据，绝不入日志/证据）。
type ProbeTarget struct {
	ChannelID   int
	Name        string
	BaseURL     string
	Key         string
	Model       string // 探测用模型（渠道 models 列表第一个，或管理员指定）
	TimeoutSecs int
}

// MaxEvidenceBytes 证据截断上限 2KB。
const MaxEvidenceBytes = 2048

// httpClient 探测共享 client（与业务 relay client 隔离，防长连接污染）。
var httpClient = &http.Client{Timeout: 60 * time.Second}

// chatRequest 向渠道发一次 OpenAI 兼容 chat 请求，返回响应体文本与状态码。
// 所有用例的统一传输层；错误带归因（不含 key）。
func chatRequest(ctx context.Context, ch *ProbeTarget, body map[string]any) (string, int, error) {
	base := strings.TrimRight(ch.BaseURL, "/")
	if base == "" {
		return "", 0, fmt.Errorf("channel base_url empty")
	}
	url := base + "/v1/chat/completions"
	payload, err := common.Marshal(body)
	if err != nil {
		return "", 0, fmt.Errorf("marshal probe request failed: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ch.Key)

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", resp.StatusCode, fmt.Errorf("read body failed: %w", err)
	}
	return string(raw), resp.StatusCode, nil
}

// chatCall 构造最小 chat 请求并解析出首个 choice 的文本内容。
func chatCall(ctx context.Context, ch *ProbeTarget, system, user string, maxTokens int, extra map[string]any) (string, string, error) {
	body := map[string]any{
		"model":      ch.Model,
		"max_tokens": maxTokens,
		"messages": []map[string]any{
			{"role": "system", "content": system},
			{"role": "user", "content": user},
		},
	}
	for k, v := range extra {
		body[k] = v
	}
	raw, status, err := chatRequest(ctx, ch, body)
	if err != nil {
		return "", raw, err
	}
	if status != http.StatusOK {
		return "", raw, fmt.Errorf("upstream http %d", status)
	}
	content, perr := parseChatContent(raw)
	if perr != nil {
		return "", raw, perr
	}
	return content, raw, nil
}

// chatResponse OpenAI chat 响应的最小解析结构。
type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens        int `json:"prompt_tokens"`
		CompletionTokens    int `json:"completion_tokens"`
		TotalTokens         int `json:"total_tokens"`
		PromptTokensDetails *struct {
			CachedTokens int `json:"cached_tokens"`
		} `json:"prompt_tokens_details"`
	} `json:"usage"`
}

// parseChatContent 从响应体解析首个 choice 文本。
func parseChatContent(raw string) (string, error) {
	var parsed chatResponse
	if err := common.Unmarshal([]byte(raw), &parsed); err != nil {
		return "", fmt.Errorf("response not valid chat json: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("response has no choices")
	}
	return parsed.Choices[0].Message.Content, nil
}

// truncateEvidence 证据截断到 2KB。
func truncateEvidence(s string) string {
	if len(s) <= MaxEvidenceBytes {
		return s
	}
	return s[:MaxEvidenceBytes] + "...(truncated)"
}
