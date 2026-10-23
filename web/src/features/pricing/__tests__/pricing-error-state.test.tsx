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

const mocks = vi.hoisted(() => ({
  refetch: vi.fn(),
}))

// 价格页的过滤器把筛选状态放在 URL 上（useFilters → useSearch），
// 单测没有路由上下文，useSearch 会读到 null router 并抛
// "Cannot read properties of null (reading 'stores')"。这里给它一份空 search。
vi.mock('@tanstack/react-router', () => ({
  useSearch: () => ({}),
  Link: ({
    to,
    children,
    ...rest
  }: {
    to: string
    children?: React.ReactNode
  }) => (
    <a href={to} {...rest}>
      {children}
    </a>
  ),
}))

// 价格页经 PriceDetails 拉入 vchart；单元测试不需要真实图表，按
// task-price-display.test.tsx 的既有做法替身掉（vchart 的 ESM 无扩展名导入
// 在 node 解析下会直接抛 "Cannot find module"，导致整个测试文件加载失败）。
vi.mock('@visactor/react-vchart', () => ({ VChart: () => null }))

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
