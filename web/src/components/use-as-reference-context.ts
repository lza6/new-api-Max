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
// 「用作参考图」能力的传递通道（T5）。
//
// ## 为什么用 Context 而不是 props
//
// 生成图在对话里以 **markdown 图片**形式返回，渲染路径是
// `Response`（`@/components/ai-elements/response`）→ 各 `Response*` 渲染器 →
// `ResponseImage`。这条链是**通用**的（全应用共用），把它逐层加上
// `onUseAsReference` 会把游乐场的业务概念渗进通用组件，也会让其它调用方被迫
// 传一个自己用不到的 prop。
//
// Context 让游乐场在**自己的边界**内声明这个能力，通用渲染器只需可选地读取：
// 没有 Provider 时读到 `undefined`，行为与改动前完全一致（零影响）。
import { createContext, useContext } from 'react'

/** 把一张图片（URL）作为参考图送入输入框。 */
export type UseAsReferenceHandler = (url: string) => void

const UseAsReferenceContext = createContext<UseAsReferenceHandler | undefined>(
  undefined
)

export const UseAsReferenceProvider = UseAsReferenceContext.Provider

/** 读取「用作参考图」处理器；未提供时返回 undefined。 */
export function useUseAsReference(): UseAsReferenceHandler | undefined {
  return useContext(UseAsReferenceContext)
}
