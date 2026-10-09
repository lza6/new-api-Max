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
// P1-6 视频生成：提交 + 轮询的 React 封装。
//
// 网络与定时器都在这里，纯逻辑（请求体构造 / 状态归一化 / 退避节奏）在
// `lib/video/video-generation-utils.ts`，后者可充分单测。
import { useCallback, useEffect, useRef, useState } from 'react'

import { getVideoTask, submitVideoGeneration } from '../api'
import {
  buildVideoGenerationRequest,
  extractTaskId,
  nextPollDelay,
  parsePollResponse,
  shouldKeepPolling,
  type VideoPollOutcome,
} from '../lib/video/video-generation-utils'
import type { PlaygroundConfig } from '../types'

export type VideoGenerationState = {
  /** 是否正在生成（提交中或轮询中）。 */
  isGenerating: boolean
  /** 归一化状态；idle = 尚未提交。 */
  status: 'idle' | 'pending' | 'succeeded' | 'failed'
  /** 成功后的视频地址。 */
  url: string | null
  /** 失败原因（可展示）。 */
  failReason: string | null
  /** 0-100，未知为 null。 */
  progress: number | null
  /** 最近一次错误（网络/提交失败）。 */
  error: string | null
}

const IDLE: VideoGenerationState = {
  isGenerating: false,
  status: 'idle',
  url: null,
  failReason: null,
  progress: null,
  error: null,
}

export function useVideoGeneration() {
  const [state, setState] = useState<VideoGenerationState>(IDLE)
  // 定时器与取消信号：卸载或重新提交时必须清干净，避免泄漏与竞态。
  const timerRef = useRef<number | null>(null)
  const abortRef = useRef<AbortController | null>(null)

  const clearTimer = useCallback(() => {
    if (timerRef.current !== null) {
      window.clearTimeout(timerRef.current)
      timerRef.current = null
    }
  }, [])

  const cancel = useCallback(() => {
    clearTimer()
    abortRef.current?.abort()
    abortRef.current = null
    setState(IDLE)
  }, [clearTimer])

  useEffect(
    () => () => {
      clearTimer()
      abortRef.current?.abort()
    },
    [clearTimer]
  )

  const generate = useCallback(
    async (input: { config: PlaygroundConfig; prompt: string; seconds?: string; size?: string }) => {
      const prompt = input.prompt.trim()
      if (prompt === '' || input.config.model === '') {
        return
      }

      // 重新提交前先把上一次的定时器与请求清掉（避免双轮询）。
      clearTimer()
      abortRef.current?.abort()
      const controller = new AbortController()
      abortRef.current = controller

      setState({ ...IDLE, isGenerating: true, status: 'pending' })

      try {
        const submitted = await submitVideoGeneration(
          buildVideoGenerationRequest({
            model: input.config.model,
            prompt,
            group: input.config.group,
            seconds: input.seconds,
            size: input.size,
          }),
          controller.signal
        )

        const taskId = extractTaskId(submitted)
        if (!taskId) {
          setState({
            ...IDLE,
            status: 'failed',
            failReason: null,
            error: 'no_task_id',
          })
          return
        }

        const startedAt = Date.now()
        let attempt = 0

        const poll = async () => {
          try {
            const raw = await getVideoTask(taskId, controller.signal)
            const outcome: VideoPollOutcome = parsePollResponse(raw)

            if (outcome.status === 'pending') {
              if (!shouldKeepPolling(outcome, startedAt)) {
                // 超时：明确告知用户，而不是永远转圈。
                setState((prev) => ({
                  ...prev,
                  isGenerating: false,
                  status: 'failed',
                  failReason: 'timeout',
                }))
                return
              }
              setState((prev) => ({
                ...prev,
                progress: outcome.progress ?? prev.progress,
              }))
              attempt += 1
              timerRef.current = window.setTimeout(poll, nextPollDelay(attempt))
              return
            }

            if (outcome.status === 'succeeded') {
              setState({
                isGenerating: false,
                status: 'succeeded',
                url: outcome.url ?? null,
                failReason: null,
                progress: 100,
                error: null,
              })
              return
            }

            setState({
              isGenerating: false,
              status: 'failed',
              url: null,
              failReason: outcome.failReason ?? null,
              progress: null,
              error: null,
            })
          } catch (error) {
            // 主动取消不算错误（用户点了取消/切页）。
            if (controller.signal.aborted) {
              return
            }
            setState((prev) => ({
              ...prev,
              isGenerating: false,
              status: 'failed',
              error: error instanceof Error ? error.message : 'poll_failed',
            }))
          }
        }

        timerRef.current = window.setTimeout(poll, nextPollDelay(0))
      } catch (error) {
        if (controller.signal.aborted) {
          return
        }
        setState({
          ...IDLE,
          status: 'failed',
          error: error instanceof Error ? error.message : 'submit_failed',
        })
      }
    },
    [clearTimer]
  )

  const reset = useCallback(() => {
    clearTimer()
    abortRef.current?.abort()
    abortRef.current = null
    setState(IDLE)
  }, [clearTimer])

  return { ...state, generate, cancel, reset }
}
