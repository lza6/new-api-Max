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
// 余额查询地址卡回归：用户反馈「不知道本站余额在哪里查」。该卡片必须把
// 站点地址 + 两条 OpenAI 兼容计费端点渲染出来，并提供复制按钮；站点地址来自
// /api/status 的 server_address，缺失时回退到当前 origin。
import { render, screen } from '@testing-library/react'
import i18next from 'i18next'
import { beforeAll, beforeEach, describe, expect, test, vi } from 'vitest'

import { WalletUsageEndpointsCard } from '@/features/wallet/components/wallet-usage-endpoints-card'

const translations: Record<string, string> = {
  'Total quota': 'Total quota',
  'Returns remaining plus consumed quota as a USD amount':
    'Returns remaining plus consumed quota as a USD amount',
  'Consumed quota': 'Consumed quota',
  'Returns the consumed amount for balance display':
    'Returns the consumed amount for balance display',
  'Balance query endpoints': 'Balance query endpoints',
  'CC Switch and other OpenAI-compatible clients read your remaining balance from these addresses automatically. Authenticate with your API key.':
    'CC Switch and other OpenAI-compatible clients read your remaining balance from these addresses automatically. Authenticate with your API key.',
  'Copy address': 'Copy address',
  'Use your API key as the Bearer token. The base URL used by every protocol is':
    'Use your API key as the Bearer token. The base URL used by every protocol is',
}

beforeAll(() => {
  i18next.addResourceBundle('en', 'translation', translations)
})

vi.mock('@/hooks/use-status', () => ({
  useStatus: () => ({
    status: { server_address: 'https://freeapi.example.com/' },
    loading: false,
    error: null,
  }),
}))

describe('WalletUsageEndpointsCard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  test('renders both billing endpoints under the trailing-slash-stripped server address', () => {
    render(<WalletUsageEndpointsCard />)

    expect(
      screen.getByText('https://freeapi.example.com/v1/dashboard/billing/subscription')
    ).toBeTruthy()
    expect(
      screen.getByText('https://freeapi.example.com/v1/dashboard/billing/usage')
    ).toBeTruthy()
    // 尾随斜杠必须被剥离，避免出现 //v1 双斜杠地址。
    expect(screen.queryByText(/example\.com\/\/v1/)).toBeNull()
  })

  test('labels each endpoint and exposes a copy control per address', () => {
    render(<WalletUsageEndpointsCard />)

    expect(screen.getByText('Total quota')).toBeTruthy()
    expect(screen.getByText('Consumed quota')).toBeTruthy()
    expect(screen.getAllByLabelText('Copy address')).toHaveLength(2)
  })

  test('states the shared protocol base URL', () => {
    render(<WalletUsageEndpointsCard />)

    expect(
      screen.getByText('https://freeapi.example.com/v1', { exact: true })
    ).toBeTruthy()
  })
})
