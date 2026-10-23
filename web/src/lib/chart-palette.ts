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
/**
 * 图表序列色：单色顺序阶（视觉体系 §4.6）。
 *
 * 为什么需要这层：VChart 默认 dataScheme 是"蓝/青/黄/橙/紫"多色盘，正是要规避的
 * AI 通用配色；而 canvas 的 fillStyle 不保证接受 `color-mix()`。所以这里把
 * 主题里的 `--primary` / `--background` 先规范化成 rgb，再按权重混出同色相阶梯，
 * 输出 `#rrggbb` —— canvas 一定能吃，且跟随 11 套预设与亮/暗主题。
 */

const RAMP_WEIGHTS = [0.95, 0.78, 0.62, 0.48, 0.36, 0.26] as const

/** 预设读取失败时的兜底色（项目默认蓝），保证图表永远有颜色可画 */
const FALLBACK_SERIES = [
  '#2f6fd0',
  '#5a8ddc',
  '#86aae5',
  '#a9c3ec',
  '#c8d8f3',
  '#e2ebf9',
]

interface Rgb {
  r: number
  g: number
  b: number
}

/**
 * 借 canvas 把任意 CSS 颜色（含 oklch()）规范化成 rgb。
 * 拿不到 2d context（SSR / jsdom）或颜色非法时返回 null，由调用方兜底。
 */
function resolveCssColor(input: string): Rgb | null {
  if (typeof document === 'undefined') {return null}
  const canvas = document.createElement('canvas')
  const ctx = canvas.getContext('2d')
  if (!ctx) {return null}

  const sentinel = '#010203'
  ctx.fillStyle = sentinel
  ctx.fillStyle = input
  const normalized = ctx.fillStyle
  if (typeof normalized !== 'string' || normalized === sentinel) {return null}

  if (normalized.startsWith('#')) {
    const hex = normalized.slice(1)
    if (hex.length === 6) {
      return {
        r: Number.parseInt(hex.slice(0, 2), 16),
        g: Number.parseInt(hex.slice(2, 4), 16),
        b: Number.parseInt(hex.slice(4, 6), 16),
      }
    }
    if (hex.length === 3) {
      return {
        r: Number.parseInt(hex[0] + hex[0], 16),
        g: Number.parseInt(hex[1] + hex[1], 16),
        b: Number.parseInt(hex[2] + hex[2], 16),
      }
    }
    return null
  }

  const match = /rgba?\(([^)]+)\)/.exec(normalized)
  if (!match) {return null}
  const parts = match[1].split(',').map((part) => Number.parseFloat(part))
  if (parts.length < 3 || parts.some((part) => Number.isNaN(part))) {return null}
  return { r: parts[0], g: parts[1], b: parts[2] }
}

function toHex(value: number): string {
  const clamped = Math.max(0, Math.min(255, Math.round(value)))
  return clamped.toString(16).padStart(2, '0')
}

function mix(from: Rgb, to: Rgb, weight: number): string {
  const w = Math.max(0, Math.min(1, weight))
  return `#${toHex(from.r * w + to.r * (1 - w))}${toHex(
    from.g * w + to.g * (1 - w)
  )}${toHex(from.b * w + to.b * (1 - w))}`
}

function readToken(name: string): string | null {
  if (typeof window === 'undefined') {return null}
  const raw = window
    .getComputedStyle(document.documentElement)
    .getPropertyValue(name)
    .trim()
  return raw === '' ? null : raw
}

/**
 * 生成 `count` 个同色相、明度递减的序列色（超出阶梯长度时循环）。
 * 每次调用都重新读令牌 —— 切换预设/亮暗后无需清缓存。
 */
export function buildChartRamp(count: number): string[] {
  const total = Math.max(1, Math.trunc(count) || 1)
  const primary = resolveCssColor(readToken('--primary') ?? '')
  const background = resolveCssColor(readToken('--background') ?? '')

  if (!primary || !background) {
    return Array.from(
      { length: total },
      (_, index) => FALLBACK_SERIES[index % FALLBACK_SERIES.length]
    )
  }

  return Array.from({ length: total }, (_, index) =>
    mix(primary, background, RAMP_WEIGHTS[index % RAMP_WEIGHTS.length])
  )
}
