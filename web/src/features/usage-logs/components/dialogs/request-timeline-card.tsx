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
import { Check, Copy, Download, ListTree } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

import type { RequestTimeline, TimelinePhase } from '../../lib/request-timeline'
import { DetailSection } from './log-detail-layout'

const STATUS_TONE: Record<TimelinePhase['status'], string> = {
  done: 'text-emerald-600 dark:text-emerald-400',
  failed: 'text-red-500',
  skipped: 'text-muted-foreground',
  info: 'text-sky-600 dark:text-sky-400',
}

function PhaseRow(props: { phase: TimelinePhase }) {
  const { t } = useTranslation()
  const { phase } = props
  const label =
    phase.key === 'inbound'
      ? t('Inbound')
      : phase.key === 'auth'
        ? t('Auth')
        : phase.key === 'channel'
          ? t('Channel selection')
          : phase.key === 'upstream'
            ? t('Upstream call')
            : phase.key === 'first_token'
              ? t('First token')
              : t('Complete')
  const statusLabel =
    phase.status === 'failed'
      ? t('Failed')
      : phase.status === 'skipped'
        ? t('Skipped')
        : phase.status === 'info'
          ? t('Info')
          : t('Done')
  return (
    <div className='flex min-w-0 items-start gap-2'>
      <span className={cn('mt-0.5 h-2 w-2 shrink-0 rounded-full', phase.status === 'done' && 'bg-emerald-500', phase.status === 'failed' && 'bg-red-500', phase.status === 'skipped' && 'bg-muted-foreground/50', phase.status === 'info' && 'bg-sky-500')} aria-hidden='true' />
      <div className='min-w-0 flex-1'>
        <div className='flex flex-wrap items-baseline gap-x-2'>
          <span className='text-xs font-medium'>{label}</span>
          <span className={cn('text-[11px]', STATUS_TONE[phase.status])}>{statusLabel}</span>
          {phase.durationMs != null && (
            <span className='text-muted-foreground font-mono text-[11px]'>
              +{phase.durationMs}ms
            </span>
          )}
        </div>
        {phase.detail && (
          <p className='text-muted-foreground min-w-0 truncate font-mono text-[11px]' title={phase.detail}>
            {phase.detail}
          </p>
        )}
      </div>
    </div>
  )
}

/**
 * B2-2 请求级 trace 时间线（黑匣子打开）。
 * 展示 入站→鉴权→渠道选择→上游调用→首包→完成/失败 各阶段；老日志缺字段
 * 时逐阶段优雅降级。JSON 导出按钮复制稳定 v1 receipts（类 hermes-trace）。
 */
export function RequestTimelineCard(props: {
  timeline: RequestTimeline
  exportJson: string
  copied: boolean
  onCopy: () => void
}) {
  const { t } = useTranslation()
  const { timeline } = props
  return (
    <DetailSection
      icon={<ListTree className='h-3.5 w-3.5' aria-hidden='true' />}
      label={t('Request timeline')}
    >
      <div className='space-y-1.5'>
        {timeline.phases.map((phase, index) => (
          <PhaseRow key={`${phase.key}-${index}`} phase={phase} />
        ))}
      </div>
      <div className='mt-2 flex justify-end'>
        <Button
          type='button'
          variant='outline'
          size='sm'
          onClick={props.onCopy}
          aria-label={t('Copy timeline JSON')}
        >
          {props.copied ? (
            <Check className='mr-1 h-3.5 w-3.5' aria-hidden='true' />
          ) : (
            <Copy className='mr-1 h-3.5 w-3.5' aria-hidden='true' />
          )}
          {props.copied ? t('Copied') : t('Copy JSON')}
          <Download className='ml-1 h-3 w-3 text-muted-foreground' aria-hidden='true' />
        </Button>
      </div>
    </DetailSection>
  )
}
