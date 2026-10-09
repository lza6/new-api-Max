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
  applyPresetToConfig,
  MAX_AGENT_PRESETS,
  MAX_PRESET_MAX_TOKENS,
  presetFromConfig,
  runeLength,
  validatePreset,
  validatePresetList,
} from '../agent-preset-utils'
import type { AgentPreset, PlaygroundConfig } from '../../../types'

const baseConfig: PlaygroundConfig = {
  model: 'deepseek-v4.1-flash',
  group: 'default',
  temperature: 0.7,
  top_p: 1,
  max_tokens: 4096,
  frequency_penalty: 0,
  presence_penalty: 0,
  seed: null,
  stream: true,
  size: '',
  reasoning_effort: '',
  system_prompt: '',
}

describe('agent preset validation', () => {
  it('requires a non-blank name and enforces the server limits', () => {
    expect(validatePreset({ id: '1', name: '' })).toBe('name_required')
    expect(validatePreset({ id: '1', name: '   ' })).toBe('name_required')
    expect(
      validatePreset({ id: '1', name: 'x'.repeat(65) })
    ).toBe('name_too_long')
    expect(
      validatePreset({ id: '1', name: 'ok', system_prompt: 'x'.repeat(8001) })
    ).toBe('system_prompt_too_long')
    expect(validatePreset({ id: '1', name: 'ok', temperature: 3 })).toBe(
      'temperature_out_of_range'
    )
    expect(validatePreset({ id: '1', name: 'ok', temperature: -1 })).toBe(
      'temperature_out_of_range'
    )
    expect(validatePreset({ id: '1', name: 'ok', max_tokens: 0 })).toBe(
      'max_tokens_out_of_range'
    )
    expect(
      validatePreset({ id: '1', name: 'ok', max_tokens: MAX_PRESET_MAX_TOKENS + 1 })
    ).toBe('max_tokens_out_of_range')
    // 边界值本身合法。
    expect(validatePreset({ id: '1', name: 'ok', temperature: 2 })).toBeNull()
    expect(
      validatePreset({ id: '1', name: 'ok', max_tokens: MAX_PRESET_MAX_TOKENS })
    ).toBeNull()
    expect(validatePreset({ id: '1', name: 'ok' })).toBeNull()
  })

  it('counts by Unicode code points, not UTF-16 units', () => {
    // 64 个汉字是 64 个码点（UTF-16 下 .length 也是 64，但 emoji 会不同）。
    // 这里用代理对（emoji）验证口径：每个 emoji 计 1。
    expect(runeLength('🎉'.repeat(64))).toBe(64)
    expect(runeLength('🎉'.repeat(65))).toBe(65)
    expect(
      validatePreset({ id: '1', name: '🎉'.repeat(65) })
    ).toBe('name_too_long')
    expect(
      validatePreset({ id: '1', name: '🎉'.repeat(64) })
    ).toBeNull()
  })

  it('rejects a list that exceeds the preset count limit', () => {
    const many: AgentPreset[] = Array.from(
      { length: MAX_AGENT_PRESETS + 1 },
      (_, i) => ({ id: String(i), name: `p${i}` })
    )
    expect(validatePresetList(many)).toBe('too_many_presets')
    expect(validatePresetList(many.slice(0, MAX_AGENT_PRESETS))).toBeNull()
  })
})

describe('applying a preset to the playground config', () => {
  it('only overrides fields the preset actually sets', () => {
    const preset: AgentPreset = { id: '1', name: 'only system', system_prompt: 'be terse' }
    const next = applyPresetToConfig(baseConfig, preset)

    expect(next.system_prompt).toBe('be terse')
    // 未设置的字段必须保持原值（不能因为套用预设把模型重置掉）。
    expect(next.model).toBe('deepseek-v4.1-flash')
    expect(next.group).toBe('default')
    expect(next.temperature).toBe(0.7)
    expect(next.max_tokens).toBe(4096)
  })

  it('treats empty strings as unset, matching the server omitempty semantics', () => {
    const preset: AgentPreset = {
      id: '1',
      name: 'blank fields',
      model: '',
      group: '',
      system_prompt: '',
      reasoning_effort: '',
    }
    const next = applyPresetToConfig(baseConfig, preset)
    expect(next.model).toBe('deepseek-v4.1-flash')
    expect(next.group).toBe('default')
    expect(next.system_prompt).toBe('')
    expect(next.reasoning_effort).toBe('')
  })

  it('does not mutate the input config', () => {
    const snapshot = { ...baseConfig }
    applyPresetToConfig(baseConfig, {
      id: '1',
      name: 'x',
      model: 'other',
      system_prompt: 'changed',
    })
    expect(baseConfig).toEqual(snapshot)
  })

  it('round-trips config → preset → config for the fields it records', () => {
    const config: PlaygroundConfig = {
      ...baseConfig,
      model: 'glm-5.3-flash',
      group: 'token计费',
      system_prompt: 'you are a Go engineer',
      reasoning_effort: 'medium',
    }
    const preset = presetFromConfig(config, 'roundtrip')
    const restored = applyPresetToConfig(baseConfig, preset)

    expect(restored.model).toBe('glm-5.3-flash')
    expect(restored.group).toBe('token计费')
    expect(restored.system_prompt).toBe('you are a Go engineer')
    expect(restored.reasoning_effort).toBe('medium')
    // 生成的预设必须自身合法（否则「存为预设」会立刻被自己的校验拒绝）。
    expect(validatePreset(preset)).toBeNull()
  })

  it('omits empty optional fields when deriving a preset from config', () => {
    const preset = presetFromConfig(
      { ...baseConfig, model: '', group: '', system_prompt: '', reasoning_effort: '' },
      'sparse'
    )
    expect(preset.model).toBeUndefined()
    expect(preset.group).toBeUndefined()
    expect(preset.system_prompt).toBeUndefined()
    expect(preset.reasoning_effort).toBeUndefined()
  })
})
