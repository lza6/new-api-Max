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
// P1-6 视频生成的**接线测试**：提交与轮询必须真的打后端既有端点
// （/v1/video/generations 与 /v1/video/generations/:id）。
//
// 只测纯函数证明不了这一点——正是「库全绿但生产零调用」的漏洞所在。
import { act, renderHook, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { useVideoGeneration } from '../use-video-generation'
import { DEFAULT_CONFIG } from '../../constants'
import { VIDEO_POLL_INITIAL_MS } from '../../lib/video/video-generation-utils'

afterEach(() => {
  vi.restoreAllMocks()
  vi.useRealTimers()
})

describe('useVideoGeneration wiring', () => {
  it('posts to the video endpoint then polls the task by id', async () => {
    const post = vi.spyOn(api, 'post').mockResolvedValue({
      data: { task_id: 'task_abc' },
    })
    const get = vi.spyOn(api, 'get').mockResolvedValue({
      data: { status: 'success', result_url: 'https://cdn/x.mp4' },
    })

    const { result } = renderHook(() => useVideoGeneration())

    await act(async () => {
      await result.current.generate({
        config: { ...DEFAULT_CONFIG, model: 'kling-v2', group: 'default' },
        prompt: 'a cat surfing',
        seconds: '8',
      })
    })

    // 提交：真实端点 + 只带用户设置的字段。
    expect(post).toHaveBeenCalledWith(
      '/v1/video/generations',
      {
        model: 'kling-v2',
        prompt: 'a cat surfing',
        group: 'default',
        seconds: '8',
      },
      expect.objectContaining({ skipErrorHandler: true })
    )

    // 轮询：用提交返回的 task_id 拼路径。
    await waitFor(() =>
      expect(get).toHaveBeenCalledWith(
        '/v1/video/generations/task_abc',
        expect.objectContaining({ skipErrorHandler: true })
      )
    )

    await waitFor(() => expect(result.current.status).toBe('succeeded'))
    expect(result.current.url).toBe('https://cdn/x.mp4')
    expect(result.current.isGenerating).toBe(false)
  })

  it('does nothing when the prompt is blank or no model is selected', async () => {
    const post = vi.spyOn(api, 'post').mockResolvedValue({ data: {} })
    const { result } = renderHook(() => useVideoGeneration())

    await act(async () => {
      await result.current.generate({
        config: { ...DEFAULT_CONFIG, model: 'kling-v2' },
        prompt: '   ',
      })
    })
    await act(async () => {
      await result.current.generate({
        config: { ...DEFAULT_CONFIG, model: '' },
        prompt: 'hello',
      })
    })

    expect(post).not.toHaveBeenCalled()
    expect(result.current.status).toBe('idle')
  })

  it('reports failure without leaving the panel spinning', async () => {
    vi.spyOn(api, 'post').mockResolvedValue({ data: { task_id: 't1' } })
    vi.spyOn(api, 'get').mockResolvedValue({
      data: { status: 'failed', fail_reason: 'content_policy' },
    })

    const { result } = renderHook(() => useVideoGeneration())
    await act(async () => {
      await result.current.generate({
        config: { ...DEFAULT_CONFIG, model: 'kling-v2' },
        prompt: 'x',
      })
    })

    await waitFor(() => expect(result.current.status).toBe('failed'))
    expect(result.current.failReason).toBe('content_policy')
    expect(result.current.isGenerating).toBe(false)
  })

  it('keeps polling while pending and stops once succeeded', async () => {
    vi.useFakeTimers()
    vi.spyOn(api, 'post').mockResolvedValue({ data: { task_id: 't2' } })
    const get = vi
      .spyOn(api, 'get')
      .mockResolvedValueOnce({ data: { status: 'processing' } })
      .mockResolvedValueOnce({ data: { status: 'completed', url: 'https://cdn/y.mp4' } })

    const { result } = renderHook(() => useVideoGeneration())
    await act(async () => {
      await result.current.generate({
        config: { ...DEFAULT_CONFIG, model: 'sora-2' },
        prompt: 'x',
      })
    })

    // 第一次轮询
    await act(async () => {
      await vi.advanceTimersByTimeAsync(VIDEO_POLL_INITIAL_MS)
    })
    expect(get).toHaveBeenCalledTimes(1)
    expect(result.current.status).toBe('pending')

    // 退避后的第二次轮询 → 成功。
    // 注意：这里不混用 waitFor（它内部依赖真实定时器）。
    await act(async () => {
      await vi.advanceTimersByTimeAsync(VIDEO_POLL_INITIAL_MS * 2)
    })
    expect(result.current.status).toBe('succeeded')
    expect(result.current.url).toBe('https://cdn/y.mp4')

    // 成功后不再轮询。
    const calls = get.mock.calls.length
    await act(async () => {
      await vi.advanceTimersByTimeAsync(VIDEO_POLL_INITIAL_MS * 8)
    })
    expect(get.mock.calls.length).toBe(calls)
  })

  it('cancel stops polling and clears state', async () => {
    vi.spyOn(api, 'post').mockResolvedValue({ data: { task_id: 't3' } })
    const get = vi.spyOn(api, 'get').mockResolvedValue({
      data: { status: 'processing' },
    })

    const { result } = renderHook(() => useVideoGeneration())
    await act(async () => {
      await result.current.generate({
        config: { ...DEFAULT_CONFIG, model: 'kling-v2' },
        prompt: 'x',
      })
    })
    await waitFor(() => expect(result.current.status).toBe('pending'))

    await act(async () => {
      result.current.cancel()
    })

    expect(result.current.status).toBe('idle')
    expect(result.current.isGenerating).toBe(false)

    const calls = get.mock.calls.length
    await new Promise((resolve) => setTimeout(resolve, VIDEO_POLL_INITIAL_MS + 50))
    expect(get.mock.calls.length).toBe(calls)
  })
})
