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
import i18next from 'i18next'
import { beforeAll, describe, expect, test } from 'vitest'

import { getFriendlyErrorMessage } from '@/lib/server-error-message'

// B6-2 前端人话映射：5 类机器可读错误 → 人话首行。
const FRIENDLY_KEYS = [
  'Insufficient quota. Please top up or redeem a quota card.',
  'The key is invalid or expired. Please rotate it on the channels page.',
  'Too many requests. Please try again later.',
  'The upstream service is temporarily unavailable. Please try again later.',
  'The content was blocked by a safety policy.',
]

function apiError(data: unknown): unknown {
  // 模拟网关返回的 OpenAI 错误信封：{error:{message,type,code}}。
  return { response: { data: { error: data } } }
}

describe('getFriendlyErrorMessage (B6-2)', () => {
  beforeAll(() => {
    const resources: Record<string, string> = {}
    for (const key of FRIENDLY_KEYS) {resources[key] = key}
    i18next.addResourceBundle('en', 'translation', resources)
  })

  // 5 类错误通过 error.type 命中（后端正归一化为 stable enum）。
  test('maps each machine error.type to a friendly first line', () => {
    const cases: Array<[unknown, string]> = [
      [
        { type: 'insufficient_quota', message: 'insufficient_user_quota' },
        FRIENDLY_KEYS[0],
      ],
      [
        { type: 'key_invalid', message: 'invalid api key' },
        FRIENDLY_KEYS[1],
      ],
      [
        { type: 'rate_limited', message: 'rate limited, please retry later' },
        FRIENDLY_KEYS[2],
      ],
      [
        { type: 'upstream_unavailable', message: 'upstream address rejected' },
        FRIENDLY_KEYS[3],
      ],
      [
        { type: 'content_filtered', message: 'prompt blocked' },
        FRIENDLY_KEYS[4],
      ],
    ]
    for (const [err, expected] of cases) {
      expect(getFriendlyErrorMessage(apiError(err))).toBe(expected)
    }
  })

  // 兼容：未命中时返回 null（保持原文案行为）。
  test('returns null when no friendly pattern matches', () => {
    expect(getFriendlyErrorMessage(apiError({ code: 'cosmic_gibberish' }))).toBeNull()
    expect(getFriendlyErrorMessage('random technical error xyz')).toBeNull()
  })

  // 401/429 等裸状态码也命中对应人话。
  test('matches via status-code hints too', () => {
    expect(
      getFriendlyErrorMessage({ response: { data: { message: '401 Unauthorized' } } })
    ).toBe(FRIENDLY_KEYS[1])
  })

  // B6-2 E2E 真实上游文案：只有 message（无 type）时也须命中人话。
  test('matches raw upstream messages without error.type', () => {
    const cases: Array<[string, string]> = [
      ['Insufficient balance', FRIENDLY_KEYS[0]],
      ['content was filtered by safety system', FRIENDLY_KEYS[4]],
      ['prompt was blocked by moderation', FRIENDLY_KEYS[4]],
    ]
    for (const [message, expected] of cases) {
      expect(
        getFriendlyErrorMessage({ response: { data: { error: { message } } } })
      ).toBe(expected)
    }
  })
})

  // G1-G3：无可用渠道 / channel 域保留码 / 内部计费技术码 → 人话（B2-3 收口）。
  test('maps channel availability, channel-domain and internal quota codes', () => {
    const cases: Array<[unknown, string]> = [
      [
        {
          type: 'get_channel_failed',
          message: '分组 default 下模型 deepseek-v4-flash 的可用渠道不存在（retry）',
        },
        'No available channel for this model right now. Please try again in a moment or choose another model.',
      ],
      [
        { type: 'channel:invalid_key', message: 'channel invalid key' },
        'The channel connection has an issue. Contact the administrator or try again later.',
      ],
      [
        { type: 'pre_consume_token_quota_failed', message: 'pre_consume_token_quota_failed' },
        'Insufficient quota. Please top up or redeem a quota card.',
      ],
    ]
    for (const [err, expected] of cases) {
      expect(getFriendlyErrorMessage(apiError(err))).toBe(expected)
    }
  })
