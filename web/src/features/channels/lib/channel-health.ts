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
import type {
  ChannelHealthScoresResponse,
  ProbeCaseResult,
  ProbeReport,
} from '../types'

export type { ProbeCaseResult, ProbeReport }

/**
 * Parsed probe history with defensive defaults so every consumer can rely on
 * non-undefined fields.
 */
export interface ParsedProbeHistory {
  reports: ProbeReport[]
  latest: ProbeReport | null
}

/** Health score snapshot for one channel, all fields defaulted. */
export interface ChannelHealthSnapshotView {
  score: number
  successRate: number
  p50LatencyMs: number
  p95LatencyMs: number
  coolCount: number
  sampleCount: number
  coolingDown: boolean
  coolUntil: number
  /** 最近一次冷却的错误类标识（B5-2 hover 原因），如 auth/rate_limited/timeout */
  lastCoolClass?: string
}

export const EMPTY_HEALTH_SNAPSHOT: ChannelHealthSnapshotView = {
  score: 0,
  successRate: 0,
  p50LatencyMs: 0,
  p95LatencyMs: 0,
  coolCount: 0,
  sampleCount: 0,
  coolingDown: false,
  coolUntil: 0,
}

function isFiniteNumber(value: unknown): value is number {
  return typeof value === 'number' && Number.isFinite(value)
}

function isProbeReport(value: unknown): value is ProbeReport {
  return typeof value === 'object' && value !== null
}

/**
 * Parse the channel `probe_result` column (json array of probe reports,
 * newest first). Returns an empty history for null/empty/malformed data.
 */
export function parseProbeHistory(raw: string | null | undefined): ParsedProbeHistory {
  if (!raw) {
    return { reports: [], latest: null }
  }
  let parsed: unknown
  try {
    parsed = JSON.parse(raw)
  } catch {
    return { reports: [], latest: null }
  }
  if (!Array.isArray(parsed)) {
    return { reports: [], latest: null }
  }
  const reports = parsed.filter(isProbeReport)
  const latest = reports.length > 0 ? reports[0] : null
  return { reports, latest }
}

/**
 * Normalize the health snapshot map from GET /api/channel/health_scores.
 * Unknown/missing entries fall back to an empty snapshot.
 */
export function toHealthSnapshotView(
  scores: NonNullable<ChannelHealthScoresResponse['data']> | undefined,
  channelId: number
): ChannelHealthSnapshotView {
  const raw = scores?.[String(channelId)]
  if (!raw || typeof raw !== 'object') {
    return EMPTY_HEALTH_SNAPSHOT
  }
  return {
    // 健康分语义域 0-100、成功率 0-1：钳制防止上游/缓存异常超大数污染聚合。
    score: isFiniteNumber(raw.score) ? Math.min(100, Math.max(0, raw.score)) : 0,
    successRate: isFiniteNumber(raw.success_rate)
      ? Math.min(1, Math.max(0, raw.success_rate))
      : 0,
    p50LatencyMs: isFiniteNumber(raw.p50_latency_ms) ? raw.p50_latency_ms : 0,
    p95LatencyMs: isFiniteNumber(raw.p95_latency_ms) ? raw.p95_latency_ms : 0,
    coolCount: isFiniteNumber(raw.cool_count) ? raw.cool_count : 0,
    sampleCount: isFiniteNumber(raw.sample_count) ? raw.sample_count : 0,
    coolingDown: raw.cooling_down === true, // 严格布尔，防 truthy 误判
    coolUntil: isFiniteNumber(raw.cool_until) ? raw.cool_until : 0,
    lastCoolClass:
      typeof raw.last_cool_class === 'string' ? raw.last_cool_class : undefined,
  }
}

/**
 * Whether the snapshot carries any observation (score > 0 or samples seen).
 * Snapshots with only a cool_count (cooldown events without request samples)
 * also count so the popover can still show details.
 */
export function hasHealthData(snapshot: ChannelHealthSnapshotView): boolean {
  return snapshot.score > 0 || snapshot.sampleCount > 0
}

/**
 * Cooldown error class id -> i18n key under cool-class.* (B5-2 hover reason).
 * Unknown/empty/ok map to the generic key so the tooltip never shows a raw
 * identifier; known classes (auth/rate_limited/server_error/timeout/...)
 * resolve to their dedicated human labels.
 */
