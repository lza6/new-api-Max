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

/** 与服务端 common 的响应信封一致。 */
export interface ApiEnvelope<T = unknown> {
  success: boolean
  message?: string
  data?: T
}

/** 采集总开关状态（对应 training_capture.GatewayStatus）。 */
export interface GatewayStatus {
  enabled: boolean
  /** false 时 `enabled` 必然为 false（云盘未配置等），前端据此禁用开关。 */
  can_enable: boolean
  /** 不能开启的可读原因。 */
  reason?: string
  sink_name: string
}

/**
 * 今日采集统计（对应 training_capture.CaptureStatsSnapshot）。
 *
 * ⚠️ **进程内计数**：进程重启后归零。前端必须把这一点告诉管理员，
 * 否则他会以为「昨天没采到数据」。
 */
export interface CaptureStats {
  /** 统计所属的 UTC 日期（YYYY-MM-DD）。 */
  day: string
  users: number
  conversations: number
  bytes_uploaded: number
  messages_uploaded: number
  total_enqueued: number
  total_dropped: number
  total_upload_errors: number
  skipped_no_consent: number
  queue_len: number
  queue_capacity: number
  /** 当前**正在上传**的字节数（瞬时值）。 */
  in_flight_bytes: number
}

/** 云盘用量（对应 controller.SinkOverview）。 */
export interface SinkOverview {
  provider: string
  /** false 时容量字段不可信，前端应显示「未知」而不是 0。 */
  quota_known: boolean
  total_bytes: number
  used_bytes: number
  remaining_bytes: number
  folders: number
  files: number
  /** true 时 folders/files 是**下界**而非总量。 */
  truncated: boolean
  last_error?: string
}

export interface CaptureOverview {
  gateway: GatewayStatus
  stats: CaptureStats
  sink: SinkOverview
}

/** 云盘文件项。 */
export interface TrainingDataFileEntry {
  path: string
  is_dir: boolean
  size_bytes: number
  modified_at: number
}

export interface FileListResponse {
  root: string
  entries: TrainingDataFileEntry[]
  count: number
  total_bytes: number
  /** true 表示列表因达到上限而**不完整**。 */
  truncated: boolean
}

export interface FilePreviewResponse {
  path: string
  compressed: boolean
  compressed_bytes: number
  plain_bytes: number
  lines: string[]
  line_count: number
}

export interface AnalysisResponse {
  // 分析走 relay 栈，返回体是标准的 chat completion，
  // 这里不逐字段建模（不同渠道可能有差异），由调用方按需读取。
  choices?: {
    message?: { role?: string; content?: string }
    delta?: { content?: string }
  }[]
  error?: { message?: string }
}
