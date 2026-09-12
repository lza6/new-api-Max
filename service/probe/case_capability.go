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
	"context"
	"fmt"
	"net/http"

	"github.com/lza6/new-api-Max/common"
)

// unmarshalChat 解析 OpenAI chat 响应（probe 包内独立于业务解析，避免
// 业务结构演进影响探测语义）。
func unmarshalChat(raw string, v *chatResponse) error {
	return common.Unmarshal([]byte(raw), v)
}

// CapabilityCase 用例 2 参数真实性：n / stop 是否被静默丢弃。
// 真实支持 n=2 的上游返回 2 个 choices；套壳/降级渠道通常静默降为 1。
type CapabilityCase struct{}

func (CapabilityCase) Name() string    { return "capability" }
func (CapabilityCase) Weight() float64 { return 20 }

func (c CapabilityCase) Run(ctx context.Context, ch *ProbeTarget) CaseResult {
	result := CaseResult{Name: c.Name(), Weight: c.Weight()}
	raw, status, err := chatRequest(ctx, ch, map[string]any{
		"model":      ch.Model,
		"max_tokens": 5,
		"n":          2,
		"stop":       []any{"END"},
		"messages": []map[string]any{
			{"role": "user", "content": "说一个字"},
		},
	})
	if err != nil {
		result.Error = err.Error()
		result.Evidence = truncateEvidence(raw)
		return result
	}
	if status != http.StatusOK {
		// 明确 4xx = 渠道/模型不支持该参数：不算假响应，按半分处理。
		result.Error = fmt.Sprintf("upstream rejected capability params (http %d)", status)
		result.Evidence = truncateEvidence(raw)
		result.Score = c.Weight() / 2
		return result
	}
	var parsed chatResponse
	if err := unmarshalChat(raw, &parsed); err != nil {
		result.Error = err.Error()
		result.Evidence = truncateEvidence(raw)
		return result
	}
	choices := len(parsed.Choices)
	if choices >= 2 {
		result.Passed = true
		result.Score = c.Weight()
	} else {
		result.Error = fmt.Sprintf("n=2 request returned %d choice(s): parameter silently dropped", choices)
	}
	result.Evidence = truncateEvidence(fmt.Sprintf("choices=%d body=%s", choices, raw))
	return result
}
