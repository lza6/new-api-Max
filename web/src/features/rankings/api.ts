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
import { api } from '@/lib/api'

import type {
  BandwidthLeaderboard,
  RankingPeriod,
  RankingsSnapshot,
} from './types'

type RankingsResponse = {
  success: boolean
  message?: string
  data: RankingsSnapshot
}

export async function getBandwidth(
  days = 30,
  limit = 10
): Promise<{ success: boolean; data: BandwidthLeaderboard }> {
  const res = await api.get('/api/rankings/bandwidth', {
    params: { days, limit },
  })
  return res.data
}
export async function getRankings(
  period: RankingPeriod
): Promise<RankingsResponse> {
  const res = await api.get('/api/rankings', { params: { period } })
  return res.data
}

export interface CompressionStatRow {
  model_name: string
  count: number
  original_bytes: number
  compressed_bytes: number
  saved_bytes: number
  saved_gb: number
  saved_mb: number
  ratio: number
}

// 按模型出站压缩统计（持久化，重启不丢）。
export async function getCompressionStats(
  limit = 50
): Promise<{ success: boolean; data: CompressionStatRow[] }> {
  const res = await api.get('/api/rankings/compression', { params: { limit } })
  return res.data
}

/** T1 反事实节省基准：单模型行。 */
export interface SavingsBaselineModel {
  model_name: string
  count: number
  original_bytes: number
  compressed_bytes: number
  saved_bytes: number
  /** saved / original，0..1 */
  saved_ratio: number
  /** 未压缩时本应上传的字节（= original_bytes） */
  counterfactual_bytes: number
}

/** T1 反事实节省基准：全站合计。 */
export interface SavingsBaseline {
  total_count: number
  total_original_bytes: number
  total_compressed_bytes: number
  total_saved_bytes: number
  /** 0..1 */
  overall_saved_ratio: number
  counterfactual_bytes: number
  /** 按观测上行带宽折算的省时（秒）；bandwidth_bps 为 0 时该值无意义 */
  saved_time_seconds: number
  bandwidth_bps: number
  /** 服务端已格式化的可读节省量（如 "163.7 MB"） */
  saved_text: string
  models: SavingsBaselineModel[]
}

/**
 * T1 统一节省口径 + 反事实基准：回答「如果不做优化，会多传多少流量」。
 * 与压缩统计同源（model_compression_stats），口径统一为 saved/original。
 */
export async function getSavingsBaseline(
  limit = 50
): Promise<{ success: boolean; data: SavingsBaseline }> {
  const res = await api.get('/api/rankings/savings-baseline', {
    params: { limit },
  })
  return res.data
}

export interface ClientUsage {
  client: string
  count: number
  share: number
}
export interface ModelClientBreakdown {
  model: string
  total: number
  clients: ClientUsage[]
}
export interface ClientStats {
  window_days: number
  total: number
  overall: ClientUsage[]
  by_model: ModelClientBreakdown[]
  avg_cache_rate: number
}
export async function getClientStats(
  days = 7,
  models = 20
): Promise<{ success: boolean; data: ClientStats }> {
  const res = await api.get('/api/rankings/clients', { params: { days, models } })
  return res.data
}
