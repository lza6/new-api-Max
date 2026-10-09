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
// T13 系统提示词在请求体里的拼装：非空时作为首条 system 消息发送。
import { describe, expect, it } from 'vitest'

import { DEFAULT_CONFIG, DEFAULT_PARAMETER_ENABLED } from '../../../constants'
import type { Message } from '../../../types'
import { buildChatCompletionPayload } from '../payload-builder'

function userMessage(content: string): Message {
  return {
    key: `k-${content}`,
    from: 'user',
    versions: [{ id: `v-${content}`, content }],
    status: 'complete',
  } as unknown as Message
}

describe('buildChatCompletionPayload system prompt', () => {
  it('prepends the system prompt as the first message', () => {
    const payload = buildChatCompletionPayload(
      [userMessage('hi')],
      { ...DEFAULT_CONFIG, model: 'm', system_prompt: 'you are a Go engineer' },
      DEFAULT_PARAMETER_ENABLED
    )

    expect(payload.messages[0]).toEqual({
      role: 'system',
      content: 'you are a Go engineer',
    })
    expect(payload.messages).toHaveLength(2)
    expect(payload.messages[1]).toMatchObject({ role: 'user' })
  })

  it('omits the system message when the prompt is blank or whitespace', () => {
    for (const value of ['', '   ', '\n']) {
      const payload = buildChatCompletionPayload(
        [userMessage('hi')],
        { ...DEFAULT_CONFIG, model: 'm', system_prompt: value },
        DEFAULT_PARAMETER_ENABLED
      )
      expect(
        payload.messages.some((m) => m.role === 'system'),
        `blank prompt ${JSON.stringify(value)} must not add a system message`
      ).toBe(false)
    }
  })

  it('does not duplicate a system message the conversation already has', () => {
    const existingSystem: Message = {
      key: 'sys',
      from: 'system',
      versions: [{ id: 'v', content: 'conversation-level system' }],
      status: 'complete',
    } as unknown as Message

    const payload = buildChatCompletionPayload(
      [existingSystem, userMessage('hi')],
      { ...DEFAULT_CONFIG, model: 'm', system_prompt: 'config-level system' },
      DEFAULT_PARAMETER_ENABLED
    )

    const systemMessages = payload.messages.filter((m) => m.role === 'system')
    expect(systemMessages).toHaveLength(1)
    expect(systemMessages[0]).toMatchObject({
      content: 'conversation-level system',
    })
  })

  it('leaves the payload unchanged when no system prompt is configured', () => {
    const payload = buildChatCompletionPayload(
      [userMessage('hi')],
      { ...DEFAULT_CONFIG, model: 'm' },
      DEFAULT_PARAMETER_ENABLED
    )
    expect(payload.messages).toHaveLength(1)
    expect(payload.messages[0]).toMatchObject({ role: 'user' })
  })
})
