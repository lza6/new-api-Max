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
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Label } from '@/components/ui/label'
import { getFreshAuthHeaders } from '@/lib/auth-session'
import { formatTimestampToDate } from '@/lib/format'
import { cn } from '@/lib/utils'

import {
  parseSseChunk,
  summarizePayload,
  type TaskEventEnvelope,
} from '../lib/task-event-stream'

type StreamPhase = 'connecting' | 'streaming' | 'done' | 'error'

const MAX_RETRY = 3
const RECONNECT_DELAY_MS = 2000

interface TaskEventStreamProps {
  taskId: string
  className?: string
}

/**
 * 任务事件流实时区块：fetch + ReadableStream 消费 SSE（EventSource 无法携带
 * Authorization header），断线自动按 since=lastSeq 续传（no gap/dup）。
 * 收到 done 事件或终态后结束流。
 */
export function TaskEventStream({ taskId, className }: TaskEventStreamProps) {
  const { t } = useTranslation()
  const [events, setEvents] = useState<TaskEventEnvelope[]>([])
  const [phase, setPhase] = useState<StreamPhase>('connecting')

  useEffect(() => {
    let cancelled = false
    const controller = new AbortController()
    let retries = 0

    const startStream = async (since: number) => {
      if (cancelled) return
      setPhase('connecting')
      try {
        const headers = await getFreshAuthHeaders()
        const query = since > 0 ? `?since=${since}` : ''
        const res = await fetch(
          `/api/task/${encodeURIComponent(taskId)}/events${query}`,
          { headers, signal: controller.signal }
        )
        if (!res.ok || !res.body) {
          throw new Error(`HTTP ${res.status}`)
        }
        setPhase('streaming')
        const reader = res.body.getReader()
        const decoder = new TextDecoder()
        let buffer = ''
        let lastSeq = since
        while (true) {
          const { done, value } = await reader.read()
          if (done) break
          buffer += decoder.decode(value, { stream: true })
          let splitAt = buffer.indexOf('\n\n')
          while (splitAt >= 0) {
            const chunk = buffer.slice(0, splitAt)
            buffer = buffer.slice(splitAt + 2)
            const parsed = parseSseChunk(chunk)
            if (parsed) {
              if (parsed.event === 'done') {
                setPhase('done')
                return
              }
              if (parsed.event === 'ping') {
                splitAt = buffer.indexOf('\n\n')
                continue
              }
              if (parsed.id) lastSeq = Math.max(lastSeq, parsed.id)
              if (parsed.data) {
                try {
                  const raw = JSON.parse(parsed.data) as TaskEventEnvelope
                  if (
                    raw &&
                    typeof raw === 'object' &&
                    typeof raw.seq === 'number' &&
                    typeof raw.type === 'string' &&
                    'payload' in raw
                  ) {
                    setEvents((prev) => [...prev, raw])
                  }
                } catch {
                  // 忽略无法解析的事件负载
                }
              }
            }
            splitAt = buffer.indexOf('\n\n')
          }
        }
        // 服务端关闭流（无 done）→ 按 lastSeq 续传
        if (!cancelled) {
          retries += 1
          if (retries <= MAX_RETRY) {
            setTimeout(() => {
              void startStream(lastSeq)
            }, RECONNECT_DELAY_MS)
          } else {
            setPhase('error')
          }
        }
      } catch (err) {
        if (cancelled) return
        if ((err as Error).name === 'AbortError') return
        retries += 1
        if (retries <= MAX_RETRY) {
          setTimeout(() => {
            void startStream(since)
          }, RECONNECT_DELAY_MS)
        } else {
          setPhase('error')
        }
      }
    }

    void startStream(0)
    return () => {
      cancelled = true
      controller.abort()
    }
  }, [taskId])

  function resolvePhaseLabel(target: StreamPhase): string {
    switch (target) {
      case 'done':
        return t('Stream ended')
      case 'error':
        return t('Stream error')
      case 'connecting':
        return t('Connecting')
      default:
        return t('Live')
    }
  }

  function phaseBadgeClass(target: StreamPhase): string {
    switch (target) {
      case 'done':
        return 'bg-muted text-muted-foreground'
      case 'error':
        return 'bg-destructive/10 text-destructive'
      case 'connecting':
        return 'bg-warning/10 text-warning'
      default:
        return 'bg-primary/10 text-primary'
    }
  }

  return (
    <section className={cn('min-w-0 space-y-1.5', className)}>
      <Label className='flex items-center gap-1.5 text-xs font-semibold'>
        {t('Task Event Stream')}
        <span
          className={cn(
            'rounded-full px-1.5 py-0.5 text-[10px] font-medium',
            phaseBadgeClass(phase)
          )}
        >
          {resolvePhaseLabel(phase)}
        </span>
      </Label>
      <div className='bg-muted/30 max-h-56 min-w-0 space-y-1 overflow-y-auto rounded-md border p-2.5'>
        {events.length === 0 ? (
          <div className='text-muted-foreground py-2 text-center text-xs'>
            {phase === 'error'
              ? t('Unable to connect to event stream')
              : t('Waiting for task events')}
          </div>
        ) : (
          events.map((ev) => (
            <div
              key={ev.seq}
              className='flex min-w-0 items-start gap-2 text-xs'
            >
              <span className='text-muted-foreground shrink-0 font-mono tabular-nums'>
                {formatTimestampToDate(ev.ts, 'seconds')}
              </span>
              <span className='bg-muted font-medium shrink-0 rounded px-1 py-0.5'>
                {t(`Event.${ev.type}`)}
              </span>
              <span className='text-muted-foreground min-w-0 flex-1 break-all'>
                {summarizePayload(ev.payload)}
              </span>
            </div>
          ))
        )}
      </div>
    </section>
  )
}