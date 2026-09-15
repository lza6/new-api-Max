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
)

// BillingConsistencyCase 用例6 计费一致性：usage 中的
// prompt_tokens + completion_tokens 应与 total_tokens 自洽（允许容差 1），
// 且三者都应 >0（真实计费而非虚报）。套壳/虚报缓存渠道常在此显形。
type BillingConsistencyCase struct{}

func (BillingConsistencyCase) Name() string    { return "billing_consistency" }
func (BillingConsistencyCase) Weight() float64 { return 10 }

func (c BillingConsistencyCase) Run(ctx context.Context, ch *ProbeTarget) CaseResult {
	result := CaseResult{Name: c.Name(), Weight: c.Weight()}
	_, usage, raw, err := chatWithUsage(ctx, ch, "你是简洁的助手。", "说一句问候。", 40)
	if err != nil {
		result.Error = "usage call: " + err.Error()
		result.Evidence = truncateEvidence(raw)
		return result
	}
	if usage == nil {
		result.Error = "upstream returned no usage"
		result.Evidence = truncateEvidence(raw)
		return result
	}
	evidence := fmt.Sprintf("prompt=%d completion=%d total=%d cached=%d",
		usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens, usage.CachedTokens)
	result.Evidence = truncateEvidence(evidence)

	if usage.PromptTokens <= 0 || usage.CompletionTokens <= 0 || usage.TotalTokens <= 0 {
		result.Error = "usage contains zero/negative token counts"
		return result
	}
	sum := usage.PromptTokens + usage.CompletionTokens
	d := sum - usage.TotalTokens
	if d < 0 {
		d = -d
	}
	if d > 1 {
		result.Error = fmt.Sprintf("usage inconsistent: prompt+completion=%d != total=%d", sum, usage.TotalTokens)
		return result
	}
	result.Passed = true
	result.Score = c.Weight()
	return result
}
