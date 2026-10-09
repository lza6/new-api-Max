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
import { describe, expect, test } from 'vitest'

import { parseTiersFromExpr } from '../lib/billing-expr'
import { getDynamicPricingSummary } from '../lib/dynamic-price'
import type { PricingModel } from '../types'

// 生产实际配置：同一模型按**分组**切换计费方式（v1.3.114 的 group 变量）。
// token计费 分组走输入长度阶梯，其他分组保持按次。
// 用户在模型广场必须看到「一次调用多少钱」，而不是这段表达式源码。
const GROUP_GATED_TIER_EXPR =
  'group == "token计费" ? (len < 200000 ? tier("tok_lt200k", fixed(0.001)) : (len < 500000 ? tier("tok_200k_500k", fixed(0.0015)) : tier("tok_ge500k", fixed(0.002)))) : tier("per_request", fixed(0.002))'

// 生产上第二个按 token 计费的模型（glm-5.3-flash），同一形态、不同价。
const GLM_GROUP_GATED_TIER_EXPR =
  'group == "token计费" ? (len < 200000 ? tier("tok_lt200k", fixed(0.004)) : (len < 500000 ? tier("tok_200k_500k", fixed(0.005)) : tier("tok_ge500k", fixed(0.006)))) : tier("per_request", fixed(0.006))'

function pricingModel(overrides: Partial<PricingModel>): PricingModel {
  return {
    id: 1,
    model_name: 'deepseek-v4.1-flash',
    quota_type: 0,
    model_ratio: 1,
    completion_ratio: 1,
    enable_groups: ['default', 'token计费'],
    ...overrides,
  }
}

describe('group-gated tier expressions render as prices, not source code', () => {
  test('parses the tier chain out of a group guard instead of falling back to source text', () => {
    const tiers = parseTiersFromExpr(GROUP_GATED_TIER_EXPR)

    // 「token计费」分组下的三档阶梯（else 分支是全站统一的按次价，不属于该分组阶梯）。
    expect(tiers.length, 'must recover the three tiered prices').toBe(3)
    expect(tiers.map((tier) => tier.fixedPrice)).toEqual([0.001, 0.0015, 0.002])
    expect(tiers.map((tier) => tier.label)).toEqual([
      'tok_lt200k',
      'tok_200k_500k',
      'tok_ge500k',
    ])
    // 长度阈值必须保留，否则用户看不出档位怎么分。
    expect(tiers[0]?.conditions).toEqual([
      { var: 'len', op: '<', value: 200000 },
    ])
    expect(tiers[1]?.conditions).toEqual([
      { var: 'len', op: '<', value: 500000 },
    ])
    expect(tiers[2]?.conditions).toEqual([])
  })

  test('summary exposes per-call prices and never the raw expression', () => {
    const summary = getDynamicPricingSummary(
      pricingModel({ billing_mode: 'tiered_expr', billing_expr: GROUP_GATED_TIER_EXPR }),
      { tokenUnit: 'K' }
    )

    expect(summary).not.toBeNull()
    expect(
      summary?.isSpecialExpression,
      'a priced tier chain must not be shown as “special billing expression”'
    ).toBe(false)
    expect(summary?.primaryEntries.length, 'must offer at least one price entry').toBeGreaterThan(0)
    // 每个条目都必须是金额，不能是表达式片段。
    for (const entry of summary?.primaryEntries ?? []) {
      expect(entry.formatted).not.toContain('tier(')
      expect(entry.formatted).not.toContain('group ==')
    }
  })

  test('the second token-billed production model expands too', () => {
    const tiers = parseTiersFromExpr(GLM_GROUP_GATED_TIER_EXPR)
    expect(tiers.map((tier) => tier.fixedPrice)).toEqual([0.004, 0.005, 0.006])
    expect(tiers.map((tier) => tier.label)).toEqual([
      'tok_lt200k',
      'tok_200k_500k',
      'tok_ge500k',
    ])
  })
})
