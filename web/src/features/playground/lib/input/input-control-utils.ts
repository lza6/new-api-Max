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
import type { GroupOption, ModelOption } from '../../types'
import { downscaleImageFile } from './image-downscale'

type InputControlStateOptions = {
  disabled?: boolean
  groups: GroupOption[]
  hasImages?: boolean
  hasStopHandler: boolean
  isGenerating?: boolean
  isModelLoading?: boolean
  models: ModelOption[]
  text: string
}

type InputControlState = {
  canSubmit: boolean
  isSelectorDisabled: boolean
  shouldShowStop: boolean
}

type SubmittableInputMessage = {
  text?: string | null
  files?: { url?: string; mediaType?: string }[]
}

export function getSubmittableInputText(
  message: SubmittableInputMessage,
  disabled?: boolean
): string | null {
  // 允许「无文字但有图片」提交（图生图 / 多图参考）：纯图片时返回空串。
  if (disabled) {
    return null
  }
  const hasImages = (message.files?.length ?? 0) > 0
  if (!message.text?.trim() && !hasImages) {
    return null
  }
  return message.text ?? ''
}

// 从输入消息中抽取图片 URL（仅图片类型），供多模态请求组装。
export function extractInputImageUrls(message: SubmittableInputMessage): string[] {
  if (!message.files?.length) {
    return []
  }
  return message.files
    .filter((f) => !!f.url && (f.mediaType?.startsWith('image/') ?? false))
    .map((f) => f.url as string)
}

// 把 blob: / http(s) URL 转成 data: URL（图片以 base64 内联发给上游，上游无需回访
// 客户端）。已是 data: 的原样返回；远端 http(s) 图直接返回（上游可自行拉取）。
//
// **转 base64 之前先降采样**：这是参考图进入上游的**唯一漏斗**，而 base64 会把
// 体积放大 ~33%、图像 token 又近似正比于像素数。手机原图（4000×3000）不缩直接
// 内联，会既显著推高用户账单、又常被上游以超限拒绝。降采样放在这里而不是上传时，
// 是因为只有此处能拿到最终要发送的字节，改一处即覆盖全部来源（上传/拖拽/粘贴）。
// 降采样失败时 fail-open 用原图（见 `downscaleImageFile`）。
export async function blobUrlToDataUrl(url: string): Promise<string> {
  if (url.startsWith('data:')) {
    return url
  }
  if (!url.startsWith('blob:')) {
    return url
  }
  const resp = await fetch(url)
  const rawBlob = await resp.blob()
  const blob = await shrinkImageBlob(rawBlob)
  return await new Promise<string>((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(String(reader.result))
    reader.onerror = () => reject(reader.error ?? new Error('read failed'))
    reader.readAsDataURL(blob)
  })
}

/**
 * 对 Blob 做降采样。非图片或无法解码的类型原样返回。
 *
 * `downscaleImageFile` 面向 `File`（携带 name）；这里从 blob URL 拿到的是无名
 * Blob，故包装成 File 后再交给同一实现——**不重复写第二套缩放逻辑**。
 */
async function shrinkImageBlob(blob: Blob): Promise<Blob> {
  if (!blob.type.startsWith('image/')) {
    return blob
  }
  try {
    const file = new File([blob], 'upload', { type: blob.type })
    const result = await downscaleImageFile(file)
    return result.file
  } catch {
    // 缩放不可用（旧浏览器/解码失败）：原样上传，不阻断。
    return blob
  }
}

export function getInputControlState({
  disabled,
  groups,
  hasImages = false,
  hasStopHandler,
  isGenerating,
  isModelLoading,
  models,
  text,
}: InputControlStateOptions): InputControlState {
  const hasModels = models.length > 0
  // 有文字或有图片（图生图/多图参考）都可提交。
  const hasSubmittableContent = text.trim().length > 0 || hasImages

  return {
    canSubmit: !disabled && hasModels && hasSubmittableContent,
    isSelectorDisabled: disabled || isModelLoading || groups.length === 0,
    shouldShowStop: Boolean(isGenerating && hasStopHandler),
  }
}
