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
// Message types
export type MessageRole = 'user' | 'assistant' | 'system'

export type MessageStatus = 'loading' | 'streaming' | 'complete' | 'error'

export type PlaygroundMessageLayoutMode = 'alternating' | 'left'

export interface MessageVersion {
  id: string
  content: string
  // 多模态：本版本附带的图片 URL（data URL 或远程 URL）。用于图生图 / 多图参考。
  // 空/未设 = 纯文本消息。持久化到 localStorage 的 schema 已含该可选字段。
  images?: string[]
}

export interface Message {
  key: string
  from: MessageRole
  versions: MessageVersion[]
  createdAt?: number
  startedAt?: number
  completedAt?: number
  durationMs?: number
  sources?: { href: string; title: string }[]
  reasoning?: {
    content: string
    duration: number
    startedAt?: number
    completedAt?: number
    durationMs?: number
  }
  isReasoningStreaming?: boolean
  isReasoningComplete?: boolean
  isContentComplete?: boolean
  status?: MessageStatus
  errorCode?: string | null
}

// API payload types
export interface ChatCompletionMessage {
  role: MessageRole
  content: string | ContentPart[]
}

export interface ContentPart {
  type: 'text' | 'image_url'
  text?: string
  image_url?: {
    url: string
  }
}

export interface ChatCompletionRequest {
  model: string
  group?: string
  messages: ChatCompletionMessage[]
  stream: boolean
  temperature?: number
  top_p?: number
  max_tokens?: number
  frequency_penalty?: number
  presence_penalty?: number
  seed?: number
  // 图片生成模型（如 gpt-image 系列走 chat/completions）支持 size 档位：
  // 宽x高，如 1024x1024 / 1536x1024 / 2048x2048 / 4096x4096。
  size?: string
  // 思考程度：none（关闭）/ low / medium / high。关闭时上游不进行思考。
  reasoning_effort?: string
}

export interface ChatCompletionChunk {
  id: string
  object: string
  created: number
  model: string
  choices: Array<{
    index: number
    delta: {
      role?: MessageRole
      content?: string
      reasoning_content?: string
    }
    finish_reason: string | null
  }>
}

export interface ChatCompletionResponse {
  id: string
  object: string
  created: number
  model: string
  choices: Array<{
    index: number
    message: {
      role: MessageRole
      content: string
      reasoning_content?: string
    }
    finish_reason: string
  }>
  usage?: {
    prompt_tokens: number
    completion_tokens: number
    total_tokens: number
  }
}

// Configuration types
export interface PlaygroundConfig {
  model: string
  group: string
  temperature: number
  top_p: number
  max_tokens: number
  frequency_penalty: number
  presence_penalty: number
  seed: number | null
  stream: boolean
  // 图片生成尺寸（宽x高）。空串 = 不发送 size（上游用默认）。
  size: string
  // 思考程度（reasoning_effort）。'' = 不发送（上游默认）；none=关闭思考；
  // 其余（low/medium/high）为可调档位。仅对支持的模型有效。
  reasoning_effort: string
  // T13 系统提示词（角色设定）。'' = 不发送。作为 messages 的首条 system 消息
  // 发送；Agent 预设应用时会写入这里。
  system_prompt: string
}

/**
 * T13 Agent 预设：可复用的对话配置快照（per-agent 能力下沉）。
 *
 * 与 `UserSetting.agent_presets` 同构（服务端存储于用户设置的 JSON 列）。
 * 应用预设 = 把这里记录的字段写进 PlaygroundConfig。
 */
export interface AgentPreset {
  id: string
  name: string
  model?: string
  group?: string
  system_prompt?: string
  temperature?: number
  max_tokens?: number
  reasoning_effort?: string
}

export interface ParameterEnabled {
  temperature: boolean
  top_p: boolean
  max_tokens: boolean
  frequency_penalty: boolean
  presence_penalty: boolean
  seed: boolean
  size: boolean
  reasoning_effort: boolean
}

/**
 * P1-6 视频生成：提交给 POST /v1/video/generations 的请求体。
 */
export interface VideoGenerationRequest {
  model: string
  prompt: string
  group?: string
  /** 时长（秒，字符串以贴合上游契约）。空 = 不发送。 */
  seconds?: string
  /** 分辨率档位，如 "1280x720"。空 = 不发送。 */
  size?: string
}

/** 归一化后的生成状态（三种语义）。 */
export type VideoGenerationStatus = 'pending' | 'succeeded' | 'failed'

/**
 * P1-6 视频任务响应（轮询）。字段取自后端 TaskDto 与 OpenAI Video API
 * 两种格式的并集；解析统一走 lib/video/video-generation-utils.ts。
 */
export interface VideoTaskResponse {
  task_id?: string
  id?: string
  status?: string
  progress?: number
  result_url?: string
  url?: string
  video_url?: string
  fail_reason?: string
  error?: string
}

// Model and group options
export interface ModelOption {
  label: string
  value: string
}

export interface GroupOption {
  label: string
  value: string
  ratio: number
  desc?: string
}
