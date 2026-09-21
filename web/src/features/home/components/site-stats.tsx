/*
Copyright (C) 2023-2026 QuantumNous
Licensed under GNU AGPL v3. See LICENSE.
*/
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { Skeleton } from '@/components/ui/skeleton'
import { formatCompactNumber, formatTraffic } from '@/lib/format'
import { requireServerSuccess } from '@/lib/server-error-message'

import { getSiteStats } from '../api'

/**
 * Authoritative site statistics (aggregates only): bandwidth served,
 * requests processed and tokens processed. Rendered on the marketing home
 * page; falls back to a compact empty state when the API is unavailable.
 */
export function SiteStats() {
  const { t } = useTranslation()
  const { data, isLoading, isError } = useQuery({
    queryKey: ['site-stats'],
    queryFn: async () =>
      requireServerSuccess(await getSiteStats()).data || null,
    staleTime: 5 * 60_000,
    refetchOnWindowFocus: false,
  })

  if (isLoading) {
    return (
      <div className='grid grid-cols-1 gap-6 pt-10 sm:grid-cols-3 md:gap-10'>
        {[0, 1, 2].map((i) => (
          <div key={i} className='flex flex-col items-center gap-1.5'>
            <Skeleton className='h-8 w-28 rounded-md' />
            <Skeleton className='h-4 w-24 rounded-md' />
          </div>
        ))}
      </div>
    )
  }

  const stats = data
    ? [
        {
          label: t('Bandwidth served'),
          value: data.total_bytes_text || formatTraffic(data.total_bytes || 0),
          title: t('Bandwidth served'),
        },
        {
          label: t('Requests processed'),
          value: formatCompactNumber(data.total_requests || 0),
          title: t('Requests processed'),
        },
        {
          label: t('Tokens processed'),
          value: formatCompactNumber(data.total_tokens || 0),
          title: t('Tokens processed'),
        },
      ]
    : null

  if (isError || !stats) {
    return null
  }

  return (
    <div className='grid grid-cols-1 gap-6 pt-10 sm:grid-cols-3 md:gap-10'>
      {stats.map((s) => (
        <div
          key={s.label}
          className='flex flex-col items-center gap-1 text-center'
        >
          <span className='text-2xl font-bold tracking-tight md:text-3xl'>
            {s.value}
          </span>
          <span className='text-muted-foreground text-xs'>{s.label}</span>
        </div>
      ))}
    </div>
  )
}
