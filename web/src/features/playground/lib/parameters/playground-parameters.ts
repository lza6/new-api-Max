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
import type { PlaygroundConfig, ParameterEnabled } from '../../types'

type ParameterValue = PlaygroundConfig[keyof PlaygroundConfig]

export type PlaygroundParameterKey = keyof ParameterEnabled

export type PlaygroundParameterControl = {
  key: PlaygroundParameterKey
  labelKey: string
  descriptionKey: string
  valueType: 'slider' | 'number'
  min: number
  max: number
  step: number
}

// 图片生成尺寸档位（gpt-image 系列走 chat/completions 时用 size 指定）。
// 以 select 控件呈现，值直接是 "宽x高" 字符串。
export type PlaygroundSizeControl = {
  key: 'size'
  labelKey: string
  descriptionKey: string
  valueType: 'select'
  options: readonly string[]
}

export const PLAYGROUND_SIZE_CONTROL: PlaygroundSizeControl = {
  key: 'size',
  labelKey: 'Image Size',
  descriptionKey: 'Output size (width x height) for image generation models',
  valueType: 'select',
  options: [
    '1024x1024',
    '1536x1024',
    '1024x1536',
    '2048x2048',
    '4096x4096',
  ],
} as const

// 图片尺寸是否适用于当前模型（宽松匹配 image / 生图关键词）。
export function isImageSizeModel(model: string): boolean {
  const m = model.toLowerCase()
  return m.includes('image') || m.includes('dall-e') || m.includes('gpt-image')
}

// 思考程度档位（reasoning_effort）：none = 关闭思考；其余为可调档。
// 空串 = 不发送（上游默认）。以 select 呈现。
export type PlaygroundReasoningControl = {
  key: 'reasoning_effort'
  labelKey: string
  descriptionKey: string
  valueType: 'select'
  options: readonly { value: string; labelKey: string }[]
}

export const PLAYGROUND_REASONING_CONTROL: PlaygroundReasoningControl = {
  key: 'reasoning_effort',
  labelKey: 'Reasoning Effort',
  descriptionKey: 'Controls how much the model thinks. Choose Off to disable thinking.',
  valueType: 'select',
  options: [
    { value: '', labelKey: 'Default' },
    { value: 'none', labelKey: 'Off (no thinking)' },
    { value: 'low', labelKey: 'Low' },
    { value: 'medium', labelKey: 'Medium' },
    { value: 'high', labelKey: 'High' },
  ],
} as const

export const PLAYGROUND_PARAMETER_CONTROLS = [
  {
    key: 'temperature',
    labelKey: 'Temperature',
    descriptionKey: 'Controls randomness and creativity',
    valueType: 'slider',
    min: 0.1,
    max: 1,
    step: 0.1,
  },
  {
    key: 'top_p',
    labelKey: 'Top P',
    descriptionKey: 'Limits token selection to a probability mass',
    valueType: 'slider',
    min: 0.1,
    max: 1,
    step: 0.1,
  },
  {
    key: 'frequency_penalty',
    labelKey: 'Frequency Penalty',
    descriptionKey: 'Reduces repeated wording',
    valueType: 'slider',
    min: -2,
    max: 2,
    step: 0.1,
  },
  {
    key: 'presence_penalty',
    labelKey: 'Presence Penalty',
    descriptionKey: 'Encourages new topics',
    valueType: 'slider',
    min: -2,
    max: 2,
    step: 0.1,
  },
  {
    key: 'max_tokens',
    labelKey: 'Max Tokens',
    descriptionKey: 'Caps the response length',
    valueType: 'number',
    min: 0,
    max: 200000,
    step: 1,
  },
  {
    key: 'seed',
    labelKey: 'Seed',
    descriptionKey: 'Keeps compatible responses more repeatable',
    valueType: 'number',
    min: 0,
    max: 2147483647,
    step: 1,
  },
] as const satisfies readonly PlaygroundParameterControl[]

export const PLAYGROUND_PARAMETER_PANEL_SCROLL_CLASS =
  'max-h-[min(28rem,calc(100vh-10rem))] overflow-y-auto pr-1'

export function normalizeParameterNumberValue(
  key: PlaygroundParameterKey,
  value: string | number
): number | null {
  if (value === '') {
    return key === 'seed' ? null : 0
  }

  const control = PLAYGROUND_PARAMETER_CONTROLS.find((item) => item.key === key)
  const parsed = typeof value === 'number' ? value : Number.parseFloat(value)

  if (!control || Number.isNaN(parsed)) {
    return key === 'seed' ? null : 0
  }

  const clamped = Math.min(control.max, Math.max(control.min, parsed))

  if (control.step >= 1) {
    return Math.trunc(clamped)
  }

  const precision = Math.max(0, String(control.step).split('.')[1]?.length ?? 0)
  return Number(clamped.toFixed(precision))
}

export function getParameterControlValueText(
  key: PlaygroundParameterKey,
  value: ParameterValue
): string {
  if (key === 'seed' && value === null) {
    return 'Not set'
  }

  return String(value)
}
