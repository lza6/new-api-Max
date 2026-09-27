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
import { Toaster, toast } from 'sonner'
import { afterEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { WebhookSettingsPage } from './webhook-settings'

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

it('loads and renders webhook settings', async () => {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: {
        enabled: false,
        url: '',
        secret: '',
        events: [],
      },
    },
  })
  renderPage()
  await waitFor(() =>
    expect(screen.getByText('Webhook Notifications')).toBeInTheDocument()
  )
  expect(screen.getByText('Webhook URL')).toBeInTheDocument()
  expect(screen.getByText('Subscribed Events')).toBeInTheDocument()
})

it('toggles subscription and saves the settings patch', async () => {
  const get = vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: { enabled: false, url: '', secret: '', events: [] },
    },
  })
  const put = vi.spyOn(api, 'put').mockResolvedValue({
    data: { success: true, message: '', data: {} },
  })
  renderPage()
  await waitFor(() =>
    expect(screen.getByText('Webhook Notifications')).toBeInTheDocument()
  )

  await userEvent.type(screen.getByLabelText('Webhook URL'), 'https://hooks.example.com/x')
  await userEvent.type(screen.getByLabelText('Webhook Secret'), 's3cret')
  await userEvent.click(screen.getByRole('checkbox', { name: /epay\.topup\.success/ }))
  await userEvent.click(screen.getByRole('button', { name: 'Save' }))

  await waitFor(() => expect(put).toHaveBeenCalled())
  const patch = put.mock.calls[0][1] as Record<string, unknown>
  expect(patch).toMatchObject({
    url: 'https://hooks.example.com/x',
    secret: 's3cret',
  })
  expect(patch.events).toEqual(['epay.topup.success'])
  expect(get).toHaveBeenCalledWith('/api/admin/webhook/settings')
})