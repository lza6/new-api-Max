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
    score: isFiniteNumber(raw.score) ? raw.score : 0,
    successRate: isFiniteNumber(raw.success_rate) ? raw.success_rate : 0,
    p50LatencyMs: isFiniteNumber(raw.p50_latency_ms) ? raw.p50_latency_ms : 0,
    p95LatencyMs: isFiniteNumber(raw.p95_latency_ms) ? raw.p95_latency_ms : 0,
    coolCount: isFiniteNumber(raw.cool_count) ? raw.cool_count : 0,
    sampleCount: isFiniteNumber(raw.sample_count) ? raw.sample_count : 0,
    coolingDown: raw.cooling_down === true,
    coolUntil: isFiniteNumber(raw.cool_until) ? raw.cool_until : 0,
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
