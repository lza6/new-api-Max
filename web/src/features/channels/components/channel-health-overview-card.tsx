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
import { Activity, HeartPulse, ShieldAlert, TrendingDown } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { IconBadge, type IconBadgeTone } from '@/components/ui/icon-badge'

import { aggregateHealthScores } from '../lib/channel-health'
import { useChannels } from './channels-provider'

/**
 * T2-2 渠道健康分聚合概览卡（纯前端、零请求）。
 * 数据源：ChannelsProvider 已注入的 healthScores（60s 缓存）。
 * 展示：平均健康分 / 可用率 / 最差渠道 / 冷却中渠道数；无数据时中性空态。
 */
export function ChannelHealthOverviewCard() {
  const { t } = useTranslation()
  const { healthScores, setOpen, setCurrentRow } = useChannels()
  const agg = aggregateHealthScores(healthScores)

  if (!agg.hasData) {
    return (
      <div className='grid grid-cols-1 rounded-lg border'>
        <div className='text-muted-foreground flex min-w-0 items-center gap-2 px-3 py-3 sm:px-5'>
          <HeartPulse className='size-4 shrink-0' />
          <span className='text-xs sm:text-sm'>{t('No health data yet')}</span>
          <span className='text-muted-foreground/60 text-[11px]'>{t('Health data refreshes automatically')}</span>
        </div>
      </div>
    )
  }

  let avgTone: IconBadgeTone = 'destructive'
  if (agg.avgScore >= 85) {
    avgTone = 'success'
  } else if (agg.avgScore >= 70) {
    avgTone = 'warning'
  }
  const stats: Array<{
    label: string
    value: string
    description: string
    icon: typeof Activity
    tone: IconBadgeTone
    onClick?: () => void
  }> = [
    {
      label: t('Average health score'),
      value: agg.avgScore.toFixed(0),
      description: t('Average score of sampled channels'),
      icon: HeartPulse,
      tone: avgTone,
    },
    {
      label: t('Available rate'),
      value: `${Math.round(agg.availableRate * 100)}%`,
      description: t('Share of sampled channels scoring 70+'),
      icon: Activity,
      tone: agg.availableRate >= 0.9 ? 'success' : 'warning',
    },
    {
      label: t('Worst channels'),
      value: agg.worst.length > 0 ? String(agg.worst[0].score) : '—',
      description: t('Lowest health score among channels'),
      icon: TrendingDown,
      tone: agg.worst.length > 0 ? 'destructive' : 'neutral',
      onClick: agg.worst.length > 0 ? () => {
        setCurrentRow({ id: agg.worst[0].channelId } as never)
        setOpen('update-channel')
      } : undefined,
    },
    {
      label: t('Channels cooling down'),
      value: String(agg.coolingCount),
      description: t('Channels currently in cooldown'),
      icon: ShieldAlert,
      tone: agg.coolingCount > 0 ? 'destructive' : 'success',
    },
  ]

  return (
    <div className='grid grid-cols-2 divide-x divide-y rounded-lg border sm:grid-cols-4 sm:divide-y-0'>
      {stats.map((item) => (
        <div
          key={item.label}
          className={`min-w-0 px-2.5 py-2.5 sm:px-4 sm:py-3 ${item.onClick ? 'cursor-pointer hover:bg-muted/40' : ''}`}
          onClick={item.onClick}
          role={item.onClick ? 'button' : undefined}
          tabIndex={item.onClick ? 0 : undefined}
          onKeyDown={item.onClick ? (e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); item.onClick?.() } } : undefined}
        >
          <div className='flex items-center gap-1.5 sm:gap-2'>
            <IconBadge tone={item.tone} size='stat'>
              <item.icon />
            </IconBadge>
            <div className='text-muted-foreground text-[11px] font-medium tracking-wider uppercase break-words sm:text-xs'>
              {item.label}
            </div>
          </div>
          <div className='text-foreground mt-1.5 font-mono text-sm font-bold tracking-tight tabular-nums sm:mt-2 sm:text-xl'>
            {item.value}
          </div>
          <div className='text-muted-foreground/60 mt-1 hidden text-xs md:block'>
            {item.description}
          </div>
        </div>
      ))}
    </div>
  )
}
