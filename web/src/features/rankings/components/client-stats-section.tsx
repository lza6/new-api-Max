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
import { MonitorSmartphone } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Skeleton } from '@/components/ui/skeleton'

import { useClientStats } from '../hooks/use-client-stats'

/**
 * 客户端使用统计：各客户端（Claude Code / Cursor / SDK…）调用占比 +
 * 近 30 天平均缓存命中率。对应后端 /api/rankings/clients。
 */
export function ClientStatsSection() {
  const { t } = useTranslation()
  const query = useClientStats(7, 20)
  const overall = query.data?.overall ?? []
  const avgCache = query.data?.avg_cache_rate ?? 0

  return (
    <section className='bg-card rounded-2xl border p-5 shadow-xs'>
      <div className='mb-4 flex items-center gap-2'>
        <MonitorSmartphone className='text-muted-foreground size-4' />
        <div>
          <h2 className='text-lg font-semibold tracking-tight'>
            {t('Client Usage')}
          </h2>
          <p className='text-muted-foreground text-xs'>
            {t('Which clients call the gateway (last 7 days)')}
          </p>
        </div>
      </div>

      {query.isLoading && (
        <div className='space-y-2'>
          {Array.from({ length: 5 }, (_, i) => (
            <Skeleton key={i} className='h-8 w-full' />
          ))}
        </div>
      )}
      {!query.isLoading && query.error && (
        <p className='text-muted-foreground text-sm'>
          {t('Unable to load client data')}
        </p>
      )}
      {!query.isLoading && !query.error && overall.length === 0 && (
        <p className='text-muted-foreground text-sm'>
          {t('No client data yet')}
        </p>
      )}
      {!query.isLoading && !query.error && overall.length > 0 && (
        <>
          {avgCache > 0 ? (
            <div className='text-muted-foreground mb-2 text-xs'>
              {t('Avg cache hit rate')}:{' '}
              <span className='text-violet-600 dark:text-violet-400 font-medium'>
                {(avgCache * 100).toFixed(0)}%
              </span>
            </div>
          ) : null}
          <ul className='divide-border/60 divide-y'>
            {overall.map((row) => (
              <li key={row.client} className='flex items-center gap-3 py-2 text-sm'>
                <span className='min-w-0 flex-1 truncate font-mono'>
                  {row.client}
                </span>
                <span className='text-muted-foreground shrink-0 text-xs tabular-nums'>
                  {row.count}
                </span>
                <div className='bg-muted/40 relative h-1.5 w-24 shrink-0 overflow-hidden rounded-full'>
                  <div
                    className='absolute inset-y-0 left-0 rounded-full bg-gradient-to-r from-blue-500 to-violet-500'
                    style={{ width: `${Math.min(100, row.share * 100)}%` }}
                  />
                </div>
                <span className='w-12 shrink-0 text-right text-xs tabular-nums'>
                  {(row.share * 100).toFixed(1)}%
                </span>
              </li>
            ))}
          </ul>
        </>
      )}
    </section>
  )
}
