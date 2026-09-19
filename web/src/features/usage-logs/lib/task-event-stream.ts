/**
 * B4-1 任务事件流（SSE）解析与摘要工具。
 * 独立文件以便纯函数单测（组件文件遵守 only-export-components 约束）。
 */

/** 后端 /api/task/:task_id/events 推送的事件信封。 */
export interface TaskEventEnvelope {
  seq: number
  ts: number
  type: string
  payload: Record<string, unknown>
}

/** 解析单个 SSE 块（按空行分隔）的 id/event/data 行；心跳注释返回 null。 */
export function parseSseChunk(
  chunk: string
): { id?: number; event?: string; data?: string } | null {
  const lines = chunk.split('\n')
  let id: number | undefined
  let event: string | undefined
  let data: string | undefined
  for (const line of lines) {
    if (line.startsWith(':')) continue // comment/heartbeat
    if (line.startsWith('id:')) {
      const v = Number(line.slice(3).trim())
      if (Number.isFinite(v)) id = v
    } else if (line.startsWith('event:')) {
      event = line.slice(6).trim()
    } else if (line.startsWith('data:')) {
      data = line.slice(5).trim()
    }
  }
  if (!id && !event && !data) return null
  return { id, event, data }
}

/** 从事件 payload 提取关键字段摘要；无字段时降级为截断 JSON。 */
export function summarizePayload(payload: Record<string, unknown>): string {
  if (!payload) return ''
  const parts: string[] = []
  const pick = (key: string) =>
    typeof payload[key] === 'string' || typeof payload[key] === 'number'
      ? String(payload[key])
      : undefined
  for (const key of [
    'status',
    'progress',
    'reason',
    'quota',
    'step',
    'platform',
    'remote_task_id_hint',
  ]) {
    const v = pick(key)
    if (v !== undefined && v !== '') parts.push(`${key}=${v}`)
  }
  if (parts.length > 0) return parts.join(' ')
  const raw = JSON.stringify(payload)
  return raw.length > 120 ? `${raw.slice(0, 120)}…` : raw
}