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
  booleanParam,
  defineSearchSchema,
  enumListParam,
  enumParam,
  numberParam,
  textListParam,
  textParam,
} from '../search-schema'

describe('search-schema 单键解析', () => {
  it('numberParam 缺失时返回 undefined，合法数字原样通过', () => {
    const field = numberParam(1)
    expect(field(undefined)).toBeUndefined()
    expect(field(7)).toBe(7)
  })

  it('numberParam 非法值回落到 fallback（含 NaN / Infinity）', () => {
    const field = numberParam(1)
    expect(field('abc')).toBe(1)
    expect(field(Number.NaN)).toBe(1)
    expect(field(Number.POSITIVE_INFINITY)).toBe(1)
    expect(field(null)).toBeUndefined()
  })

  it('textParam 只接受字符串', () => {
    expect(textParam('gpt-4o')).toBe('gpt-4o')
    expect(textParam(123)).toBeUndefined()
    expect(textParam(undefined)).toBeUndefined()
  })

  it('booleanParam 只接受真正的布尔值', () => {
    expect(booleanParam(true)).toBe(true)
    expect(booleanParam('true')).toBeUndefined()
    expect(booleanParam(undefined)).toBeUndefined()
  })

  it('enumParam 保留白名单内取值，其它回落到 fallback', () => {
    const field = enumParam(['card', 'table'], undefined)
    expect(field('card')).toBe('card')
    expect(field('list')).toBeUndefined()
    expect(field(undefined)).toBeUndefined()
  })

  it('textListParam 把单值归一成数组，非字符串元素回落到 fallback', () => {
    const field = textListParam([])
    expect(field(['1', '2'])).toEqual(['1', '2'])
    expect(field('1')).toEqual(['1'])
    expect(field(1)).toEqual([])
    expect(field(undefined)).toBeUndefined()
  })

  it('enumListParam 任一元素越界即整体回落到 fallback', () => {
    const field = enumListParam(['-1', '1', '2'], [])
    expect(field(['1', '2'])).toEqual(['1', '2'])
    expect(field('1')).toEqual(['1'])
    expect(field(['1', '9'])).toEqual([])
    expect(field('')).toBeUndefined()
  })
})

describe('defineSearchSchema', () => {
  const schema = defineSearchSchema({
    page: numberParam(1),
    pageSize: numberParam(undefined),
    filter: textParam,
    status: textListParam([]),
  })

  it('解析合法搜索串并保留默认值行为', () => {
    expect(schema({ page: 3, filter: 'gpt', status: ['1'] })).toEqual({
      page: 3,
      filter: 'gpt',
      status: ['1'],
    })
  })

  it('解析出 undefined 的键不写入结果（对应 zod .optional()）', () => {
    const result = schema({ pageSize: 'bad', filter: 42 })
    expect('pageSize' in result).toBe(false)
    expect('filter' in result).toBe(false)
    expect('status' in result).toBe(false)
  })

  it('空搜索对象返回空结果', () => {
    expect(schema({})).toEqual({})
  })
})
