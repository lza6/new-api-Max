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
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, test, vi } from 'vitest'

import { getModelPricing, saveModelPricing } from '@/features/model-pricing/api'

import { BulkPricingDialog } from '../bulk-pricing-dialog'

vi.mock('@/features/model-pricing/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/features/model-pricing/api')>()
  return {
    ...actual,
    saveModelPricing: vi.fn(),
    useCanEditModelPricing: () => true,
    getModelPricing: vi.fn().mockResolvedValue({
      entries: [],
      options: {
        ModelPrice: '',
        ModelRatio: '',
        CompletionRatio: '',
        CacheRatio: '',
        CreateCacheRatio: '',
        ImageRatio: '',
        AudioRatio: '',
        AudioCompletionRatio: '',
        'billing_setting.billing_mode': '',
        'billing_setting.billing_expr': '',
      },
      empty_version: 'empty-v1',
    }),
  }
})

vi.mock('@/stores/auth-store', () => ({
  useAuthStore: () => ({
    auth: { user: { role: 100 } },
  }),
}))

function renderDialog(models = [{ model_name: 'a' }, { model_name: 'b' }]) {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={qc}>
      <BulkPricingDialog models={models} onClose={vi.fn()} onSuccess={vi.fn()} />
    </QueryClientProvider>
  )
}

describe('BulkPricingDialog', () => {
  beforeEach(() => {
    vi.mocked(saveModelPricing).mockReset()
  })

  test('saves batch pricing for all selected models', async () => {
    const user = userEvent.setup()
    renderDialog()
    await user.type(screen.getByLabelText('Model ratio'), '2')
    await user.click(screen.getByRole('button', { name: 'Apply to all selected' }))
    expect(saveModelPricing).toHaveBeenCalledWith([
      { model_name: 'a', expected_version: 'empty-v1', pricing: { model_ratio: 2 } },
      { model_name: 'b', expected_version: 'empty-v1', pricing: { model_ratio: 2 } },
    ])
  })

  test('passes existing version for optimistic concurrency when entry exists', async () => {
    vi.mocked(getModelPricing).mockResolvedValue({
      entries: [
        { model_name: 'a', version: 'v-1', configured: {}, effective: {} },
      ],
      options: {
        ModelPrice: '',
        ModelRatio: '',
        CompletionRatio: '',
        CacheRatio: '',
        CreateCacheRatio: '',
        ImageRatio: '',
        AudioRatio: '',
        AudioCompletionRatio: '',
        'billing_setting.billing_mode': '',
        'billing_setting.billing_expr': '',
      },
      empty_version: 'empty-v1',
    })
    const user = userEvent.setup()
    renderDialog()
    await user.type(screen.getByLabelText('Cache ratio'), '0.5')
    await user.click(screen.getByRole('button', { name: 'Apply to all selected' }))
    expect(saveModelPricing).toHaveBeenCalledWith([
      { model_name: 'a', expected_version: 'v-1', pricing: { cache_ratio: 0.5 } },
      { model_name: 'b', expected_version: 'empty-v1', pricing: { cache_ratio: 0.5 } },
    ])
  })

  test('disables apply when no values entered', () => {
    renderDialog()
    expect(
      screen.getByRole('button', { name: 'Apply to all selected' })
    ).toBeDisabled()
  })
})