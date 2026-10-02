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
import { useQuery } from '@tanstack/react-query'
import { Activity, ArrowDown, ArrowUp, RefreshCw } from 'lucide-react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { ErrorState } from '@/components/error-state'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'

import { getLiveRequests } from '../api'
import type {
  LiveRequestEntry,
  LiveRequestPhase,
  LiveRequestsData,
} from '../types'

const POLL_INTERVAL_MS = 2000
const SKELETON_KEYS = [
  'live-skeleton-1',
  'live-skeleton-2',
  'live-skeleton-3',
  'live-skeleton-4',
  'live-skeleton-5',
  'live-skeleton-6',
]

const PHASE_STYLE: Record<LiveRequestPhase, string> = {
  received: 'bg-slate-100 text-slate-700 dark:bg-slate-500/15 dark:text-slate-300',
  upstream: 'bg-blue-50 text-blue-700 dark:bg-blue-500/15 dark:text-blue-300',
  streaming: 'bg-cyan-50 text-cyan-700 dark:bg-cyan-500/15 dark:text-cyan-300',
  done: 'bg-emerald-50 text-emerald-700 dark:bg-emerald-500/15 dark:text-emerald-300',
  error: 'bg-red-50 text-red-700 dark:bg-red-500/15 dark:text-red-300',
}

function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) {return '0 B'}
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const index = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1)
  const value = bytes / 1024 ** index
  return `${new Intl.NumberFormat(undefined, {
    maximumFractionDigits: index === 0 ? 0 : 1,
  }).format(value)} ${units[index]}`
}

function formatDuration(ms: number): string {
  if (!Number.isFinite(ms) || ms <= 0) {return '0ms'}
  if (ms < 1000) {return `${Math.round(ms)}ms`}
  return `${(ms / 1000).toFixed(1)}s`
}

function formatMbps(value: number): string {
  if (!Number.isFinite(value)) {return '-'}
  return `${value.toFixed(2)} MB/s`
}

type StatCardProps = {
  label: string
  value: ReactNode
  hint?: ReactNode
  icon?: ReactNode
  tone?: 'default' | 'good' | 'warn'
}

const STAT_TONE_CLASS: Record<'default' | 'good' | 'warn', string> = {
  default: 'text-foreground',
  good: 'text-emerald-600 dark:text-emerald-400',
  warn: 'text-amber-600 dark:text-amber-400',
}

function StatCard(props: StatCardProps) {
  const toneClass = STAT_TONE_CLASS[props.tone ?? 'default']
  return (
    <div className='bg-card rounded-lg border p-3'>
      <div className='text-muted-foreground flex items-center gap-1.5 text-xs'>
        {props.icon}
        <span className='truncate'>{props.label}</span>
      </div>
      <div className={cn('mt-1.5 text-lg font-semibold tabular-nums', toneClass)}>
        {props.value}
      </div>
      {props.hint ? (
        <div className='text-muted-foreground mt-0.5 truncate text-[11px]'>
          {props.hint}
        </div>
      ) : null}
    </div>
  )
}

function CompressionHint(props: { entry: LiveRequestEntry }) {
  const { t } = useTranslation()
  const { entry } = props
  if (!entry.compressed) {
    // 未压缩：显示原始字节；未采集（0）时显示占位，避免误读为「0 字节请求」。
    return (
      <span className='text-muted-foreground text-xs tabular-nums'>
        {entry.original_bytes > 0 ? formatBytes(entry.original_bytes) : '—'}
      </span>
    )
  }
  const ratio =
    entry.original_bytes > 0
      ? (entry.compressed_bytes / entry.original_bytes) * 100
      : 0
  return (
    <TooltipProvider delay={100}>
      <Tooltip>
        <TooltipTrigger
          render={
            <span className='inline-flex items-center gap-1 text-xs tabular-nums'>
              <Badge
                variant='outline'
                className='border-emerald-200 bg-emerald-50 px-1 text-[10px] text-emerald-700 dark:border-emerald-500/30 dark:bg-emerald-500/15 dark:text-emerald-300'
              >
                {ratio.toFixed(0)}%
              </Badge>
              <span className='text-muted-foreground'>
                {formatBytes(entry.compressed_bytes)}
              </span>
            </span>
          }
        />
        <TooltipContent>
          {t('{{before}} → {{after}} (compressed)', {
            before: formatBytes(entry.original_bytes),
            after: formatBytes(entry.compressed_bytes),
          })}
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  )
}

