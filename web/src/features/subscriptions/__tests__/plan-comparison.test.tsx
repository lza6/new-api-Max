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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { PlanComparison } from '../components/plan-comparison'
import type { PlanRecord, SelfSubscriptionData } from '../types'

const mocks = vi.hoisted(() => ({
  currentUser: null as { id: number; username: string; quota: number } | null,
  getPublicPlans: vi.fn(),
  getSelfSubscriptionFull: vi.fn(),
  paySubscriptionBalance: vi.fn(),
  toastSuccess: vi.fn(),
  formatQuota: vi.fn(),
}))

vi.mock('@/stores/auth-store', () => ({
  useAuthStore: (selector: (state: { auth: { user: unknown } }) => unknown) =>
    selector({ auth: { user: mocks.currentUser } }),
}))

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string) => key,
    i18n: { resolvedLanguage: 'en', language: 'en' },
  }),
}))

vi.mock('@/components/layout', () => ({
  PublicLayout: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
}))

vi.mock('@/components/page-transition', () => ({
  PageTransition: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
}))

vi.mock('@tanstack/react-router', () => ({
  Link: ({
    to,
    children,
    ...props
  }: {
    to: string
    children?: React.ReactNode
  }) => (
    <a href={to} {...props}>
      {children}
    </a>
  ),
}))

vi.mock('@/features/subscriptions/api', () => ({
  getPublicPlans: mocks.getPublicPlans,
  getSelfSubscriptionFull: mocks.getSelfSubscriptionFull,
  paySubscriptionBalance: mocks.paySubscriptionBalance,
}))

vi.mock('@/features/subscriptions/components/dialogs/subscription-purchase-dialog', () => ({
  SubscriptionPurchaseDialog: () => null,
}))

vi.mock('@/lib/format', () => ({
  formatQuota: mocks.formatQuota,
}))

vi.mock('sonner', () => ({
  toast: { success: mocks.toastSuccess, error: vi.fn() },
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

function makeSelf(
  overrides?: Partial<SelfSubscriptionData>
): SelfSubscriptionData {
  const now = Math.floor(Date.now() / 1000)
  return {
    billing_preference: 'subscription_first',
    subscriptions: [
      {
        subscription: {
          id: 1,
          user_id: 1,
          plan_id: 1,
          status: 'active',
          start_time: now - 3600,
          end_time: now + 86400,
          amount_total: 100000,
          amount_used: 20000,
          rpm_override: 0,
          concurrency_override: 0,
        },
      },
    ],
    all_subscriptions: [
      {
        subscription: {
          id: 1,
          user_id: 1,
          plan_id: 1,
          status: 'active',
          start_time: now - 3600,
          end_time: now + 86400,
          amount_total: 100000,
          amount_used: 20000,
          rpm_override: 0,
          concurrency_override: 0,
        },
      },
    ],
    ...overrides,
  }
}

function okResponse(data?: unknown) {
  return { success: true, data }
}

function renderPage() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <PlanComparison />
    </QueryClientProvider>
  )
}

beforeEach(() => {
  mocks.currentUser = null
  mocks.getPublicPlans.mockReset()
  mocks.getSelfSubscriptionFull.mockReset()
  mocks.paySubscriptionBalance.mockReset()
  mocks.toastSuccess.mockReset()
  mocks.formatQuota.mockImplementation((quota: number) => String(quota))
})

afterEach(() => {
  cleanup()
})

describe('PlanComparison', () => {
  it('shows a sign-in CTA and skips API calls when not signed in', async () => {
    renderPage()

    expect(
      await screen.findByText(
        'Sign in to view subscription plans, check your balances and renew with one click.'
      )
    ).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Sign in' })).toHaveAttribute(
      'href',
      '/sign-in'
    )
    expect(screen.getByRole('link', { name: 'Back to Pricing' })).toHaveAttribute(
      'href',
      '/pricing'
    )
    expect(mocks.getPublicPlans).not.toHaveBeenCalled()
    expect(mocks.getSelfSubscriptionFull).not.toHaveBeenCalled()
  })

  it('renders the comparison table with price, quota, duration, concurrency and RPM columns', async () => {
    mocks.currentUser = { id: 1, username: 'tester', quota: 5000 }
    mocks.getPublicPlans.mockResolvedValue(okResponse([makePlan(1)]))
    mocks.getSelfSubscriptionFull.mockResolvedValue(
      okResponse(makeSelf({ subscriptions: [], all_subscriptions: [] }))
    )

    renderPage()

    expect(await screen.findByRole('table')).toBeInTheDocument()
    for (const header of [
      'Plan',
      'Price',
      'Validity Period',
      'Plan Quota',
      'Concurrency',
      'RPM',
      'Status',
      'Action',
    ]) {
      expect(screen.getByRole('columnheader', { name: header })).toBeInTheDocument()
    }
    expect(screen.getByText('Plan 1')).toBeInTheDocument()
    expect(screen.getByText('$10.00')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Subscribe' })).toBeInTheDocument()
  })

  it('marks an active subscription and renews it with one click via balance pay', async () => {
    mocks.currentUser = { id: 1, username: 'tester', quota: 5000 }
    mocks.getPublicPlans.mockResolvedValue(okResponse([makePlan(1)]))
    mocks.getSelfSubscriptionFull.mockResolvedValue(okResponse(makeSelf()))
    mocks.paySubscriptionBalance.mockResolvedValue({ success: true })

    const user = userEvent.setup()
    renderPage()

    expect(await screen.findByText('Active')).toBeInTheDocument()
    expect(screen.getByText('Remaining 80000')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Renew Now' }))
    expect(mocks.paySubscriptionBalance).toHaveBeenCalledWith({ plan_id: 1 })
    expect(mocks.toastSuccess).toHaveBeenCalledWith(
      'Subscription renewed successfully'
    )
  })

  it('shows the empty state when no plans are on sale', async () => {
    mocks.currentUser = { id: 1, username: 'tester', quota: 5000 }
    mocks.getPublicPlans.mockResolvedValue(okResponse([]))
    mocks.getSelfSubscriptionFull.mockResolvedValue(
      okResponse(makeSelf({ subscriptions: [], all_subscriptions: [] }))
    )

    renderPage()

    expect(
      await screen.findByText('No subscription plans are currently available.')
    ).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Back to Pricing' })).toHaveAttribute(
      'href',
      '/pricing'
    )
  })

  it('shows the error state with retry when loading fails', async () => {
    mocks.currentUser = { id: 1, username: 'tester', quota: 5000 }
    mocks.getPublicPlans.mockRejectedValue(new Error('network down'))

    renderPage()

    expect(
      await screen.findByText('Failed to load subscription plans. Please try again.')
    ).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Retry' })).toBeInTheDocument()
  })
})