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
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { DeletePlanDialog } from '../components/dialogs/delete-plan-dialog'
import type { PlanRecord } from '../types'

const mocks = vi.hoisted(() => ({
  deletePlan: vi.fn(),
  toastSuccess: vi.fn(),
  toastError: vi.fn(),
  setOpen: vi.fn(),
  triggerRefresh: vi.fn(),
  open: 'delete-plan' as string | null,
  currentRow: null as PlanRecord | null,
}))

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))

vi.mock('sonner', () => ({
  toast: { success: mocks.toastSuccess, error: mocks.toastError },
}))

vi.mock('@/features/subscriptions/api', () => ({
  deletePlan: mocks.deletePlan,
}))

vi.mock('@/features/subscriptions/components/subscriptions-provider', () => ({
  useSubscriptions: () => ({
    open: mocks.open,
    setOpen: mocks.setOpen,
    currentRow: mocks.currentRow,
    triggerRefresh: mocks.triggerRefresh,
  }),
}))

function planRecord(id: number, title: string): PlanRecord {
  return {
    plan: {
      id,
      title,
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
      total_amount: 0,
      concurrency_limit: 0,
      rpm_limit: 0,
      models: '',
    },
  }
}

describe('DeletePlanDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.open = 'delete-plan'
    mocks.currentRow = planRecord(7, 'Starter')
  })

  afterEach(() => {
    cleanup()
  })

  it('renders the plan title in the confirmation only when opened for deletion', () => {
    mocks.open = 'toggle-status'
    const { container } = render(<DeletePlanDialog />)
    expect(container).toBeEmptyDOMElement()
  })

  it('does not render when no plan row is selected', () => {
    mocks.currentRow = null
    const { container } = render(<DeletePlanDialog />)
    expect(container).toBeEmptyDOMElement()
  })

  it('calls deletePlan for the selected plan and refreshes on success', async () => {
    const user = userEvent.setup()
    mocks.deletePlan.mockResolvedValue({ success: true })
    render(<DeletePlanDialog />)

    expect(screen.getByText('Delete plan {{plan}}?')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Delete' }))

    await waitFor(() => {
      expect(mocks.deletePlan).toHaveBeenCalledWith(7)
    })
    expect(mocks.triggerRefresh).toHaveBeenCalledTimes(1)
    expect(mocks.setOpen).toHaveBeenCalledWith(null)
    expect(mocks.toastSuccess).toHaveBeenCalledWith('Deleted')
  })

  it('keeps the dialog open and surfaces the server reason when deletion is refused', async () => {
    const user = userEvent.setup()
    mocks.deletePlan.mockResolvedValue({
      success: false,
      message: '该套餐下仍有 2 条订阅记录，请先删除或作废这些订阅',
    })
    render(<DeletePlanDialog />)

    await user.click(screen.getByRole('button', { name: 'Delete' }))

    await waitFor(() => {
      expect(mocks.deletePlan).toHaveBeenCalledWith(7)
    })
    // The plan was NOT removed: no refresh, no close, no success toast.
    expect(mocks.triggerRefresh).not.toHaveBeenCalled()
    expect(mocks.setOpen).not.toHaveBeenCalledWith(null)
    expect(mocks.toastSuccess).not.toHaveBeenCalled()
    // The server's refusal reason reaches the user.
    expect(mocks.toastError).toHaveBeenCalled()
  })
})
