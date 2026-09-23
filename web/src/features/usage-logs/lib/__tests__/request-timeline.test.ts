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
  buildRequestTimeline,
  exportTimelineJson,
  formatDurationMs,
  type TimelineSource,
} from '../request-timeline'

describe('buildRequestTimeline', () => {
  test('successful stream request yields full phase chain', () => {
    const src: TimelineSource = {
      created_at: 1_700_000_000,
      use_time: 3.2,
      frt: 480,
      use_channel: [7, 12],
      request_path: '/v1/chat/completions',
      stream_status: { status: 'ok', end_reason: 'done', end_error: '' },
    }
    const tl = buildRequestTimeline(src)
    expect(tl.ok).toBe(true)
    const keys = tl.phases.map((p) => p.key)
    expect(keys).toEqual([
      'inbound',
      'auth',
      'channel',
      'upstream',
      'first_token',
      'complete',
    ])
    const firstToken = tl.phases.find((p) => p.key === 'first_token')
    expect(firstToken?.offsetMs).toBe(480)
    const complete = tl.phases.find((p) => p.key === 'complete')
    expect(complete?.offsetMs).toBe(3200)
    const channel = tl.phases.find((p) => p.key === 'channel')
    expect(channel?.detail).toBe('7 -> 12')
  })

  test('failed request surfaces fail phase with reason', () => {
    const src: TimelineSource = {
      created_at: 1_700_000_000,
      use_time: 0.4,
      frt: null,
      stream_status: { status: 'error', end_reason: 'timeout', end_error: 'upstream timeout' },
    }
    const tl = buildRequestTimeline(src)
    expect(tl.ok).toBe(false)
    expect(tl.failReason).toBe('upstream timeout')
    const complete = tl.phases.find((p) => p.key === 'complete')
    expect(complete?.status).toBe('failed')
    expect(complete?.detail).toBe('upstream timeout')
  })

  test('legacy log without extra fields degrades gracefully', () => {
    const src: TimelineSource = { created_at: 1_700_000_000, use_time: 1.1 }
    const tl = buildRequestTimeline(src)
    expect(tl.phases.length).toBeGreaterThanOrEqual(4)
    // 无失败标记 → ok
    expect(tl.ok).toBe(true)
    // 无渠道链 → channel 阶段跳过（仅 upstream 之前无 detail）
    const channel = tl.phases.find((p) => p.key === 'channel')
    expect(channel).toBeUndefined()
  })

  test('affinity-only channel info produces info phase', () => {
    const src: TimelineSource = {
      created_at: 1_700_000_000,
      use_time: 2.0,
      frt: 300,
      channel_affinity: { rule_name: 'latency', using_group: 'default' },
    }
    const tl = buildRequestTimeline(src)
    const channel = tl.phases.find((p) => p.key === 'channel')
    expect(channel?.status).toBe('info')
    expect(channel?.detail).toContain('latency')
  })
})

describe('exportTimelineJson', () => {
  test('produces stable v1 schema', () => {
    const src: TimelineSource = {
      created_at: 1_700_000_000,
      use_time: 1.5,
      frt: 200,
      request_path: '/v1/chat/completions',
    }
    const tl = buildRequestTimeline(src)
    const json = JSON.parse(exportTimelineJson(src, tl))
    expect(json.schema).toBe('new-api.request-timeline.v1')
    expect(Array.isArray(json.phases)).toBe(true)
    expect(json.source.use_time_ms).toBe(1500)
    expect(json.source.frt_ms).toBe(200)
    // 稳定结构：每个 phase 都有 phase/status/offset_ms/duration_ms/detail
    for (const p of json.phases) {
      expect(typeof p.phase).toBe('string')
      expect(typeof p.offset_ms).toBe('number')
    }
  })
})

describe('formatDurationMs', () => {
  test('converts ms to human-readable units', () => {
    expect(formatDurationMs(350)).toBe('350ms')
    expect(formatDurationMs(1500)).toBe('1.5s')
    expect(formatDurationMs(135000)).toBe('2m 15s')
    expect(formatDurationMs(90000)).toBe('1m 30s')
    expect(formatDurationMs(3600000)).toBe('1h')
    expect(formatDurationMs(0)).toBe('0ms')
    expect(formatDurationMs(-5)).toBe('0ms')
  })
})

describe('buildRequestTimeline non-stream upstream inference', () => {
  test('failed non-stream request marks upstream failed with duration', () => {
    const src: TimelineSource = {
      created_at: 1_700_000_000,
      use_time: 135,
      frt: null,
      stream_status: { status: 'error', end_reason: 'error', end_error: 'Upstream request failed' },
    }
    const tl = buildRequestTimeline(src)
    const upstream = tl.phases.find((p) => p.key === 'upstream')
    expect(upstream?.status).toBe('failed')
    expect(upstream?.durationMs).toBe(135000)
    const complete = tl.phases.find((p) => p.key === 'complete')
    expect(complete?.status).toBe('failed')
    expect(complete?.durationMs).toBe(0)
    expect(tl.ok).toBe(false)
  })

  test('successful non-stream request marks upstream done with duration', () => {
    const src: TimelineSource = { created_at: 1_700_000_000, use_time: 2, frt: null }
    const tl = buildRequestTimeline(src)
    const upstream = tl.phases.find((p) => p.key === 'upstream')
    expect(upstream?.status).toBe('done')
    expect(upstream?.durationMs).toBe(2000)
    expect(tl.ok).toBe(true)
  })
})
