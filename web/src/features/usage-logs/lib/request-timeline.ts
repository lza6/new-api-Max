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

// B2-2 请求级 trace 时间线（黑匣子打开）。
// 零额外计算：阶段全部由 consume log 已有字段推导——
//   - 入站/鉴权：created_at - use_time（请求开始），鉴权在入站后立即完成
//   - 渠道选择：admin_info.use_channel（重试链）/ channel_affinity（健康分依据）
//   - 上游调用/首包：other.frt（首包耗时 ms）
//   - 完成/失败：log.use_time（总耗时 s）+ stream_status（end_reason/end_error）
// 老日志缺字段时逐阶段优雅降级（不报错，只展示可推导的节点）。

export type TimelinePhaseStatus = 'done' | 'failed' | 'skipped' | 'info'

export interface TimelinePhase {
  /** 稳定阶段标识（i18n 键后缀用），如 'inbound' | 'auth' | 'channel' | 'upstream' | 'first_token' | 'complete' */
  key: string
  /** 阶段状态 */
  status: TimelinePhaseStatus
  /** 相对请求开始的偏移毫秒（0 = 入站） */
  offsetMs: number
  /** 该阶段自身的耗时毫秒（与前一段的差） */
  durationMs?: number
  /** 补充说明（渠道链、错误原因等），可空 */
  detail?: string
  /** 补充说明的原始值（如 end_error），用于 JSON 导出 */
  detailRaw?: string
}

export interface RequestTimeline {
  /** 是否成功（stream_status.done / 无失败标记） */
  ok: boolean
  /** 失败原因（失败时） */
  failReason?: string
  /** 阶段列表（至少入站；老日志缺字段时只保留可推导节点） */
  phases: TimelinePhase[]
}

export interface TimelineSource {
  created_at: number
  use_time: number
  frt?: number | null
  use_channel?: number[]
  channel_affinity?: {
    rule_name?: string
    selected_group?: string
    using_group?: string
    key_hint?: string
  }
  stream_status?: {
    status?: string
    end_reason?: string
    end_error?: string
    error_count?: number
  }
  request_path?: string
  /** T5-2：后端结构化的时间线 stage（name/elapsed_ms/status）；存在时优先消费。 */
  timeline_stages?: Array<{
    name: string
    elapsed_ms: number
    status?: string
  }>
}

/** 从 consume log 构造请求时间线。老日志缺字段时优雅降级。 */
/** 把毫秒换算成人类可读时长：350ms / 1.5s / 2m 15s / 1h。 */
export function formatDurationMs(ms: number): string {
  if (!Number.isFinite(ms) || ms < 0) return '0ms'
  if (ms < 1000) return Math.round(ms) + 'ms'
  const totalSec = ms / 1000
  if (totalSec < 60) {
    const s = Math.round(totalSec * 10) / 10
    return s + 's'
  }
  const m = Math.floor(totalSec / 60)
  const s = Math.round(totalSec % 60)
  if (m < 60) {
    return s > 0 ? m + 'm ' + s + 's' : m + 'm'
  }
  const h = Math.floor(m / 60)
  const remM = m % 60
  return remM > 0 ? h + 'h ' + remM + 'm' : h + 'h'
}

