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
import { api, type ApiRequestConfig } from '@/lib/api'

import { buildQueryParams } from './lib/query-params'
import { parseTaskArtifactsResponse } from './lib/task-artifacts'
import type {
  GetLogsParams,
  GetLogsResponse,
  GetLogStatsParams,
  GetLogStatsResponse,
  GetMidjourneyLogsParams,
  GetSubscriptionLogsResponse,
  GetTaskLogsParams,
  TaskArtifactsResponse,
  UserInfo,
} from './types'

export interface LogsTrafficDaily {
	date: string
	requests: number
	bytes: number
	mb: number
}

export interface LogsTrafficData {
	days: number
	total_requests: number
	total_bytes: number
	total_mb: number
	by_day: LogsTrafficDaily[]
}

export async function getLogsTraffic(days: number): Promise<{
	success: boolean
	data?: LogsTrafficData
}> {
	const res = await api.get<{ success: boolean; data?: LogsTrafficData }>(
		'/api/log/traffic',
		{ params: { days } }
	)
	return res.data
}

export interface BandwidthLeaderboardRow {
	/** 每日排行：日期 YYYY-MM-DD */
	date?: string
	/** 模型排行：模型名 */
	model?: string
	requests: number
	bytes: number
	bytes_text: string
}

export interface BandwidthLeaderboardData {
	days: number
	limit: number
	leaderboard: BandwidthLeaderboardRow[]
}

export async function getBandwidthLeaderboard(
	days = 30,
	limit = 10
): Promise<{ success: boolean; data?: BandwidthLeaderboardData }> {
	const res = await api.get<{ success: boolean; data?: BandwidthLeaderboardData }>(
		'/api/log/bandwidth/leaderboard',
		{ params: { days, limit } }
	)
	return res.data
}

export async function getModelBandwidthLeaderboard(
	days = 30,
	limit = 10
): Promise<{ success: boolean; data?: BandwidthLeaderboardData }> {
	const res = await api.get<{ success: boolean; data?: BandwidthLeaderboardData }>(
		'/api/log/bandwidth/model-leaderboard',
		{ params: { days, limit } }
	)
	return res.data
}

// ============================================================================
// Generic API Helpers
// ============================================================================

function buildApiPath(endpoint: string, isAdmin: boolean): string {
  return isAdmin ? endpoint : `${endpoint}/self`
}

async function fetchLogs<T>(
  endpoint: string,
  params: T,
  isAdmin: boolean
): Promise<GetLogsResponse> {
  const paramRecord = params as unknown as Record<string, unknown>
  const queryParams = buildQueryParams({
    p: paramRecord.p || 1,
    page_size: paramRecord.page_size || 20,
    ...params,
  })
  const path = buildApiPath(endpoint, isAdmin)
  const res = await api.get(`${path}?${queryParams}`)
  return res.data
}

async function fetchLogStats<T>(
  endpoint: string,
  params: T,
  isAdmin: boolean
): Promise<GetLogStatsResponse> {
  const queryParams = buildQueryParams(
    params as unknown as Record<string, unknown>
  )
  const path = buildApiPath(endpoint, isAdmin)
  const res = await api.get(`${path}/stat?${queryParams}`)
  return res.data
}

// ============================================================================
// Common Log APIs
// ============================================================================

export const getAllLogs = (params: GetLogsParams = {}) =>
  fetchLogs('/api/log', params, true)

export const getUserLogs = (
  params: Omit<GetLogsParams, 'username' | 'channel'> = {}
) => fetchLogs('/api/log', params, false)

export const getLogStats = (params: GetLogStatsParams = {}) =>
  fetchLogStats('/api/log', params, true)

export const getUserLogStats = (
  params: Omit<GetLogStatsParams, 'username' | 'channel'> = {}
) => fetchLogStats('/api/log', params, false)

export async function getUserInfo(
  userId: number
): Promise<{ success: boolean; message?: string; data?: UserInfo }> {
  const res = await api.get(`/api/user/${userId}`)
  return res.data
}

// ============================================================================
// MjProxy (Drawing) Logs API
// ============================================================================

export const getAllMidjourneyLogs = (params: GetMidjourneyLogsParams) =>
  fetchLogs('/api/mj', params, true)

export const getUserMidjourneyLogs = (params: GetMidjourneyLogsParams) =>
  fetchLogs('/api/mj', params, false)

// ============================================================================
// Task Logs API
// ============================================================================

export const getAllTaskLogs = (params: GetTaskLogsParams) =>
  fetchLogs('/api/task', params, true)

export const getUserTaskLogs = (params: GetTaskLogsParams) =>
  fetchLogs('/api/task', params, false)

// ============================================================================
// Subscription Logs API (admin)
// ============================================================================

export interface GetSubscriptionLogsParams {
  p?: number
  page_size?: number
  username?: string
  status?: string
  source?: string
  start_time?: number
  end_time?: number
}

export async function getSubscriptionLogs(
  params: GetSubscriptionLogsParams
): Promise<GetSubscriptionLogsResponse> {
  const queryParams = buildQueryParams({
    p: params.p || 1,
    page_size: params.page_size || 20,
    ...params,
  })
  const res = await api.get(`/api/subscription/admin/logs?${queryParams}`)
  return res.data
}

const taskArtifactRequestConfig = {
  skipBusinessError: true,
  skipErrorHandler: true,
} satisfies ApiRequestConfig

export async function getTaskArtifacts(taskId: string) {
  const response = await api.get<TaskArtifactsResponse>(
    `/api/task/${encodeURIComponent(taskId)}/artifacts`,
    taskArtifactRequestConfig
  )
  return parseTaskArtifactsResponse(response.data)
}

export interface LogCostDetail {
  log_id: number
  model_name: string
  quota: number
  prompt_tokens: number
  completion_tokens: number
  model_ratio: number
  group_ratio: number
  completion_ratio: number
  cache_ratio: number
  tier_matched?: unknown
  api_equivalent_usd?: number
  shadow_known?: boolean
}

/** T4-2：用户版费用明细（B5-2 接口前端接入）。
 *  返回单条消费日志的计费分段与影子价，供日志详情「费用明细」面板渲染。 */
export async function getLogCostDetail(logId: number): Promise<LogCostDetail> {
  const res = await api.get<{ success: boolean; data: LogCostDetail }>(
    `/api/log/usage/${logId}/cost-detail`
  )
  return res.data.data
}
