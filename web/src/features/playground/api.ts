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
import { api } from '@/lib/api'
import { requireServerSuccess } from '@/lib/server-error-message'

import { API_ENDPOINTS } from './constants'
import type {
  ChatCompletionRequest,
  ChatCompletionResponse,
  ModelOption,
  GroupOption,
  VideoGenerationRequest,
} from './types'

/**
 * Send chat completion request (non-streaming)
 */
export async function sendChatCompletion(
  payload: ChatCompletionRequest,
  signal?: AbortSignal
): Promise<ChatCompletionResponse> {
  const res = await api.post(API_ENDPOINTS.CHAT_COMPLETIONS, payload, {
    signal,
    skipErrorHandler: true,
  } as Record<string, unknown>)
  return res.data
}

/**
 * Get user available models
 */
export async function getUserModels(group: string): Promise<ModelOption[]> {
  const res = await api.get(API_ENDPOINTS.USER_MODELS, {
    params: { group },
  })
  const { data } = res
  requireServerSuccess(data)

  if (!data.success || !Array.isArray(data.data)) {
    return []
  }

  return data.data.map((model: string) => ({
    label: model,
    value: model,
  }))
}

/**
 * Get user groups
 */
export async function getUserGroups(): Promise<GroupOption[]> {
  const res = await api.get(API_ENDPOINTS.USER_GROUPS)
  const { data } = res
  requireServerSuccess(data)

  if (!data.success || !data.data) {
    return []
  }

  const groupData = data.data as Record<string, { desc: string; ratio: number }>

  // label is for button display (name only); desc is for dropdown content
  return Object.entries(groupData).map(([group, info]) => ({
    label: group,
    value: group,
    ratio: info.ratio,
    desc: info.desc,
  }))
}

// ============================================================================
// P1-6 视频生成（复用后端既有 task 平台端点）
// ============================================================================

/**
 * 提交一次视频生成任务，返回任务 id。
 *
 * 后端 `POST /v1/video/generations` 走 task 平台（异步），立即返回任务 id，
 * 之后由 `getVideoTask` 轮询。失败时抛出（由调用方统一展示）。
 */
export async function submitVideoGeneration(
  payload: VideoGenerationRequest,
  signal?: AbortSignal
): Promise<unknown> {
  const res = await api.post(API_ENDPOINTS.VIDEO_GENERATIONS, payload, {
    signal,
    skipErrorHandler: true,
  } as Record<string, unknown>)
  return res.data
}

/**
 * 轮询一次视频任务状态。
 * 后端返回 OpenAI Video API 格式或 TaskDto（两种都由
 * `lib/video/video-generation-utils.ts` 归一化）。
 */
export async function getVideoTask(
  taskId: string,
  signal?: AbortSignal
): Promise<unknown> {
  const res = await api.get(
    `${API_ENDPOINTS.VIDEO_GENERATIONS}/${encodeURIComponent(taskId)}`,
    { signal, skipErrorHandler: true } as Record<string, unknown>
  )
  return res.data
}
