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

import {
  aggregateHealthScores,
  getProbeResults,
  coolClassLabelKey,
  gradeToVariant,
  hasHealthData,
  parseProbeHistory,
  scoreToVariant,
  toHealthSnapshotView,
} from '../channel-health'

const validReportJson = JSON.stringify([
  {
    probed_at: 1700000000,
    model: 'gpt-4o-mini',
    score: 70,
    grade: 'B',
    total_weight: 80,
    duration_ms: 1234,
    results: [
      {
        name: 'model_identity',
        passed: true,
        score: 25,
        weight: 25,
        evidence: 'model echoed correctly',
      },
      {
        name: 'cache',
        passed: false,
        score: 0,
        weight: 20,
        evidence: 'no cache headers',
        error: 'cache miss',
      },
    ],
  },
])

describe('parseProbeHistory', () => {
  test('returns empty history for null, empty, and malformed input', () => {
    expect(parseProbeHistory(null)).toEqual({ reports: [], latest: null })
    expect(parseProbeHistory(undefined)).toEqual({ reports: [], latest: null })
    expect(parseProbeHistory('')).toEqual({ reports: [], latest: null })
    expect(parseProbeHistory('not json')).toEqual({ reports: [], latest: null })
    expect(parseProbeHistory('{"not":"an array"}')).toEqual({
      reports: [],
      latest: null,
    })
  })

  test('parses a valid report array with newest first', () => {
    const parsed = parseProbeHistory(validReportJson)
    expect(parsed.reports).toHaveLength(1)
    expect(parsed.latest?.grade).toBe('B')
    expect(parsed.latest?.model).toBe('gpt-4o-mini')
    expect(parsed.latest?.probed_at).toBe(1700000000)
  })

  test('drops non-object entries but keeps valid ones', () => {
    const mixed = JSON.stringify([42, 'oops', { grade: 'A' }])
    const parsed = parseProbeHistory(mixed)
    expect(parsed.reports).toHaveLength(1)
    expect(parsed.latest?.grade).toBe('A')
  })
})

describe('gradeToVariant', () => {
  test('maps grades A/B to success, C to warning, D to orange, F to danger', () => {
    expect(gradeToVariant('A')).toBe('success')
    expect(gradeToVariant('B')).toBe('success')
    expect(gradeToVariant('C')).toBe('warning')
    expect(gradeToVariant('D')).toBe('orange')
    expect(gradeToVariant('F')).toBe('danger')
  })

  test('falls back to danger for unknown grades', () => {
    expect(gradeToVariant(undefined)).toBe('danger')
    expect(gradeToVariant('X')).toBe('danger')
  })
})

describe('scoreToVariant', () => {
  test('applies 85/70/55 thresholds', () => {
    expect(scoreToVariant(100)).toBe('success')
    expect(scoreToVariant(85)).toBe('success')
    expect(scoreToVariant(84.9)).toBe('warning')
    expect(scoreToVariant(70)).toBe('warning')
    expect(scoreToVariant(69.9)).toBe('orange')
    expect(scoreToVariant(55)).toBe('orange')
    expect(scoreToVariant(54.9)).toBe('danger')
    expect(scoreToVariant(0)).toBe('danger')
  })
})

describe('toHealthSnapshotView', () => {
  test('returns empty snapshot for missing entries and non-object values', () => {
    expect(toHealthSnapshotView(undefined, 1).score).toBe(0)
    expect(toHealthSnapshotView({}, 1).sampleCount).toBe(0)
    expect(toHealthSnapshotView({ '1': 'bad' as never }, 1).coolingDown).toBe(
      false
    )
  })

  test('normalizes backend snake_case fields with defaults', () => {
    const view = toHealthSnapshotView(
      {
        '7': {
          score: 92.4,
          success_rate: 0.98,
          p50_latency_ms: 120,
          p95_latency_ms: 400,
          cool_count: 2,
          sample_count: 50,
          cooling_down: true,
          cool_until: 1700001000,
        },
      },
      7
    )
    expect(view.score).toBe(92.4)
    expect(view.successRate).toBe(0.98)
    expect(view.p95LatencyMs).toBe(400)
    expect(view.sampleCount).toBe(50)
    expect(view.coolCount).toBe(2)
    expect(view.coolingDown).toBe(true)
    expect(view.coolUntil).toBe(1700001000)
  })

  test('coerces non-finite numbers to zero', () => {
    const view = toHealthSnapshotView(
      {
        '3': {
          score: Number.NaN,
          success_rate: Number.POSITIVE_INFINITY,
          p50_latency_ms: 0,
          p95_latency_ms: 0,
          cool_count: 0,
          sample_count: 0,
          cooling_down: false,
          cool_until: 0,
        },
      },
      3
    )
    expect(view.score).toBe(0)
    expect(view.successRate).toBe(0)
  })
})

