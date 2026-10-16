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
import type { TFunction } from 'i18next'
import {
  CalendarClock,
  Coins,
  Hourglass,
  TrendingDown,
  Wallet,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { IconBadge, type IconBadgeTone } from '@/components/ui/icon-badge'
import { Skeleton } from '@/components/ui/skeleton'
import { formatNumber, formatQuota } from '@/lib/format'
import { cn } from '@/lib/utils'

import { getOpsOverview } from '../api'
import { OPS_AVERAGE_DAYS, OPS_QUERY_KEY } from '../constants'
import type { OpsOverview } from '../types'

interface OverviewCard {
  key: string
  label: string
  value: string
  fullValue: string
  desc: string
  icon: typeof Coins
  tone: IconBadgeTone
}

function buildCards(overview: OpsOverview, t: TFunction): OverviewCard[] {
  const daysLabel =
    overview.estimated_remaining_days == null
      ? t('N/A')
      : formatNumber(Math.round(overview.estimated_remaining_days))

  return [
    {
      key: 'today',
      label: t("Today's Consumption"),
      value: formatQuota(overview.today_quota),
      fullValue: formatQuota(overview.today_quota),
      // 明白标注时区：同一面板上的采集统计用的是 UTC 日，
      // 不标注会让运营者以为两个"今日"是同一口径（盲点 B15）。
      desc: overview.today_quota_timezone
        ? t('Today ({{tz}} time)', { tz: overview.today_quota_timezone })
        : t('Today'),
      icon: CalendarClock,
      tone: 'info',
    },
    {
      key: 'total',
      label: t('Total Consumption'),
      value: formatQuota(overview.total_quota),
      fullValue: formatQuota(overview.total_quota),
      desc: t('Total'),
      icon: Coins,
      tone: 'primary',
    },
    {
      key: 'remaining',
      label: t('Total Remaining Quota (Excluding Admins)'),
      value: formatQuota(overview.total_remaining_quota),
      fullValue: formatQuota(overview.total_remaining_quota),
      desc: `${t('Users')}: ${formatNumber(overview.user_count)}`,
      icon: Wallet,
      tone: 'success',
    },
    {
      key: 'average',
      label: t('Daily Average Consumption'),
      value: formatQuota(overview.daily_average_quota),
      fullValue: formatQuota(overview.daily_average_quota),
      desc: t('Averaged over the last {{days}} days', {
        days: overview.average_over_days,
      }),
      icon: TrendingDown,
      tone: 'warning',
    },
    {
      key: 'days',
      label: t('Estimated Days Remaining'),
      value: daysLabel,
      fullValue: daysLabel,
      desc: t('Remaining quota ÷ daily average consumption'),
      icon: Hourglass,
      tone: 'chart-4',
    },
  ]
}

/**
 * 运营概览卡片：当日/累计消耗、非管理员用户剩余额度、日均消耗、估算可运营天数。
 * quota 值统一经 formatQuota 换算为站点展示货币。
 */
export function OpsOverviewCards() {
  const { t } = useTranslation()
  const query = useQuery<OpsOverview>({
    queryKey: [OPS_QUERY_KEY, 'overview', OPS_AVERAGE_DAYS],
    queryFn: () => getOpsOverview(OPS_AVERAGE_DAYS),
    staleTime: 60_000,
    retry: false,
  })

  const cards = query.data ? buildCards(query.data, t) : []
  const showSkeleton = query.isLoading
  const showError = query.isError

  return (
    <div className='overflow-hidden rounded-lg border'>
      <div className='divide-border/60 grid grid-cols-2 divide-x sm:grid-cols-3 lg:grid-cols-5'>
        {showSkeleton || showError
          ? CARD_PLACEHOLDERS.map((key, index) => (
              <div
                key={key}
                className={cn(
                  'min-w-0 px-2.5 py-1.5 sm:px-5 sm:py-4',
                  index === CARD_PLACEHOLDERS.length - 1 &&
                    'col-span-2 sm:col-span-1'
                )}
              >
                <Skeleton className='h-4 w-24' />
                {showError ? (
                  <div className='text-muted-foreground mt-1 font-mono text-base leading-tight font-bold tracking-tight tabular-nums sm:mt-2 sm:text-2xl sm:leading-normal'>
                    --
                  </div>
                ) : (
                  <Skeleton className='mt-1 h-5 w-20 sm:mt-2 sm:h-7' />
                )}
              </div>
            ))
          : cards.map((card) => {
              const Icon = card.icon
              return (
                <div
                  key={card.key}
                  className='min-w-0 px-2.5 py-1.5 sm:px-5 sm:py-4'
                >
                  <div className='flex min-w-0 items-center gap-1.5 sm:gap-2'>
                    <IconBadge
                      tone={card.tone}
                      size='stat'
                      className='size-4 rounded-sm sm:size-7 sm:rounded-md'
                    >
                      <Icon />
                    </IconBadge>
                    <div className='text-muted-foreground truncate text-[11px] leading-4 font-medium tracking-wide uppercase sm:text-xs sm:tracking-wider'>
                      {card.label}
                    </div>
                  </div>
                  <div
                    className='text-foreground mt-1 max-w-full truncate font-mono text-base leading-tight font-bold tracking-tight tabular-nums sm:mt-2 sm:text-2xl sm:leading-normal'
                    title={card.fullValue}
                  >
                    {card.value}
                  </div>
                  <div className='text-muted-foreground/60 mt-1 hidden text-xs md:block'>
                    {card.desc}
                  </div>
                </div>
              )
            })}
      </div>
    </div>
  )
}

const CARD_PLACEHOLDERS = [
  'today',
  'total',
  'remaining',
  'average',
  'days',
] as const
