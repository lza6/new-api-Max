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
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import type { RelayRateLimitOverrides } from '../../api'
import { UserRateLimitOverrideSection } from '../user-rate-limit-override-section'

const i18n = createInstance()
await i18n.init({
  lng: 'en',
  resources: { en: { translation: {} } },
  initAsync: false,
})

const base: RelayRateLimitOverrides = {
  base_enabled: true,
  base_concurrency: 3,
  base_rpm: 120,
  group_overrides: {},
  user_overrides: {},
}

function mockGet(data: RelayRateLimitOverrides) {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data },
  })
}

function renderSection(userId = 42, group?: string) {
  return render(
    <I18nextProvider i18n={i18n}>
      <UserRateLimitOverrideSection userId={userId} group={group} />
    </I18nextProvider>
  )
}

it('shows the system default tier and source when there are no overrides', async () => {
  mockGet(base)
  renderSection()
  expect(await screen.findByText('System default')).toBeInTheDocument()
  expect(screen.getByText('3')).toBeInTheDocument()
  expect(screen.getByText('120')).toBeInTheDocument()
})

it('shows the group override tier and its source for users without a personal override', async () => {
  mockGet({
    ...base,
    group_overrides: { vip: { concurrency: 10, rpm: 600 } },
  })
  renderSection(42, 'vip')
  expect(
    await screen.findByText('Group override (vip)')
  ).toBeInTheDocument()
  expect(screen.getByText('10')).toBeInTheDocument()
  expect(screen.getByText('600')).toBeInTheDocument()
})

it('shows the user override tier and its source with the highest priority', async () => {
  mockGet({
    ...base,
    group_overrides: { vip: { concurrency: 10, rpm: 600 } },
    user_overrides: { '42': { concurrency: 1, rpm: 30 } },
  })
  renderSection(42, 'vip')
  expect(await screen.findByText('User override')).toBeInTheDocument()
  expect(screen.getByText('1')).toBeInTheDocument()
  expect(screen.getByText('30')).toBeInTheDocument()
})

it('saves a per-user override through the user override API', async () => {
  mockGet(base)
  const put = vi.spyOn(api, 'put').mockResolvedValue({
    data: { success: true },
  })
  renderSection()
  await screen.findByText('System default')
  const user = userEvent.setup()
  await user.clear(screen.getByLabelText('Concurrency / s'))
  await user.type(screen.getByLabelText('Concurrency / s'), '5')
  await user.clear(screen.getByLabelText('RPM'))
  await user.type(screen.getByLabelText('RPM'), '300')
  await user.click(screen.getByRole('button', { name: 'Save rate limit' }))
  await waitFor(() =>
    expect(put).toHaveBeenCalledWith(
      '/api/option/relay/rate_limit/overrides/user',
      { user_id: 42, concurrency: 5, rpm: 300 }
    )
  )
})

it('removes the personal override and falls back to the group tier', async () => {
  mockGet({
    ...base,
    group_overrides: { vip: { concurrency: 10, rpm: 600 } },
    user_overrides: { '42': { concurrency: 1, rpm: 30 } },
  })
  const put = vi.spyOn(api, 'put').mockResolvedValue({
    data: { success: true },
  })
  renderSection(42, 'vip')
  await screen.findByText('User override')
  await userEvent.click(
    screen.getByRole('button', { name: 'Remove override' })
  )
  await waitFor(() =>
    expect(put).toHaveBeenCalledWith(
      '/api/option/relay/rate_limit/overrides/user',
      { user_id: 42, concurrency: 0, rpm: 0 }
    )
  )
})