export function coolClassLabelKey(coolClass: string | undefined): string {
  if (coolClass && coolClass !== 'ok') {
    return `cool-class.${coolClass}`
  }
  return 'cool-class.unknown'
}

export type ProbeGradeVariant =
  | 'success'
  | 'warning'
  | 'orange'
  | 'danger'
  | 'neutral'

/** Map a probe grade letter to a StatusBadge variant (A/B green, C yellow, D orange, F red). */
export function gradeToVariant(grade: string | undefined): ProbeGradeVariant {
  switch (grade) {
    case 'A':
    case 'B':
      return 'success'
    case 'C':
      return 'warning'
    case 'D':
      return 'orange'
    default:
      return 'danger'
  }
}

/** Map a 0-100 health score to a StatusBadge variant. */
export function scoreToVariant(score: number): ProbeGradeVariant {
  if (score >= 85) {
    return 'success'
  }
  if (score >= 70) {
    return 'warning'
  }
  if (score >= 55) {
    return 'orange'
  }
  return 'danger'
}

/** Safe access to a probe case result list. */
export function getProbeResults(report: ProbeReport | null): ProbeCaseResult[] {
  if (!report || !Array.isArray(report.results)) {
    return []
  }
  return report.results.filter(isProbeReport)
}


// ============================================================================
// T2-2 聚合概览
// ============================================================================

export interface ChannelHealthAggregate {
  /** 有样本渠道的平均健康分（0-100）；无样本/无数据时 0 */
  avgScore: number
  /** 最差渠道（按 score 升序、冷却优先）前 N 条的 score + 冷却标记 */
  worst: Array<{ score: number; coolingDown: boolean; sampleCount: number }>
  /** 可用率 = score>=70 且有样本的渠道数 / 有样本渠道数；无样本时 0 */
  availableRate: number
  /** 有样本渠道数 */
  totalSampled: number
  /** 冷却中渠道数 */
  coolingCount: number
  /** 是否存在任何健康数据（有快照且 hasHealthData） */
  hasData: boolean
}

const AVAILABLE_SCORE_THRESHOLD = 70

/**
 * 聚合所有渠道健康分快照（纯前端、零请求）。数据源为 provider 已注入的
 * healthScores；仅统计「有观测」渠道（hasHealthData），无样本渠道不参与均值，
 * 避免新渠道拉低概览。可用率沿用 scoreToVariant 的 warning 分界（>=70）。
 */
export function aggregateHealthScores(
  scores: NonNullable<ChannelHealthScoresResponse['data']> | null | undefined
): ChannelHealthAggregate {
  const empty: ChannelHealthAggregate = {
    avgScore: 0,
    worst: [],
    availableRate: 0,
    totalSampled: 0,
    coolingCount: 0,
    hasData: false,
  }
  if (!scores) {
    return empty
  }
  const entries = Object.entries(scores)
    .map(([id]) => ({ id, view: toHealthSnapshotView(scores, Number(id)) }))
    .filter(({ view }) => hasHealthData(view))
  if (entries.length === 0) {
    return empty
  }

  const sampled = entries.filter(({ view }) => view.sampleCount > 0)
  const totalSampled = sampled.length
  const avgScore =
    totalSampled > 0
      ? sampled.reduce((sum, { view }) => sum + view.score, 0) / totalSampled
      : 0
  const availableCount = sampled.filter(
    ({ view }) => view.score >= AVAILABLE_SCORE_THRESHOLD
  ).length
  const availableRate = totalSampled > 0 ? availableCount / totalSampled : 0
  const coolingCount = entries.filter(({ view }) => view.coolingDown).length

  // 最差渠道：冷却优先（冷却中视为最差），再按 score 升序，取前 3。
  const sorted = [...entries].sort((a, b) => {
    if (a.view.coolingDown !== b.view.coolingDown) {
      return a.view.coolingDown ? -1 : 1
    }
    return a.view.score - b.view.score
  })
  const worst = sorted.slice(0, 3).map(({ view }) => ({
    score: view.score,
    coolingDown: view.coolingDown,
    sampleCount: view.sampleCount,
  }))

  return {
    avgScore,
    worst,
    availableRate,
    totalSampled,
    coolingCount,
    hasData: true,
  }
}