function FirstTokenCell(props: { entry: LiveRequestEntry }) {
  const { t } = useTranslation()
  const { entry } = props
  if (entry.first_response_ms <= 0) {
    return <span className='text-muted-foreground tabular-nums'>—</span>
  }
  const hasSplit = entry.upstream_ttfb_ms >= 0
  if (!hasSplit) {
    return <span className='tabular-nums'>{formatDuration(entry.first_response_ms)}</span>
  }
  // 上传耗时（负值/未采集视为 0）与上游首字节耗时。
  const uploadMs = Math.max(0, entry.upstream_upload_ms)
  const ttfbMs = Math.max(0, entry.upstream_ttfb_ms)
  const total = Math.max(1, uploadMs + ttfbMs)
  const uploadPct = (uploadMs / total) * 100
  return (
    <TooltipProvider delay={100}>
      <Tooltip>
        <TooltipTrigger
          render={
            <span className='inline-flex flex-col gap-0.5 tabular-nums'>
              <span>{formatDuration(entry.first_response_ms)}</span>
              {/* 分段条：上传（amber）vs 上游（blue） */}
              <span className='bg-muted flex h-1 w-16 overflow-hidden rounded-full'>
                <span className='bg-amber-500' style={{ width: `${uploadPct}%` }} />
                <span className='bg-blue-500' style={{ width: `${100 - uploadPct}%` }} />
              </span>
            </span>
          }
        />
        <TooltipContent className='max-w-60'>
          <div className='space-y-1 text-xs'>
            <div className='flex items-center justify-between gap-3'>
              <span className='flex items-center gap-1.5'>
                <span className='size-2 rounded-full bg-amber-500' aria-hidden='true' />
                {t('Upload to upstream')}
              </span>
              <span className='font-mono'>{formatDuration(uploadMs)}</span>
            </div>
            <div className='flex items-center justify-between gap-3'>
              <span className='flex items-center gap-1.5'>
                <span className='size-2 rounded-full bg-blue-500' aria-hidden='true' />
                {t('Upstream first token')}
              </span>
              <span className='font-mono'>{formatDuration(ttfbMs)}</span>
            </div>
          </div>
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  )
}

/** 阶段徽标：流式图标带 aria-label；失败行 hover 显示 status_code / error_msg。 */
function PhaseBadge(props: { entry: LiveRequestEntry; label: string }) {
  const { t } = useTranslation()
  const { entry, label } = props
  const badge = (
    <Badge
      variant='secondary'
      className={cn('gap-1 px-1.5 text-[10px]', PHASE_STYLE[entry.phase])}
    >
      {entry.is_stream ? (
        <span aria-label={t('Streaming')} role='img'>
          ⚡
        </span>
      ) : null}
      {label}
    </Badge>
  )
  const errorDetail =
    entry.error_msg || (entry.status_code ? `HTTP ${entry.status_code}` : '')
  if (entry.phase !== 'error' || !errorDetail) {
    return badge
  }
  return (
    <TooltipProvider delay={100}>
      <Tooltip>
        <TooltipTrigger render={<span className='inline-flex'>{badge}</span>} />
        <TooltipContent className='max-w-72 break-words'>
          {entry.status_code ? `HTTP ${entry.status_code} · ` : ''}
          {entry.error_msg || t('Failed')}
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  )
}

function RequestRow(props: { entry: LiveRequestEntry }) {
  const { t } = useTranslation()
  const { entry } = props
  const phaseLabel = t(
    {
      received: 'Received',
      upstream: 'Upstream',
      streaming: 'Streaming',
      done: 'Done',
      error: 'Error',
    }[entry.phase] ?? entry.phase
  )
  return (
    <div className='hover:bg-muted/30 grid grid-cols-[minmax(120px,1.4fr)_minmax(90px,1fr)_minmax(80px,0.9fr)_minmax(90px,1fr)_minmax(70px,0.7fr)_minmax(70px,0.7fr)] items-center gap-2 border-b px-3 py-2 text-xs last:border-b-0'>
      <div className='min-w-0'>
        <div className='truncate font-medium'>{entry.model || '-'}</div>
        <div className='text-muted-foreground truncate font-mono text-[10px]'>
          {entry.user_name || `#${entry.user_id}`}
        </div>
      </div>
      <div className='min-w-0 truncate'>
        {entry.channel_name || (entry.channel_id ? `#${entry.channel_id}` : '-')}
        {entry.retry_index > 0 ? (
          <span className='text-muted-foreground ml-1 text-[10px]'>
            {t('retry {{n}}', { n: entry.retry_index })}
          </span>
        ) : null}
      </div>
      <div className='min-w-0'>
        <CompressionHint entry={entry} />
      </div>
      <div className='min-w-0'>
        <PhaseBadge entry={entry} label={phaseLabel} />
      </div>
      <div className='min-w-0'>
        <FirstTokenCell entry={entry} />
      </div>
      <div className='text-right tabular-nums'>
        {formatDuration(entry.elapsed_ms)}
      </div>
    </div>
  )
}

