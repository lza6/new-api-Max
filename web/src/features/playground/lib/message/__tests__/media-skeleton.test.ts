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
  isMediaModel,
  mediaSkeletonAspectClass,
  parseSizeAspect,
} from '../media-skeleton'

describe('parseSizeAspect', () => {
  test('reduces common image sizes to their ratio', () => {
    expect(parseSizeAspect('1024x1024')).toBe('1:1')
    expect(parseSizeAspect('1792x1024')).toBe('7:4')
    expect(parseSizeAspect('1024x1792')).toBe('4:7')
    expect(parseSizeAspect('1536x1024')).toBe('3:2')
    expect(parseSizeAspect('1024x1536')).toBe('2:3')
    expect(parseSizeAspect('1024x768')).toBe('4:3')
    expect(parseSizeAspect('768x1024')).toBe('3:4')
    expect(parseSizeAspect('1920x1080')).toBe('16:9')
  })

  test('accepts the multiplication sign as a separator', () => {
    // 上游/用户配置里出现过 `×`；不认就会静默退化成 1:1。
    expect(parseSizeAspect('1024×1536')).toBe('2:3')
  })

  test('tolerates surrounding whitespace', () => {
    expect(parseSizeAspect('  1024x1536  ')).toBe('2:3')
  })

  test('returns null for unparseable input instead of guessing', () => {
    // 猜一个比例会让骨架形状与成品不符，失去「预留形状」的意义。
    expect(parseSizeAspect('')).toBeNull()
    expect(parseSizeAspect('auto')).toBeNull()
    expect(parseSizeAspect('1024')).toBeNull()
    expect(parseSizeAspect('1024x')).toBeNull()
    expect(parseSizeAspect('1024xabc')).toBeNull()
    expect(parseSizeAspect('abcxdef')).toBeNull()
  })

  test('returns null for zero or negative dimensions', () => {
    expect(parseSizeAspect('0x1024')).toBeNull()
    expect(parseSizeAspect('1024x0')).toBeNull()
  })

  test('handles extreme ratios without producing NaN', () => {
    // 公约数退化为 1 时仍要给出可用的比例字符串。
    const ratio = parseSizeAspect('10000x3')
    expect(ratio).toBe('10000:3')
    expect(ratio).not.toContain('NaN')
  })
})

describe('mediaSkeletonAspectClass', () => {
  test('maps known ratios to Tailwind aspect classes', () => {
    expect(mediaSkeletonAspectClass('1024x1024')).toBe('aspect-square')
    expect(mediaSkeletonAspectClass('1920x1080')).toBe('aspect-video')
    expect(mediaSkeletonAspectClass('1024x768')).toBe('aspect-[4/3]')
  })

  test('falls back to a square for unmapped ratios rather than returning empty', () => {
    // 返回空类名会让骨架塌成 0 高度——那正是要避免的问题。
    expect(mediaSkeletonAspectClass('1792x1024')).toBe('aspect-square')
    expect(mediaSkeletonAspectClass('')).toBe('aspect-square')
    expect(mediaSkeletonAspectClass('auto')).toBe('aspect-square')
  })

  test('never returns an empty string', () => {
    for (const size of ['', 'auto', '1024x1024', 'bogus', '0x0']) {
      expect(mediaSkeletonAspectClass(size).length).toBeGreaterThan(0)
    }
  })
})

describe('isMediaModel', () => {
  test('reports image and video generation models as media', () => {
    for (const model of [
      'dall-e-3',
      'gpt-image-1',
      'flux-pro',
      'imagen-3',
      'nanobanana-2',
      'seedream-3',
      'veo-3',
      'kling-v2',
      'midjourney-v6',
    ]) {
      expect(isMediaModel(model), model).toBe(true)
    }
  })

  test('reports text models as non-media', () => {
    // 文本首字很快，给它显示图片骨架是无意义的视觉噪声。
    for (const model of ['deepseek-v4.1-flash', 'gpt-6-luna', 'claude-opus-5-5']) {
      expect(isMediaModel(model), model).toBe(false)
    }
  })

  test('reports an empty model as non-media', () => {
    expect(isMediaModel('')).toBe(false)
    expect(isMediaModel('   ')).toBe(false)
  })

  test('is case-insensitive', () => {
    expect(isMediaModel('DALL-E-3')).toBe(true)
    expect(isMediaModel('GPT-Image-1')).toBe(true)
  })
})