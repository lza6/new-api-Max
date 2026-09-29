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
  ConfirmPaymentComplianceResponse,
  FetchUpstreamRatiosRequest,
  LogCleanupTask,
  SystemOptionsResponse,
  SystemTaskListResponse,
  SystemTaskResponse,
  UpdateOptionRequest,
  UpdateOptionResponse,
  UpstreamChannelsResponse,
  UpstreamRatiosResponse,
} from './types'

export async function getSystemOptions() {
  const res = await api.get<SystemOptionsResponse>('/api/option/')
  return res.data
}

export async function updateSystemOption(request: UpdateOptionRequest) {
  const res = await api.put<UpdateOptionResponse>('/api/option/', request)
  return res.data
}

export async function confirmPaymentCompliance() {
  const res = await api.post<ConfirmPaymentComplianceResponse>(
    '/api/option/payment_compliance',
    { confirmed: true }
  )
  return res.data
}

export async function startLogCleanupTask(targetTimestamp: number) {
  const res = await api.post<SystemTaskResponse<LogCleanupTask>>(
    '/api/system-task/log-cleanup',
    null,
    {
      params: { target_timestamp: targetTimestamp },
    }
  )
  return res.data
}

export async function getCurrentLogCleanupTask() {
  const res = await api.get<SystemTaskResponse<LogCleanupTask | null>>(
    '/api/system-task/current',
    {
      params: { type: 'log_cleanup' },
    }
  )
  return res.data
}

export async function getSystemTask(taskId: string) {
  const res = await api.get<SystemTaskResponse<LogCleanupTask>>(
    `/api/system-task/${taskId}`
  )
  return res.data
}

export async function listSystemTasks(limit = 20) {
  const res = await api.get<SystemTaskListResponse>('/api/system-task/list', {
    params: { limit },
  })
  return res.data
}

export async function resetModelRatios() {
  const res = await api.post<UpdateOptionResponse>(
    '/api/option/rest_model_ratio'
  )
  return res.data
}

export async function getUpstreamChannels() {
  const res = await api.get<UpstreamChannelsResponse>(
    '/api/ratio_sync/channels'
  )
  return res.data
}

export async function fetchUpstreamRatios(request: FetchUpstreamRatiosRequest) {
  const res = await api.post<UpstreamRatiosResponse>(
    '/api/ratio_sync/fetch',
    request
  )
  return res.data
}

// ============================================================================
// 数据库导出 / 导入（灾备）
// ============================================================================

/** 备份预览：将包含哪些表、各表行数。 */
export async function getDatabaseBackupInfo() {
  const res = await api.get<{
    success: boolean
    data?: { tables: string[]; counts: Record<string, number>; total: number; hint?: string }
  }>('/api/system/db/export/info')
  return res.data
}

/**
 * 下载数据库备份（gzip 压缩的 JSON Lines）。
 * 用 axios 的 blob 响应类型触发浏览器下载，不经过内存字符串拼接。
 */
export async function downloadDatabaseBackup(includeLogs: boolean) {
  const res = await api.get('/api/system/db/export', {
    params: includeLogs ? { include_logs: 'true' } : undefined,
    responseType: 'blob',
    // 备份可能较大，给足超时（默认 30s 会截断大库导出）。
    timeout: 30 * 60 * 1000,
  })
  return res as unknown as { data: Blob; headers: Record<string, string> }
}

/** 上传备份文件并导入（只插入缺失行，不删除既有数据）。 */
export async function importDatabaseBackup(file: File) {
  const form = new FormData()
  form.append('file', file)
  const res = await api.post<{
    success: boolean
    message?: string
    data?: {
      inserted: Record<string, number>
      skipped: Record<string, number>
      total: number
      errors?: string[]
    }
  }>('/api/system/db/import', form, {
    headers: { 'Content-Type': 'multipart/form-data' },
    timeout: 30 * 60 * 1000,
  })
  return res.data
}
