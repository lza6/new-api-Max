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
import { describe, expect, it } from 'vitest'

import {
  buildVideoGenerationRequest,
  extractResultUrl,
  extractTaskId,
  isPollTimedOut,
  isVideoModel,
  nextPollDelay,
  normalizeVideoStatus,
  parsePollResponse,
  shouldKeepPolling,
  VIDEO_POLL_MAX_MS,
  VIDEO_POLL_TIMEOUT_MS,
} from '../video-generation-utils'

describe('video generation request', () => {
  it('sends only the fields the user actually set', () => {
    const payload = buildVideoGenerationRequest({
      model: 'kling-v2',
      prompt: 'a cat surfing',
    })
    // 未设置的可选字段不能发给上游（上游普遍会因此 400）。
    expect(payload).toEqual({ model: 'kling-v2', prompt: 'a cat surfing' })
    expect('seconds' in payload).toBe(false)
    expect('size' in payload).toBe(false)
    expect('group' in payload).toBe(false)
  })

  it('includes optional fields when provided', () => {
    const payload = buildVideoGenerationRequest({
      model: 'kling-v2',
      prompt: 'a cat surfing',
      group: 'default',
      seconds: '8',
      size: '1280x720',
    })
    expect(payload).toEqual({
      model: 'kling-v2',
      prompt: 'a cat surfing',
      group: 'default',
      seconds: '8',
      size: '1280x720',
    })
  })
})

describe('video task id extraction', () => {
  it('accepts task_id, id and taskId', () => {
    expect(extractTaskId({ task_id: 'a' })).toBe('a')
    expect(extractTaskId({ id: 'b' })).toBe('b')
    expect(extractTaskId({ taskId: 'c' })).toBe('c')
    expect(extractTaskId({ data: { task_id: 'nested' } })).toBeNull()
    expect(extractTaskId({ task_id: '   ' })).toBeNull()
    expect(extractTaskId(null)).toBeNull()
    expect(extractTaskId('not-an-object')).toBeNull()
  })
})

describe('video status normalization', () => {
  it('collapses the divergent upstream vocabularies to three states', () => {
    for (const raw of ['success', 'SUCCEEDED', 'completed', 'complete']) {
      expect(normalizeVideoStatus(raw)).toBe('succeeded')
    }
    for (const raw of ['failure', 'failed', 'error']) {
      expect(normalizeVideoStatus(raw)).toBe('failed')
    }
    for (const raw of ['queued', 'processing', 'in_progress', 'submitted', '', undefined]) {
      expect(normalizeVideoStatus(raw)).toBe('pending')
    }
  })

  it('treats a missing status as pending, never as success', () => {
    // 关键安全语义：状态未知时必须继续等待，绝不能误判成功。
    expect(parsePollResponse({}).status).toBe('pending')
    expect(parsePollResponse(null).status).toBe('pending')
    expect(parsePollResponse('nope').status).toBe('pending')
  })
})

describe('video result url extraction', () => {
  it('finds the url across the shapes the backend may return', () => {
    expect(extractResultUrl({ result_url: 'https://x/a.mp4' })).toBe('https://x/a.mp4')
    expect(extractResultUrl({ url: 'https://x/b.mp4' })).toBe('https://x/b.mp4')
    expect(extractResultUrl({ video_url: 'https://x/c.mp4' })).toBe('https://x/c.mp4')
    expect(extractResultUrl({ data: [{ url: 'https://x/d.mp4' }] })).toBe('https://x/d.mp4')
    expect(extractResultUrl({ data: [] })).toBeNull()
    expect(extractResultUrl({})).toBeNull()
  })

  it('surfaces the failure reason when the task failed', () => {
    expect(parsePollResponse({ status: 'failed', fail_reason: 'nsfw' })).toMatchObject({
      status: 'failed',
      failReason: 'nsfw',
    })
    expect(parsePollResponse({ status: 'error', error: 'boom' })).toMatchObject({
      status: 'failed',
      failReason: 'boom',
    })
  })

  it('passes progress through while pending', () => {
    expect(parsePollResponse({ status: 'processing', progress: 42 })).toEqual({
      status: 'pending',
      progress: 42,
    })
  })
})

describe('polling cadence', () => {
  it('backs off exponentially and caps at the maximum', () => {
    expect(nextPollDelay(0)).toBe(3000)
    expect(nextPollDelay(1)).toBe(6000)
    expect(nextPollDelay(2)).toBe(12000)
    expect(nextPollDelay(3)).toBe(VIDEO_POLL_MAX_MS)
    expect(nextPollDelay(50)).toBe(VIDEO_POLL_MAX_MS)
  })

  it('stops polling at the overall deadline', () => {
    const start = 1_000_000
    expect(isPollTimedOut(start, start + VIDEO_POLL_TIMEOUT_MS - 1)).toBe(false)
    expect(isPollTimedOut(start, start + VIDEO_POLL_TIMEOUT_MS)).toBe(true)
    expect(shouldKeepPolling({ status: 'pending' }, start, start + 1)).toBe(true)
    expect(
      shouldKeepPolling({ status: 'pending' }, start, start + VIDEO_POLL_TIMEOUT_MS)
    ).toBe(false)
    // 已到终态就不再轮询。
    expect(shouldKeepPolling({ status: 'succeeded' }, start, start + 1)).toBe(false)
    expect(shouldKeepPolling({ status: 'failed' }, start, start + 1)).toBe(false)
  })
})

describe('video model detection', () => {
  it('recognizes common video model families', () => {
    for (const model of [
      'kling-v2',
      'jimeng-video',
      'sora-2',
      'veo-3',
      'hailuo-02',
      'vidu-q1',
      'doubao-video',
      'runway-gen4',
      'pika-1.5',
      'luma-ray2',
      'seedance-1',
    ]) {
      expect(isVideoModel(model), model).toBe(true)
    }
  })

  it('does not misclassify chat and image models', () => {
    for (const model of [
      'deepseek-v4.1-flash',
      'glm-5.3-flash',
      'gpt-image-1',
      'claude-sonnet-4',
      '',
    ]) {
      expect(isVideoModel(model), model).toBe(false)
    }
  })
})
