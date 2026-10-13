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
import { describe, expect, test } from 'vitest'

import {
  computeDownscaleSize,
  IMAGE_MAX_DIMENSION,
  isCanvasDecodableType,
} from '../image-downscale'

type Size = { width: number; height: number }

/**
 * 断言非空并收窄类型。用例里大量需要「先断言算出了尺寸，再读它的字段」；
 * 用 `!` 会被 lint 禁止（`no-non-null-assertion`），而 `?.` 会让断言静默失效
 * （读不到就得到 undefined，测试反而可能通过）。所以显式收窄。
 */
function expectSize(size: Size | null): Size {
  if (size === null) {
    throw new Error('期望算出一个尺寸，实际得到 null')
  }
  return size
}

// 只测纯函数部分。canvas 那半段（drawImage/toBlob）依赖真实浏览器能力，
// 在 jsdom 下无法有意义地断言，交给真实浏览器 E2E 覆盖。
describe('computeDownscaleSize', () => {
  test('does not upscale images already within the limit', () => {
    // 放大只会凭空增加字节，不会增加信息量。
    expect(computeDownscaleSize(800, 600)).toEqual({ width: 800, height: 600 })
    expect(computeDownscaleSize(IMAGE_MAX_DIMENSION, IMAGE_MAX_DIMENSION)).toEqual({
      width: IMAGE_MAX_DIMENSION,
      height: IMAGE_MAX_DIMENSION,
    })
  })

  test('scales a large landscape image by its long edge, preserving aspect ratio', () => {
    const size = expectSize(computeDownscaleSize(4000, 3000))
    expect(size.width).toBe(IMAGE_MAX_DIMENSION)
    // 4000→1568 的比例同样作用于高：3000 × 0.392 = 1176
    expect(size.height).toBe(1176)
    expect(size.width / size.height).toBeCloseTo(4000 / 3000, 2)
  })

  test('scales a large portrait image by its long edge', () => {
    // 手机竖拍：长边是**高**，必须也能正确缩。
    const size = expectSize(computeDownscaleSize(3000, 4000))
    expect(size.height).toBe(IMAGE_MAX_DIMENSION)
    expect(size.width).toBe(1176)
  })

  test('caps the long edge at exactly the limit', () => {
    expect(computeDownscaleSize(10000, 10000)).toEqual({
      width: IMAGE_MAX_DIMENSION,
      height: IMAGE_MAX_DIMENSION,
    })
  })

  test('never produces a zero dimension for extreme aspect ratios', () => {
    // 极瘦长的图（如 10000×3）缩放后高度会算成 ~0；canvas 会因此抛错。
    const wide = expectSize(computeDownscaleSize(10000, 3))
    expect(wide.width).toBe(IMAGE_MAX_DIMENSION)
    expect(wide.height).toBeGreaterThanOrEqual(1)

    const tall = expectSize(computeDownscaleSize(3, 10000))
    expect(tall.width).toBeGreaterThanOrEqual(1)
  })

  test('honours a custom max dimension', () => {
    expect(computeDownscaleSize(1000, 500, 100)).toEqual({ width: 100, height: 50 })
  })

  test('returns null for invalid input instead of guessing a size', () => {
    // 猜一个尺寸出来会让 canvas 抛错或产出畸形图片。
    expect(computeDownscaleSize(0, 100)).toBeNull()
    expect(computeDownscaleSize(100, 0)).toBeNull()
    expect(computeDownscaleSize(-1, 100)).toBeNull()
    expect(computeDownscaleSize(Number.NaN, 100)).toBeNull()
    expect(computeDownscaleSize(100, Number.POSITIVE_INFINITY)).toBeNull()
    expect(computeDownscaleSize(100, 100, 0)).toBeNull()
    expect(computeDownscaleSize(100, 100, Number.NaN)).toBeNull()
  })

  test('rounds to integer pixels', () => {
    // canvas 只接受整数尺寸；小数会被静默截断导致比例失真。
    const size = expectSize(computeDownscaleSize(3333, 1111))
    expect(Number.isInteger(size.width)).toBe(true)
    expect(Number.isInteger(size.height)).toBe(true)
  })
})

describe('isCanvasDecodableType', () => {
  test('accepts common web image types', () => {
    for (const type of ['image/jpeg', 'image/png', 'image/webp', 'image/gif', 'image/bmp']) {
      expect(isCanvasDecodableType(type), type).toBe(true)
    }
  })

  test('is case-insensitive', () => {
    expect(isCanvasDecodableType('IMAGE/PNG')).toBe(true)
  })

  test('rejects types canvas cannot decode', () => {
    // HEIC（iPhone 默认）与 SVG 不能被 drawImage 处理；这些必须原样上传
    // 而不是在缩放环节抛错把上传整个打断。
    for (const type of ['image/heic', 'image/svg+xml', 'image/tiff', 'application/pdf', '']) {
      expect(isCanvasDecodableType(type), type).toBe(false)
    }
  })
})
