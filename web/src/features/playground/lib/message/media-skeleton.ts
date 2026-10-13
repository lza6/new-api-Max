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
// 生成中骨架的几何与可见性判定（T9）。
//
// ## 为什么需要它
//
// 生图/生视频比文本慢得多（数秒到数分钟）。期间只显示一个转圈，
// 用户会以为卡住；而如果成品到达时容器从 0 高度突然长出一块图/视频，
// 下方内容会被整体推下去（CLS）。
//
// 骨架**预留成品的形状**同时解决这两点。让骨架的比例跟随用户实际选择的
// size（而非一律 1:1），成品落位时几乎不产生位移。
//
// 纯函数 + 常量，便于单测；不引用 React 或网络。

import { isVideoModel } from '../video/video-generation-utils'

/**
 * size 字符串（如 `1024x1536`）→ Tailwind 宽高比类。
 *
 * 只覆盖 Tailwind 内置的 `aspect-*` 档位；不在表内的比例走 `aspect-square`
 * 兜底——**不动态拼类名**（Tailwind 的 JIT 扫不到运行时拼出的类名，
 * 拼出来的类不会生成样式，骨架会塌成 0 高度，那正是要避免的问题）。
 */
const ASPECT_CLASS_BY_RATIO: Record<string, string> = {
  '1:1': 'aspect-square',
  '16:9': 'aspect-video',
  '9:16': 'aspect-[9/16]',
  '4:3': 'aspect-[4/3]',
  '3:4': 'aspect-[3/4]',
  '3:2': 'aspect-[3/2]',
  '2:3': 'aspect-[2/3]',
}

const DEFAULT_ASPECT_CLASS = 'aspect-square'

/** 把 `宽x高` 解析并约分为最简比例。无法解析时返回 null。 */
export function parseSizeAspect(size: string): string | null {
  if (!size) {
    return null
  }
  const match = /^(\d+)\s*[x×]\s*(\d+)$/.exec(size.trim())
  if (!match) {
    return null
  }
  const width = Number(match[1])
  const height = Number(match[2])
  if (!Number.isFinite(width) || !Number.isFinite(height) || width <= 0 || height <= 0) {
    return null
  }
  const divisor = greatestCommonDivisor(width, height)
  return `${width / divisor}:${height / divisor}`
}

function greatestCommonDivisor(a: number, b: number): number {
  let x = a
  let y = b
  while (y !== 0) {
    const next = x % y
    x = y
    y = next
  }
  return x === 0 ? 1 : x
}

/**
 * size → Tailwind 宽高比类。空/非法输入走 1:1 兜底。
 *
 * 兜底而非返回空：骨架的目的是占位，**任何**比例都比 0 高度强。
 */
export function mediaSkeletonAspectClass(size: string): string {
  const ratio = parseSizeAspect(size)
  if (!ratio) {
    return DEFAULT_ASPECT_CLASS
  }
  return ASPECT_CLASS_BY_RATIO[ratio] ?? DEFAULT_ASPECT_CLASS
}

/**
 * 该模型是否「生成媒体」（图片/视频）——只有这些才需要长耗时的骨架。
 *
 * 文本模型首字很快，给它显示图片骨架是无意义的视觉噪声。
 */
export function isMediaModel(model: string): boolean {
  const name = (model || '').toLowerCase()
  if (name === '') {
    return false
  }
  return (
    isVideoModel(name) ||
    name.includes('image') ||
    name.includes('dall-e') ||
    name.includes('dalle') ||
    name.includes('gpt-image') ||
    name.includes('gptimage') ||
    name.includes('flux') ||
    name.includes('imagen') ||
    name.includes('nanobanana') ||
    name.includes('seedream') ||
    name.includes('ideogram') ||
    name.includes('midjourney')
  )
}
