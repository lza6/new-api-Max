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
// 模型能力分类：选择器据此把模型分成文本/图片/视频/音频/向量几组。
import { getModelKindLabel } from './lib/models/model-kind'
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
 * 把 `/api/user/models` 返回的**模型名数组**映射为选择器用的 `ModelOption[]`。
 *
 * 抽成导出的**纯函数**有两个理由：
 *  1. 分类逻辑（`category`）是可以直接单测的业务规则，不需要为了测它去 mock
 *     整个 axios 实例——mock 网络层在完整测试套件里会因模块注册表已被其它文件
 *     加载而失效（表现为「单独跑通过、全量跑 Network Error」）。
 *  2. 该映射是选择器分组的**唯一数据来源**，显式导出后可以被契约测试直接覆盖。
 *
 * 非数组 / 非字符串项一律跳过：上游改返回结构时宁可少几个模型，也不要让
 * 选择器渲染出 `undefined` 条目。
 */
export function mapUserModels(raw: unknown): ModelOption[] {
  if (!Array.isArray(raw)) {
    return []
  }
  return raw
    .filter((model): model is string => typeof model === 'string' && model !== '')
    .map((model) => ({
      label: model,
      value: model,
      // 按能力归类，供模型选择器分组（文本/图片/视频/音频/向量）。
      // `/api/user/models` 只返回模型名字符串、不带能力元数据，所以这里用规则分类；
      // 不设置 category 时选择器会把所有模型塞进一个 "Other" 组。
      category: getModelKindLabel(model),
    }))
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

  if (!data.success) {
    return []
  }

  return mapUserModels(data.data)
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
