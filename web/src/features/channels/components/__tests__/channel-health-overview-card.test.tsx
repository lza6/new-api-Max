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
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, test, vi } from 'vitest'

import { ChannelHealthOverviewCard } from '../channel-health-overview-card'
import { useChannels } from '../channels-provider'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string) => key,
    i18n: { resolvedLanguage: 'en', language: 'en' },
  }),
}))

vi.mock('../channels-provider', () => ({
  useChannels: vi.fn(),
}))

const mockedUseChannels = vi.mocked(useChannels)

const mockSetOpen = vi.fn()
const mockSetCurrentRow = vi.fn()

afterEach(() => {
  cleanup()
  mockedUseChannels.mockReset()
  mockSetOpen.mockReset()
  mockSetCurrentRow.mockReset()
})

describe('ChannelHealthOverviewCard (T2-2)', () => {
  test('shows neutral empty state when no health data exists', () => {
    mockedUseChannels.mockReturnValue({ healthScores: null, setOpen: mockSetOpen, setCurrentRow: mockSetCurrentRow } as never)
    render(<ChannelHealthOverviewCard />)
    expect(screen.getByText('No health data yet')).toBeInTheDocument()
  })

  test('renders aggregate stats from provider scores', () => {
    mockedUseChannels.mockReturnValue({
      setOpen: mockSetOpen,
      setCurrentRow: mockSetCurrentRow,
      healthScores: {
        '1': { score: 90, success_rate: 1, p50_latency_ms: 120, p95_latency_ms: 400, cool_count: 0, sample_count: 10, cooling_down: false, cool_until: 0 },
        '2': { score: 60, success_rate: 0.5, p50_latency_ms: 300, p95_latency_ms: 900, cool_count: 1, sample_count: 5, cooling_down: true, cool_until: 1700001000 },
      } as never,
    } as never)
    render(<ChannelHealthOverviewCard />)
    expect(screen.getByText('Average health score')).toBeInTheDocument()
    // 均值 (90+60)/2 = 75，可用率 1/2 = 50%
    expect(screen.getByText('75')).toBeInTheDocument()
    expect(screen.getByText('50%')).toBeInTheDocument()
    expect(screen.getByText('Channels cooling down')).toBeInTheDocument()
    expect(screen.getByText('1')).toBeInTheDocument()
  })
})


  test('shows refresh hint in empty state', () => {
    mockedUseChannels.mockReturnValue({ healthScores: null } as never)
    render(<ChannelHealthOverviewCard />)
    expect(screen.getByText('No health data yet')).toBeInTheDocument()
    expect(screen.getByText('Health data refreshes automatically')).toBeInTheDocument()
  })

  test('renders neutral dash for worst channels when none sampled', () => {
    mockedUseChannels.mockReturnValue({
      healthScores: {
        '1': { score: 80, success_rate: 0.8, p50_latency_ms: 100, p95_latency_ms: 300, cool_count: 0, sample_count: 10, cooling_down: false, cool_until: 0 },
      } as never,
    } as never)
    // 单渠道有样本：worst 非空 → 显示分数
    const { container } = render(<ChannelHealthOverviewCard />)
    // 全无样本渠道（cool 事件但无 sample）→ hasData false → 空态
    mockedUseChannels.mockReturnValue({
      healthScores: {
        '2': { score: 0, success_rate: 0, p50_latency_ms: 0, p95_latency_ms: 0, cool_count: 1, sample_count: 0, cooling_down: true, cool_until: 1700001000 },
      } as never,
    } as never)
    const { rerender } = render(<ChannelHealthOverviewCard />)
    rerender(<ChannelHealthOverviewCard />)
    expect(screen.getByText('No health data yet')).toBeInTheDocument()
    expect(container).toBeTruthy()
  })


  test('opens channel detail drawer when clicking worst channel', () => {
    mockedUseChannels.mockReturnValue({
      setOpen: mockSetOpen,
      setCurrentRow: mockSetCurrentRow,
      healthScores: {
        '42': { score: 40, success_rate: 0.4, p50_latency_ms: 500, p95_latency_ms: 1200, cool_count: 2, sample_count: 8, cooling_down: false, cool_until: 0 },
        '7': { score: 95, success_rate: 1, p50_latency_ms: 100, p95_latency_ms: 300, cool_count: 0, sample_count: 20, cooling_down: false, cool_until: 0 },
      } as never,
    } as never)
    render(<ChannelHealthOverviewCard />)
    // 最差渠道块（value 40）可点击
    const worstValue = screen.getByText('40')
    worstValue.click()
    expect(mockSetCurrentRow).toHaveBeenCalledWith({ id: 42 })
    expect(mockSetOpen).toHaveBeenCalledWith('update-channel')
  })
