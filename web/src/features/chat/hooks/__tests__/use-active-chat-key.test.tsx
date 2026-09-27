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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, renderHook, waitFor } from '@testing-library/react'
import { beforeEach, expect, it, vi } from 'vitest'

import { useAuthStore } from '@/stores/auth-store'

import { useActiveChatKey } from '../use-active-chat-key'

const mocks = vi.hoisted(() => ({
  revealSingleKey: vi.fn(),
}))

vi.mock('@/features/keys/hooks/use-token-key-disclosure', () => ({
  useTokenKeyDisclosure: () => ({
    revealSingleKey: mocks.revealSingleKey,
    revealKeysBatch: vi.fn(),
    verification: { dialogProps: {} },
  }),
}))

vi.mock('@/features/keys/api', () => ({
  getApiKeys: vi.fn().mockResolvedValue({
    success: true,
    data: { items: [{ id: 7, status: 1 }] },
  }),
}))

function makeWrapper() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const wrapper = ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  )
  return { wrapper, client }
}

beforeEach(() => {
  useAuthStore.setState((state) => ({
    auth: {
      ...state.auth,
      user: { id: 1, username: 'e2e-user', role: 0 },
    },
  }))
  mocks.revealSingleKey.mockReset()
})

it('cancelled verification is not a permanent loading dead-end and can be retried', async () => {
  // 第一次验证被取消（resolve null），第二次重试成功返回 key。
  mocks.revealSingleKey
    .mockResolvedValueOnce(null)
    .mockResolvedValueOnce('sk-retry-ok')

  const { wrapper } = makeWrapper()
  const { result } = renderHook(() => useActiveChatKey(true), { wrapper })

  // 首次取消 → isPending 结束、进入错误态（可恢复，非死循环）。
  await waitFor(() => expect(result.current.isError).toBe(true))
  expect(result.current.isPending).toBe(false)
  expect(result.current.data).toBeUndefined()
  expect(result.current.error?.message).toContain('Verification cancelled')

  // retry() 清除失败标记 → 重新拉起验证 → 成功后返回 key。
  act(() => {
    result.current.retry()
  })
  await waitFor(() => expect(result.current.data).toBe('sk-retry-ok'))
  expect(result.current.isError).toBe(false)
  expect(mocks.revealSingleKey).toHaveBeenCalledTimes(2)
})

it('successful reveal exposes the key and clears error state', async () => {
  mocks.revealSingleKey.mockResolvedValue('sk-first')
  const { wrapper } = makeWrapper()
  const { result } = renderHook(() => useActiveChatKey(true), { wrapper })
  await waitFor(() => expect(result.current.data).toBe('sk-first'))
  expect(result.current.isPending).toBe(false)
  expect(result.current.isError).toBe(false)
})