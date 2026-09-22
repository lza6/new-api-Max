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
import { render, screen, waitFor } from '@testing-library/react'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { BandwidthSection } from '../components/bandwidth-section'

const i18n = createInstance()
await i18n.init({
  lng: 'en',
  resources: { en: { translation: {} } },
  initAsync: false,
})

function renderSection() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        <BandwidthSection />
      </QueryClientProvider>
    </I18nextProvider>
  )
}

it('renders model traffic rows with human-readable bytes', async () => {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: {
        days: 30,
        limit: 10,
        period_end: 0,
        leaderboard: [
          { model: 'deepseek-v4-flash', requests: 120, bytes: 2147483648, bytes_text: '2.00 GB' },
          { model: 'kilwa-grok', requests: 5, bytes: 262144, bytes_text: '256.00 KB' },
        ],
      },
    },
  })
  renderSection()
  expect(await screen.findByText('deepseek-v4-flash')).toBeInTheDocument()
  expect(screen.getByText('2.00 GB')).toBeInTheDocument()
  expect(screen.getByText('Requests: 120')).toBeInTheDocument()
  expect(screen.getByText('kilwa-grok')).toBeInTheDocument()
  expect(screen.getByText('256.00 KB')).toBeInTheDocument()
})

it('shows an empty state when there is no traffic', async () => {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: { days: 30, limit: 10, period_end: 0, leaderboard: [] },
    },
  })
  renderSection()
  expect(await screen.findByText('No traffic data yet')).toBeInTheDocument()
})

it('shows an error state when the request fails', async () => {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: false, message: 'boom' },
  })
  renderSection()
  await waitFor(() =>
    expect(screen.getByText('Unable to load traffic data')).toBeInTheDocument()
  )
})
