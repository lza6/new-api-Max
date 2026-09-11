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

// CacheCase 用例 3 缓存真实性：同前缀两次请求，检查第二次响应是否真的
// 计入 cache token（usage.prompt_tokens_details.cached_tokens > 0）。
// 不支持缓存的渠道返回 usage 无 cached_tokens 字段 → 按半分（不算作假）。
type CacheCase struct{}

func (CacheCase) Name() string    { return "cache" }
func (CacheCase) Weight() float64 { return 20 }

// cacheProbePrefix 重复的长前缀，最大化命中 prompt cache 的概率。
const cacheProbePrefix = "背景设定（请记住，之后会用到）：星河号飞船的乘员共 7 人，" +
	"船长林远、领航员苏晴、工程师赵石、医生何静、通讯员陈默、" +
	"植物学家周蕊、机械师高翔。飞船于 2147 年离开地球。"

func (c CacheCase) Run(ctx context.Context, ch *ProbeTarget) CaseResult {
	result := CaseResult{Name: c.Name(), Weight: c.Weight()}
	messages := []map[string]any{
		{"role": "user", "content": cacheProbePrefix + "\n\n只回答一个字：好"},
	}
	base := map[string]any{
		"model":      ch.Model,
		"max_tokens": 5,
		"messages":   messages,
	}
	// 第一次请求（写缓存）。
	_, _, err := chatRequest(ctx, ch, base)
	if err != nil {
		result.Error = "first call: " + err.Error()
		return result
	}
	// 第二次请求（应命中缓存）。
	raw, status, err := chatRequest(ctx, ch, base)
	if err != nil {
		result.Error = "second call: " + err.Error()
		return result
	}
	if status != 200 {
		result.Error = fmt.Sprintf("second call http %d", status)
		result.Evidence = truncateEvidence(raw)
		return result
	}
	var parsed chatResponse
	if err := unmarshalChat(raw, &parsed); err != nil {
		result.Error = err.Error()
		result.Evidence = truncateEvidence(raw)
		return result
	}
	if parsed.Usage == nil {
		result.Error = "second response has no usage"
		result.Evidence = truncateEvidence(raw)
		return result
	}
	cached := 0
	if parsed.Usage.PromptTokensDetails != nil {
		cached = parsed.Usage.PromptTokensDetails.CachedTokens
	}
	if cached > 0 {
		result.Passed = true
		result.Score = c.Weight()
		result.Evidence = truncateEvidence(fmt.Sprintf("cached_tokens=%d prompt_tokens=%d", cached, parsed.Usage.PromptTokens))
		return result
	}
	// 渠道不回传缓存明细：无法证伪，按半分。
	result.Score = c.Weight() / 2
	result.Error = "no cached_tokens in usage (cache unsupported or not reported)"
	result.Evidence = truncateEvidence(raw)
	return result
}
