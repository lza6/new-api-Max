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
// 会话侧边栏折叠状态（T1）：持久化到 localStorage，刷新后保持。
//
// 独立成 hook 而不是塞进 `index.tsx` 的 `useState`：折叠状态与对话数据无关，
// 放进主组件会让「切换会话」的重渲染顺带重算它；也让它的读写可以被单测覆盖。
//
// 读写失败（隐私模式/localStorage 被禁）一律降级为「不持久化但可用」——
// 折叠是可选的视觉偏好，绝不能因为它不可写就让整个游乐场报错。
import { useCallback, useState } from 'react'

import { STORAGE_KEYS } from '../constants'

/** 默认展开：新用户需要先看到会话列表，才知道这里有多会话能力。 */
export const SIDEBAR_DEFAULT_COLLAPSED = false

export function readSidebarCollapsed(): boolean {
  try {
    const raw = localStorage.getItem(STORAGE_KEYS.SIDEBAR_COLLAPSED)
    if (raw === null) {
      return SIDEBAR_DEFAULT_COLLAPSED
    }
    // 只认显式的 'true'；其余（含损坏值）都当展开处理，避免误折叠让用户
    // 以为会话全丢了。
    return raw === 'true'
  } catch {
    return SIDEBAR_DEFAULT_COLLAPSED
  }
}

export function writeSidebarCollapsed(collapsed: boolean): void {
  try {
    localStorage.setItem(STORAGE_KEYS.SIDEBAR_COLLAPSED, String(collapsed))
  } catch {
    // 忽略：偏好写入失败不影响功能。
  }
}

export function useSidebarCollapsed() {
  const [collapsed, setCollapsed] = useState<boolean>(() =>
    readSidebarCollapsed()
  )

  const toggle = useCallback(() => {
    setCollapsed((previous) => {
      const next = !previous
      writeSidebarCollapsed(next)
      return next
    })
  }, [])

  return { collapsed, toggle }
}
