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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { HardDrive, Loader2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Badge } from '@/components/ui/badge'

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Switch } from '@/components/ui/switch'
import { handleServerError } from '@/lib/handle-server-error'

import { getTrainingDataOverview, setTrainingDataEnabled } from '../api'
import type { CaptureStats, GatewayStatus, SinkOverview } from '../types'
import { formatBytes } from '../lib/format'
import { DailyHistoryCard } from './daily-history-card'
import { useState } from 'react'

function StatCard(props: { label: string; value: string; hint?: string }) {
  return (
    <div className='bg-background/60 rounded-xl border p-3'>
      <p className='text-muted-foreground text-xs'>{props.label}</p>
      <p className='mt-1 font-mono text-lg font-semibold tabular-nums'>
        {props.value}
      </p>
      {props.hint && (
        <p className='text-muted-foreground mt-0.5 text-[11px]'>
          {props.hint}
        </p>
      )}
    </div>
  )
}

/** 云盘用量卡（配额未知时明确显示「未知」而不是 0）。 */
function SinkCard(props: { sink: SinkOverview }) {
  const { t } = useTranslation()
  const sink = props.sink

  if (!sink.quota_known) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className='flex items-center gap-2 text-sm'>
            <HardDrive className='h-4 w-4' />
            {t('Cloud storage')}
          </CardTitle>
        </CardHeader>
        <CardContent className='space-y-1'>
          {/* 不把 0 当成「云盘满了」：明确说未知 + 原因。 */}
          <p className='text-sm'>{t('Quota unknown')}</p>
          {sink.last_error && (
            <p className='text-muted-foreground text-xs break-all'>
              {sink.last_error}
            </p>
          )}
        </CardContent>
      </Card>
    )
  }

  const usedPct =
    sink.total_bytes > 0
      ? Math.min(100, (sink.used_bytes / sink.total_bytes) * 100)
      : 0

  return (
    <Card>
      <CardHeader>
        <CardTitle className='flex items-center gap-2 text-sm'>
          <HardDrive className='h-4 w-4' />
          {t('Cloud storage')}
          <Badge variant='secondary' className='ml-auto font-normal'>
            {sink.provider}
          </Badge>
        </CardTitle>
      </CardHeader>
      <CardContent className='space-y-3'>
        <div className='grid grid-cols-3 gap-2'>
          <StatCard label={t('Total')} value={formatBytes(sink.total_bytes)} />
          <StatCard label={t('Used')} value={formatBytes(sink.used_bytes)} />
          <StatCard
            label={t('Remaining')}
            value={formatBytes(sink.remaining_bytes)}
          />
        </div>
        <div
          className='bg-muted h-2 w-full overflow-hidden rounded-full'
          role='progressbar'
          aria-valuenow={Math.round(usedPct)}
          aria-valuemin={0}
          aria-valuemax={100}
        >
          <div
            className='bg-primary h-full transition-[width]'
            style={{ width: `${usedPct}%` }}
          />
        </div>
        <p className='text-muted-foreground text-xs'>
          {t('{{folders}} folders / {{files}} files', {
            folders: sink.folders,
            files: sink.files,
          })}
          {sink.truncated && ` · ${t('(at least, list was truncated)')}`}
        </p>
        {/*
          配额已知 ≠ 统计完整：多账号里可能只有一个账号的 CountItems 失败，
          此时 folders/files 是部分之和。后端会带 last_error 并把 truncated
          置 true；这里必须显示，否则管理员看到的是一个偏小却"看起来完整"的数字。
        */}
        {sink.last_error && (
          <p className='text-destructive text-xs break-all'>
            {sink.last_error}
          </p>
        )}
      </CardContent>
    </Card>
  )
}

/** 今日采集统计卡。 */
function StatsCard(props: { stats: CaptureStats }) {
  const { t } = useTranslation()
  const s = props.stats
  const queuePct =
    s.queue_capacity > 0 ? (s.queue_len / s.queue_capacity) * 100 : 0

  return (
    <Card>
      <CardHeader>
        <CardTitle className='text-sm'>
          {t('Today (UTC {{day}})', { day: s.day })}
        </CardTitle>
      </CardHeader>
      <CardContent className='space-y-3'>
        <div className='grid grid-cols-2 gap-2 sm:grid-cols-4'>
          <StatCard label={t('Users')} value={String(s.users)} />
          <StatCard
            label={t('Conversations')}
            value={String(s.conversations)}
          />
          <StatCard
            label={t('Uploaded')}
            value={formatBytes(s.bytes_uploaded)}
          />
          <StatCard
            label={t('Messages')}
            value={String(s.messages_uploaded)}
          />
        </div>

        <div className='grid grid-cols-2 gap-2 sm:grid-cols-4'>
          <StatCard
            label={t('Queue')}
            value={`${s.queue_len} / ${s.queue_capacity}`}
            hint={`${Math.round(queuePct)}% ${t('full')}`}
          />
          <StatCard
            label={t('In flight')}
            value={formatBytes(s.in_flight_bytes)}
            hint={t('uploading now')}
          />
          <StatCard
            label={t('Dropped')}
            value={String(s.total_dropped)}
            hint={t('queue full, best-effort')}
          />
          <StatCard
            label={t('Upload errors')}
            value={String(s.total_upload_errors)}
          />
          <StatCard
            label={t('Retried')}
            value={String(s.retried)}
            hint={t('failures re-queued for retry')}
          />
        </div>

        {/*
          「取不到文本」要单独显示：这个数字持续增长通常不是用户没发文本，
          而是 CountToken 被关了 —— 功能开着却一个人都采不到。
          不显示的话，这类**配置型静默失效**只能靠翻 /metrics 才发现。
        */}
        {s.skipped_no_text > 0 && (
          <p className='text-muted-foreground text-[11px] leading-relaxed'>
            {t(
              'Skipped {{count}} requests with no extractable text. A steadily rising number usually means token counting is disabled, so no conversation text is produced.',
              { count: s.skipped_no_text }
            )}
          </p>
        )}

        {/*
          **必须披露**：统计是进程内计数，重启归零。
          不写这句，管理员会以为「昨天没采到数据」。
        */}
        <p className='text-muted-foreground text-[11px] leading-relaxed'>
          {t(
            'These counters live in memory and reset when the service restarts; they cover the current day only.'
          )}
        </p>
      </CardContent>
    </Card>
  )
}

