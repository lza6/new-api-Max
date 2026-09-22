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
import { Activity } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Skeleton } from '@/components/ui/skeleton'
import { formatTraffic } from '@/lib/format'

import { useBandwidth } from '../hooks/use-bandwidth'

/**
 * Model traffic leaderboard: ranks models by request/response bytes over the
 * last 30 days, formatted as human-readable traffic (KB/MB/GB/TB).
 * Mirrors the backend /api/rankings/bandwidth endpoint.
 */
export function BandwidthSection() {
  const { t } = useTranslation()
  const query = useBandwidth(30)
  const rows = query.data?.leaderboard ?? []

  return (
    <section className='bg-card rounded-2xl border p-5 shadow-xs'>
      <div className='mb-4 flex items-center gap-2'>
        <Activity className='text-muted-foreground size-4' />
        <div>
          <h2 className='text-lg font-semibold tracking-tight'>
            {t('Model Traffic Leaderboard')}
          </h2>
          <p className='text-muted-foreground text-xs'>
            {t('Traffic by model over the last 30 days')}
          </p>
        </div>
      </div>

      {query.isLoading && (
        <div className='space-y-2'>
          {Array.from({ length: 6 }, (_, i) => (
            <Skeleton key={i} className='h-8 w-full' />
          ))}
        </div>
      )}
      {!query.isLoading && query.error && (
        <p className='text-muted-foreground text-sm'>
          {t('Unable to load traffic data')}
        </p>
      )}
      {!query.isLoading && !query.error && rows.length === 0 && (
        <p className='text-muted-foreground text-sm'>
          {t('No traffic data yet')}
        </p>
      )}
      {!query.isLoading && !query.error && rows.length > 0 && (
        <ul className='divide-border/60 divide-y'>
          {rows.map((row, index) => (
            <li
              key={row.model}
              className='flex items-center gap-3 py-2.5'
            >
              <span className='text-muted-foreground/80 w-6 shrink-0 text-right font-mono text-xs tabular-nums'>
                {index + 1}.
              </span>
              <span className='text-foreground min-w-0 flex-1 truncate font-mono text-sm font-medium'>
                {row.model}
              </span>
              <span className='text-muted-foreground shrink-0 text-xs tabular-nums'>
                {t('Requests')}: {row.requests}
              </span>
              <span className='text-foreground shrink-0 font-mono text-sm font-semibold tabular-nums'>
                {row.bytes_text || formatTraffic(row.bytes)}
              </span>
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}
