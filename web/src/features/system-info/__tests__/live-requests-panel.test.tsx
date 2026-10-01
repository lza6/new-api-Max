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
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import { LiveRequestsPanel } from '../components/live-requests-panel'

const apiMocks = vi.hoisted(() => ({
  getLiveRequests: vi.fn(),
}))

vi.mock('../api', () => ({
  getLiveRequests: apiMocks.getLiveRequests,
}))

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))

let client: QueryClient

beforeEach(() => {
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
})

afterEach(() => {
  cleanup()
  client.clear()
  vi.clearAllMocks()
})

function baseData(overrides: Record<string, unknown> = {}) {
  return {
    active: [],
    finished: [],
    active_count: 0,
    compressed_count: 0,
    original_bytes_sum: 0,
    compressed_bytes_sum: 0,
    avg_compression_ratio: 0,
    avg_first_response_ms: 0,
    avg_upload_ms: -1,
    avg_upstream_ttfb_ms: -1,
    network_in_mbps: 1.25,
    network_out_mbps: 3.5,
    concurrency: { enabled: false, active: 0, waiting: 0, limit: 0 },
    compression_enabled: true,
    compression_threshold_kb: 1024,
    ...overrides,
  }
}

function renderPanel() {
  return render(
    <QueryClientProvider client={client}>
      <LiveRequestsPanel />
    </QueryClientProvider>
  )
}

describe('LiveRequestsPanel', () => {
  test('renders live request row with compression ratio and first token', async () => {
    apiMocks.getLiveRequests.mockResolvedValue({
      success: true,
      message: '',
      data: baseData({
        active_count: 1,
        compressed_count: 1,
        original_bytes_sum: 2_000_000,
        compressed_bytes_sum: 20_000,
        avg_compression_ratio: 0.01,
        active: [
          {
            request_id: 'req-1',
            user_id: 7,
            user_name: 'alice',
            model: 'deepseek-v4.1-flash',
            group: 'default',
            channel_id: 49,
            channel_name: 'ch49',
            is_stream: true,
            phase: 'streaming',
            started_at: 1,
            elapsed_ms: 1234,
            retry_index: 0,
            original_bytes: 2_000_000,
            compressed_bytes: 20_000,
            compressed: true,
            first_response_ms: 480,
            upstream_connect_ms: 120,
            upstream_upload_ms: 40,
            upstream_ttfb_ms: 440,
          },
        ],
      }),
    })

    renderPanel()

    expect(await screen.findByText('deepseek-v4.1-flash')).toBeInTheDocument()
    // 压缩比 1%（20KB / 2MB）
    expect(screen.getAllByText('1%').length).toBeGreaterThan(0)
    // 首字 480ms
    expect(screen.getByText('480ms')).toBeInTheDocument()
    // 阶段标签（streaming 的 i18n 键）
    expect(screen.getByText('Streaming')).toBeInTheDocument()
  })

  test('shows empty state when no active requests', async () => {
    apiMocks.getLiveRequests.mockResolvedValue({
      success: true,
      message: '',
      data: baseData(),
    })

    renderPanel()

    expect(
      await screen.findByText('No requests in progress.')
    ).toBeInTheDocument()
  })

  test('shows error state on failed response', async () => {
    apiMocks.getLiveRequests.mockResolvedValue({
      success: false,
      message: 'boom',
      data: undefined,
    })

    renderPanel()

    expect(
      await screen.findByText('We could not load live requests.')
    ).toBeInTheDocument()
  })
})
