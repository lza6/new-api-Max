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
import { requireServerSuccess } from '@/lib/server-error-message'

import type {
  ModelCostRule,
  OpsDaySeriesItem,
  OpsOverview,
  OpsRankingResult,
} from './types'

/** GET /api/ops/overview —— 当日/累计消耗、用户剩余额度合计、日均消耗与运营天数估算。 */
export async function getOpsOverview(days?: number): Promise<OpsOverview> {
  const res = await api.get('/api/admin/ops/overview', {
    params: days == null ? undefined : { days },
  })
  return requireServerSuccess(res.data)?.data as OpsOverview
}

/** GET /api/ops/consumption-ranking —— 消耗排行榜（按用户聚合窗口内 consume 额度）。 */
export async function getOpsConsumptionRanking(
  days: number,
  limit: number
): Promise<OpsRankingResult> {
  const res = await api.get('/api/admin/ops/consumption-ranking', {
    params: { days, limit },
  })
  return requireServerSuccess(res.data)?.data as OpsRankingResult
}

/** POST /api/ops/grant-quota —— 给指定用户加额度（服务端复用既有加额度逻辑）。 */
export async function grantUserQuota(payload: {
  user_id: number
  quota: number
}): Promise<void> {
  const res = await api.post('/api/admin/ops/grant-quota', payload)
  requireServerSuccess(res.data)
}

/** GET /api/admin/ops/daily —— 逐日营收/成本/利润（本地时区）。 */
export async function getOpsDailySeries(
  days = 30
): Promise<{ days: number; items: OpsDaySeriesItem[] }> {
  const res = await api.get('/api/admin/ops/daily', { params: { days } })
  return requireServerSuccess(res.data)?.data as {
    days: number
    items: OpsDaySeriesItem[]
  }
}

/** GET /api/admin/ops/model-costs —— 成本配置历史（append-only）。 */
export async function getModelCosts(params?: {
  channel_id?: number
  model?: string
  limit?: number
}): Promise<{ items: ModelCostRule[] }> {
  const res = await api.get('/api/admin/ops/model-costs', { params })
  return requireServerSuccess(res.data)?.data as {
    items: ModelCostRule[]
  }
}

/** POST /api/admin/ops/model-costs —— 追加一条成本规则（不修改历史）。 */
export async function createModelCost(payload: {
  channel_id: number
  model: string
  billing_mode: 'call' | 'token'
  cost_per_call_cny: number
  cost_input_per_million_cny: number
  cost_output_per_million_cny: number
  effective_from?: number
  note?: string
}): Promise<void> {
  const res = await api.post('/api/admin/ops/model-costs', payload)
  requireServerSuccess(res.data)
}
