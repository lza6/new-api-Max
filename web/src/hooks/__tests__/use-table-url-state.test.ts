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
import { act, renderHook } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import { useTableUrlState } from '../use-table-url-state'

// 稳定引用：`search`/`columnFilters` 每次渲染都换引用会触发 hook 内部的
// 「URL 变化 → 重设列筛选 → 再渲染」循环（测试里表现为 worker OOM）。
const EMPTY_SEARCH: Record<string, unknown> = {}
const STATUS_FILTERS = [
  { columnId: 'status', searchKey: 'status', type: 'array' as const },
]

type NavigateCall = {
  search: (prev: Record<string, unknown>) => Record<string, unknown>
  replace?: boolean
}

const lastCall = (navigate: {
  mock: { calls: unknown[][] }
}): NavigateCall => navigate.mock.calls.at(-1)?.[0] as NavigateCall

describe('useTableUrlState — URL 写历史语义', () => {
  it('全局搜索用 replace 写 URL（避免每敲一个字压一条历史）', () => {
    const navigate = vi.fn()
    const { result } = renderHook(() =>
      useTableUrlState({ search: EMPTY_SEARCH, navigate })
    )

    act(() => {
      result.current.onGlobalFilterChange?.('abc')
    })

    const call = lastCall(navigate)
    expect(call.replace).toBe(true)
    expect(call.search({ page: 1 })).toEqual({
      page: undefined,
      filter: 'abc',
    })
  })

  it('列筛选用 replace 写 URL', () => {
    const navigate = vi.fn()
    const { result } = renderHook(() =>
      useTableUrlState({
        search: EMPTY_SEARCH,
        navigate,
        columnFilters: STATUS_FILTERS,
      })
    )

    act(() => {
      result.current.onColumnFiltersChange([{ id: 'status', value: ['1'] }])
    })

    const call = lastCall(navigate)
    expect(call.replace).toBe(true)
    expect(call.search({ page: 1 })).toEqual({
      page: undefined,
      status: ['1'],
    })
  })

  it('翻页仍然是普通导航（保留历史，便于后退回到上一页）', () => {
    const navigate = vi.fn()
    const { result } = renderHook(() =>
      useTableUrlState({ search: EMPTY_SEARCH, navigate })
    )

    act(() => {
      result.current.onPaginationChange({ pageIndex: 1, pageSize: 20 })
    })

    const call = lastCall(navigate)
    expect(call.replace).toBeUndefined()
    expect(call.search({})).toEqual({ page: 2, pageSize: 20 })
  })
})
