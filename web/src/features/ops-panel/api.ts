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

import type { OpsOverview, OpsRankingResult } from './types'

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
