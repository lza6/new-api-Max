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

import { buildChartRamp } from '../chart-palette'

const HEX = /^#[0-9a-f]{6}$/i

/**
 * 图表序列色契约（视觉体系 §4.6）：
 * 1. 永远是 canvas 能吃的 `#rrggbb`（不能是 color-mix() 之类的 CSS 函数）；
 * 2. 长度必须满足调用方（VChart ordinal range 直接按索引取色）；
 * 3. 同一序列内必须互不相同，否则多序列图会变成一片单色。
 */
describe('buildChartRamp', () => {
  it('returns canvas-safe hex colours even without a 2d context', () => {
    const ramp = buildChartRamp(5)

    expect(ramp).toHaveLength(5)
    ramp.forEach((color) => expect(color).toMatch(HEX))
  })

  it('keeps every series colour distinct', () => {
    const ramp = buildChartRamp(6)

    expect(new Set(ramp.map((color) => color.toLowerCase())).size).toBe(6)
  })

  it('always covers the requested domain length, cycling past the ramp size', () => {
    const ramp = buildChartRamp(11)

    expect(ramp).toHaveLength(11)
    expect(ramp[0]).toBe(ramp[6])
  })

  it('never asks the caller to handle a zero or negative length', () => {
    expect(buildChartRamp(0)).toHaveLength(1)
    expect(buildChartRamp(-3)).toHaveLength(1)
  })
})
