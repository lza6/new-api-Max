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
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { api } from '@/lib/api'

export interface SiteSubscriptionStats {
  total_plans: number
  total_subscriptions: number
  active_subscriptions: number
  expiring_soon_7d: number
  new_last_30d: number
  by_plan: { plan_id: number; title: string; total: number; active: number }[]
}

function StatItem({ label, value }: { label: string; value: number }) {
  return (
    <div className='flex flex-col gap-0.5'>
      <span className='text-muted-foreground/70 text-[11px]'>{label}</span>
      <span className='tabular-nums text-lg font-semibold'>{value}</span>
    </div>
  )
}

/**
 * T15-A: 公开只读站点订阅统计（聚合计数，无用户/订单明细）。
 * 端点不可用（如数据库异常）时静默不渲染，不影响定价页主功能。
 */
export function SiteSubscriptionStatsCard() {
  const { t } = useTranslation()
  const [stats, setStats] = useState<SiteSubscriptionStats | null>(null)

  useEffect(() => {
    let cancelled = false
    api
      .get('/v1/stats/subscriptions')
      .then((r) => {
        if (!cancelled && r.data?.data) {
          setStats(r.data.data as SiteSubscriptionStats)
        }
      })
      .catch(() => {
        // Public read-only; failing silently keeps the pricing page functional.
      })
    return () => {
      cancelled = true
    }
  }, [])

  if (!stats) {return null}

  const plans = stats.by_plan ?? []

  return (
    <div className='mx-auto mb-6 w-full max-w-2xl'>
      <div className='bg-background/60 flex flex-wrap items-start justify-between gap-4 rounded-2xl border px-4 py-3 shadow-card'>
        <div className='flex flex-wrap gap-6'>
          <StatItem
            label={t('Active subscriptions')}
            value={stats.active_subscriptions}
          />
          <StatItem
            label={t('Total subscriptions')}
            value={stats.total_subscriptions}
          />
          <StatItem label={t('Total plans')} value={stats.total_plans} />
          <StatItem
            label={t('Expiring soon (7 days)')}
            value={stats.expiring_soon_7d}
          />
          <StatItem
            label={t('New subscriptions (30 days)')}
            value={stats.new_last_30d}
          />
        </div>
        {plans.length > 0 && (
          <div className='text-muted-foreground/70 min-w-[9rem] space-y-1 text-[11px]'>
            <div className='font-medium text-foreground/70'>
              {t('By plan')}
            </div>
            {plans.slice(0, 5).map((plan) => (
              <div
                key={plan.plan_id}
                className='flex items-center justify-between gap-3'
              >
                <span className='truncate'>{plan.title || `#${plan.plan_id}`}</span>
                <span className='tabular-nums'>{plan.active}</span>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}
