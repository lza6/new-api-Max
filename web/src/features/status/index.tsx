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
import { useTranslation } from 'react-i18next'

import { ErrorState } from '@/components/error-state'
import { PublicLayout } from '@/components/layout'
import { LoadingState } from '@/components/loading-state'
import { StatusBadge } from '@/components/status-badge'
import { formatTimestampRelative } from '@/lib/format'
import { requireServerSuccess } from '@/lib/server-error-message'

import { getPublicStatus } from './api'
import {
  StatusBackgroundCard,
  StatusDependenciesCard,
  StatusTrafficCard,
} from './components'
import type { PublicStatusData } from './types'

/**
 * Public service status page (/status).
 *
 * Everything rendered here comes from `GET /api/status/page`, which reports
 * only what the running instance can observe: dependency probes, request-log
 * counts for a stated window, and background loop heartbeats. The page
 * deliberately claims no uptime percentage or SLA — the gateway keeps no
 * availability history, so such a number could not be substantiated. The
 * "How these figures are measured" panel states the caliber of each figure.
 */

/** Splits a duration into whole days/hours/minutes for localized display. */
function uptimeParts(seconds: number): {
  days: number
  hours: number
  minutes: number
} {
  const safe = Math.max(0, Math.floor(seconds))
  return {
    days: Math.floor(safe / 86400),
    hours: Math.floor((safe % 86400) / 3600),
    minutes: Math.floor((safe % 3600) / 60),
  }
}

/** Shared hero + measurement-caliber panel, so loading/error states stay short. */
function StatusHero(props: { data: PublicStatusData | null }) {
  const { t } = useTranslation()
  const data = props.data
  const overallOk = data?.status === 'operational'

  return (
    <section className='space-y-4'>
      <div className='flex flex-wrap items-center gap-x-3 gap-y-2'>
        <h1 className='text-[clamp(1.75rem,4vw,2.5rem)] leading-[1.15] font-bold tracking-tight'>
          {t('Service status')}
        </h1>
        {data != null && (
          <StatusBadge
            variant={overallOk ? 'success' : 'warning'}
            size='md'
            copyable={false}
            label={
              overallOk
                ? t('All reported checks passed')
                : t('One or more checks need attention')
            }
          />
        )}
      </div>
      <p className='text-muted-foreground max-w-2xl text-sm'>
        {t(
          'Measurements of this gateway taken by the gateway itself: dependency reachability, request-log statistics and background task activity.'
        )}
      </p>
      {data != null && (
        <dl className='text-muted-foreground flex flex-wrap gap-x-6 gap-y-1 text-xs'>
          <div className='flex gap-1.5'>
            <dt>{t('Version')}</dt>
            <dd className='text-foreground font-medium'>{data.version}</dd>
          </div>
          <div className='flex gap-1.5'>
            <dt>{t('Uptime')}</dt>
            <dd className='text-foreground font-medium tabular-nums'>
              {t(
                '{{days}}d {{hours}}h {{minutes}}m',
                uptimeParts(data.uptime_seconds)
              )}
            </dd>
          </div>
          <div className='flex gap-1.5'>
            <dt>{t('Data refreshed')}</dt>
            <dd className='text-foreground font-medium'>
              {formatTimestampRelative(data.generated_at)}
            </dd>
          </div>
        </dl>
      )}
    </section>
  )
}

/** The honesty panel: what each figure means, and what is deliberately absent. */
function StatusCaliberNotice() {
  const { t } = useTranslation()

  return (
    <section className='border-border/60 bg-muted/20 rounded-xl border p-5'>
      <h2 className='text-sm font-semibold'>
        {t('How these figures are measured')}
      </h2>
      <ul className='text-muted-foreground mt-3 list-disc space-y-1.5 pl-4 text-xs'>
        <li>
          {t(
            'Dependency states are probe results taken on the server, not an external periodic monitor.'
          )}
        </li>
        <li>
          {t(
            'Request and error counts come from this deployment’s own request logs, over the window printed on the card.'
          )}
        </li>
        <li>
          {t(
            'Availability history is not sampled or retained here, so no uptime percentage, SLA or guarantee is published — none could be substantiated.'
          )}
        </li>
        <li>
          {t(
            'The server refreshes these figures at most every 30 seconds; this page asks for them once a minute.'
          )}
        </li>
      </ul>
    </section>
  )
}

export function ServiceStatus() {
  const { t } = useTranslation()
  const query = useQuery({
    queryKey: ['public-status'],
    queryFn: async () => requireServerSuccess(await getPublicStatus()),
    staleTime: 30_000,
    refetchInterval: 60_000,
    retry: false,
  })

  const data = query.data?.data ?? null

  if (query.isLoading) {
    return (
      <PublicLayout>
        <div className='space-y-6'>
          <StatusHero data={null} />
          <LoadingState message={t('Loading service status...')} />
        </div>
      </PublicLayout>
    )
  }

  if (data == null) {
    return (
      <PublicLayout>
        <div className='space-y-6'>
          <StatusHero data={null} />
          <ErrorState
            title={t('Unable to load service status')}
            description={t(
              'The status endpoint did not return data. Try again in a moment.'
            )}
            onRetry={() => {
              void query.refetch()
            }}
          />
        </div>
      </PublicLayout>
    )
  }

  return (
    <PublicLayout>
      <div className='space-y-6'>
        <StatusHero data={data} />
        <StatusTrafficCard traffic={data.traffic} />
        <div className='grid gap-4 lg:grid-cols-2'>
          <StatusDependenciesCard components={data.components} />
          <StatusBackgroundCard background={data.background} />
        </div>
        <StatusCaliberNotice />
      </div>
    </PublicLayout>
  )
}
