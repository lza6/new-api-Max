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
// shadow_price.go 提供 B5-2 微美元影子价（api_equivalent_usd）口径。
//
// 设计约束（安全）：
//   - 影子价只做统计口径，绝不参与 quota 计算（quota 换算仍走 common/quota_math.go）。
//   - 价格为 "API 等效美元"：若该请求走公开 API 按官方 USD 单价应花费多少（微美元）。
//   - 未收录定价的模型返回 (0, false)：不估算、不编造，前端如实显示"未收录"。
//   - 计算用饱和 clamp，禁止溢出（单值边界保守）。
package model

// usdPricePerMillion 一个模型的官方 USD 单价（美元 / 百万 tokens）。
type usdPricePerMillion struct {
	Input  float64 // 输入单价
	Output float64 // 输出单价
}

// modelShadowPrices 内置已确认公开 USD 定价的模型表。
// 仅收录可核实的官方/公开价格，注释标注来源；新模型由 B5-4 模型目录同步
// 扩展（届时影子价优先读取价格表 USD 字段，此表为 fallback）。
// 单位：美元 / 1M tokens。
var modelShadowPrices = map[string]usdPricePerMillion{
	// OpenAI gpt-4o：$2.50 / $10.00 per 1M
	"gpt-4o": {Input: 2.50, Output: 10.00},
	// OpenAI gpt-4o-mini：$0.15 / $0.60 per 1M
	"gpt-4o-mini": {Input: 0.15, Output: 0.60},
	// OpenAI o3-mini：$1.10 / $4.40 per 1M
	"o3-mini": {Input: 1.10, Output: 4.40},
	// DeepSeek V3（deepseek-chat）：$0.27 / $1.10 per 1M；缓存命中输入 $0.07
	"deepseek-chat": {Input: 0.27, Output: 1.10},
	// DeepSeek R1（deepseek-reasoner）：$0.55 / $2.19 per 1M
	"deepseek-reasoner": {Input: 0.55, Output: 2.19},
	// Anthropic Claude Sonnet 4：$3.00 / $15.00 per 1M
	"claude-sonnet-4": {Input: 3.00, Output: 15.00},
	// Anthropic Claude Haiku 3.5：$0.80 / $4.00 per 1M
	"claude-haiku-3.5": {Input: 0.80, Output: 4.00},
	// Google Gemini 2.5 Pro：$1.25 / $10.00 per 1M
	"gemini-2.5-pro": {Input: 1.25, Output: 10.00},
	// Google Gemini 2.5 Flash：$0.30 / $2.50 per 1M
	"gemini-2.5-flash": {Input: 0.30, Output: 2.50},
}

// maxShadowUsdMicros 单次请求影子价饱和上界（微美元）。
// 保守取 1e12 微美元 = $1,000,000，远超真实单请求费用，防溢出。
const maxShadowUsdMicros = 1e12

// ComputeApiEquivalentUsd 计算某模型的 API 等效美元（微美元）。
// 未知模型返回 (0, false)，不估算；负 token 按 0 处理；溢出饱和到上界。
// 本函数只做统计口径，禁止在影子价路径使用 quota 换算函数。
func ComputeApiEquivalentUsd(model string, promptTokens, completionTokens int) (int64, bool) {
	price, ok := modelShadowPrices[model]
	if !ok {
		return 0, false
	}
	p := float64(promptTokens)
	c := float64(completionTokens)
	if p < 0 {
		p = 0
	}
	if c < 0 {
		c = 0
	}
	micros := (p*price.Input + c*price.Output) / 1e6 * 1e6
	if micros < 0 {
		return 0, true
	}
	if micros > maxShadowUsdMicros {
		return int64(maxShadowUsdMicros), true
	}
	return int64(micros), true
}