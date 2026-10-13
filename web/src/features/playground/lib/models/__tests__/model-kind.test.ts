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
import { describe, expect, test } from 'vitest'

import {
  getModelKind,
  groupModelsByKind,
  isMediaGenerationKind,
  MODEL_KIND_ORDER,
} from '../model-kind'

describe('getModelKind', () => {
  test('classifies plain conversational models as text', () => {
    // 这是最重要的默认：绝大多数模型是文本对话，认不出必须落到 text，
    // 不能误判成 media（否则选择器会把文本模型塞进「图片」分组）。
    for (const model of [
      'deepseek-v4.1-flash',
      'gpt-6-luna',
      'claude-opus-5-5',
      'glm-5.3-flash',
      'qwen-max',
      'kimi-k3',
    ]) {
      expect(getModelKind(model)).toBe('text')
    }
  })

  test('classifies image generation models as image', () => {
    for (const model of [
      'dall-e-3',
      'gpt-image-1',
      'gptimage-2',
      'flux-pro',
      'imagen-3',
      'midjourney-v6',
      'stable-diffusion-xl',
      'nanobanana-2',
      'seedream-3',
      'agnes-image',
    ]) {
      expect(getModelKind(model)).toBe('image')
    }
  })

  test('classifies video models as video, including names that also contain image', () => {
    for (const model of [
      'veo-3',
      'kling-v2',
      'sora-2',
      'runway-gen3',
      'hailuo-02',
      'seedance-1',
    ]) {
      expect(getModelKind(model)).toBe('video')
    }
    // 边界：同时含 'image' 与视频关键词时必须判为视频 —— 视频优先于图片，
    // 否则「文生视频」会被归到图片分组，用户点进去拿不到尺寸/时长参数。
    expect(getModelKind('veo-image')).toBe('video')
  })

  test('classifies audio models as audio', () => {
    for (const model of [
      'whisper-1',
      'tts-1-hd',
      'gpt-4o-audio-preview',
      'suno-v4',
      'lyrics-gen',
    ]) {
      expect(getModelKind(model)).toBe('audio')
    }
  })

  test('classifies embedding and rerank models as embedding', () => {
    for (const model of [
      'text-embedding-3-large',
      'bge-m3',
      'bge-reranker-v2',
      'embed-multilingual',
    ]) {
      expect(getModelKind(model)).toBe('embedding')
    }
  })

  test('treats empty or whitespace model names as text', () => {
    // 空模型名不能抛错，也不能被归成 media —— 后续逻辑依赖它给出一个可用选择。
    expect(getModelKind('')).toBe('text')
    expect(getModelKind('   ')).toBe('text')
  })

  test('is case-insensitive', () => {
    expect(getModelKind('DALL-E-3')).toBe('image')
    expect(getModelKind('Whisper-1')).toBe('audio')
    expect(getModelKind('VEO-3')).toBe('video')
  })
})

describe('isMediaGenerationKind', () => {
  test('reports media generators as media and text as non-media', () => {
    expect(isMediaGenerationKind('image')).toBe(true)
    expect(isMediaGenerationKind('video')).toBe(true)
    expect(isMediaGenerationKind('audio')).toBe(true)
    // 文本与向量不产生媒体，选择器不该给它们显示尺寸/时长参数。
    expect(isMediaGenerationKind('text')).toBe(false)
    expect(isMediaGenerationKind('embedding')).toBe(false)
  })
})

describe('groupModelsByKind', () => {
  test('groups models by kind in the declared order', () => {
    const groups = groupModelsByKind([
      { value: 'veo-3' },
      { value: 'deepseek-v4.1-flash' },
      { value: 'dall-e-3' },
      { value: 'whisper-1' },
    ])

    expect(groups.map((g) => g.kind)).toEqual(['text', 'image', 'video', 'audio'])
    expect(groups[0].models.map((m) => m.value)).toEqual(['deepseek-v4.1-flash'])
    expect(groups[1].models.map((m) => m.value)).toEqual(['dall-e-3'])
  })

  test('omits empty kinds so no empty group headers render', () => {
    const groups = groupModelsByKind([{ value: 'gpt-6-luna' }])
    expect(groups).toHaveLength(1)
    expect(groups[0].kind).toBe('text')
  })

  test('preserves the original order within a kind', () => {
    // 后端返回顺序通常按相关度排好；组内重排会让用户找不到常用模型。
    const groups = groupModelsByKind([
      { value: 'gpt-6-luna' },
      { value: 'deepseek-v4.1-flash' },
      { value: 'glm-5.3-flash' },
    ])
    expect(groups[0].models.map((m) => m.value)).toEqual([
      'gpt-6-luna',
      'deepseek-v4.1-flash',
      'glm-5.3-flash',
    ])
  })

  test('returns an empty array for no models', () => {
    expect(groupModelsByKind([])).toEqual([])
  })

  test('carries through extra fields on the model objects', () => {
    // 选择器要靠 label 渲染、靠 value 提交；归组不能把字段丢掉。
    const groups = groupModelsByKind([
      { value: 'dall-e-3', label: 'DALL·E 3', description: 'image model' },
    ])
    expect(groups[0].models[0]).toEqual({
      value: 'dall-e-3',
      label: 'DALL·E 3',
      description: 'image model',
    })
  })
})

describe('MODEL_KIND_ORDER', () => {
  test('starts with text as the most common kind', () => {
    expect(MODEL_KIND_ORDER[0]).toBe('text')
  })

  test('covers every kind exactly once', () => {
    // 漏一个 kind 会让该类模型在分组时被静默丢弃（flatMap 找不到顺序就不产出）。
    expect(new Set(MODEL_KIND_ORDER).size).toBe(MODEL_KIND_ORDER.length)
    expect(MODEL_KIND_ORDER).toEqual(['text', 'image', 'video', 'audio', 'embedding'])
  })
})
