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
// 把「用作参考图」的历史图片 URL 送入输入框附件区（T5）。
//
// ## 为什么需要一个桥接组件
//
// 附件状态由 `PromptInput` 自己的 context 持有，只有它的子组件能用
// `usePromptInputAttachments()`。而触发方（对话里的图片按钮）在**消息区**，
// 不在输入框子树里。因此：消息区只把「待加入的图片 URL」交给游乐场根组件的
// state，本组件（位于 `PromptInput` 内部）订阅它并真正加入附件。
//
// ## 为什么是「下载成 File」而不是直接塞 URL
//
// `attachments.add` 的既有契约只接受 `File[]`。把远端 URL 直接放进附件列表会
// 绕过该契约，并让后续的 `blobUrlToDataUrl`（它只处理 blob:/data:）无法内联发送，
// 图片会静默丢失。下载成 File 后，它走的是与「用户自己选图」**完全相同**的路径
// ——包括提交前的降采样，因此不会因为「来自历史图片」而绕过体积保护。
import { useEffect, useRef } from 'react'

import { usePromptInputAttachments } from '@/components/ai-elements/prompt-input'
import { guessImageFileName } from '@/lib/image-file-name'

type ReferenceImageBridgeProps = {
  /** 待加入附件的图片 URL；null 表示无待处理项。 */
  pendingReference: string | null
  /** 处理完成（成功或失败）后回调，由父组件清空 pendingReference。 */
  onConsumed: () => void
  /** 加入失败时回调（如跨域读取被拒），供父组件提示用户。 */
  onError?: (url: string) => void
}

export function ReferenceImageBridge(props: ReferenceImageBridgeProps) {
  const attachments = usePromptInputAttachments()
  // 用 ref 记住处理过的 URL，避免同一个 URL 因父组件重渲染被重复加入。
  const handledRef = useRef<string | null>(null)
  // 回调与 attachments 都存进 ref：它们每次渲染都是新引用，放进依赖数组会让
  // effect 反复触发；而本 effect 真正该响应的只有「待处理 URL 变了」。
  const attachmentsRef = useRef(attachments)
  attachmentsRef.current = attachments
  const onConsumedRef = useRef(props.onConsumed)
  onConsumedRef.current = props.onConsumed
  const onErrorRef = useRef(props.onError)
  onErrorRef.current = props.onError

  useEffect(() => {
    const url = props.pendingReference
    if (!url || handledRef.current === url) {
      return
    }
    handledRef.current = url

    let cancelled = false
    void (async () => {
      try {
        const resp = await fetch(url)
        if (!resp.ok) {
          throw new Error(`fetch failed: ${resp.status}`)
        }
        const blob = await resp.blob()
        const type = blob.type || 'image/png'
        const file = new File([blob], guessImageFileName(url, 'reference.png'), {
          type,
        })
        if (!cancelled) {
          attachmentsRef.current.add([file])
        }
      } catch {
        // 跨域 / 网络失败：如实告知，不要静默什么都不发生。
        if (!cancelled) {
          onErrorRef.current?.(url)
        }
      } finally {
        if (!cancelled) {
          onConsumedRef.current()
        }
      }
    })()

    return () => {
      cancelled = true
    }
  }, [props.pendingReference])

  return null
}
