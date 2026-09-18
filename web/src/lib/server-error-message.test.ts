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
import { describe, expect, test } from 'vitest'

import {
  getFriendlyErrorMessage,
  getServerErrorMessageKey,
  safeServerErrorMessage,
} from './server-error-message'

describe('server error message mapping', () => {
  test('maps the active-session limit to recovery instructions', () => {
    const message = getServerErrorMessageKey({ code: 'AUTH_SESSION_LIMIT' })

    expect(message ?? '').toMatch(/Sign out other sessions/)
    expect(message ?? '').toMatch(/reset your password/)
  })

  test('maps an Axios-shaped issuance limit to rolling-window guidance', () => {
    const message = getServerErrorMessageKey({
      response: { data: { code: 'AUTH_SESSION_ISSUANCE_LIMIT' } },
    })

    expect(message ?? '').toMatch(/rolling window/)
    expect(getServerErrorMessageKey({ code: 'UNKNOWN_CODE' })).toBe(null)
  })

  test('maps stable Telegram bind errors without exposing server text', () => {
    const expected = {
      TELEGRAM_BIND_DISABLED: 'Telegram binding is disabled.',
      TELEGRAM_BIND_INVALID_REQUEST:
        'The Telegram authorization request is invalid or expired.',
      TELEGRAM_BIND_FLOW_INVALID:
        'This Telegram binding request has expired or has already been used.',
      TELEGRAM_BIND_SESSION_INVALID:
        'The login session that started this Telegram binding is no longer valid.',
      TELEGRAM_BIND_ALREADY_BOUND: 'This Telegram account is already bound.',
      TELEGRAM_BIND_USER_DELETED: 'This user account no longer exists.',
      TELEGRAM_BIND_USER_DISABLED: 'This user account is disabled.',
      TELEGRAM_BIND_INTERNAL_ERROR:
        'Telegram binding failed. Please try again.',
    }

    for (const [code, message] of Object.entries(expected)) {
      expect(getServerErrorMessageKey({ code })).toBe(message)
    }

    expect(
      getServerErrorMessageKey({
        response: {
          data: { code: 'TELEGRAM_BIND_INTERNAL_ERROR', message: 'raw detail' },
        },
      })
    ).toBe(expected.TELEGRAM_BIND_INTERNAL_ERROR)
  })
})

describe('friendly error message mapping (B2-3)', () => {
  test('maps channel cooldown to a recoverable explainer', () => {
    expect(
      getFriendlyErrorMessage({
        message: 'this channel is cooling down after recent failures',
      })
    ).toBe(
      'This channel is cooling down after recent failures. Please try again in a moment.'
    )
  })

  test('maps an IP ban to a reattempt hint', () => {
    expect(
      getFriendlyErrorMessage({
        response: {
          data: {
            error: { type: 'ip_banned', message: 'IP banned for unusual traffic' },
          },
        },
      })
    ).toBe(
      'Your IP address is temporarily banned due to unusual traffic. Please try again later.'
    )
  })

  test('maps a missing model to a pick-another hint', () => {
    expect(
      getFriendlyErrorMessage({
        message: 'model_not_found: no available model for this request',
      })
    ).toBe('The requested model is not available. Please pick another model.')
  })

  test('matches the prefixed playground error format', () => {
    // message-error-utils feeds getFriendlyErrorMessage({ message: content });
    // the content is usually a localized title followed by the technical detail.
    expect(
      getFriendlyErrorMessage({ message: 'Request failed: channel is cooling down' })
    ).toBe(
      'This channel is cooling down after recent failures. Please try again in a moment.'
    )
  })

  test('keeps the earlier B6-2 mappings intact', () => {
    expect(
      getFriendlyErrorMessage({
        response: {
          data: {
            error: { type: 'insufficient_quota', message: 'quota exceeded' },
          },
        },
      })
    ).toBe('Insufficient quota. Please top up or redeem a quota card.')
    expect(getFriendlyErrorMessage({ message: 'invalid api key' })).toBe(
      'The key is invalid or expired. Please rotate it on the channels page.'
    )
    expect(getFriendlyErrorMessage({ message: 'rate limited by upstream, 429' })).toBe(
      'Too many requests. Please try again later.'
    )
    expect(getFriendlyErrorMessage({ message: 'upstream error: bad gateway' })).toBe(
      'The upstream service is temporarily unavailable. Please try again later.'
    )
    expect(
      getFriendlyErrorMessage({
        response: {
          data: {
            error: { type: 'content_filtered', message: 'blocked by content policy' },
          },
        },
      })
    ).toBe('The content was blocked by a safety policy.')
  })

  test('returns null for unrelated technical text', () => {
    expect(
      getFriendlyErrorMessage({ message: 'some unrelated technical detail' })
    ).toBeNull()
  })

  test('never overrides dedicated security messages with friendly text', () => {
    expect(
      getFriendlyErrorMessage({
        [safeServerErrorMessage]: true,
        message: 'channel is cooling down',
      })
    ).toBeNull()
  })
})
