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
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import {
  DEFAULT_CURRENCY_CONFIG,
  useSystemConfigStore,
} from '@/stores/system-config-store'

// FormNavigationGuard 依赖 TanStack Router 的 useBlocker；单元测试无路由上下文。
vi.mock('@tanstack/react-router', () => ({
  useBlocker: () => ({
    status: 'idle',
    proceed: () => undefined,
    reset: () => undefined,
  }),
}))

// SettingsPageFormActions 经 Portal 渲染到 SettingsPage 的 actions 容器
// （无 Provider 时返回 null）。单元测试直接 mock 为纯按钮。
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

import { CheckinSettingsSection } from '../checkin-settings-section'

const previousConfig = useSystemConfigStore.getState().config

const defaultValues = {
  enabled: true,
  minQuota: 500000,
  maxQuota: 800000,
}

beforeEach(() => {
  useSystemConfigStore
    .getState()
    .setConfig({ currency: { ...DEFAULT_CURRENCY_CONFIG } })
  vi.spyOn(api, 'put').mockResolvedValue({ data: { success: true } } as never)
})

const queryClients: QueryClient[] = []

afterEach(() => {
  for (const queryClient of queryClients) {
    queryClient.clear()
  }
  queryClients.length = 0
  vi.restoreAllMocks()
  useSystemConfigStore.getState().setConfig(previousConfig)
})

function renderSectionWith(values: { enabled: boolean; minQuota: number; maxQuota: number }): void {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <CheckinSettingsSection defaultValues={values} />
    </QueryClientProvider>
  )
  queryClients.push(queryClient)
}

function renderSection(): void {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <CheckinSettingsSection defaultValues={defaultValues} />
    </QueryClientProvider>
  )
  queryClients.push(queryClient)
}

describe('CheckinSettingsSection', () => {
  test('accepts decimal currency amounts without Invalid input', async () => {
    const user = userEvent.setup()
    renderSection()

    // 回填为显示货币金额：500000/500000 = 1，800000/500000 = 1.6
    const minInput = screen.getByLabelText(
      'Minimum check-in quota'
    ) as HTMLInputElement
    const maxInput = screen.getByLabelText(
      'Maximum check-in quota'
    ) as HTMLInputElement
    expect(minInput.value).toBe('1')
    expect(maxInput.value).toBe('1.6')

    await user.clear(minInput)
    await user.type(minInput, '0.5')
    await user.clear(maxInput)
    await user.type(maxInput, '0.8')

    await user.click(
      screen.getByRole('button', { name: 'Save check-in settings' })
    )

    // 小数点金额通过校验并按 quota 单位换算提交（0.5/0.8 CNY*500000）
    await waitFor(() => {
      expect(api.put).toHaveBeenCalledWith(
        '/api/option/',
        expect.objectContaining({
          key: 'checkin_setting.min_quota',
          value: '250000',
        })
      )
    })
    expect(api.put).toHaveBeenCalledWith(
      '/api/option/',
      expect.objectContaining({
        key: 'checkin_setting.max_quota',
        value: '400000',
      })
    )
    expect(screen.queryByText(/Invalid input/)).not.toBeInTheDocument()
  })

  test('does not show Invalid input when check-in options are undefined (fresh admin)', async () => {
    renderSectionWith({
      enabled: true,
      minQuota: undefined as unknown as number,
      maxQuota: undefined as unknown as number,
    })

    const minInput = screen.getByLabelText(
      'Minimum check-in quota'
    ) as HTMLInputElement
    const maxInput = screen.getByLabelText(
      'Maximum check-in quota'
    ) as HTMLInputElement
    expect(minInput.value).toBe('0')
    expect(maxInput.value).toBe('0')
    expect(screen.queryByText(/Invalid input/)).not.toBeInTheDocument()
  })

  test('rejects max < min with a friendly message and does not save', async () => {
    const user = userEvent.setup()
    renderSection()

    const minInput = screen.getByLabelText(
      'Minimum check-in quota'
    ) as HTMLInputElement
    const maxInput = screen.getByLabelText(
      'Maximum check-in quota'
    ) as HTMLInputElement
    await user.clear(minInput)
    await user.type(minInput, '1.5')
    await user.clear(maxInput)
    await user.type(maxInput, '0.8')

    await user.click(
      screen.getByRole('button', { name: 'Save check-in settings' })
    )

    expect(
      screen.getByText('Check-in max must be at least the minimum')
    ).toBeInTheDocument()
    expect(api.put).not.toHaveBeenCalled()
  })
})