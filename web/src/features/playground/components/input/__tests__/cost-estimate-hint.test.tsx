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
*/
import { beforeEach, describe, expect, test, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'

import type { PricingModel } from '@/features/pricing/types'

import { CostEstimateHint } from '../cost-estimate-hint'
import type { EstimateGroup, EstimatePlan } from '../../../lib/cost-estimate'

const plan: EstimatePlan = {
  modelName: 'gpt-4o-mini',
  group: 'default',
  kind: 'task',
  multiplierFields: ['duration'],
}

const groups: EstimateGroup[] = [
  { group: 'default', ratio: 1 },
  { group: 'premium', ratio: 2 },
]

function baseModel(overrides: Partial<PricingModel> = {}): PricingModel {
  return {
    id: 1,
    model_name: 'gpt-4o-mini',
    quota_type: 0,
    model_ratio: 1,
    completion_ratio: 3,
    enable_groups: ['default', 'premium'],
    group_ratio: { default: 1, premium: 2 },
    ...overrides,
  }
}

const { mockModels } = vi.hoisted(() => ({ mockModels: [] as PricingModel[] }))

vi.mock('@/features/pricing/hooks/use-pricing-data', () => ({
  usePricingData: () => ({ models: mockModels }),
}))

function seedI18n() {
  i18next.addResourceBundle('en', 'translation', {
    'Estimated cost range': 'Estimated cost range',
    'Estimated token usage': 'Estimated token usage',
    Multiplier: 'Multiplier',
    'Estimated multiplier for {{fields}}':
      'Estimated multiplier for {{fields}}',
    'Estimate is for reference only, actual billing prevails':
      'Estimate is for reference only, actual billing prevails',
  })
}

beforeEach(() => {
  seedI18n()
  mockModels.splice(0, mockModels.length)
})

describe('CostEstimateHint', () => {
  test('renders nothing when pricing data is absent', () => {
    render(
      <CostEstimateHint
        plan={plan}
        groups={groups}
        promptTokens={1}
        completionTokens={1}
      />
    )
    expect(
      screen.queryByRole('button', { name: 'Estimated cost range' })
    ).not.toBeInTheDocument()
  })

  test('renders nothing when a token plan has no billing expression', () => {
    mockModels.push(baseModel())
    render(
      <CostEstimateHint
        plan={{ ...plan, kind: 'token' }}
        groups={groups}
        promptTokens={1}
        completionTokens={1}
      />
    )
    expect(
      screen.queryByRole('button', { name: 'Estimated cost range' })
    ).not.toBeInTheDocument()
  })

  test('shows a task cost range per group when pricing data is available', async () => {
    mockModels.push(
      baseModel({
        billing_mode: 'tiered_expr',
        billing_expr:
          'u("seconds") > 10 ? tier("long", u("seconds") * 0.5) : tier("short", u("seconds") * 0.2)',
        billing_usage_schema: {
          duration: { type: 'number', unit: 'second' },
          seconds: { type: 'number', unit: 'second' },
        },
        billing_usage_examples: [
          { label: '5s', facts: { duration: 5, seconds: 5 } },
          { label: '30s', facts: { duration: 30, seconds: 30 } },
        ],
      })
    )
    const user = userEvent.setup()
    render(
      <CostEstimateHint
        plan={plan}
        groups={groups}
        promptTokens={1}
        completionTokens={1}
      />
    )
    await user.click(
      screen.getByRole('button', { name: 'Estimated cost range' })
    )
    expect(screen.getByText('Multiplier')).toBeVisible()
    expect(screen.getByText('default')).toBeVisible()
    expect(screen.getByText('premium')).toBeVisible()
    expect(
      screen.getByText(/Estimate is for reference only/)
    ).toBeVisible()
  })

  test('shows a token range and preset buttons for a token plan', async () => {
    mockModels.push(
      baseModel({
        billing_mode: 'tiered_expr',
        billing_expr: 'tier("base", p * 2 + c * 8)',
      })
    )
    const user = userEvent.setup()
    render(
      <CostEstimateHint
        plan={{ ...plan, kind: 'token' }}
        groups={groups}
        promptTokens={1000}
        completionTokens={500}
      />
    )
    await user.click(
      screen.getByRole('button', { name: 'Estimated cost range' })
    )
    expect(screen.getByText('4k / 2k')).toBeVisible()
    expect(screen.getByText(/Estimated token usage/)).toBeVisible()
    expect(
      screen.getByText(/Estimate is for reference only/)
    ).toBeVisible()
  })
})
