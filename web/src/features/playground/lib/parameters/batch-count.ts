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
// 批量生成张数（T10）：`n` 参数的档位与边界（纯函数，便于单测）。
//
// ## 为什么张数必须有上界（计费安全问题）
//
// `n` 是**计费乘数**：一次请求生成 N 张就按 N 倍计费。若前端不设上界，用户可以
// 传出任意大的 n，让单次请求的计费算术逼近溢出边界。后端已有硬上界
// （`dto.MaxImageN` = 128），前端也必须自设一道——前端是**用户体验**上的第一道
// 闸门，让用户在点发送前就知道上限，而不是提交后被后端拒绝。
//
// ## 为什么前端档位远小于后端上界
//
// 后端 128 是为「算术安全」设的，不是为「体验」设的。真实使用里一次生成
// 4 张已经很宽裕；给到 128 只会让用户误点出一个天价请求。前端取
// `MAX_BATCH_COUNT = 8`：够用、且即使误操作代价可控。
//
// 这**不是**在削弱后端保护——后端 128 仍然在，前端只是更保守。

/** 前端允许的最大生成张数。 */
export const MAX_BATCH_COUNT = 8

/** 默认生成张数（与 OpenAI 的 `n` 默认值一致）。 */
export const DEFAULT_BATCH_COUNT = 1

/**
 * 可选档位。刻意用**有限档位**而不是自由输入：
 * 张数直接乘进账单，让用户在滑杆/输入框里手滑出一个大数字的代价太高。
 */
export const BATCH_COUNT_OPTIONS = [1, 2, 3, 4, 6, 8] as const

/**
 * 把任意输入归一为**合法档位**。
 *
 * 契约：
 *  - 非法（NaN / 非整数 / <1 / >MAX）一律回退 `DEFAULT_BATCH_COUNT`
 *    （而不是夹到边界——用户想传 999 却静默变成 8，那 8 倍的账单是意外）。
 *  - 合法但不在档位表里的值（如 5）**保留**：档位表是 UI 建议，不是数据契约；
 *    用户从历史配置读回 5 时不能把它改掉。
 */
export function normalizeBatchCount(value: unknown): number {
  let parsed = Number.NaN
  if (typeof value === 'number') {
    parsed = value
  } else if (typeof value === 'string' && value.trim() !== '') {
    parsed = Number(value)
  }
  if (!Number.isFinite(parsed) || !Number.isInteger(parsed)) {
    return DEFAULT_BATCH_COUNT
  }
  if (parsed < 1 || parsed > MAX_BATCH_COUNT) {
    return DEFAULT_BATCH_COUNT
  }
  return parsed
}

/**
 * 构造要发给上游的 `n` 值。
 *
 * 返回 `undefined` 表示**不发送**该字段（张数为默认 1 时）——与项目既有约定一致：
 * 未显式设置的可选参数不应出现在请求体里，避免上游按「用户显式要求 n=1」处理
 * （部分上游因此走不同的响应结构）。显式选 1 也一样不发送：语义等价。
 */
export function toRequestBatchCount(value: number): number | undefined {
  const normalized = normalizeBatchCount(value)
  return normalized === DEFAULT_BATCH_COUNT ? undefined : normalized
}

/** 该模型是否支持一次生成多张。 */
export function supportsBatchCount(model: string): boolean {
  const m = (model || '').toLowerCase()
  if (m === '') {
    return false
  }
  // 只有图片生成模型有 `n` 语义；文本/视频/音频都没有。
  return (
    m.includes('image') ||
    m.includes('dall-e') ||
    m.includes('dalle') ||
    m.includes('gpt-image') ||
    m.includes('gptimage') ||
    m.includes('flux') ||
    m.includes('imagen') ||
    m.includes('nanobanana') ||
    m.includes('seedream') ||
    m.includes('ideogram')
  )
}
