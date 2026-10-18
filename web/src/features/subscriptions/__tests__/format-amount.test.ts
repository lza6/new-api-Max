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
import { describe, expect, test } from 'vitest'

import { formatAmount, formatPlanPrice } from '../lib/format'

describe('formatAmount', () => {
  test('uses ¥ for CNY', () => {
    expect(formatAmount(12.3, 'CNY')).toBe('¥12.30')
  })

  test('uses $ for USD', () => {
    expect(formatAmount(12.3, 'USD')).toBe('$12.30')
  })

  test('defaults to USD when currency is missing', () => {
    expect(formatAmount(5)).toBe('$5.00')
  })

  test('formatPlanPrice delegates to formatAmount', () => {
    expect(formatPlanPrice({ price_amount: 9.9, currency: 'USD' })).toBe('$9.90')
    expect(formatPlanPrice({ price_amount: 100, currency: 'CNY' })).toBe(
      '¥100.00'
    )
  })
})