export function buildRequestTimeline(src: TimelineSource): RequestTimeline {
  // T5-2：后端已提供结构化 stage 时直接消费（真实耗时），不再纯推测。
  // 旧日志无 timeline_stages 时走下方原有推导逻辑（向后兼容）。
  if (Array.isArray(src.timeline_stages) && src.timeline_stages.length > 0) {
    const phases: TimelinePhase[] = []
    for (const stage of src.timeline_stages) {
      const offsetMs = Math.max(0, Math.round(stage.elapsed_ms || 0))
      const status: TimelinePhase['status'] =
        stage.status === 'failed'
          ? 'failed'
          : stage.status === 'info'
            ? 'info'
            : stage.status === 'skipped'
              ? 'skipped'
              : 'done'
      const key = stageMnemonic(stage.name)
      phases.push({ key, status, offsetMs, durationMs: offsetMs })
    }
    const stream = src.stream_status
    const failed =
      !!stream &&
      (stream.status === 'error' ||
        stream.status === 'failed' ||
        !!stream.end_error ||
        (typeof stream.error_count === 'number' && stream.error_count > 0))
    const failReason = stream?.end_error || stream?.end_reason || undefined
    if (phases.length === 0) {
      phases.push({ key: 'inbound', status: 'done', offsetMs: 0 })
    }
    return { ok: !failed, failReason, phases }
  }

  const totalMs = Math.max(0, Math.round((src.use_time || 0) * 1000))
  const frtMs = src.frt != null && src.frt > 0 ? Math.round(src.frt) : undefined

  const stream = src.stream_status
  const failed =
    !!stream &&
    (stream.status === 'error' ||
      stream.status === 'failed' ||
      !!stream.end_error ||
      (typeof stream.error_count === 'number' && stream.error_count > 0))
  const failReason = stream?.end_error || stream?.end_reason || undefined

  const phases: TimelinePhase[] = [
    {
      key: 'inbound',
      status: 'done',
      offsetMs: 0,
      detail: src.request_path,
      detailRaw: src.request_path,
    },
    {
      key: 'auth',
      status: 'done',
      offsetMs: 0,
    },
  ]

  // 渠道选择（admin 重试链 / 健康分依据）。
  const chain = src.use_channel
  const affinity = src.channel_affinity
  if (chain && chain.length > 0) {
    phases.push({
      key: 'channel',
      status: 'done',
      offsetMs: 0,
      detail: chain.join(' -> '),
      detailRaw: chain.join(' -> '),
    })
  } else if (affinity?.rule_name || affinity?.using_group) {
    phases.push({
      key: 'channel',
      status: 'info',
      offsetMs: 0,
      detail: [affinity.rule_name, affinity.using_group]
        .filter(Boolean)
        .join(' · '),
      detailRaw: JSON.stringify(affinity),
    })
  }

  // 上游调用 + 首包。
  let upstreamStatus: TimelinePhase['status'] = 'skipped'
  if (frtMs != null) {
    upstreamStatus = 'done'
  } else if (totalMs > 0) {
    upstreamStatus = failed ? 'failed' : 'done'
  }
  phases.push({
    key: 'upstream',
    status: upstreamStatus,
    offsetMs: 0,
    durationMs: frtMs == null && totalMs > 0 ? totalMs : undefined,
  })
  if (frtMs != null) {
    phases.push({
      key: 'first_token',
      status: 'done',
      offsetMs: frtMs,
      durationMs: frtMs,
    })
  }

  // 完成/失败。
  phases.push({
    key: 'complete',
    status: failed ? 'failed' : 'done',
    offsetMs: totalMs,
    durationMs: frtMs != null ? Math.max(0, totalMs - frtMs) : 0,
    detail: failReason,
    detailRaw: failReason,
  })

  return {
    ok: !failed,
    failReason,
    phases,
  }
}

/** 导出 JSON（类 hermes-trace receipts）：稳定结构，供审计粘贴。 */
export function exportTimelineJson(
  src: TimelineSource,
  timeline: RequestTimeline
): string {
  return JSON.stringify(
    {
      schema: 'new-api.request-timeline.v1',
      ok: timeline.ok,
      fail_reason: timeline.failReason ?? null,
      phases: timeline.phases.map((p) => ({
        phase: p.key,
        status: p.status,
        offset_ms: p.offsetMs,
        duration_ms: p.durationMs ?? null,
        detail: p.detailRaw ?? null,
      })),
      source: {
        created_at: src.created_at,
        use_time_ms: Math.round((src.use_time || 0) * 1000),
        frt_ms: src.frt != null && src.frt > 0 ? Math.round(src.frt) : null,
        request_path: src.request_path ?? null,
      },
    },
    null,
    2
  )
}
//PROBE

/** stageMnemonic T5-2：后端 stage 名 → 前端稳定 phase key（i18n 后缀兼容）。 */
function stageMnemonic(name: string): string {
  switch (name) {
    case 'inbound':
      return 'inbound'
    case 'auth':
      return 'auth'
    case 'channel':
      return 'channel'
    case 'upstream':
      return 'upstream'
    case 'upstream_first_byte':
      return 'first_token'
    case 'complete':
      return 'complete'
    case 'total_to_first_response':
      return 'first_token'
    default:
      return name.replace(/[^a-z0-9_]/gi, '_').toLowerCase() || 'inbound'
  }
}