function RequestTableHeader() {
  const { t } = useTranslation()
  return (
    <div className='text-muted-foreground bg-muted/40 grid grid-cols-[minmax(120px,1.4fr)_minmax(90px,1fr)_minmax(80px,0.9fr)_minmax(90px,1fr)_minmax(70px,0.7fr)_minmax(70px,0.7fr)] items-center gap-2 border-b px-3 py-1.5 text-[11px] font-medium'>
      <div>{t('Model / User')}</div>
      <div>{t('Channel')}</div>
      <div>{t('Body')}</div>
      <div>{t('Phase')}</div>
      <div>{t('First token')}</div>
      <div className='text-right'>{t('Elapsed')}</div>
    </div>
  )
}

function LiveRequestsContent(props: { data: LiveRequestsData }) {
  const { t } = useTranslation()
  const data = props.data
  const compressionRatioPct = Math.round((data.avg_compression_ratio || 0) * 100)
  const savedBytes = Math.max(0, data.original_bytes_sum - data.compressed_bytes_sum)

  return (
    <div className='space-y-3'>
      <div className='grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-6'>
        <StatCard
          label={t('Active requests')}
          value={data.active_count}
          hint={
            data.concurrency.enabled
              ? t('{{active}} / {{limit}} · {{waiting}} waiting', {
                  active: data.concurrency.active,
                  limit: data.concurrency.limit,
                  waiting: data.concurrency.waiting,
                })
              : t('Concurrency bucket off')
          }
          icon={<Activity className='size-3.5' aria-hidden='true' />}
        />
        <StatCard
          label={t('Compression')}
          value={data.compression_enabled ? t('On') : t('Off')}
          hint={t('threshold ≥ {{kb}} KB', { kb: data.compression_threshold_kb })}
          tone={data.compression_enabled ? 'good' : 'default'}
        />
        <StatCard
          label={t('Avg compression ratio')}
          value={data.compressed_count > 0 ? `${compressionRatioPct}%` : '—'}
          hint={
            data.compressed_count > 0
              ? t('{{n}} compressed · saved {{saved}}', {
                  n: data.compressed_count,
                  saved: formatBytes(savedBytes),
                })
              : t('No compressed requests yet')
          }
          tone={compressionRatioPct > 0 && compressionRatioPct < 100 ? 'good' : 'default'}
        />
        <StatCard
          label={t('Avg first token')}
          value={data.avg_first_response_ms > 0 ? formatDuration(data.avg_first_response_ms) : '—'}
          hint={
            data.avg_upload_ms >= 0 || data.avg_upstream_ttfb_ms >= 0
              ? t('upload {{upload}} · upstream {{upstream}}', {
                  upload: formatDuration(Math.max(0, data.avg_upload_ms)),
                  upstream: formatDuration(Math.max(0, data.avg_upstream_ttfb_ms)),
                })
              : undefined
          }
        />
        <StatCard
          label={t('Network in')}
          value={formatMbps(data.network_in_mbps)}
          icon={<ArrowDown className='size-3.5' aria-hidden='true' />}
        />
        <StatCard
          label={t('Network out')}
          value={formatMbps(data.network_out_mbps)}
          icon={<ArrowUp className='size-3.5' aria-hidden='true' />}
        />
      </div>

      <div className='overflow-hidden rounded-lg border'>
        <div className='flex items-center justify-between border-b px-3 py-2'>
          <div className='flex items-center gap-2'>
            <span className='relative flex size-2'>
              <span className='absolute inline-flex size-full animate-ping rounded-full bg-emerald-400 opacity-75' />
              <span className='relative inline-flex size-2 rounded-full bg-emerald-500' />
            </span>
            <span className='text-sm font-medium'>{t('In progress')}</span>
            <span className='text-muted-foreground text-xs'>({data.active.length})</span>
          </div>
        </div>
        <RequestTableHeader />
        {data.active.length === 0 ? (
          <div className='text-muted-foreground px-3 py-6 text-center text-xs'>
            {t('No requests in progress.')}
          </div>
        ) : (
          <div className='max-h-[420px] overflow-y-auto'>
            {data.active.map((entry) => (
              <RequestRow key={entry.request_id} entry={entry} />
            ))}
          </div>
        )}
      </div>

      {data.finished.length > 0 ? (
        <div className='overflow-hidden rounded-lg border'>
          <div className='border-b px-3 py-2'>
            <span className='text-sm font-medium'>{t('Recently finished')}</span>
            <span className='text-muted-foreground ml-2 text-xs'>({data.finished.length})</span>
          </div>
          <RequestTableHeader />
          <div className='max-h-[320px] overflow-y-auto'>
            {data.finished.map((entry) => (
              <RequestRow
                key={`${entry.request_id}-${entry.finished_at}`}
                entry={entry}
                />
            ))}
          </div>
        </div>
      ) : null}
    </div>
  )
}

