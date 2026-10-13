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
import { beforeEach, describe, expect, test, vi } from 'vitest'

// `/api/user/models` 只返回模型名数组、**不带能力元数据**。这里是「分类真正生效」
// 的最后一环：若 `getUserModels` 忘了写 `category`，选择器会把全部模型塞进一个
// "Other" 组——用户在同一个下拉里看到文本模型和文生图模型（就是用户上报的现象）。
// 这条用例锁死「返回的每个模型都带 category」。
const getUserMock = vi.fn()

vi.mock('@/lib/api', () => ({
  api: {
    get: (...args: unknown[]) => getUserMock(...args),
    post: vi.fn(),
  },
}))

vi.mock('@/lib/server-error-message', () => ({
  requireServerSuccess: () => {},
}))

describe('getUserModels', () => {
  beforeEach(() => {
    getUserMock.mockReset()
  })

  test('attaches a category to every model so the selector can group them', async () => {
    getUserMock.mockResolvedValue({
      data: {
        success: true,
        data: ['deepseek-v4.1-flash', 'dall-e-3', 'veo-3', 'whisper-1', 'bge-m3'],
      },
    })
    const { getUserModels } = await import('../api')

    const models = await getUserModels('default')

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
    expect(models.every((m) => typeof m.label === 'string' && m.label.length > 0)).toBe(true)
  })

  test('returns an empty list when the server reports failure', async () => {
    getUserMock.mockResolvedValue({ data: { success: false, data: [] } })
    const { getUserModels } = await import('../api')

    expect(await getUserModels('default')).toEqual([])
  })

  test('returns an empty list when data is not an array', async () => {
    getUserMock.mockResolvedValue({ data: { success: true, data: null } })
    const { getUserModels } = await import('../api')

    expect(await getUserModels('default')).toEqual([])
  })
})
