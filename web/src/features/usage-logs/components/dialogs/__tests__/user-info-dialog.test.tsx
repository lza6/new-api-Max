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
// 生产回归（2026-09-29）：管理员点用户时「有时能显示、有时提示没有该用户」。
// 根因之一是弹窗把「请求失败」与「用户真的不存在」渲染成同一个空态。
// 本用例锁定：失败时必须显示错误文案 + 重试按钮，不得显示空态文案；
// 重试成功后必须渲染用户信息。
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { beforeAll, beforeEach, describe, expect, test, vi } from 'vitest'

import { UserInfoDialog } from '@/features/usage-logs/components/dialogs/user-info-dialog'

const getUserInfo = vi.fn()

vi.mock('@/features/usage-logs/api', () => ({
  getUserInfo: (...args: unknown[]) => getUserInfo(...args),
}))

const translations: Record<string, string> = {
  'User Information': 'User Information',
  'View detailed information about this user including balance, usage statistics, and invitation details.':
    'View detailed information about this user including balance, usage statistics, and invitation details.',
  Username: 'Username',
  Balance: 'Balance',
  'Used Quota': 'Used Quota',
  'Request Count': 'Request Count',
  'No user information available': 'No user information available',
  'Failed to fetch user information': 'Failed to fetch user information',
  Retry: 'Retry',
  Close: 'Close',
}

beforeAll(() => {
  i18next.addResourceBundle('en', 'translation', translations)
})

function renderDialog() {
  return render(
    <UserInfoDialog userId={9980} open onOpenChange={() => undefined} />
  )
}

describe('UserInfoDialog failure handling', () => {
  beforeEach(() => {
    getUserInfo.mockReset()
    vi.spyOn(console, 'error').mockImplementation(() => undefined)
  })

  test('shows the error state instead of the empty state when the request fails', async () => {
    getUserInfo.mockRejectedValue(new Error('rate limit check failed'))

    renderDialog()

    await waitFor(() =>
      expect(screen.getByText('Failed to fetch user information')).toBeTruthy()
    )
    expect(screen.queryByText('No user information available')).toBeNull()
    expect(screen.getByRole('button', { name: 'Retry' })).toBeTruthy()
  })

  test('recovers on retry once the request succeeds', async () => {
    getUserInfo.mockRejectedValueOnce(new Error('temporary failure'))
    getUserInfo.mockResolvedValueOnce({
      success: true,
      data: {
        id: 9980,
        username: 'recovered-user',
        quota: 100000,
        used_quota: 20000,
        request_count: 12,
      },
    })

    renderDialog()

    const retry = await screen.findByRole('button', { name: 'Retry' })
    await userEvent.click(retry)

    await waitFor(() =>
      expect(screen.getByText('recovered-user')).toBeTruthy()
    )
    expect(screen.queryByText('No user information available')).toBeNull()
  })

  test('keeps the empty state for a successful response with no payload', async () => {
    getUserInfo.mockResolvedValue({ success: true, data: null })

    renderDialog()

    await waitFor(() =>
      expect(screen.getByText('No user information available')).toBeTruthy()
    )
    expect(screen.queryByRole('button', { name: 'Retry' })).toBeNull()
  })
})
