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
// T13 Agent 预设的**纯逻辑层**：校验 + 预设→PlaygroundConfig 的映射。
//
// 这些函数不碰网络、不碰 React，因此可以直接单测，并且被「保存」与「应用」
// 两条路径共用，避免校验规则出现两份实现而分叉。

import type { AgentPreset, PlaygroundConfig } from '../../types'

/** 与服务端 relaykit/dto 的上界保持一致（前端先拦，避免用户撞 400）。 */
export const MAX_AGENT_PRESETS = 20
export const MAX_PRESET_NAME_RUNES = 64
export const MAX_PRESET_SYSTEM_RUNES = 8000
export const MAX_PRESET_MAX_TOKENS = 32000
export const MIN_TEMPERATURE = 0
export const MAX_TEMPERATURE = 2

/** 按 Unicode 码点计数（与后端 utf8.RuneCountInString 同口径）。 */
export function runeLength(value: string): number {
  return [...value].length
}

export type PresetValidationError =
  | 'name_required'
  | 'name_too_long'
  | 'system_prompt_too_long'
  | 'temperature_out_of_range'
  | 'max_tokens_out_of_range'
  | 'too_many_presets'

/**
 * 校验单个预设。返回错误码（null = 合法）。
 * 与后端 parseAgentPresets 的边界完全一致。
 */
export function validatePreset(preset: AgentPreset): PresetValidationError | null {
  const name = preset.name.trim()
  if (name === '') {
    return 'name_required'
  }
  if (runeLength(name) > MAX_PRESET_NAME_RUNES) {
    return 'name_too_long'
  }
  if (runeLength(preset.system_prompt ?? '') > MAX_PRESET_SYSTEM_RUNES) {
    return 'system_prompt_too_long'
  }
  if (preset.temperature !== undefined) {
    if (
      !Number.isFinite(preset.temperature) ||
      preset.temperature < MIN_TEMPERATURE ||
      preset.temperature > MAX_TEMPERATURE
    ) {
      return 'temperature_out_of_range'
    }
  }
  if (preset.max_tokens !== undefined) {
    if (
      !Number.isInteger(preset.max_tokens) ||
      preset.max_tokens <= 0 ||
      preset.max_tokens > MAX_PRESET_MAX_TOKENS
    ) {
      return 'max_tokens_out_of_range'
    }
  }
  return null
}

/** 校验整个列表（含数量上限）。返回第一个错误码（null = 全部合法）。 */
export function validatePresetList(
  presets: AgentPreset[]
): PresetValidationError | null {
  if (presets.length > MAX_AGENT_PRESETS) {
    return 'too_many_presets'
  }
  for (const preset of presets) {
    const error = validatePreset(preset)
    if (error) {
      return error
    }
  }
  return null
}

/**
 * 把预设应用到当前配置，返回**新的**配置对象（不可变，不修改入参）。
 *
 * 语义：只有预设显式给出的字段才覆盖当前值——未设置的字段保持用户当前选择，
 * 这样「只设了系统提示词的预设」不会顺手把模型重置掉。
 * 空字符串视为「未设置」（与后端 `omitempty` 语义一致）。
 */
export function applyPresetToConfig(
  config: PlaygroundConfig,
  preset: AgentPreset
): PlaygroundConfig {
  const next: PlaygroundConfig = { ...config }

  if (preset.model !== undefined && preset.model !== '') {
    next.model = preset.model
  }
  if (preset.group !== undefined && preset.group !== '') {
    next.group = preset.group
  }
  if (preset.system_prompt !== undefined && preset.system_prompt !== '') {
    next.system_prompt = preset.system_prompt
  }
  if (preset.temperature !== undefined) {
    next.temperature = preset.temperature
  }
  if (preset.max_tokens !== undefined) {
    next.max_tokens = preset.max_tokens
  }
  if (preset.reasoning_effort !== undefined && preset.reasoning_effort !== '') {
    next.reasoning_effort = preset.reasoning_effort
  }
  return next
}

/**
 * 从当前配置反推一个预设（「把当前配置存为预设」）。
 * 只记录非空字段，保持与 applyPresetToConfig 的对称性。
 */
export function presetFromConfig(
  config: PlaygroundConfig,
  name: string
): AgentPreset {
  const preset: AgentPreset = {
    id:
      typeof crypto !== 'undefined' && 'randomUUID' in crypto
        ? crypto.randomUUID()
        : `preset-${Date.now()}`,
    name,
  }
  if (config.model) {
    preset.model = config.model
  }
  if (config.group) {
    preset.group = config.group
  }
  if (config.system_prompt) {
    preset.system_prompt = config.system_prompt
  }
  preset.temperature = config.temperature
  preset.max_tokens = config.max_tokens
  if (config.reasoning_effort) {
    preset.reasoning_effort = config.reasoning_effort
  }
  return preset
}
