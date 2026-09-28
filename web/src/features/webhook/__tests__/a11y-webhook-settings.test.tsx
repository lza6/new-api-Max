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
// T7-C3 a11y：webhook 设置页（真实业务组件）必须通过 axe 扫描。
// 覆盖表单 label 关联（URL/Secret/Switch/事件 checkbox）、aria 与基础可访问性。
import { render, screen, waitFor } from '@testing-library/react'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { Toaster, toast } from 'sonner'
import { afterEach, expect, it, vi } from 'vitest'
import { axe } from 'vitest-axe'

import { api } from '@/lib/api'

import { WebhookSettingsPage } from '../webhook-settings'

const i18n = createInstance()
await i18n.init({
  lng: 'en',
  resources: {
    en: {
      translation: {
        'Webhook Notifications': 'Webhook Notifications',
        'Webhook URL': 'Webhook URL',
        'Webhook Secret': 'Webhook Secret',
        Enabled: 'Enabled',
        'Subscribed Events': 'Subscribed Events',
        Save: 'Save',
        'Loading...': 'Loading...',
      },
    },
  },
  initAsync: false,
})

afterEach(() => {
  vi.restoreAllMocks()
  toast.dismiss()
})

function renderPage() {
  return render(
    <I18nextProvider i18n={i18n}>
      <WebhookSettingsPage />
      <Toaster />
    </I18nextProvider>
  )
}

it('webhook settings page passes the axe scan with labeled form controls', async () => {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: {
        enabled: true,
        url: 'https://hooks.example.com/x',
        secret: 's3cret',
        events: ['epay.topup.success', 'task.settled'],
      },
    },
  })
  const { container } = renderPage()
  await waitFor(() =>
    expect(screen.getByText('Webhook Notifications')).toBeInTheDocument()
  )

  // 关键表单控件必须可通过 label 或 role 定位（可访问名称存在）。
  expect(screen.getByLabelText('Webhook URL')).toBeInTheDocument()
  expect(screen.getByLabelText('Webhook Secret')).toBeInTheDocument()
  expect(screen.getByRole('switch', { name: 'Enabled' })).toBeInTheDocument()
  expect(
    screen.getByRole('checkbox', { name: /epay\.topup\.success/ })
  ).toBeInTheDocument()
  expect(
    screen.getByRole('checkbox', { name: /task\.settled/ })
  ).toBeInTheDocument()

  const results = await axe(container)
  expect(results.violations).toEqual([])
})
