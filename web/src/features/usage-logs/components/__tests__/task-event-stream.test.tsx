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
import { afterEach, beforeAll, describe, expect, test, vi } from 'vitest'

import { parseSseChunk, summarizePayload } from '../../lib/task-event-stream'
import { TaskEventStream } from '../task-event-stream'

vi.mock('@/lib/auth-session', () => ({
  getFreshAuthHeaders: vi.fn().mockResolvedValue({ Authorization: 'Bearer x' }),
}))

const i18nKeys = {
  'Task Event Stream': 'Task Event Stream',
  'Stream ended': 'Stream ended',
  Live: 'Live',
  'Waiting for task events': 'Waiting for task events',
  'Event.submitted': 'Submitted',
  'Event.succeeded': 'Succeeded',
}

describe('parseSseChunk (B4-1)', () => {
  test('parses id/event/data lines', () => {
    expect(
      parseSseChunk(
        'id: 7\nevent: succeeded\ndata: {"seq":7,"ts":100,"type":"succeeded","payload":{}}'
      )
    ).toEqual({
      id: 7,
      event: 'succeeded',
      data: '{"seq":7,"ts":100,"type":"succeeded","payload":{}}',
    })
  })

  test('ignores heartbeat comment lines', () => {
    expect(parseSseChunk(': ping')).toBeNull()
  })

  test('parses done event', () => {
    expect(parseSseChunk('event: done\ndata: {}')).toEqual({
      event: 'done',
      data: '{}',
    })
  })
})

describe('summarizePayload (B4-1)', () => {
  test('picks known scalar fields', () => {
    expect(
      summarizePayload({ status: 'SUCCESS', progress: '100%', quota: 12 })
    ).toBe('status=SUCCESS progress=100% quota=12')
  })

  test('falls back to trimmed JSON for unknown payloads', () => {
    const out = summarizePayload({ random: 'x'.repeat(200) })
    expect(out.length).toBeLessThanOrEqual(121)
    expect(out.endsWith('…')).toBe(true)
  })
})

describe('TaskEventStream (B4-1)', () => {
  beforeAll(() => {
    i18next.addResourceBundle('en', 'translation', i18nKeys)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  test('renders pushed events and ends on done', async () => {
    const stream = new ReadableStream({
      start(controller) {
        const encoder = new TextEncoder()
        controller.enqueue(
          encoder.encode(
            'id: 1\nevent: submitted\ndata: {"seq":1,"ts":1726000000,"type":"submitted","payload":{"status":"SUBMITTED"}}\n\n'
          )
        )
        controller.enqueue(encoder.encode('event: done\ndata: {}\n\n'))
        controller.close()
      },
    })
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({ ok: true, status: 200, body: stream })
    )
    render(<TaskEventStream taskId='task-1' />)
    expect(await screen.findByText('Submitted')).toBeInTheDocument()
    expect(await screen.findByText('Stream ended')).toBeInTheDocument()
    expect(
      screen.getByText('status=SUBMITTED')
    ).toBeInTheDocument()
  })
})