export function LiveRequestsPanel() {
  const { t } = useTranslation()
  const query = useQuery({
    queryKey: ['system-info', 'live-requests'],
    queryFn: async () => {
      const res = await getLiveRequests()
      if (!res.success || !res.data) {
        throw new Error(res.message || 'Failed to load live requests')
      }
      return res.data
    },
    staleTime: POLL_INTERVAL_MS,
    retry: false,
    refetchInterval: POLL_INTERVAL_MS,
  })

  const refreshing = query.isFetching && !query.isLoading

  return (
    <section className='bg-card overflow-hidden rounded-lg border shadow-xs'>
      <div className='flex flex-col gap-3 border-b px-4 py-3 sm:flex-row sm:items-center sm:justify-between sm:px-5'>
        <div className='flex items-center gap-2'>
          <span className='bg-muted text-muted-foreground inline-flex size-7 items-center justify-center rounded-md'>
            <Activity className='size-4' aria-hidden='true' />
          </span>
          <div>
            <h3 className='text-sm font-semibold'>{t('Live Requests')}</h3>
            <p className='text-muted-foreground mt-0.5 text-xs'>
              {t('Real-time request throughput, compression and latency.')}
            </p>
          </div>
        </div>
        <div className='flex items-center gap-2'>
          <span className='text-muted-foreground text-xs' aria-live='polite'>
            {t('Auto-refreshing every {{seconds}}s', {
              seconds: POLL_INTERVAL_MS / 1000,
            })}
          </span>
          <Button
            type='button'
            variant='outline'
            size='sm'
            onClick={() => void query.refetch()}
            disabled={query.isFetching}
            aria-label={t('Refresh')}
          >
            <RefreshCw
              data-icon='inline-start'
              className={cn('size-3.5', refreshing && 'animate-spin')}
              aria-hidden='true'
            />
            {t('Refresh')}
          </Button>
        </div>
      </div>

      <div className='p-4 sm:p-5' aria-busy={query.isFetching}>
        {renderPanelBody(query, t)}
      </div>
    </section>
  )
}

function renderPanelBody(
  query: ReturnType<typeof useQuery<LiveRequestsData>>,
  t: (key: string, options?: Record<string, unknown>) => string
) {
  if (query.isLoading) {
    return (
      <div className='space-y-2'>
        <div className='grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-6'>
          {SKELETON_KEYS.map((key) => (
            <Skeleton key={key} className='h-20 w-full rounded-lg' />
          ))}
        </div>
        <Skeleton className='h-40 w-full rounded-lg' />
      </div>
    )
  }
  if (query.isError) {
    return (
      <ErrorState
        title={t('We could not load live requests.')}
        description={query.error instanceof Error ? query.error.message : undefined}
        onRetry={() => void query.refetch()}
        className='min-h-[220px]'
      />
    )
  }
  if (query.data) {
    return <LiveRequestsContent data={query.data} />
  }
  return null
}
