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
import { ChevronDown } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  ADMIN_PERMISSION_ACTIONS,
  ADMIN_PERMISSION_RESOURCES,
  hasPermission,
} from '@/lib/admin-permissions'
import { StatusBadge } from '@/components/status-badge'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import {
  Popover,
  PopoverContent,
  PopoverHeader,
  PopoverTitle,
  PopoverTrigger,
} from '@/components/ui/popover'
import { formatTimestampToDate } from '@/lib/format'
import { requireServerSuccess } from '@/lib/server-error-message'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/stores/auth-store'

import { getChannelHealthScores } from '../api'
import {
  coolClassLabelKey,
  formatRelativeTime,
  getProbeResults,
  gradeToVariant,
  hasHealthData,
  parseProbeHistory,
  scoreToVariant,
  toHealthSnapshotView,
  type ChannelHealthSnapshotView,
  type ProbeGradeVariant,
  type ProbeReport,
} from '../lib'
import type { Channel } from '../types'
import { useChannels } from './channels-provider'

const HEALTH_SCORES_STALE_TIME = 60 * 1000

function formatPercent(rate: number): string {
  return `${Math.round(rate * 100)}%`
}

function HealthDetailRow(props: { label: string; value: string }) {
  return (
    <div className='flex items-center justify-between gap-3'>
      <span className='text-muted-foreground text-xs'>{props.label}</span>
      <span className='text-xs font-medium'>{props.value}</span>
    </div>
  )
}

function ProbeCaseItem(props: { name: string; passed: boolean; error?: string }) {
  const { t } = useTranslation()
  return (
    <div className='space-y-0.5'>
      <div className='flex items-center justify-between gap-3'>
        <span className='font-mono text-xs'>{props.name}</span>
        <StatusBadge
          label={props.passed ? t('Success') : t('Failed')}
          variant={props.passed ? 'success' : 'danger'}
          size='sm'
          copyable={false}
        />
      </div>
      {props.error && (
        <p className='text-destructive text-xs break-all'>{props.error}</p>
      )}
    </div>
  )
}

function ProbeEvidence(props: { evidence: string }) {
  const { t } = useTranslation()
  const [expanded, setExpanded] = useState(false)
  return (
    <Collapsible open={expanded} onOpenChange={setExpanded}>
      <CollapsibleTrigger
        render={
          <button
            type='button'
            className='text-muted-foreground hover:text-foreground flex items-center gap-1 text-xs transition-colors'
          />
        }
      >
        <ChevronDown
          className={cn('h-3 w-3 transition-transform', expanded && 'rotate-180')}
          aria-hidden='true'
        />
        {t('Evidence')}
      </CollapsibleTrigger>
      <CollapsibleContent>
        <p className='text-muted-foreground mt-1 text-xs break-all whitespace-pre-wrap'>
          {props.evidence}
        </p>
      </CollapsibleContent>
    </Collapsible>
  )
}

export function HealthPopoverContent(props: {
  report: ProbeReport
  snapshot: ChannelHealthSnapshotView
}) {
  const { t, i18n } = useTranslation()
  const { report, snapshot } = props
  const locale = i18n.resolvedLanguage || i18n.language
  const results = getProbeResults(report)

  return (
    <div className='space-y-3'>
      <div className='space-y-1'>
        <HealthDetailRow
          label={t('Latest probe')}
          value={formatTimestampToDate(report.probed_at)}
        />
        <HealthDetailRow label={t('Model')} value={report.model || '-'} />
        <HealthDetailRow label={t('Grade')} value={report.grade || '-'} />
        <HealthDetailRow label={t('Score')} value={String(report.score ?? '-')} />
        {report.duration_ms !== undefined && (
          <HealthDetailRow
            label={t('Duration')}
            value={`${report.duration_ms}ms`}
          />
        )}
        {report.skipped && (
          <p className='text-muted-foreground text-xs'>
            {t('Skipped')}: {report.skipped}
          </p>
        )}
      </div>

      {results.length > 0 && (
        <div className='space-y-1.5'>
          <p className='text-xs font-medium'>{t('Probe result')}</p>
          <div className='space-y-1.5'>
            {results.map((result) => (
              <div
                key={`${result.name ?? 'case'}-${result.error ?? ''}`}
                className='space-y-0.5'
              >
                <ProbeCaseItem
                  name={result.name ?? '-'}
                  passed={result.passed === true}
                  error={result.error}
                />
                {result.evidence && <ProbeEvidence evidence={result.evidence} />}
              </div>
            ))}
          </div>
        </div>
      )}

      <div className='space-y-1 border-t pt-2'>
        <p className='text-xs font-medium'>{t('Health details')}</p>
        <HealthDetailRow
          label={t('Success rate')}
          value={formatPercent(snapshot.successRate)}
        />
        <HealthDetailRow
          label={t('P95 latency')}
          value={`${snapshot.p95LatencyMs}ms`}
        />
        <HealthDetailRow
          label={t('P50 latency')}
          value={`${snapshot.p50LatencyMs}ms`}
        />
        <HealthDetailRow label={t('Samples')} value={String(snapshot.sampleCount)} />
        <HealthDetailRow
          label={t('Cooldowns')}
          value={String(snapshot.coolCount)}
        />
        {snapshot.coolingDown && snapshot.coolUntil > 0 && (
          <div className='space-y-1'>
            <p className='text-warning text-xs'>
              {t('Cooling down')} · {formatRelativeTime(snapshot.coolUntil, locale)}
            </p>
            {snapshot.lastCoolClass ? (
              <p className='text-muted-foreground text-xs'>
                {t('Reason:')} {t(coolClassLabelKey(snapshot.lastCoolClass))}
              </p>
            ) : null}
          </div>
        )}
      </div>
    </div>
  )
}

