/**
 * 8.x 日志 JSON 导出：抽取纯函数便于单测。
 * 输入日志字段 + 请求时间线，产出可下载的结构化 JSON 对象。
 */

/** 导出所需的最小日志字段子集。 */
export interface LogExportSource {
  id?: number
  created_at?: number
  type?: number
  user_id?: number
  username?: string
  model_name?: string
  channel?: number
  channel_name?: string | null
  token_name?: string
  quota?: number
  prompt_tokens?: number
  completion_tokens?: number
  use_time?: number
  is_stream?: boolean
  group?: string
  ip?: string
  request_id?: string
  upstream_request_id?: string | null
  content?: string
  other?: unknown
}

/** 构建完整导出负载（含请求时间线，时间线字段可能为空 = 老日志优雅降级）。 */
export function buildLogExportPayload(
  log: LogExportSource,
  timelineJson: unknown
): Record<string, unknown> {
  return {
    id: log.id,
    created_at: log.created_at,
    type: log.type,
    user_id: log.user_id,
    username: log.username,
    model_name: log.model_name,
    channel: log.channel,
    channel_name: log.channel_name,
    token_name: log.token_name,
    quota: log.quota,
    prompt_tokens: log.prompt_tokens,
    completion_tokens: log.completion_tokens,
    use_time: log.use_time,
    is_stream: log.is_stream,
    group: log.group,
    ip: log.ip,
    request_id: log.request_id,
    upstream_request_id: log.upstream_request_id,
    content: log.content,
    other: log.other,
    timeline: timelineJson,
  }
}

/** 生成下载文件名（request_id 优先，回退 id，兜底 log）。 */
export function logExportFilename(
  requestId: string | undefined,
  id: number | undefined
): string {
  return `log-${requestId || id || 'log'}.json`
}
