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
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { afterEach, expect, it, vi } from 'vitest'

import { Pricing } from '../index'
import * as usePricingDataModule from '../hooks/use-pricing-data'
import * as useStatusModule from '@/hooks/use-status'

const mocks = vi.hoisted(() => ({
  refetch: vi.fn(),
}))

vi.mock('@/features/pricing/hooks/use-pricing-data', () => ({
  usePricingData: () => ({
    models: [],
    vendors: [],
    groupRatio: {},
    usableGroup: {},
    endpointMap: {},
    autoGroups: [],
    isLoading: false,
    error: new Error('boom'),
    refetch: mocks.refetch,
    priceRate: 1,
    usdExchangeRate: 1,
  }),
}))

vi.mock('@/hooks/use-status', () => ({
  useStatus: () => ({ status: { price: 1, usd_exchange_rate: 1 } }),
}))

vi.mock('@/components/layout', () => ({
  PublicLayout: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
}))

const i18n = createInstance()
await i18n.init({
  lng: 'en',
  resources: {
    en: {
      translation: {
        'Failed to load pricing. Please try again.': 'Failed to load pricing. Please try again.',
        Retry: 'Retry',
      },
    },
  },
  initAsync: false,
})

afterEach(() => vi.clearAllMocks())

// §4.2.1：定价查询失败必须展示人话错误 + 重试，而非「无模型」假空态。
it('renders a friendly error and retry instead of a false-empty list', async () => {
  render(
    <I18nextProvider i18n={i18n}>
      <Pricing />
    </I18nextProvider>
  )
  expect(
    screen.getByText('Failed to load pricing. Please try again.')
  ).toBeInTheDocument()
  await userEvent.click(screen.getByRole('button', { name: 'Retry' }))
  expect(mocks.refetch).toHaveBeenCalledTimes(1)
  // 不出现误导向的空态「no models」
  expect(screen.queryByText(/no models/i)).not.toBeInTheDocument()
})
