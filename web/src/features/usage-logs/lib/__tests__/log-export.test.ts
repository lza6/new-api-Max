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
  buildLogExportPayload,
  logExportFilename,
  type LogExportSource,
} from '../log-export'

describe('buildLogExportPayload (8.x JSON export)', () => {
  test('includes full structured log + timeline', () => {
    const log: LogExportSource = {
      id: 42,
      created_at: 1789800000,
      type: 2,
      user_id: 7,
      username: 'alice',
      model_name: 'gpt-4o-mini',
      channel: 20,
      channel_name: '稳定渠道',
      quota: 123,
      prompt_tokens: 1000,
      completion_tokens: 100,
      request_id: 'req-abc',
      other: { model_ratio: 5 },
    }
    const timeline = { stages: [{ name: 'relay', duration_ms: 3 }] }
    const payload = buildLogExportPayload(log, timeline)
    expect(payload.id).toBe(42)
    expect(payload.model_name).toBe('gpt-4o-mini')
    expect(payload.quota).toBe(123)
    expect(payload.other).toEqual({ model_ratio: 5 })
    expect(payload.timeline).toEqual(timeline)
  })

  test('old logs without timeline degrade gracefully (timeline null)', () => {
    const log: LogExportSource = { id: 1, type: 2 }
    const payload = buildLogExportPayload(log, null)
    expect(payload.id).toBe(1)
    expect(payload.timeline).toBeNull()
    expect(payload.request_id).toBeUndefined()
  })
})

describe('logExportFilename (8.x)', () => {
  test('prefers request_id, falls back to id/log', () => {
    expect(logExportFilename('req-1', 5)).toBe('log-req-1.json')
    expect(logExportFilename(undefined, 5)).toBe('log-5.json')
    expect(logExportFilename(undefined, undefined)).toBe('log-log.json')
  })
})