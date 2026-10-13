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
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import { STORAGE_KEYS } from '../../constants'
import {
  readSidebarCollapsed,
  SIDEBAR_DEFAULT_COLLAPSED,
  writeSidebarCollapsed,
} from '../use-sidebar-collapsed'

// 这些用例直接测 localStorage 读写（纯函数层），不渲染组件：
// 折叠偏好的关键契约是「读写失败也不能让游乐场报错」，用渲染测不出这一点。
describe('sidebar collapsed persistence', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  afterEach(() => {
    vi.restoreAllMocks()
    localStorage.clear()
  })

  test('defaults to expanded when nothing is stored', () => {
    // 新用户要先看到会话列表，才知道有多会话能力。
    expect(SIDEBAR_DEFAULT_COLLAPSED).toBe(false)
    expect(readSidebarCollapsed()).toBe(false)
  })

  test('round-trips a collapsed preference', () => {
    writeSidebarCollapsed(true)
    expect(readSidebarCollapsed()).toBe(true)

    writeSidebarCollapsed(false)
    expect(readSidebarCollapsed()).toBe(false)
  })

  test('reads a value written under the documented storage key', () => {
    // 锁死键名：改了键名会让所有老用户的偏好静默失效。
    localStorage.setItem(STORAGE_KEYS.SIDEBAR_COLLAPSED, 'true')
    expect(readSidebarCollapsed()).toBe(true)
  })

  test('treats a corrupted value as expanded rather than collapsed', () => {
    // 损坏值若被当成 collapsed，用户会以为会话全丢了——这是更糟的失败方向。
    for (const bad of ['yes', '1', '{}', '', 'TRUE']) {
      localStorage.setItem(STORAGE_KEYS.SIDEBAR_COLLAPSED, bad)
      expect(readSidebarCollapsed(), `value=${bad}`).toBe(false)
    }
  })

  test('falls back to expanded when reading throws', () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('localStorage disabled')
    })
    expect(readSidebarCollapsed()).toBe(false)
  })

  test('does not throw when writing fails', () => {
    // 隐私模式/配额满：偏好写不进去也不能让折叠按钮把页面搞崩。
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('quota exceeded')
    })
    expect(() => writeSidebarCollapsed(true)).not.toThrow()
  })
})
