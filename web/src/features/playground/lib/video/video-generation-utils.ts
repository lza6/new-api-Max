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
// P1-6 游乐场视频生成：与后端 task 平台对接的纯逻辑层。
//
// 后端契约（已存在，本次只在前端消费）：
//   POST /v1/video/generations        → 提交，返回 { task_id }
//   GET  /v1/video/generations/:id    → 轮询，OpenAI Video API 格式
//
// 这里只放**纯函数**（请求体构造 + 状态判定 + 轮询节奏），不碰网络与 React，
// 因此可以充分单测；网络调用留在 hooks/video 里。
import type {
  VideoGenerationRequest,
  VideoGenerationStatus,
  VideoTaskResponse,
} from '../../types'

/** 轮询起始间隔与上限（指数退避，避免把上游打爆又保证及时反馈）。 */
export const VIDEO_POLL_INITIAL_MS = 3000
export const VIDEO_POLL_MAX_MS = 15000
/** 单次生成的整体等待上限，避免轮询无限进行（对齐多数视频上游的排队时长）。 */
export const VIDEO_POLL_TIMEOUT_MS = 10 * 60 * 1000

/** 下一次轮询间隔（指数退避，封顶）。 */
export function nextPollDelay(attempt: number): number {
  const delay = VIDEO_POLL_INITIAL_MS * 2 ** Math.max(attempt, 0)
  return Math.min(delay, VIDEO_POLL_MAX_MS)
}

/** 是否已到整体等待上限。 */
export function isPollTimedOut(
  startedAt: number,
  now: number = Date.now()
): boolean {
  return now - startedAt >= VIDEO_POLL_TIMEOUT_MS
}

/**
 * 构造提交给 /v1/video/generations 的请求体。
 *
 * 只带上真正有值的字段——空字符串/未设置的参数不能发给上游（上游普遍会因此 400，
 * 且与「用户没填」的语义不符）。
 */
export function buildVideoGenerationRequest(input: {
  model: string
  prompt: string
  group?: string
  seconds?: string
  size?: string
}): VideoGenerationRequest {
  const payload: VideoGenerationRequest = {
    model: input.model,
    prompt: input.prompt,
  }
  if (input.group) {
    payload.group = input.group
  }
  if (input.seconds) {
    payload.seconds = input.seconds
  }
  if (input.size) {
    payload.size = input.size
  }
  return payload
}

/**
 * 从提交响应里取任务 id。
 * 后端可能返回 `task_id` 或 `id`（OpenAI Video API 用 `id`），两者都接受。
 */
export function extractTaskId(response: unknown): string | null {
  if (!response || typeof response !== 'object') {
    return null
  }
  const record = response as Record<string, unknown>
  for (const key of ['task_id', 'id', 'taskId']) {
    const value = record[key]
    if (typeof value === 'string' && value.trim() !== '') {
      return value
    }
  }
  return null
}

/**
 * 归一化轮询响应的状态。
 *
 * 上游/适配层的状态词表不完全统一（queued/processing/completed/failed 与
 * submitted/in_progress/success/failure 并存），这里收敛成三种终态语义。
 */
export function normalizeVideoStatus(
  raw: string | undefined
): VideoGenerationStatus {
  switch ((raw ?? '').toLowerCase()) {
    case 'success':
    case 'succeeded':
    case 'completed':
    case 'complete':
      return 'succeeded'
    case 'failure':
    case 'failed':
    case 'error':
      return 'failed'
    default:
      return 'pending'
  }
}

export type VideoPollOutcome = {
  status: VideoGenerationStatus
  /** 成功时可播放/下载的地址（上游直链或产物代理地址）。 */
  url?: string
  /** 失败原因（可展示）。 */
  failReason?: string
  /** 0-100，未知时为 undefined。 */
  progress?: number
}

/**
 * 解析一次轮询响应，得出本次轮询的结论（纯函数）。
 *
 * 成功时优先取 `result_url` / `url` / `data[0].url`：
 * 后端 `TaskModel2Dto` 给的是 `result_url`，而 OpenAI Video API 格式给的是
 * 顶层 `url` 或 `video_url`，两种都要认。
 */
export function parsePollResponse(response: unknown): VideoPollOutcome {
  if (!response || typeof response !== 'object') {
    return { status: 'pending' }
  }
  const record = response as VideoTaskResponse & Record<string, unknown>

  const status = normalizeVideoStatus(
    typeof record.status === 'string' ? record.status : undefined
  )
  if (status === 'pending') {
    return {
      status,
      progress:
        typeof record.progress === 'number' ? record.progress : undefined,
    }
  }
  if (status === 'failed') {
    return {
      status,
      failReason:
        (typeof record.fail_reason === 'string' && record.fail_reason) ||
        (typeof record.error === 'string' && record.error) ||
        undefined,
    }
  }

  return { status, url: extractResultUrl(record) ?? undefined }
}

/**
 * 从响应里找出可用的结果 URL。
 * 覆盖后端 TaskDto（`result_url`）、OpenAI Video API（`url`/`video_url`）
 * 与部分上游的 `data[].url` 形态。
 */
export function extractResultUrl(response: unknown): string | null {
  if (!response || typeof response !== 'object') {
    return null
  }
  const record = response as Record<string, unknown>

  for (const key of ['result_url', 'url', 'video_url']) {
    const value = record[key]
    if (typeof value === 'string' && value.trim() !== '') {
      return value
    }
  }

  // OpenAI Video API 有时把结果放在 data 数组里。
  if (Array.isArray(record.data)) {
    for (const item of record.data) {
      if (item && typeof item === 'object') {
        const url = (item as Record<string, unknown>).url
        if (typeof url === 'string' && url.trim() !== '') {
          return url
        }
      }
    }
  }
  return null
}

/** 是否应该继续轮询（未到终态且未超时）。 */
export function shouldKeepPolling(
  outcome: VideoPollOutcome,
  startedAt: number,
  now: number = Date.now()
): boolean {
  return outcome.status === 'pending' && !isPollTimedOut(startedAt, now)
}

/**
 * 判断当前模型是否走视频生成（宽松关键词匹配）。
 *
 * 与既有的 `isImageSizeModel` 同一策略：模型命名约定不统一（各家上游自定义），
 * 精确清单会随上游新增而失效，因此用关键词启发式，宁可漏判也不错判——
 * 用户始终可以手动切换模式兜底。
 */
export function isVideoModel(model: string): boolean {
  const m = (model || '').toLowerCase()
  if (m === '') {
    return false
  }
  // 常见视频模型/平台关键词：kling(可灵)、jimeng(即梦)、sora、veo、
  // hailuo(海螺)、vidu、doubao-video、runway、pika、luma、seedance 等。
  return [
    'video',
    'kling',
    'jimeng',
    'sora',
    'veo',
    'hailuo',
    'vidu',
    'runway',
    'pika',
    'luma',
    'seedance',
    'wan2',
    'wanx-video',
  ].some((keyword) => m.includes(keyword))
}
