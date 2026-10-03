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
export type SystemInstanceStatus = 'online' | 'stale'

export type SystemInstanceInfo = {
  schema_version?: number
  node?: {
    name?: string
    source?: string
    manually_configured?: boolean
    should_configure_manually?: boolean
    [key: string]: unknown
  }
  role?: {
    is_master?: boolean
    [key: string]: unknown
  }
  runtime?: {
    version?: string
    goos?: string
    goarch?: string
    started_at?: number
    [key: string]: unknown
  }
  host?: {
    hostname?: string
    [key: string]: unknown
  }
  resources?: {
    cpu?: {
      usage_percent?: number
      [key: string]: unknown
    }
    memory?: {
      usage_percent?: number
      [key: string]: unknown
    }
    storage?: {
      total_bytes?: number
      used_bytes?: number
      free_bytes?: number
      used_percent?: number
      [key: string]: unknown
    }
    [key: string]: unknown
  }
  [key: string]: unknown
}

export type SystemInstance = {
  node_name: string
  status: SystemInstanceStatus
  stale_after_seconds: number
  started_at: number
  last_seen_at: number
  info?: SystemInstanceInfo
}

export type SystemInstanceListResponse = {
  success: boolean
  message: string
  data?: SystemInstance[]
}

export type SystemInstanceDeleteResponse = {
  success: boolean
  message: string
  data?: {
    deleted_count: number
  }
}

export type LiveRequestPhase =
  | 'received'
  | 'upstream'
  | 'streaming'
  | 'done'
  | 'error'

export type LiveRequestEntry = {
  request_id: string
  user_id: number
  user_name: string
  model: string
  group: string
  channel_id: number
  channel_name: string
  is_stream: boolean
  phase: LiveRequestPhase
  started_at: number
  elapsed_ms: number
  retry_index: number
  original_bytes: number
  compressed_bytes: number
  compressed: boolean
  first_response_ms: number
  upstream_connect_ms: number
  upstream_upload_ms: number
  upstream_ttfb_ms: number
  status_code?: number
  error_msg?: string
  finished_at?: number
}

export type LiveConcurrencyStats = {
  enabled: boolean
  active: number
  waiting: number
  limit: number
}

export type LiveRequestsData = {
  active: LiveRequestEntry[]
  finished: LiveRequestEntry[]
  active_count: number
  compressed_count: number
  original_bytes_sum: number
  compressed_bytes_sum: number
  avg_compression_ratio: number
  avg_first_response_ms: number
  avg_upload_ms: number
  avg_upstream_ttfb_ms: number
  network_in_mbps: number
  network_out_mbps: number
  concurrency: LiveConcurrencyStats
  compression_enabled: boolean
  compression_threshold_kb: number
  // 出站压缩累积统计（进程内，自进程启动累计）。
  compression_total_count: number
  compression_total_original_bytes: number
  compression_total_compressed_bytes: number
  compression_total_saved_bytes: number
  // 4.2.4 中继 gopool worker 可观测。
  relay_workers: number
  relay_workers_max: number
}

export type LiveRequestsResponse = {
  success: boolean
  message: string
  data?: LiveRequestsData
}
