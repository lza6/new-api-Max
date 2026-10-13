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
// 模型能力分类（纯函数，无 React/网络依赖，便于单测）。
//
// ## 为什么需要它
//
// `/api/user/models` 只返回**模型名字符串数组**（`api.ts` 的 `getUserModels`），
// 不携带能力元数据。游乐场的模型选择器因此只能把全部模型平铺成一列——用户在一个
// 下拉里同时看到文本对话模型和文生图/文生视频模型，选错就发错请求。
//
// 这里按**模型名规则**做一次保守分类，把选择器拆成「文本 / 图片 / 视频 / 音频 /
// 向量」几类。规则式而非元数据式是**刻意的**：新增模型无需改后端即可被归类，
// 代价是可能出现边界误判——所以每个分类都只匹配**明确的**关键词，拿不准一律
// 归入文本（最安全的默认：文本对话是绝大多数模型的能力）。
//
// ## 与既有判定的关系
//
// 视频判定复用 `lib/video/video-generation-utils.ts` 的 `isVideoModel`，
// 不重复维护第二份视频关键词表——两处漂移会让「选择器把它当视频、提交时却不走
// 视频分支」这类不一致重新出现。

import { isVideoModel } from '../video/video-generation-utils'

/** 模型能力类别。`text` 是兜底类别。 */
export type ModelKind = 'text' | 'image' | 'video' | 'audio' | 'embedding'

/**
 * 各类别的展示顺序。文本在最前（最常用），其余按使用频率排列。
 * 选择器按本顺序渲染分组。
 */
export const MODEL_KIND_ORDER: ModelKind[] = [
  'text',
  'image',
  'video',
  'audio',
  'embedding',
]

/**
 * i18n 键。与选择器的 `{{category}} Models` 模板衔接：
 * `t(MODEL_KIND_LABEL_KEY[kind])` 得到分组标题。
 */
export const MODEL_KIND_LABEL_KEY: Record<ModelKind, string> = {
  text: 'Text',
  image: 'Image',
  video: 'Video',
  audio: 'Audio',
  embedding: 'Embedding',
}

// 图片生成模型关键词（文生图 / 图生图 / 图片编辑）。
// 只列**明确**表示图片生成的词，不列 `dall-e` 之外可能的歧义词。
const IMAGE_KEYWORDS = [
  'dall-e',
  'dalle',
  'gpt-image',
  'gptimage',
  'image',
  'imagen',
  'flux',
  'midjourney',
  'stable-diffusion',
  'sd3',
  'sdxl',
  'nanobanana',
  'nano-banana',
  'seedream',
  'seededit',
  'kolors',
  'ideogram',
  'recraft',
  'playground-v',
  'qwen-image',
  'wanx',
  'agnes-image',
  'mj',
]

// 音频模型关键词（TTS / 语音识别 / 音乐生成）。
const AUDIO_KEYWORDS = [
  'tts',
  'whisper',
  'audio',
  'speech',
  'voice',
  'suno',
  'music',
  'lyrics',
  'transcribe',
  'transcription',
  'realtime',
]

// 向量模型关键词。
const EMBEDDING_KEYWORDS = ['embedding', 'embed', 'bge-', 'text-embedding', 'rerank']

function matchesAny(haystack: string, needles: string[]): boolean {
  return needles.some((needle) => haystack.includes(needle))
}

/**
 * 判定模型能力类别。
 *
 * 优先级（**顺序即契约**，不可随意调换）：
 *  1. 视频 —— 复用既有 `isVideoModel`，避免两套视频关键词漂移
 *  2. 向量 —— `embedding`/`rerank` 等词几乎不会出现在别的类别里
 *  3. 音频
 *  4. 图片
 *  5. 兜底文本
 *
 * 视频排在图片之前是必要的：`veo-image` 之类的名字同时含 `image`，但它是视频能力。
 */
export function getModelKind(model: string): ModelKind {
  const name = (model || '').toLowerCase().trim()
  if (name === '') {
    return 'text'
  }
  if (isVideoModel(name)) {
    return 'video'
  }
  if (matchesAny(name, EMBEDDING_KEYWORDS)) {
    return 'embedding'
  }
  if (matchesAny(name, AUDIO_KEYWORDS)) {
    return 'audio'
  }
  if (matchesAny(name, IMAGE_KEYWORDS)) {
    return 'image'
  }
  return 'text'
}

/**
 * 报告该类别是否「生成媒体」（图片/视频/音频）——即需要尺寸/时长等生成参数，
 * 且不参与对话历史。
 *
 * 选择器据此决定是否展示生成参数区，**不**让文本模型看到无关的尺寸选择。
 */
export function isMediaGenerationKind(kind: ModelKind): boolean {
  return kind === 'image' || kind === 'video' || kind === 'audio'
}

/**
 * 把模型列表按类别归组，保持类别顺序与组内原始顺序（后端返回顺序通常已按相关度
 * 排列，重排会让用户找不到常模型）。
 *
 * 空类别不产出条目——避免选择器里出现空分组标题。
 */
export function groupModelsByKind<T extends { value: string }>(
  models: T[]
): Array<{ kind: ModelKind; models: T[] }> {
  const buckets = new Map<ModelKind, T[]>()
  for (const model of models) {
    const kind = getModelKind(model.value)
    const bucket = buckets.get(kind)
    if (bucket) {
      bucket.push(model)
    } else {
      buckets.set(kind, [model])
    }
  }
  return MODEL_KIND_ORDER.flatMap((kind) => {
    const bucket = buckets.get(kind)
    return bucket && bucket.length > 0 ? [{ kind, models: bucket }] : []
  })
}