/**
 * 采集总览面板：总开关 + 今日统计 + 云盘配额。
 */
export function CaptureOverviewPanel() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [disableConfirmOpen, setDisableConfirmOpen] = useState(false)

  const overviewQuery = useQuery({
    queryKey: ['training-data', 'overview'],
    queryFn: async () => {
      const res = await getTrainingDataOverview()
      if (!res.success) {
        throw new Error(res.message || t('Failed to load'))
      }
      return res.data
    },
    // 队列长度/在传字节是实时指标，轮询让面板「活着」。
    refetchInterval: 5000,
  })

  const toggleMutation = useMutation({
    mutationFn: async (enabled: boolean) => {
      const res = await setTrainingDataEnabled(enabled)
      if (!res.success) {
        throw new Error(res.message || t('Failed to update'))
      }
      return res.data
    },
    onSuccess: () => {
      toast.success(t('Settings updated'))
      setDisableConfirmOpen(false)
      void queryClient.invalidateQueries({ queryKey: ['training-data'] })
    },
    onError: (error) => handleServerError(error, t('Failed to update')),
  })

  if (overviewQuery.isLoading) {
    return <LoadingState />
  }
  if (overviewQuery.isError || !overviewQuery.data) {
    return (
      <ErrorState
        title={t('Failed to load')}
        description={overviewQuery.error?.message}
        onRetry={() => void overviewQuery.refetch()}
      />
    )
  }

  const { gateway, stats, sink } = overviewQuery.data

  return (
    <div className='space-y-4'>
      <Card>
        <CardContent className='flex flex-wrap items-center justify-between gap-3 pt-6'>
          <div className='min-w-0'>
            <p className='text-sm font-medium'>
              {t('Training data collection')}
            </p>
            <p className='text-muted-foreground text-xs'>
              {describeGateway(gateway, t)}
            </p>
          </div>
          <div className='flex items-center gap-2'>
            {toggleMutation.isPending && (
              <Loader2 className='text-muted-foreground size-4 animate-spin' />
            )}
            <Switch
              checked={gateway.enabled}
              // 云盘未配置时**禁用开关**并显示原因 ——
              // 允许打开会让人以为在采集，实际数据全被丢。
              disabled={!gateway.can_enable || toggleMutation.isPending}
              onCheckedChange={(next) => {
                if (next) {
                  toggleMutation.mutate(true)
                  return
                }
                setDisableConfirmOpen(true)
              }}
              aria-label={t('Training data collection')}
            />
          </div>
        </CardContent>
      </Card>

      <div className='grid gap-4 xl:grid-cols-2'>
        <StatsCard stats={stats} />
        <SinkCard sink={sink} />
      </div>

      {/* 「几月几日采了多少」——取自云盘目录树，可回溯历史（用户明确要求）。 */}
      <DailyHistoryCard />

      <ConfirmDialog
        open={disableConfirmOpen}
        onOpenChange={setDisableConfirmOpen}
        title={t('Turn off training data collection')}
        desc={t(
          'New requests will stop being collected immediately. Data already uploaded is kept. Users will no longer see the consent switch.'
        )}
        destructive
        isLoading={toggleMutation.isPending}
        confirmText={t('Turn off')}
        handleConfirm={() => toggleMutation.mutate(false)}
      />
    </div>
  )
}

/**
 * 把总开关状态转成一句人话。
 *
 * 抽成函数而非嵌套三元：三层嵌套三元可读性差（oxlint 的
 * no-nested-ternary 规则会拦），且这里的**语义顺序**很重要 ——
 * 「不可用」优先于「开关是开的」，否则会出现「云盘没配却显示采集中」。
 */
function describeGateway(
  gateway: GatewayStatus,
  t: (key: string) => string
): string {
  if (!gateway.can_enable) {
    return gateway.reason ?? t('Not available')
  }
  if (gateway.enabled) {
    return t('Collecting from consenting users')
  }
  return t('Turned off; no data is collected')
}
