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

import {
  PRESET_DEFAULT_RADIUS,
  PRESET_DEFAULT_SCALE,
  resolveThemeFont,
  resolveThemeRadius,
  resolveThemeScale,
  THEME_PRESET_VALUES,
} from '../theme-customization'

/**
 * 预设解析契约（视觉体系 D4）：`ink` 是结构差异套 —— 它通过预设提示收紧
 * 圆角与密度，但用户显式选择必须永远赢。这条契约决定外观抽屉是否可信。
 */
describe('ink preset registration', () => {
  it('is part of the selectable preset registry', () => {
    expect(THEME_PRESET_VALUES.has('ink')).toBe(true)
  })

  it('declares structural hints for radius and density', () => {
    expect(PRESET_DEFAULT_RADIUS.ink).toBe('sm')
    expect(PRESET_DEFAULT_SCALE.ink).toBe('sm')
  })
})

describe('resolveThemeRadius / resolveThemeScale', () => {
  it('defers to the preset hint while the user has not chosen', () => {
    expect(resolveThemeRadius('default', 'ink')).toBe('sm')
    expect(resolveThemeScale('default', 'ink')).toBe('sm')
  })

  it('keeps the project default for presets without a hint', () => {
    expect(resolveThemeRadius('default', 'default')).toBe('default')
    expect(resolveThemeScale('default', 'anthropic')).toBe('default')
  })

  it('never lets a preset override an explicit user choice', () => {
    expect(resolveThemeRadius('xl', 'ink')).toBe('xl')
    expect(resolveThemeScale('lg', 'ink')).toBe('lg')
  })

  it('leaves the font axis behaviour untouched', () => {
    expect(resolveThemeFont('default', 'ink')).toBe('sans')
    expect(resolveThemeFont('serif', 'ink')).toBe('serif')
  })
})
