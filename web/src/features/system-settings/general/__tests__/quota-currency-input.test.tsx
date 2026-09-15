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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import i18next from 'i18next'
import { afterEach, beforeAll, beforeEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { useSystemConfigStore, DEFAULT_CURRENCY_CONFIG } from '@/stores/system-config-store'

// FormNavigationGuard 依赖 TanStack Router 的 useBlocker；单元测试无路由上下文。
vi.mock('@tanstack/react-router', () => ({
  useBlocker: () => ({ status: 'idle', proceed: () => undefined, reset: () => undefined }),
}))

// SettingsPageFormActions 经 Portal 渲染到 SettingsPage 的 actions 容器（无
// Provider 时返回 null）。单元测试直接 mock FormActions 为纯按钮，
// 保留组件其余真实导出（类型导出除外）。
vi.mock('../../components/settings-page-context', () => ({
  SettingsPageProvider: (props: { children: ReactNode }) => props.children,
  useSuppressSettingsSectionHeader: () => false,
  SettingsPageTitleStatusPortal: () => null,
  SettingsPageActionsPortal: (props: { children: ReactNode }) => props.children,
  SettingsPageFormActions: (props: {
    onSave: () => void
    isSaving?: boolean
    isSaveDisabled?: boolean
    saveLabel?: string
  }) => (
    <button type='button' onClick={props.onSave}>
      {props.saveLabel ?? 'Save Changes'}
    </button>
  ),
}))

import { QuotaSettingsSection } from '../quota-settings-section'

const previousConfig = useSystemConfigStore.getState().config

const i18nKeys = {
  'Quota Settings': 'Quota Settings',
  'New User Quota': 'New User Quota',
  'Amount in {{currency}} given to new users (= {{formattedQuota}})':
    'Amount in {{currency}} given to new users (= {{formattedQuota}})',
}

const defaultValues = {
  QuotaForNewUser: 500000,
  PreConsumedQuota: 0,
  QuotaForInviter: 0,
  QuotaForInvitee: 0,
  TopUpLink: '',
  general_setting: { docs_link: '' },
  quota_setting: { enable_free_model_pre_consume: true },
}

beforeEach(() => {
  useSystemConfigStore
    .getState()
    .setConfig({ currency: { ...DEFAULT_CURRENCY_CONFIG } })
  vi.spyOn(api, 'put').mockResolvedValue({ data: { success: true } })
})

const queryClients: QueryClient[] = []

afterEach(() => {
  // 先清未完成的 React Query 请求再 restore mock，避免跨用例泄漏。
  for (const queryClient of queryClients) {
    queryClient.clear()
  }
  queryClients.length = 0
  vi.restoreAllMocks()
  useSystemConfigStore.getState().setConfig(previousConfig)
})

function renderSection(): QueryClient {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <QuotaSettingsSection defaultValues={defaultValues} />
    </QueryClientProvider>
  )
  return queryClient
}

describe('quota settings currency-amount input', () => {
  beforeAll(() => {
    i18next.addResourceBundle('en', 'translation', i18nKeys)
  })

  test('backfills the New User Quota input as a display amount (500000 quota = $1)', () => {
    queryClients.push(renderSection())
    const input = screen.getByLabelText('New User Quota') as HTMLInputElement
    // QuotaPerUnit=500000 → 500000 quota = $1.00 显示金额。
    expect(input.value).toBe('1')
  })

  test('submits raw quota units when the admin enters a currency amount', async () => {
    const user = userEvent.setup()
    queryClients.push(renderSection())
    const input = screen.getByLabelText('New User Quota')
    await user.clear(input)
    await user.type(input, '2')
    // SettingsPageFormActions 经 mock 后直接渲染在表单内。
    const saveButton = screen.getByRole('button', { name: 'Save Changes' })
    await user.click(saveButton)
    // 提交经 React Query mutation 异步完成，等待 put 被调用。
    await vi.waitFor(() => {
      expect(vi.mocked(api.put).mock.calls.length).toBeGreaterThan(0)
    })
    const quotaCall = vi
      .mocked(api.put)
      .mock.calls.find(
        (call) => (call[1] as { key: string }).key === 'QuotaForNewUser'
      )
    expect(quotaCall).toBeDefined()
    // $2 × 500000 = 1000000 raw quota units。
    expect((quotaCall?.[1] as { value: number } | undefined)?.value).toBe(
      1000000
    )
  })
})
