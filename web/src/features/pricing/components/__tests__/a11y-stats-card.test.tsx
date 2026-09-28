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
// T7-C3 a11y：公开只读站点订阅统计卡（真实业务组件）必须通过 axe 扫描。
// 覆盖统计值渲染与按档位（by_plan）明细的可读性。
import { render, screen, waitFor } from '@testing-library/react'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { afterEach, expect, it, vi } from 'vitest'
import { axe } from 'vitest-axe'

import { api } from '@/lib/api'

import { SiteSubscriptionStatsCard } from '../site-subscription-stats-card'

const i18n = createInstance()
await i18n.init({
  lng: 'en',
  resources: {
    en: {
      translation: {
        'Active subscriptions': 'Active subscriptions',
        'Total subscriptions': 'Total subscriptions',
        'Total plans': 'Total plans',
        'Expiring soon (7 days)': 'Expiring soon (7 days)',
        'New subscriptions (30 days)': 'New subscriptions (30 days)',
        'By plan': 'By plan',
      },
    },
  },
  initAsync: false,
})

afterEach(() => {
  vi.restoreAllMocks()
})

it('site subscription stats card passes the axe scan and renders plan breakdown', async () => {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: {
        total_plans: 2,
        total_subscriptions: 150,
        active_subscriptions: 120,
        expiring_soon_7d: 8,
        new_last_30d: 30,
        by_plan: [
          { plan_id: 1, title: 'Free', total: 100, active: 80 },
          { plan_id: 2, title: 'Pro', total: 50, active: 40 },
        ],
      },
    },
  })
  const { container } = render(
    <I18nextProvider i18n={i18n}>
      <SiteSubscriptionStatsCard />
    </I18nextProvider>
  )
  await waitFor(() =>
    expect(screen.getByText('Active subscriptions')).toBeInTheDocument()
  )

  expect(screen.getByText('120')).toBeInTheDocument()
  expect(screen.getByText('150')).toBeInTheDocument()
  expect(screen.getByText('2')).toBeInTheDocument()
  expect(screen.getByText('8')).toBeInTheDocument()
  expect(screen.getByText('30')).toBeInTheDocument()
  expect(screen.getByText('By plan')).toBeInTheDocument()
  expect(screen.getByText('Free')).toBeInTheDocument()
  expect(screen.getByText('Pro')).toBeInTheDocument()
  expect(screen.getByText('80')).toBeInTheDocument()
  expect(screen.getByText('40')).toBeInTheDocument()

  const results = await axe(container)
  expect(results.violations).toEqual([])
})
