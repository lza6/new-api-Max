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
  BATCH_COUNT_OPTIONS,
  DEFAULT_BATCH_COUNT,
  MAX_BATCH_COUNT,
  normalizeBatchCount,
  supportsBatchCount,
  toRequestBatchCount,
} from '../batch-count'

describe('normalizeBatchCount', () => {
  test('accepts every offered preset', () => {
    for (const option of BATCH_COUNT_OPTIONS) {
      expect(normalizeBatchCount(option), String(option)).toBe(option)
    }
  })

  test('accepts valid values that are not in the preset list', () => {
    // 档位表是 UI 建议而不是数据契约：从历史配置读回 5 时不能把它改掉，
    // 否则用户的选择会被静默改写。
    expect(normalizeBatchCount(5)).toBe(5)
    expect(normalizeBatchCount(7)).toBe(7)
  })

  test('accepts numeric strings', () => {
    expect(normalizeBatchCount('3')).toBe(3)
    expect(normalizeBatchCount(' 4 ')).toBe(4)
  })

  test('falls back to the default for out-of-range values instead of clamping', () => {
    // 夹到边界会让「用户传 999」变成「静默生成 8 张」——那 8 倍的账单是意外。
    // 回退默认值则是最小代价、且不会偷偷多花钱。
    expect(normalizeBatchCount(0)).toBe(DEFAULT_BATCH_COUNT)
    expect(normalizeBatchCount(-1)).toBe(DEFAULT_BATCH_COUNT)
    expect(normalizeBatchCount(MAX_BATCH_COUNT + 1)).toBe(DEFAULT_BATCH_COUNT)
    expect(normalizeBatchCount(999)).toBe(DEFAULT_BATCH_COUNT)
    expect(normalizeBatchCount(Number.MAX_SAFE_INTEGER)).toBe(DEFAULT_BATCH_COUNT)
  })

  test('falls back for non-integers and non-numeric input', () => {
    expect(normalizeBatchCount(1.5)).toBe(DEFAULT_BATCH_COUNT)
    expect(normalizeBatchCount('two')).toBe(DEFAULT_BATCH_COUNT)
    expect(normalizeBatchCount('')).toBe(DEFAULT_BATCH_COUNT)
    expect(normalizeBatchCount('   ')).toBe(DEFAULT_BATCH_COUNT)
    expect(normalizeBatchCount(Number.NaN)).toBe(DEFAULT_BATCH_COUNT)
    expect(normalizeBatchCount(Number.POSITIVE_INFINITY)).toBe(DEFAULT_BATCH_COUNT)
    expect(normalizeBatchCount(null)).toBe(DEFAULT_BATCH_COUNT)
    expect(normalizeBatchCount(undefined)).toBe(DEFAULT_BATCH_COUNT)
    expect(normalizeBatchCount({})).toBe(DEFAULT_BATCH_COUNT)
    expect(normalizeBatchCount([])).toBe(DEFAULT_BATCH_COUNT)
  })

  test('never exceeds the documented maximum', () => {
    // 上限是计费安全边界：任何输入都不得返回超过它的值。
    for (const input of [MAX_BATCH_COUNT, MAX_BATCH_COUNT + 1, 1e9, '1000000']) {
      expect(normalizeBatchCount(input), String(input)).toBeLessThanOrEqual(
        MAX_BATCH_COUNT
      )
    }
  })
})

describe('toRequestBatchCount', () => {
  test('omits the field for the default count', () => {
    // 未显式设置的可选参数不应出现在请求体里：部分上游会因「用户显式要求 n=1」
    // 走不同的响应结构。
    expect(toRequestBatchCount(DEFAULT_BATCH_COUNT)).toBeUndefined()
    expect(toRequestBatchCount(1)).toBeUndefined()
  })

  test('sends the value when greater than the default', () => {
    expect(toRequestBatchCount(2)).toBe(2)
    expect(toRequestBatchCount(8)).toBe(8)
    expect(toRequestBatchCount(4)).toBe(4)
  })

  test('never sends an out-of-range value', () => {
    expect(toRequestBatchCount(0)).toBeUndefined()
    expect(toRequestBatchCount(-5)).toBeUndefined()
    expect(toRequestBatchCount(999)).toBeUndefined()
    expect(toRequestBatchCount(Number.NaN)).toBeUndefined()
  })

  test('never returns a value above the maximum', () => {
    for (const input of [1, 2, 3, 4, 5, 6, 7, 8, 9, 100, -1, Number.NaN]) {
      const out = toRequestBatchCount(input)
      if (out !== undefined) {
        expect(out, String(input)).toBeLessThanOrEqual(MAX_BATCH_COUNT)
        expect(out, String(input)).toBeGreaterThan(1)
      }
    }
  })
})

describe('supportsBatchCount', () => {
  test('accepts image generation models', () => {
    for (const model of [
      'dall-e-3',
      'gpt-image-1',
      'flux-pro',
      'imagen-3',
      'nanobanana-2',
      'seedream-3',
      'ideogram-v2',
    ]) {
      expect(supportsBatchCount(model), model).toBe(true)
    }
  })

  test('rejects text, video, audio and embedding models', () => {
    // 只有图片生成模型有 n 语义。给文本模型显示「生成张数」是没有意义的控件。
    for (const model of [
      'deepseek-v4.1-flash',
      'gpt-6-luna',
      'veo-3',
      'kling-v2',
      'whisper-1',
      'text-embedding-3-large',
    ]) {
      expect(supportsBatchCount(model), model).toBe(false)
    }
  })

  test('rejects an empty model name', () => {
    expect(supportsBatchCount('')).toBe(false)
    expect(supportsBatchCount('   ')).toBe(false)
  })

  test('is case-insensitive', () => {
    expect(supportsBatchCount('DALL-E-3')).toBe(true)
    expect(supportsBatchCount('GPT-Image-1')).toBe(true)
  })
})

describe('BATCH_COUNT_OPTIONS', () => {
  test('is sorted ascending and within the bound', () => {
    const sorted = [...BATCH_COUNT_OPTIONS].sort((a, b) => a - b)
    expect([...BATCH_COUNT_OPTIONS]).toEqual(sorted)
    for (const option of BATCH_COUNT_OPTIONS) {
      expect(option).toBeGreaterThanOrEqual(1)
      expect(option).toBeLessThanOrEqual(MAX_BATCH_COUNT)
    }
  })

  test('includes the default count', () => {
    // 默认值不在档位里会让 UI 无法回显当前选择。
    expect(BATCH_COUNT_OPTIONS).toContain(DEFAULT_BATCH_COUNT)
  })
})
