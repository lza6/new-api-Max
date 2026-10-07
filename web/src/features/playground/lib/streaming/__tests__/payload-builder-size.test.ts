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
// 图片尺寸参数（size）在请求体里的拼装：仅启用且非空时发送。
import { describe, expect, it } from 'vitest'

import { DEFAULT_CONFIG, DEFAULT_PARAMETER_ENABLED } from '../../../constants'
import type { Message } from '../../../types'
import { buildChatCompletionPayload } from '../payload-builder'

const messages: Message[] = [
  {
    key: 'k1',
    from: 'user',
    versions: [{ id: 'v1', content: '一只橘猫' }],
    status: 'complete',
  } as unknown as Message,
]

describe('buildChatCompletionPayload size', () => {
  it('omits size when parameter disabled', () => {
    const payload = buildChatCompletionPayload(messages, {
      ...DEFAULT_CONFIG,
      model: 'gpt-image-2.5',
      size: '1024x1024',
    }, { ...DEFAULT_PARAMETER_ENABLED, size: false })
    expect(payload.size).toBeUndefined()
  })

  it('sends size when enabled and set', () => {
    const payload = buildChatCompletionPayload(messages, {
      ...DEFAULT_CONFIG,
      model: 'gpt-image-2.5',
      size: '2048x2048',
    }, { ...DEFAULT_PARAMETER_ENABLED, size: true })
    expect(payload.size).toBe('2048x2048')
  })

  it('omits size when enabled but empty', () => {
    const payload = buildChatCompletionPayload(messages, {
      ...DEFAULT_CONFIG,
      model: 'gpt-image-2.5',
      size: '',
    }, { ...DEFAULT_PARAMETER_ENABLED, size: true })
    expect(payload.size).toBeUndefined()
  })
})

// 图生图 / 多图参考：带附图的消息应组装为多模态 content parts。
describe('buildChatCompletionPayload multimodal images', () => {
  it('builds image_url parts for a message with images', () => {
    const withImage: Message[] = [
      {
        key: 'k2',
        from: 'user',
        versions: [
          {
            id: 'v2',
            content: '改成水彩风格',
            images: ['data:image/png;base64,AAAA', 'data:image/png;base64,BBBB'],
          },
        ],
        status: 'complete',
      } as unknown as Message,
    ]
    const payload = buildChatCompletionPayload(
      withImage,
      { ...DEFAULT_CONFIG, model: 'gpt-4o' },
      DEFAULT_PARAMETER_ENABLED
    )
    const content = payload.messages[0].content
    expect(Array.isArray(content)).toBe(true)
    const parts = content as { type: string; image_url?: { url: string } }[]
    expect(parts[0].type).toBe('text')
    expect(parts.filter((p) => p.type === 'image_url')).toHaveLength(2)
  })

  it('keeps plain string content when no images', () => {
    const payload = buildChatCompletionPayload(
      messages,
      { ...DEFAULT_CONFIG, model: 'gpt-4o' },
      DEFAULT_PARAMETER_ENABLED
    )
    expect(typeof payload.messages[0].content).toBe('string')
  })
})
