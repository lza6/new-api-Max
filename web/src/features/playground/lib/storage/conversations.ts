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
// 游乐场多会话（P0-4）：把「单会话」升级为「会话列表 + 当前会话」。
//
// 设计：会话列表与当前会话 id 存 localStorage；每个会话的消息沿用既有
// `messagesSchema` 校验/裁剪逻辑。**向后兼容**：旧版单会话数据
// （STORAGE_KEYS.MESSAGES = 'playground_messages'）在首次加载时迁移为第一个会话，
// 迁移后清理旧键，避免二次迁移。
//
// 该模块只做纯数据读写（无 React），便于单测与复用。
import { nanoid } from 'nanoid'
import { z } from 'zod'

import { STORAGE_KEYS } from '../../constants'
import { messagesSchema } from './storage-schema'
import type { Message } from '../../types'

const CONVERSATIONS_KEY = 'playground_conversations'
const ACTIVE_CONVERSATION_KEY = 'playground_active_conversation'
const MAX_CONVERSATIONS = 50

export interface Conversation {
  id: string
  title: string
  createdAt: number
  updatedAt: number
  messages: Message[]
}

const conversationSchema = z.object({
  id: z.string(),
  title: z.string(),
  createdAt: z.number(),
  updatedAt: z.number(),
  messages: messagesSchema,
})

const conversationsSchema = z.array(conversationSchema)

function readRaw(key: string): string | null {
  try {
    return localStorage.getItem(key)
  } catch {
    return null
  }
}

function writeRaw(key: string, value: string): boolean {
  try {
    localStorage.setItem(key, value)
    return true
  } catch {
    // localStorage 不可用 / 配额满：返回 false，由调用方降级处理。
    return false
  }
}

/** 从首条用户消息生成会话标题（截断）。无用户消息时用占位。 */
export function deriveConversationTitle(messages: Message[]): string {
  const firstUser = messages.find((m) => m.from === 'user')
  const text = firstUser?.versions?.[0]?.content?.trim() ?? ''
  if (!text) {
    return 'New chat'
  }
  const oneLine = text.replace(/\s+/g, ' ')
  return oneLine.length > 40 ? `${oneLine.slice(0, 40)}…` : oneLine
}

export function createConversation(messages: Message[] = []): Conversation {
  const now = Date.now()
  return {
    id: nanoid(),
    title: deriveConversationTitle(messages),
    createdAt: now,
    updatedAt: now,
    messages,
  }
}

/**
 * 加载会话列表。首次调用时若存在旧版单会话数据，则迁移为第一个会话。
 * 返回值永不为空（至少含一个会话），保证 UI 有可用会话。
 *
 * 容错：**逐会话**校验 —— 单个会话数据损坏不会丢弃整个列表（否则一条坏消息
 * 会导致用户全部会话消失）。损坏的会话被安全跳过。
 */
export function loadConversations(): Conversation[] {
  const migrated = migrateLegacyMessages()
  const raw = readRaw(CONVERSATIONS_KEY)
  if (raw) {
    let decoded: unknown
    try {
      decoded = JSON.parse(raw)
    } catch {
      decoded = null
    }
    if (Array.isArray(decoded)) {
      const salvaged: Conversation[] = []
      for (const item of decoded) {
        const result = conversationSchema.safeParse(item)
        if (result.success) {
          salvaged.push(result.data as Conversation)
        }
      }
      if (salvaged.length > 0) {
        return salvaged
      }
    }
  }
  return migrated ?? [createConversation()]
}

/** 保存会话列表（裁剪到上限，保留最近更新的）。
 *
 * 配额/校验失败时**逐会话降级重试**：从最旧的会话开始丢弃，直到写入成功——
 * 避免「附图撑爆 localStorage 导致当前会话完全无法持久化」。 */
export function saveConversations(conversations: Conversation[]): void {
  const ordered = [...conversations]
    .sort((a, b) => b.updatedAt - a.updatedAt)
    .slice(0, MAX_CONVERSATIONS)
    .sort((a, b) => a.createdAt - b.createdAt)

  for (let drop = 0; drop < ordered.length; drop++) {
    // drop 个最旧的会话（保持至少一个）。
    const candidate = ordered.slice(drop)
    let serialized: string
    try {
      const parsed = conversationsSchema.parse(candidate)
      serialized = JSON.stringify(parsed)
    } catch {
      // 某会话校验失败：继续丢弃更旧的再试。
      continue
    }
    if (writeRaw(CONVERSATIONS_KEY, serialized)) {
      return
    }
    // 写入失败（配额满）：丢弃下一个最旧的再试。
  }
}

export function loadActiveConversationId(): string | null {
  return readRaw(ACTIVE_CONVERSATION_KEY)
}

export function saveActiveConversationId(id: string): void {
  writeRaw(ACTIVE_CONVERSATION_KEY, id)
}

/**
 * 迁移旧版单会话（playground_messages）到会话列表。
 * 返回迁移出的会话（无旧数据则 null）。迁移成功后删除旧键，幂等。
 */
function migrateLegacyMessages(): Conversation[] | null {
  // 已经有会话列表 → 不迁移（避免覆盖）。
  if (readRaw(CONVERSATIONS_KEY)) {
    return null
  }
  const legacy = readRaw(STORAGE_KEYS.MESSAGES)
  if (!legacy) {
    return null
  }
  try {
    const parsed = messagesSchema.parse(JSON.parse(legacy)) as Message[]
    if (parsed.length === 0) {
      return null
    }
    const conversation = createConversation(parsed)
    saveConversations([conversation])
    saveActiveConversationId(conversation.id)
    // 删除旧键，避免下次重复迁移。
    try {
      localStorage.removeItem(STORAGE_KEYS.MESSAGES)
    } catch {
      // ignore
    }
    return [conversation]
  } catch {
    return null
  }
}
