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

import { mapUserModels } from '../api'

// `/api/user/models` 只返回模型名数组、**不带能力元数据**。这里是「分类真正生效」
// 的最后一环：若映射忘了写 `category`，选择器会把全部模型塞进一个 "Other" 组——
// 用户在同一个下拉里看到文本模型和文生图模型（就是用户上报的现象）。
//
// 直接测纯函数而不是 mock axios：此前那版用 `vi.mock('@/lib/api')`，
// **单独跑通过、全量跑报 Network Error** —— 其它测试文件先加载了 `@/lib/api`，
// 模块注册表已缓存，factory 不再生效，请求打到了真实 XHR。
// 测纯函数没有这个顺序依赖。
describe('mapUserModels', () => {
  test('attaches a category to every model so the selector can group them', () => {
    const models = mapUserModels([
      'deepseek-v4.1-flash',
      'dall-e-3',
      'veo-3',
      'whisper-1',
      'bge-m3',
    ])

    expect(models.map((m) => m.value)).toEqual([
      'deepseek-v4.1-flash',
      'dall-e-3',
      'veo-3',
      'whisper-1',
      'bge-m3',
    ])
    // 每个模型都必须有 category，且是选择器可直接展示/翻译的标签。
    expect(models.map((m) => m.category)).toEqual([
      'Text',
      'Image',
      'Video',
      'Audio',
      'Embedding',
    ])
    expect(
      models.every((m) => typeof m.label === 'string' && m.label.length > 0)
    ).toBe(true)
  })

  test('preserves the server-provided order', () => {
    // 后端返回顺序通常已按相关度排好，重排会让用户找不到常用模型。
    const models = mapUserModels(['glm-5.3-flash', 'gpt-6-luna', 'deepseek-v4.1-flash'])
    expect(models.map((m) => m.value)).toEqual([
      'glm-5.3-flash',
      'gpt-6-luna',
      'deepseek-v4.1-flash',
    ])
  })

  test('returns an empty list when data is not an array', () => {
    // 上游改返回结构时宁可少几个模型，也不要渲染出 undefined 条目。
    expect(mapUserModels(null)).toEqual([])
    expect(mapUserModels(undefined)).toEqual([])
    expect(mapUserModels('gpt-4o')).toEqual([])
    expect(mapUserModels({})).toEqual([])
    expect(mapUserModels(42)).toEqual([])
  })

  test('returns an empty list for an empty array', () => {
    expect(mapUserModels([])).toEqual([])
  })

  test('skips non-string and empty entries instead of rendering undefined rows', () => {
    const models = mapUserModels(['gpt-6-luna', null, 7, '', { name: 'x' }, 'dall-e-3'])

    expect(models.map((m) => m.value)).toEqual(['gpt-6-luna', 'dall-e-3'])
    expect(models).toHaveLength(2)
  })
})
