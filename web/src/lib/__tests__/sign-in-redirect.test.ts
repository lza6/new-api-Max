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
import { describe, expect, it } from 'vitest'

import { signInRedirectTarget } from '../http-client'

describe('signInRedirectTarget', () => {
  it('keeps the un-prefixed default locale and forwards the returnTo path', () => {
    expect(signInRedirectTarget('/dashboard')).toBe(
      '/sign-in?redirect=%2Fdashboard'
    )
  })

  it('keeps the language prefix of the expired session path', () => {
    expect(signInRedirectTarget('/en/dashboard')).toBe(
      '/en/sign-in?redirect=%2Fen%2Fdashboard'
    )
  })

  it('keeps non-latin language prefixes such as zh-TW', () => {
    expect(signInRedirectTarget('/zh-TW/usage-logs/common')).toBe(
      '/zh-TW/sign-in?redirect=%2Fzh-TW%2Fusage-logs%2Fcommon'
    )
  })

  it('preserves search and hash in the returnTo value', () => {
    expect(
      signInRedirectTarget('/ja/usage-logs/common', '?page=2&type=1', '#top')
    ).toBe(
      `/ja/sign-in?redirect=${encodeURIComponent('/ja/usage-logs/common?page=2&type=1#top')}`
    )
  })

  it('returns null when already on the same-locale sign-in page', () => {
    expect(signInRedirectTarget('/sign-in')).toBeNull()
    expect(signInRedirectTarget('/en/sign-in')).toBeNull()
    expect(signInRedirectTarget('/zh-TW/sign-in')).toBeNull()
  })
})
