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
*/
// P0-4 多会话：创建/标题推导/旧单会话迁移。
import { beforeEach, describe, expect, it } from 'vitest'

import type { Message } from '../../../types'
import {
  createConversation,
  deriveConversationTitle,
  loadConversations,
  saveConversations,
} from '../conversations'

const userMessage = (text: string): Message =>
  ({
    key: `k-${text}`,
    from: 'user',
    versions: [{ id: 'v1', content: text }],
    status: 'complete',
  }) as unknown as Message

describe('conversations (P0-4)', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('deriveConversationTitle uses first user message', () => {
    expect(deriveConversationTitle([userMessage('帮我写个快排')])).toBe(
      '帮我写个快排'
    )
    expect(deriveConversationTitle([])).toBe('New chat')
  })

  it('deriveConversationTitle truncates long input', () => {
    const long = 'x'.repeat(100)
    const title = deriveConversationTitle([userMessage(long)])
    expect(title.length).toBeLessThanOrEqual(41) // 40 + …
  })

  it('loadConversations returns at least one conversation', () => {
    const convs = loadConversations()
    expect(convs.length).toBeGreaterThanOrEqual(1)
    expect(convs[0].id).toBeTruthy()
  })

  it('migrates legacy single-conversation messages', () => {
    // 旧版单会话数据（playground_messages）。
    localStorage.setItem(
      'playground_messages',
      JSON.stringify([userMessage('旧会话内容')])
    )
    const convs = loadConversations()
    expect(convs).toHaveLength(1)
    expect(convs[0].messages).toHaveLength(1)
    // 迁移后清理旧键，避免二次迁移。
    expect(localStorage.getItem('playground_messages')).toBeNull()
  })

  it('save + reload round-trips conversations', () => {
    const a = createConversation([userMessage('会话A')])
    const b = createConversation([userMessage('会话B')])
    saveConversations([a, b])
    const loaded = loadConversations()
    expect(loaded.map((c) => c.id).sort()).toEqual([a.id, b.id].sort())
  })
})
