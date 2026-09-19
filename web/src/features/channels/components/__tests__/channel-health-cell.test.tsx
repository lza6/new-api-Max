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

import { HealthPopoverContent } from '../channel-health-cell'
import type { ChannelHealthSnapshotView } from '../../lib/channel-health'
import type { ProbeReport } from '../../types'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string) => key,
    i18n: { resolvedLanguage: 'en', language: 'en' },
  }),
}))

const report: ProbeReport = {
  probed_at: 1700000000,
  model: 'deepseek-v4-flash',
  score: 90,
  grade: 'A',
}

const baseSnapshot: ChannelHealthSnapshotView = {
  score: 90,
  successRate: 0.9,
  p50LatencyMs: 1200,
  p95LatencyMs: 2400,
  coolCount: 1,
  sampleCount: 10,
  coolingDown: false,
  coolUntil: 0,
}

afterEach(() => {
  cleanup()
})

describe('ChannelHealthCell cooldown hover-why (B5-2/B2-4)', () => {
  test('shows cooldown reason and expiry when cooling down', () => {
    render(
      <HealthPopoverContent
        report={report}
        snapshot={{
          ...baseSnapshot,
          coolingDown: true,
          coolUntil: 1700003600,
          lastCoolClass: 'rate_limited',
        }}
      />
    )

    expect(screen.getByText(/Cooling down/)).toBeInTheDocument()
    expect(screen.getByText(/Reason:/)).toBeInTheDocument()
    // i18n mock returns the key: the mapped class label key must be present.
    expect(screen.getByText(/cool-class\.rate_limited/)).toBeInTheDocument()
  })

  test('omits the reason line when no cooldown class was recorded', () => {
    render(
      <HealthPopoverContent
        report={report}
        snapshot={{
          ...baseSnapshot,
          coolingDown: true,
          coolUntil: 1700003600,
        }}
      />
    )

    expect(screen.getByText(/Cooling down/)).toBeInTheDocument()
    expect(screen.queryByText(/Reason:/)).not.toBeInTheDocument()
  })

  test('shows no cooldown block when the channel is not cooling', () => {
    render(<HealthPopoverContent report={report} snapshot={baseSnapshot} />)

    expect(screen.queryByText(/Cooling down/)).not.toBeInTheDocument()
    expect(screen.queryByText(/Reason:/)).not.toBeInTheDocument()
  })
})