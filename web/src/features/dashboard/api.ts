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
  FlowQuotaDataItem,
  QuotaDataItem,
  UptimeGroupResult,
} from './types'

// ============================================================================
// Dashboard APIs
// ============================================================================

// ----------------------------------------------------------------------------
// Quota & Usage Data
// ----------------------------------------------------------------------------

// Get user quota data within a time range
// Admin users get all users' data by default.
export async function getUserQuotaDates(
  params: {
    start_timestamp: number
    end_timestamp: number
    default_time?: string
    username?: string
  },
  isAdmin = false
) {
  const endpoint = isAdmin ? '/api/data' : '/api/data/self'
  const res = await api.get<{ success: boolean; data: QuotaDataItem[] }>(
    endpoint,
    { params }
  )
  return res.data
}

// ----------------------------------------------------------------------------
// System Monitoring
// ----------------------------------------------------------------------------

export async function getUserQuotaDataByUsers(params: {
  start_timestamp: number
  end_timestamp: number
}) {
  const res = await api.get<{ success: boolean; data: QuotaDataItem[] }>(
    '/api/data/users',
    { params }
  )
  return res.data
}

export async function getFlowQuotaDates(
  params: {
    start_timestamp: number
    end_timestamp: number
    default_time?: string
    username?: string
  },
  isAdmin = false
) {
  const endpoint = isAdmin ? '/api/data/flow' : '/api/data/flow/self'
  const res = await api.get<{
    success: boolean
    data?: FlowQuotaDataItem[]
    message?: string
  }>(endpoint, { params })
  return res.data
}

// Get uptime monitoring status for all services
export async function getUptimeStatus() {
  const res = await api.get<{ success: boolean; data: UptimeGroupResult[] }>(
    '/api/uptime/status'
  )
  return res.data
}

// ----------------------------------------------------------------------------
// Usage / cost report
// ----------------------------------------------------------------------------

export type UsageReportGroupBy = 'model' | 'channel' | 'day'

export interface UsageReportRow {
  key: string
  requests: number
  prompt_tokens: number
  completion_tokens: number
  total_tokens: number
  quota: number
  request_bytes: number
  response_bytes: number
  total_bytes: number
}

export interface UsageReportData {
  group_by: UsageReportGroupBy
  start: number
  end: number
  rows: UsageReportRow[]
  totals: UsageReportRow
  totals_mb: number
  row_count: number
}

export async function getUsageReport(params: {
  group_by: UsageReportGroupBy
  start?: number
  end?: number
}) {
  const res = await api.get<{
    success: boolean
    data?: UsageReportData
    message?: string
  }>('/api/log/report', { params })
  return res.data
}

// exportUsageReportCsv fetches the CSV as a blob (authenticated) so the caller
// can trigger a download without exposing the bearer token in a URL.
export async function exportUsageReportCsv(params: {
  group_by: UsageReportGroupBy
  start?: number
  end?: number
}): Promise<Blob> {
  const res = await api.get('/api/log/report/export', {
    params,
    responseType: 'blob',
  })
  return res.data as Blob
}
