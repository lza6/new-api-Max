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
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { WebProtectionPage } from './web-protection-page'

const defaultSettings = {
  enabled: true,
  limit_per_second: 5,
  burst: 40,
  auto_ban: true,
  auto_ban_threshold_per_minute: 60,
  auto_ban_minutes: 1440,
  log_enabled: true,
  window_seconds: 60,
  allowed_paths: ['/admin', '/api/health'],
  blocked_paths: ['/admin/debug'],
  ua_allowlist: ['MyAppBot/1.0'],
}

beforeEach(() => {
  vi.spyOn(api, 'get').mockImplementation(async (url: string) => {
    if (url === '/api/admin/web-protection/settings') {
      return { data: { data: defaultSettings } }
    }
    throw new Error(`Unexpected GET ${url}`)
  })
  vi.spyOn(api, 'put').mockResolvedValue({ data: { success: true } } as never)
})

afterEach(() => {
  vi.restoreAllMocks()
})

describe('WebProtectionPage policy dimensions', () => {
  test('renders the path and User-Agent policy fields with saved values and saves them as arrays', async () => {
    const user = userEvent.setup()
    render(<WebProtectionPage />)

    const allowed = await screen.findByLabelText('Allowed paths')
    const blocked = screen.getByLabelText('Blocked paths')
    const ua = screen.getByLabelText('User-Agent allowlist')

    // 回填：数组字段以换行文本展示
    expect(allowed).toHaveValue('/admin\n/api/health')
    expect(blocked).toHaveValue('/admin/debug')
    expect(ua).toHaveValue('MyAppBot/1.0')

    // 逗号 + 换行混合输入，保存时转成数组
    await user.clear(allowed)
    await user.type(allowed, '/admin,/api/health\n/api/v2')
    await user.clear(blocked)
    await user.type(blocked, '/admin/debug, /api/sensitive')
    await user.clear(ua)
    await user.type(ua, 'MyAppBot/1.0,healthcheck')

    await user.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => {
      expect(api.put).toHaveBeenCalledWith(
        '/api/admin/web-protection/settings',
        expect.objectContaining({
          enabled: true,
          limit_per_second: 5,
          burst: 40,
          auto_ban: true,
          auto_ban_threshold_per_minute: 60,
          auto_ban_minutes: 1440,
          log_enabled: true,
          window_seconds: 60,
          allowed_paths: ['/admin', '/api/health', '/api/v2'],
          blocked_paths: ['/admin/debug', '/api/sensitive'],
          ua_allowlist: ['MyAppBot/1.0', 'healthcheck'],
        })
      )
    })
  })
})