/**
 * Channel health column cell (B4-3). Shows a probe grade badge when a probe
 * report exists, otherwise a health score badge, otherwise a neutral
 * "Not probed" badge. Hover/click opens a popover with probe details.
 */
export function ChannelHealthCell(props: { channel: Channel }) {
  const { t } = useTranslation()
  const currentUser = useAuthStore((s) => s.auth.user)
  const canReadHealth = hasPermission(
    currentUser,
    ADMIN_PERMISSION_RESOURCES.CHANNEL,
    ADMIN_PERMISSION_ACTIONS.READ
  )
  const [popoverOpen, setPopoverOpen] = useState(false)

  // 健康分快照已由 ChannelsProvider 统一拉取（60s 缓存），此处复用 context
  // 数据而非重复请求；fallback 到本组件直接查询（Provider 未注入时）。
  const { healthScores: contextHealthScores } = useChannels()
  const query = useQuery({
    queryKey: ['channels', 'health_scores'],
    queryFn: async () => requireServerSuccess(await getChannelHealthScores()),
    enabled: canReadHealth && !contextHealthScores,
    staleTime: HEALTH_SCORES_STALE_TIME,
  })

  const channel = props.channel
  const { latest } = useMemo(
    () => parseProbeHistory(channel.probe_result),
    [channel.probe_result]
  )
  const snapshot = useMemo(
    () =>
      toHealthSnapshotView(
        query.data?.data ?? contextHealthScores ?? undefined,
        channel.id
      ),
    [query.data, contextHealthScores, channel.id]
  )

  let variant: ProbeGradeVariant
  let label: string
  if (latest) {
    variant = gradeToVariant(latest.grade)
    label = latest.grade || String(latest.score ?? '-')
  } else if (hasHealthData(snapshot)) {
    variant = scoreToVariant(snapshot.score)
    label = String(Math.round(snapshot.score))
  } else {
    variant = 'neutral'
    label = t('Not probed')
  }

  const hasDetails = latest !== null || hasHealthData(snapshot)

  return (
    <Popover open={popoverOpen} onOpenChange={setPopoverOpen}>
      <PopoverTrigger
        disabled={!hasDetails}
        openOnHover
        delay={200}
        closeDelay={100}
        render={
          <span
            role={hasDetails ? 'button' : undefined}
            tabIndex={hasDetails ? 0 : -1}
            aria-label={hasDetails ? t('Health details') : undefined}
            onClick={(e) => e.stopPropagation()}
            onKeyDown={(e) => {
              if (!hasDetails) {
                return
              }
              if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault()
                setPopoverOpen(true)
              }
            }}
            className={cn(hasDetails ? 'cursor-pointer' : 'cursor-default')}
          />
        }
      >
        <StatusBadge
          label={label}
          variant={variant}
          size='sm'
          copyable={false}
          pulse={snapshot.coolingDown}
        />
      </PopoverTrigger>
      {latest && (
        <PopoverContent align='start' className='w-80 max-w-[90vw]'>
          <PopoverHeader>
            <PopoverTitle>{t('Health')}</PopoverTitle>
          </PopoverHeader>
          <HealthPopoverContent report={latest} snapshot={snapshot} />
        </PopoverContent>
      )}
    </Popover>
  )
}
