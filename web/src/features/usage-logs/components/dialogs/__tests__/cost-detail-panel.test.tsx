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
import { render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { vi, describe, it, expect, beforeEach } from 'vitest'

import { CostDetailPanel } from '../details-dialog'

vi.mock('../../../api', () => ({
  getLogCostDetail: vi.fn(),
}))

import { getLogCostDetail } from '../../../api'
import type { LogCostDetail } from '../../../api'

const sample: LogCostDetail = {
  log_id: 1,
  model_name: 'gpt-4o-mini',
  quota: 123,
  prompt_tokens: 1000,
  completion_tokens: 100,
  model_ratio: 1,
  group_ratio: 2,
  completion_ratio: 1,
  cache_ratio: 0,
  tier_matched: 'standard',
  api_equivalent_usd: 45,
  shadow_known: true,
}

function renderPanel() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <CostDetailPanel logId={1} />
    </QueryClientProvider>
  )
}

describe('CostDetailPanel (T4-2)', () => {
  beforeEach(() => {
    vi.mocked(getLogCostDetail).mockReset()
  })

  it('renders cost ratios and tier when API returns data', async () => {
    vi.mocked(getLogCostDetail).mockResolvedValue(sample)
    renderPanel()
    expect(await screen.findByText('Cost Detail')).toBeTruthy()
    expect(await screen.findByText('Model ratio')).toBeTruthy()
    expect(screen.getByText('Group ratio')).toBeTruthy()
    expect(screen.getAllByText('1').length).toBeGreaterThanOrEqual(2)
    expect(screen.getByText('standard')).toBeTruthy()
    // shadow price: 45 / 1e6 USD
    expect(screen.getByText(/\$0\.000045 USD/)).toBeTruthy()
  })

  it('renders nothing on API failure (graceful degradation)', async () => {
    vi.mocked(getLogCostDetail).mockRejectedValue(new Error('boom'))
    const { container } = renderPanel()
    await waitFor(() => {
      expect(container.childElementCount).toBe(0)
    })
  })
})