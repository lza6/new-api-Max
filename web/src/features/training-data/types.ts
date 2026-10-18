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
  /**
   * 上传失败后**成功回队重试**的次数。
   *
   * 与 `total_upload_errors` 一起看：失败数上升但 retried 也上升 = 偶发抖动自愈；
   * 失败数持续上升而 retried 不涨 = 队列/水位/归属判定有持续故障。
   */
  retried: number
  skipped_no_consent: number
  /**
   * 因**取不到任何文本**而跳过的次数。
   *
   * 这个数字持续增长通常不是「用户没发文本」，而是 `CountToken` 被关闭
   * （关闭后上游不产出 `CombineText`）—— 即「功能开着但一个人都采不到」。
   * 后端已把它接入 /metrics（`training_capture_skipped_no_text_total`），
   * 管理端也要能看到，否则这类配置型静默失效只能靠翻监控发现。
   */
  skipped_no_text: number
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

/**
 * 某一天的采集规模（由云盘目录树聚合，对应 `DailyCaptureStat`）。
 *
 * 与 `CaptureStats` 的区别（两者回答不同问题，不要混用）：
 *  - `CaptureStats`：**进程内**计数，实时且含未上传成功的量，但重启归零、只有今天。
 *  - `DailyCaptureStat`：**云盘落盘**聚合，可回溯任意历史日期，但只反映「已上传成功」的量。
 */
export interface DailyCaptureStat {
  /** 规范化日期 YYYY-MM-DD（与一级目录名一致）。 */
  date: string
  /** 该日期下有采集数据的用户数。 */
  users: number
  /** 该日期下的「用户-模型」组合数。 */
  models: number
  /** 分片文件数。 */
  parts: number
  /** 分片总字节数（**压缩后**，即云盘实际占用）。 */
  bytes: number
}

/** 按日期查询采集量的响应。 */
export interface TrainingDataByDateResponse {
  /** 进程内计数所属日期（UTC）。 */
  today: string
  /** 进程内计数；仅当查询的是今天时有值，历史日期为 null。 */
  stats: CaptureStats | null
  /** 云盘聚合出的每日数据（查询历史日期时为那一天，可能为空数组）。 */
  days: DailyCaptureStat[]
  /** 请求的日期（未指定时不返回）。 */
  date?: string
  /** 进程内计数是否可用（历史日期为 false）。 */
  known?: boolean
  /** `known=false` 时的原因说明。 */
  reason?: string
  /** 云盘列举被截断时 true —— 此时 `days` 是**下界**。 */
  daily_truncated?: boolean
  /** 云盘读取失败时的错误（不影响 `stats`）。 */
  daily_error?: string
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

/**
 * 对话分析接口的响应。**三种形态共用一个 HTTP 200**，只能按字段区分：
 *  - 成功：上游原始 chat completion（`choices` 在顶层，**无 `success`**）
 *    —— 该接口把请求交给 relay 栈，relay 直接写下上游响应。
 *  - 参数/读取错误：本项目的信封 `{success:false, message}`
 *  - relay 内部失败（无可用渠道/上游报错）：`{error:{message}}`
 *
 * 判据必须写 `success === false`，**不能**写 `!success` ——
 * 后者在成功时因 `undefined` 判真，会让成功路径必然抛错。
 */
export interface AnalysisResponse {
  // 分析走 relay 栈，返回体是标准的 chat completion，
  // 这里不逐字段建模（不同渠道可能有差异），由调用方按需读取。
  choices?: {
    message?: { role?: string; content?: string }
    delta?: { content?: string }
  }[]
  // 本项目信封（错误路径）。
  success?: boolean
  message?: string
  // relay 内部失败。
  error?: { message?: string }
}

// ---------- 一键导出 / 下载（增量需求） ----------

/** 导出状态。 */
export type ExportStatus = 'pending' | 'building' | 'ready' | 'failed'

/** 一个导出任务（对应 model.TrainingDataExport）。 */
export interface TrainingDataExport {
  id: number
  status: ExportStatus
  date_from: string
  date_to: string
  user_id: number
  model: string
  archive_format: string
  /** 产物的相对路径（未就绪时为空）。 */
  object_path: string
  size_bytes: number
  part_count: number
  record_count: number
  /** 失败原因（status 为 failed 时非空）。 */
  error_msg: string
  created_by: number
  /** 已完成的下载次数（明细以 downloads 为准）。 */
  download_count: number
  created_at: string
  updated_at: string
}

/** 一次下载记录（对应 model.TrainingDataExportDownload）。 */
export interface TrainingDataExportDownload {
  id: number
  export_id: number
  downloader_id: number
  /** browser / direct。 */
  source: string
  bytes_sent: number
  range_header: string
  completed: boolean
  started_at: number
  completed_at: number
  created_at: string
  updated_at: string
}

export interface ExportListResponse {
  exports: TrainingDataExport[]
  total: number
  offset: number
  limit: number
}

export interface ExportDetailResponse {
  export: TrainingDataExport
  downloads: TrainingDataExportDownload[]
}

/** 直链签发结果。 */
export interface ExportLinkResponse {
  url: string
  expires_at: number
  export_id: number
  /** 签发该链接的管理员 ID（下载记录会归到他名下）。 */
  issuer_id: number
  size_bytes: number
  filename: string
  /** 是否支持断点续传（服务端声明）。 */
  resumable: boolean
  accept_ranges: string
}
