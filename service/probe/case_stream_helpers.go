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

package probe

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// streamChat 发送流式 chat 请求，返回按顺序收集的所有 data 行（SSE）。
// 用于流完整性用例：校验渠道是否真的以增量形式返回内容。
func streamChat(ctx context.Context, ch *ProbeTarget, system, user string, maxTokens int) ([]string, string, error) {
	payload, err := commonMarshal(map[string]any{
		"model":      ch.Model,
		"max_tokens": maxTokens,
		"stream":     true,
		"messages": []map[string]any{
			{"role": "system", "content": system},
			{"role": "user", "content": user},
		},
	})
	if err != nil {
		return nil, "", err
	}
	base := strings.TrimRight(ch.BaseURL, "/")
	if base == "" {
		return nil, "", errEmptyBaseURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v1/chat/completions", strings.NewReader(payload))
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ch.Key)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, rerr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if rerr != nil {
			return nil, "", rerr
		}
		return nil, string(raw), errUpstreamHTTP(resp.StatusCode)
	}

	var chunks []string
	scanner := bufio.NewScanner(io.LimitReader(resp.Body, 1<<20))
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "data:") {
			continue
		}
		payload = strings.TrimSpace(strings.TrimPrefix(trimmed, "data:"))
		if payload == "[DONE]" || payload == "" {
			continue
		}
		chunks = append(chunks, payload)
		if len(chunks) >= 32 {
			break // 上限，防止异常渠道刷流
		}
	}
	if err := scanner.Err(); err != nil {
		return chunks, "", err
	}
	return chunks, "", nil
}

// chatWithUsage 发送一次 chat 请求并返回内容、usage 与原始响应。
// 供计费一致性用例校验 usage（prompt/completion/total）真实性。
func chatWithUsage(ctx context.Context, ch *ProbeTarget, system, user string, maxTokens int) (string, *usageInfo, string, error) {
	raw, status, err := chatRequest(ctx, ch, map[string]any{
		"model":      ch.Model,
		"max_tokens": maxTokens,
		"messages": []map[string]any{
			{"role": "system", "content": system},
			{"role": "user", "content": user},
		},
	})
	if err != nil {
		return "", nil, raw, err
	}
	if status != http.StatusOK {
		return "", nil, raw, errUpstreamHTTP(status)
	}
	var parsed chatResponse
	if err := commonUnmarshal([]byte(raw), &parsed); err != nil {
		return "", nil, raw, err
	}
	if len(parsed.Choices) == 0 {
		return "", nil, raw, errNoChoices
	}
	content := parsed.Choices[0].Message.Content
	var usage *usageInfo
	if parsed.Usage != nil {
		usage = &usageInfo{
			PromptTokens:     parsed.Usage.PromptTokens,
			CompletionTokens: parsed.Usage.CompletionTokens,
			TotalTokens:      parsed.Usage.TotalTokens,
			CachedTokens:     0,
		}
		if parsed.Usage.PromptTokensDetails != nil {
			usage.CachedTokens = parsed.Usage.PromptTokensDetails.CachedTokens
		}
	}
	return content, usage, raw, nil
}

// usageInfo 一次响应的 token 使用情况（计费一致性用例）。
type usageInfo struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	CachedTokens     int
}

var (
	errEmptyBaseURL = errMsg("channel base_url empty")
	errNoChoices    = errMsg("response has no choices")
)

func errMsg(m string) error { return &probeErr{m} }

type probeErr struct{ m string }

func (e *probeErr) Error() string { return e.m }

func errUpstreamHTTP(code int) error { return &probeErr{fmt.Sprintf("upstream http %d", code)} }
