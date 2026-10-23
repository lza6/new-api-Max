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

/**
 * i18n 默认语言契约：
 *  - 新访客（无 localStorage 选择）→ 默认简体中文（zhCN），不被浏览器语言带走
 *  - 已显式选择的用户 → 尊重其选择
 *  - setLanguage → 加载语言包 + 持久化 + 切换
 *
 * config.ts 有模块级副作用（i18n.init），用 vi.resetModules 每次重载以注入不同的
 * localStorage 初始状态。
 */

function setupLocalStorage(initial: Record<string, string> = {}) {
  const store = new Map(Object.entries(initial))
  // jsdom 29 里 `window` 是不可重定义属性，`vi.stubGlobal('window', ...)` 会抛
  // "Cannot redefine property: window"。被测代码读的是 `window.localStorage`。
  // 打桩要打在**实例**上而不是 `Storage.prototype`：threads 池下每个测试文件有独立
  // realm，全局 `Storage` 与 `window.localStorage` 的类原型不是同一个，打原型会
  // 失效（表现为「换个 pool 就红」）。
  const storage = window.localStorage
  vi.spyOn(storage, 'getItem').mockImplementation((key: string) =>
    store.has(key) ? store.get(key)! : null
  )
  vi.spyOn(storage, 'setItem').mockImplementation(
    (key: string, value: string) => {
      store.set(key, value)
    }
  )
  vi.spyOn(storage, 'removeItem').mockImplementation((key: string) => {
    store.delete(key)
  })
  vi.spyOn(storage, 'clear').mockImplementation(() => {
    store.clear()
  })
  return store
}

beforeEach(() => {
  vi.resetModules()
})

afterEach(() => {
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
  vi.resetModules()
})

describe('i18n default language', () => {
  test('new visitor with no stored choice defaults to zhCN (not browser English)', async () => {
    setupLocalStorage({})
    const mod = await import('../config')
    expect(mod.default.language).toBe('zhCN')
  })

  test('visitor with explicit stored choice keeps it', async () => {
    setupLocalStorage({ i18nextLng: 'en' })
    const mod = await import('../config')
    expect(mod.default.language).toBe('en')
  })

  test('stored zh-TW is normalized to zhTW', async () => {
    setupLocalStorage({ i18nextLng: 'zh-TW' })
    const mod = await import('../config')
    expect(mod.default.language).toBe('zhTW')
  })

  test('setLanguage persists choice and applies it', async () => {
    const store = setupLocalStorage({})
    const mod = await import('../config')
    await mod.setLanguage('ja')
    expect(mod.default.language).toBe('ja')
    expect(store.get('i18nextLng')).toBe('ja')
  })
})
