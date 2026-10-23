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
/**
 * 路由 `validateSearch` 用的**零依赖**搜索参数校验。
 *
 * 为什么不用 zod：TanStack Router 的 autoCodeSplitting 只把路由的 `component`
 * 拆成异步 chunk，路由选项（含 `validateSearch` 的 schema）仍在入口同步图里。
 * 因此「每个路由用 zod 定义搜索 schema」会让 zod（实测 73 KB raw / 20 KB gzip）
 * 进入落地页首屏同步链。这里用扁平的小解析函数替代，语义与被替换的 zod 写法
 * 逐条对应（见每个工厂的注释），校验逻辑一条不少。
 *
 * 约定：入参是 `?a=1` 经 TanStack 默认 `parseSearch`（JSON.parse 化）后的原始值，
 * 因此 `page=2` 是 number、`filter=abc` 是 string、重复键是 string 数组。
 */

/** 单个搜索键的解析函数：原始值 → 类型化值。 */
export type SearchField<T> = (value: unknown) => T

/** `validateSearch` 接受的纯函数形式。 */
export type SearchValidator<T> = (search: Record<string, unknown>) => T

const isFiniteNumber = (value: unknown): value is number =>
  typeof value === 'number' && Number.isFinite(value)

const isAbsent = (value: unknown): boolean =>
  value === undefined || value === null || value === ''

/** `z.number().optional().catch(fallback)`：缺失 → undefined，非法值 → fallback。 */
export function numberParam(
  fallback: number | undefined
): SearchField<number | undefined> {
  return (value) => {
    if (isAbsent(value)) {return undefined}
    return isFiniteNumber(value) ? value : fallback
  }
}

/** `z.string().optional()`：缺失或非字符串 → undefined。 */
export const textParam: SearchField<string | undefined> = (value) =>
  typeof value === 'string' ? value : undefined

/** `z.boolean().optional()`：仅接受真正的布尔值（`show_history=true`）。 */
export const booleanParam: SearchField<boolean | undefined> = (value) =>
  typeof value === 'boolean' ? value : undefined

/** `z.enum(values).optional().catch(fallback)`。 */
export function enumParam<const T extends string>(
  values: readonly T[],
  fallback: T | undefined
): SearchField<T | undefined> {
  return (value) => {
    if (isAbsent(value)) {return undefined}
    return typeof value === 'string' &&
      (values as readonly string[]).includes(value)
      ? (value as T)
      : fallback
  }
}

/**
 * 把单值归一成数组：`?status=1` 与 `?status=1&status=2` 都得到数组，
 * 对应原 schema 里的 `z.preprocess(...)`。
 */
function toList(value: unknown): unknown[] {
  return Array.isArray(value) ? value : [value]
}

/** `z.array(z.string()).optional().catch(fallback)`。 */
export function textListParam(
  fallback: string[]
): SearchField<string[] | undefined> {
  return (value) => {
    if (isAbsent(value)) {return undefined}
    const list = toList(value)
    return list.every((item) => typeof item === 'string')
      ? (list as string[])
      : fallback
  }
}

/** `z.array(z.enum(values)).optional().catch(fallback)`。 */
export function enumListParam<const T extends string>(
  values: readonly T[],
  fallback: T[]
): SearchField<T[] | undefined> {
  return (value) => {
    if (isAbsent(value)) {return undefined}
    const list = toList(value)
    const allValid = list.every(
      (item) =>
        typeof item === 'string' && (values as readonly string[]).includes(item)
    )
    return allValid ? (list as T[]) : fallback
  }
}

/**
 * 由字段表构造 `validateSearch`。字段解析出 `undefined` 时该键不写入结果，
 * 与 zod `.optional()` 产出的可选属性一致。
 */
export function defineSearchSchema<const T extends Record<string, SearchField<unknown>>>(
  fields: T
): SearchValidator<Partial<{ [K in keyof T]: ReturnType<T[K]> }>> {
  return (search) => {
    const result: Record<string, unknown> = {}
    for (const key of Object.keys(fields)) {
      const value = fields[key](search[key])
      if (value !== undefined) {result[key] = value}
    }
    return result as Partial<{ [K in keyof T]: ReturnType<T[K]> }>
  }
}
