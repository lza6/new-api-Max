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
// T3 请求解释页：白话版渲染（延迟/缓存/流量）+ 空态降级。
import { render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { vi, describe, it, expect, beforeEach } from 'vitest'

import { TransparencyPanel } from '../transparency-panel'

vi.mock('../../../api', () => ({
  getLogTransparency: vi.fn(),
}))

import { getLogTransparency } from '../../../api'
import type { LogTransparencyView } from '../../../api'

function renderPanel() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <TransparencyPanel logId={1} />
    </QueryClientProvider>
  )
}

describe('TransparencyPanel (T3)', () => {
  beforeEach(() => {
    vi.mocked(getLogTransparency).mockReset()
  })

  it('renders plain-language explanation with latency and cache', async () => {
    const view: LogTransparencyView = {
      plain: {
        model: 'gpt-4o',
        tokens: 1100,
        prompt_tokens: 1000,
        completion_tokens: 100,
        first_token_ms: 850,
        upstream_wait_ms: 1200,
        request_bytes: 2048,
        response_bytes: 4096,
        cache_hit_rate: 0.5,
        quota: 123,
        summary: 'Request completed.',
      },
    }
    vi.mocked(getLogTransparency).mockResolvedValue(view)
    renderPanel()
    await waitFor(() => {
      expect(screen.getByText('Request Explanation')).toBeInTheDocument()
    })
    expect(screen.getByText('Request completed.')).toBeInTheDocument()
    expect(screen.getByText('850 ms')).toBeInTheDocument()
    expect(screen.getByText('50.0%')).toBeInTheDocument()
  })

  it('degrades gracefully when no explanation is available', async () => {
    vi.mocked(getLogTransparency).mockResolvedValue({ plain: {} })
    renderPanel()
    await waitFor(() => {
      expect(
        screen.getByText('No explanation available for this request.')
      ).toBeInTheDocument()
    })
  })
})
