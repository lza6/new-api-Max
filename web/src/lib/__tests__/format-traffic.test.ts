import { describe, expect, test } from 'vitest'

import { formatTraffic } from '../format'

describe('formatTraffic', () => {
  test('handles zero, negative and non-finite', () => {
    expect(formatTraffic(0)).toBe('0 B')
    expect(formatTraffic(-42)).toBe('0 B')
    expect(formatTraffic(Number.NaN)).toBe('0 B')
    expect(formatTraffic(Number.POSITIVE_INFINITY)).toBe('0 B')
  })

  test('formats bytes and blocks', () => {
    expect(formatTraffic(512)).toBe('512 B')
    expect(formatTraffic(1024)).toBe('1.00 KB')
    expect(formatTraffic(2048 + 512)).toBe('2.50 KB')
    expect(formatTraffic(1024 * 1024)).toBe('1.00 MB')
    expect(formatTraffic(1024 * 1024 * 1024)).toBe('1.00 GB')
    expect(formatTraffic(1536 * 1024 * 1024)).toBe('1.50 GB')
    expect(formatTraffic(1024 * 1024 * 1024 * 1024)).toBe('1.00 TB')
  })
})
