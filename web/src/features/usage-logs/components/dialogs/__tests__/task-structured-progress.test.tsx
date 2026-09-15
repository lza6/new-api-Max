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
import { render, screen } from '@testing-library/react'
import i18next from 'i18next'
import { beforeAll, describe, expect, test } from 'vitest'

import { readTaskStructuredProgress } from '../../../types'
import { TaskStructuredProgressRow } from '../task-details-dialog'

const i18nKeys = {
  Step: 'Step',
  Progress: 'Progress',
}

describe('readTaskStructuredProgress (B5-3)', () => {
  test('parses current/total/step from data.progress', () => {
    expect(
      readTaskStructuredProgress({
        progress: { event_type: 'generate', current: 3, total: 10, step: 'voice' },
        refund: { quota: 5 },
      })
    ).toEqual({
      current: 3,
      total: 10,
      step: 'voice',
      event_type: 'generate',
    })
  })

  test('returns null for non-object / non-object progress / invalid numbers', () => {
    expect(readTaskStructuredProgress(null)).toBeNull()
    expect(readTaskStructuredProgress('40%')).toBeNull()
    expect(readTaskStructuredProgress({})).toBeNull()
    expect(readTaskStructuredProgress({ progress: '40%' })).toBeNull()
    expect(readTaskStructuredProgress({ progress: { current: 1, total: 0 } })).toBeNull()
    expect(
      readTaskStructuredProgress({ progress: { current: 'a', total: 10 } })
    ).toBeNull()
  })

  test('omits missing step/event_type fields', () => {
    expect(
      readTaskStructuredProgress({ progress: { current: 5, total: 20 } })
    ).toEqual({ current: 5, total: 20 })
  })
})

describe('TaskStructuredProgressRow (B5-3)', () => {
  beforeAll(() => {
    i18next.addResourceBundle('en', 'translation', i18nKeys)
  })

  test('renders percent, current/total and step label', () => {
    render(<TaskStructuredProgressRow current={3} total={10} step='voice' />)
    expect(screen.getByText('Step: voice')).toBeInTheDocument()
    expect(screen.getByText('3/10 (30%)')).toBeInTheDocument()
    expect(screen.getByRole('progressbar')).toHaveAttribute('aria-valuenow', '30')
  })

  test('clamps percent into 0..100 and falls back to generic label without step', () => {
    render(<TaskStructuredProgressRow current={120} total={100} />)
    expect(screen.getByRole('progressbar')).toHaveAttribute('aria-valuenow', '100')
    expect(screen.getByText('Progress')).toBeInTheDocument()
  })
})