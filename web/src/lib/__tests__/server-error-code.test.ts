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

import { getServerErrorMessageKey } from '@/lib/server-error-message'

/**
 * 后端 `ApiError` 的兜底契约（尼尔森 H9.1 / N15）：
 * 命中底层错误的响应带 `code: "internal_error"`，前端必须按 code 映射成
 * 可重试的通用文案，而不是把 `sql: no rows in result set` 之类的串展示给用户。
 */
describe('internal_error code mapping', () => {
  it('maps the backend internal_error code to the generic retry message', () => {
    expect(
      getServerErrorMessageKey({
        success: false,
        code: 'internal_error',
        message: 'sql: no rows in result set',
        request_id: 'abc123',
      })
    ).toBe('The request could not be completed. Please try again later.')
  })

  it('keeps user-facing messages untouched when no internal code is present', () => {
    expect(
      getServerErrorMessageKey({ success: false, message: '额度不足，请充值' })
    ).toBeNull()
  })
})
