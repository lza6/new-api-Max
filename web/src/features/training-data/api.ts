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
  ApiEnvelope,
  CaptureOverview,
  FileListResponse,
  FilePreviewResponse,
  AnalysisResponse,
  ExportListResponse,
  ExportDetailResponse,
  TrainingDataExport,
  ExportLinkResponse,
} from './types'

// 训练数据采集管理端 API（Spec-008）。
//
// 全部接口挂在 AdminAuth() 之下（见 router/api-router.go），
// 因此这里不需要额外传管理员身份 —— 越权由服务端拒绝。

/** 总览：总开关状态 + 今日统计 + 云盘配额。 */
export async function getTrainingDataOverview(): Promise<
  ApiEnvelope<CaptureOverview>
> {
  const res = await api.get('/api/admin/training-data/overview')
  return res.data
}

/** 列出云盘目录树（可指定子目录与条目上限）。 */
export async function listTrainingDataFiles(
  path: string,
  max?: number
): Promise<ApiEnvelope<FileListResponse>> {
  const res = await api.get('/api/admin/training-data/files', {
    params: { path, max },
  })
  return res.data
}

/** 预览一个分片（服务端自动解压）。 */
export async function previewTrainingDataFile(
  path: string
): Promise<ApiEnvelope<FilePreviewResponse>> {
  const res = await api.get('/api/admin/training-data/file', {
    params: { path },
  })
  return res.data
}

/**
 * 删除一个分片。
 *
 * `confirm=1` 是**服务端强制**的（缺少即拒绝），不是前端装饰 ——
 * 传它是为了让「误调用」无法静默删数据。
 */
export async function deleteTrainingDataFile(
  path: string
): Promise<ApiEnvelope<{ deleted: string }>> {
  const res = await api.delete('/api/admin/training-data/file', {
    params: { path, confirm: 1 },
  })
  return res.data
}

/**
 * 对话深度分析 / 继续追问。
 *
 * ⚠️ 这是**真实调用模型**的接口：会消耗额度（记在发起分析的管理员账上）。
 * 前端必须在按钮上明确提示这一点。
 */
export async function analyzeTrainingDataConversation(payload: {
  path: string
  model?: string
  messages?: { role: string; content: string }[]
}): Promise<ApiEnvelope<AnalysisResponse>> {
  const res = await api.post('/api/admin/training-data/analyze', payload)
  return res.data
}

/** 切换总开关。 */
export async function setTrainingDataEnabled(enabled: boolean): Promise<
  ApiEnvelope<{ enabled: boolean; can_enable: boolean; reason?: string }>
> {
  const res = await api.put('/api/admin/training-data/enabled', { enabled })
  return res.data
}

// ---------- 一键导出 / 下载（增量需求） ----------

/** 创建一个导出任务（同步打包，返回就绪的导出记录）。 */
export async function createTrainingDataExport(payload: {
  date_from: string
  date_to: string
  user_id?: number
  model?: string
}): Promise<ApiEnvelope<TrainingDataExport>> {
  const res = await api.post('/api/admin/training-data/export', payload)
  return res.data
}

/** 分页列出导出任务。 */
export async function listTrainingDataExports(
  offset = 0,
  limit = 50
): Promise<ApiEnvelope<ExportListResponse>> {
  const res = await api.get('/api/admin/training-data/export', {
    params: { offset, limit },
  })
  return res.data
}

/** 读取单个导出 + 其下载记录。 */
export async function getTrainingDataExportDetail(
  id: number
): Promise<ApiEnvelope<ExportDetailResponse>> {
  const res = await api.get('/api/admin/training-data/export/detail', {
    params: { id },
  })
  return res.data
}

/**
 * 为导出签发一个限时直链（可供 curl / aria2 / wget 使用，支持断点续传）。
 *
 * ⚠️ 直链 token 绑定导出 ID 且**限时**；过期后需重新签发。
 */
export async function issueTrainingDataExportLink(
  id: number,
  ttlSeconds?: number
): Promise<ApiEnvelope<ExportLinkResponse>> {
  const res = await api.post('/api/admin/training-data/export/link', {
    id,
    ttl_seconds: ttlSeconds ?? 0,
  })
  return res.data
}

/** 删除导出任务及其产物（`confirm=1` 由服务端强制）。 */
export async function deleteTrainingDataExport(
  id: number
): Promise<ApiEnvelope<{ deleted: number }>> {
  const res = await api.delete('/api/admin/training-data/export', {
    params: { id, confirm: 1 },
  })
  return res.data
}