describe('hasHealthData', () => {
  test('is true when score or samples exist and false otherwise', () => {
    expect(hasHealthData({ ...toHealthSnapshotView(undefined, 1), score: 10 })).toBe(true)
    expect(
      hasHealthData({ ...toHealthSnapshotView(undefined, 1), sampleCount: 3 })
    ).toBe(true)
    expect(hasHealthData(toHealthSnapshotView(undefined, 1))).toBe(false)
  })
})

describe('getProbeResults', () => {
  test('returns case list from a report and empty for null/malformed', () => {
    const parsed = parseProbeHistory(validReportJson)
    expect(getProbeResults(parsed.latest)).toHaveLength(2)
    expect(getProbeResults(null)).toEqual([])
    expect(
      getProbeResults({ results: 'bad' as unknown as undefined[] } as never)
    ).toEqual([])
  })
})

// B5-2：toHealthSnapshotView 透传 last_cool_class → lastCoolClass。
test('maps last_cool_class to lastCoolClass for cooling hover reason', () => {
  const view = toHealthSnapshotView(
    { '7': { cooling_down: true, cool_until: 1700001000, last_cool_class: 'rate_limited' } } as never,
    7
  )
  expect(view.lastCoolClass).toBe('rate_limited')
})

// B5-2：缺少 last_cool_class 时 lastCoolClass 为 undefined（老快照兼容）。
test('leaves lastCoolClass undefined when absent', () => {
  const view = toHealthSnapshotView(
    { '8': { cooling_down: true, cool_until: 1700001000 } } as never,
    8
  )
  expect(view.lastCoolClass).toBeUndefined()
})

describe('coolClassLabelKey', () => {
  test('maps known cooldown classes to their cool-class.* keys', () => {
    expect(coolClassLabelKey('auth')).toBe('cool-class.auth')
    expect(coolClassLabelKey('rate_limited')).toBe('cool-class.rate_limited')
    expect(coolClassLabelKey('server_error')).toBe('cool-class.server_error')
    expect(coolClassLabelKey('timeout')).toBe('cool-class.timeout')
    expect(coolClassLabelKey('bad_request')).toBe('cool-class.bad_request')
    expect(coolClassLabelKey('capability')).toBe('cool-class.capability')
  })

  test('falls back to the generic key for missing/ok/unknown classes', () => {
    expect(coolClassLabelKey(undefined)).toBe('cool-class.unknown')
    expect(coolClassLabelKey('ok')).toBe('cool-class.unknown')
    expect(coolClassLabelKey('unknown')).toBe('cool-class.unknown')
    expect(coolClassLabelKey('')).toBe('cool-class.unknown')
  })
})


describe('aggregateHealthScores (T2-2)', () => {
  const fixture = (id: string, score: number, sampleCount: number, coolingDown = false) => ({
    [id]: { score, success_rate: sampleCount > 0 ? 1 : 0, p50_latency_ms: 0, p95_latency_ms: 0, cool_count: 0, sample_count: sampleCount, cooling_down: coolingDown, cool_until: coolingDown ? 1700001000 : 0 },
  })

  test('returns neutral empty aggregate for null/empty/no-data input', () => {
    expect(aggregateHealthScores(null)).toMatchObject({ hasData: false, avgScore: 0, totalSampled: 0 })
    expect(aggregateHealthScores({})).toMatchObject({ hasData: false })
    // 仅有冷却计数、无样本：hasHealthData=false → 不算数据
    expect(aggregateHealthScores({ '1': { score: 0, success_rate: 0, p50_latency_ms: 0, p95_latency_ms: 0, cool_count: 1, sample_count: 0, cooling_down: false, cool_until: 0 } } as never)).toMatchObject({ hasData: false })
  })

  test('computes average only over sampled channels', () => {
    const scores = { ...fixture('1', 90, 10), ...fixture('2', 50, 5), ...fixture('3', 0, 0) }
    const agg = aggregateHealthScores(scores as never)
    expect(agg.hasData).toBe(true)
    expect(agg.totalSampled).toBe(2)
    expect(agg.avgScore).toBeCloseTo(70, 5)
  })

  test('computes available rate with 70+ threshold', () => {
    const scores = { ...fixture('1', 90, 10), ...fixture('2', 60, 5) }
    const agg = aggregateHealthScores(scores as never)
    expect(agg.availableRate).toBeCloseTo(0.5, 5)
  })

  test('ranks worst channels cooling-first then by score, capped at 3', () => {
    const scores = {
      ...fixture('1', 30, 5),
      ...fixture('2', 88, 8, true),
      ...fixture('3', 45, 4),
      ...fixture('4', 99, 9),
    }
    const agg = aggregateHealthScores(scores as never)
    expect(agg.worst).toHaveLength(3)
    expect(agg.worst[0]).toMatchObject({ score: 88, coolingDown: true })
    expect(agg.worst[1].score).toBe(30)
    expect(agg.worst[2].score).toBe(45)
  })

  test('counts cooling channels across all entries', () => {
    const scores = { ...fixture('1', 90, 10), ...fixture('2', 80, 8, true) }
    const agg = aggregateHealthScores(scores as never)
    expect(agg.coolingCount).toBe(1)
  })
})
