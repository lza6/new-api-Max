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
import { toast } from 'sonner'

import {
  getFriendlyErrorMessage,
  getServerErrorMessage,
  getServerErrorSources,
  isServerErrorCancelled,
} from './server-error-message'

// 「同一个错误只提示一次」的去重表。生产环境里它会随会话存活，这是刻意的；
// 但 vitest 用 `vmThreads` 池，同一 worker 内的测试文件共享模块实例，
// 于是前一个用例弹过的错误会把后一个用例的同文案错误吞掉（表现为跨文件
// 顺序依赖的假失败）。测试侧通过 resetServerErrorDedupe() 在每例后清空。
let reportedErrors = new WeakSet<object>()

/** 清空去重表。仅供测试隔离使用；生产代码不要调用。 */
export function resetServerErrorDedupe(): void {
  reportedErrors = new WeakSet<object>()
}

/** Also used when a failure has already been presented inline. */
export function markServerErrorHandled(error: unknown): void {
  for (const source of getServerErrorSources(error)) {reportedErrors.add(source)}
}

export function handleServerError(
  error: unknown,
  fallbackMessage?: string,
  presentation?: { title: string; description?: string }
): void {
  if (isServerErrorCancelled(error)) {return}
  const sources = getServerErrorSources(error)
  const reported = sources.some((source) => reportedErrors.has(source))
  markServerErrorHandled(error)
  if (reported) {return}
  if (presentation?.description) {
    const message =
      presentation.title || getServerErrorMessage(error, fallbackMessage)
    toast.error(message, { description: presentation.description })
    return
  }
  // B6-2：命中人话映射时，首行显示人话，原始技术消息放 description 折叠。
  const friendly = getFriendlyErrorMessage(error)
  if (friendly) {
    const technical =
      presentation?.title || getServerErrorMessage(error, fallbackMessage)
    toast.error(friendly, {
      description: technical === friendly ? undefined : technical,
    })
    return
  }
  const message =
    presentation?.title || getServerErrorMessage(error, fallbackMessage)
  toast.error(message)
}
