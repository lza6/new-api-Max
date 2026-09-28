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
import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { PlansCompareTable } from '../components/plans-compare-table'
import type { PlanRecord, UserSubscriptionRecord } from '../types'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string) => key,
    i18n: { resolvedLanguage: 'en', language: 'en' },
  }),
}))

vi.mock('@tanstack/react-router', () => ({
  Link: ({ to, children }: { to: string; children?: React.ReactNode }) => (
    <a href={to}>{children}</a>
  ),
}))

vi.mock('@/lib/format', () => ({
  formatQuota: (quota: number) => String(quota),
}))

function makePlan(id: number, overrides?: Partial<PlanRecord['plan']>): PlanRecord {
  return {
    plan: {
      id,
      title: `Plan ${id}`,
      subtitle: `Subtitle ${id}`,
      price_amount: 10,
      currency: 'USD',
      duration_unit: 'month',
      duration_value: 1,
      quota_reset_period: 'never',
      enabled: true,
      sort_order: 0,
      allow_balance_pay: true,
      allow_wallet_overflow: true,
      max_purchase_per_user: 0,
      total_amount: 100000,
      concurrency_limit: 5,
      rpm_limit: 60,
      models: '',
      ...overrides,
    },
  }
}

function activeSubscription(
  planId: number,
  used = 20000
): UserSubscriptionRecord {
  const now = Math.floor(Date.now() / 1000)
  return {
    subscription: {
      id: planId,
      user_id: 1,
      plan_id: planId,
      status: 'active',
      start_time: now - 3600,
      end_time: now + 86400,
      amount_total: 100000,
      amount_used: used,
      rpm_override: 0,
      concurrency_override: 0,
    },
  }
}

const baseProps = {
  plans: [] as PlanRecord[],
  subscriptions: [] as UserSubscriptionRecord[],
  isSignedIn: true,
  renewingPlanId: null,
  onRenew: vi.fn(),
  onSubscribe: vi.fn(),
}

afterEach(() => {
  cleanup()
})

describe('PlansCompareTable', () => {
  it('renders comparison dimensions as column headers', () => {
    render(
      <PlansCompareTable
        {...baseProps}
        plans={[makePlan(1), makePlan(2)]}
      />
    )
    const headers = screen.getAllByRole('columnheader')
    const names = headers.map((h) => h.textContent)
    expect(names).toEqual(
      expect.arrayContaining([
        'Plan',
        'Price',
        'Validity Period',
        'Plan Quota',
        'Concurrency',
        'RPM',
        'Status',
        'Action',
      ])
    )
  })

  it('shows plan price, quota, duration, concurrency and RPM values', () => {
    render(<PlansCompareTable {...baseProps} plans={[makePlan(1)]} />)
    expect(screen.getByText('Plan 1')).toBeInTheDocument()
    expect(screen.getByText('$10.00')).toBeInTheDocument()
    expect(screen.getByText('100000')).toBeInTheDocument()
    expect(screen.getByText('5')).toBeInTheDocument()
    expect(screen.getByText('60')).toBeInTheDocument()
    expect(screen.getByText('1 months')).toBeInTheDocument()
  })

  it('renders an active subscription badge with remaining quota and a renew action', async () => {
    const user = userEvent.setup()
    const renew = vi.fn()
    render(
      <PlansCompareTable
        {...baseProps}
        onRenew={renew}
        plans={[makePlan(1)]}
        subscriptions={[activeSubscription(1)]}
      />
    )

    expect(screen.getByText('Active')).toBeInTheDocument()
    expect(screen.getByText('Remaining 80000')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Renew Now' }))
    expect(renew).toHaveBeenCalledWith(1)
  })

  it('shows a subscribe action for unsubscribed plans', async () => {
    const user = userEvent.setup()
    const subscribe = vi.fn()
    render(
      <PlansCompareTable
        {...baseProps}
        onSubscribe={subscribe}
        plans={[makePlan(1)]}
      />
    )

    await user.click(screen.getByRole('button', { name: 'Subscribe' }))
    expect(subscribe).toHaveBeenCalledWith(expect.objectContaining({ plan: expect.objectContaining({ id: 1 }) }))
  })

  it('routes balance-disabled plans to the wallet', () => {
    render(
      <PlansCompareTable
        {...baseProps}
        plans={[makePlan(1, { allow_balance_pay: false })]}
      />
    )
    expect(screen.getByRole('link', { name: 'Buy in Wallet' })).toHaveAttribute(
      'href',
      '/wallet'
    )
  })

  it('shows the expiry status for previously-subscribed but expired plans', () => {
    render(
      <PlansCompareTable
        {...baseProps}
        plans={[makePlan(1)]}
        subscriptions={[
          {
            subscription: {
              id: 1,
              user_id: 1,
              plan_id: 1,
              status: 'active',
              start_time: 1000,
              end_time: 2000,
              amount_total: 100000,
              amount_used: 100,
              rpm_override: 0,
              concurrency_override: 0,
            },
          },
        ]}
      />
    )
    expect(screen.getByText('Expired')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Resubscribe' })).toBeInTheDocument()
  })
})