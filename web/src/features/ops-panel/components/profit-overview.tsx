import { useQuery } from '@tanstack/react-query'
import { TrendingUp, TriangleAlert } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Skeleton } from '@/components/ui/skeleton'
import { formatNumber, formatQuota } from '@/lib/format'
import { cn } from '@/lib/utils'

import { getOpsOverview } from '../api'
import { OPS_AVERAGE_DAYS, OPS_QUERY_KEY } from '../constants'
import type { OpsOverview } from '../types'

interface ProfitRow {
  key: string
  label: string
  revenue: number
  cost: number
  profit: number
  margin: number | null
}

function buildRows(d: OpsOverview, t: (k: string) => string): ProfitRow[] {
  return [
    {
      key: 'today',
      label: t('Today'),
      revenue: d.today_revenue_quota,
      cost: d.today_cost_quota,
      profit: d.today_profit_quota,
      margin: d.today_margin_percent,
    },
    {
      key: 'month',
      label: t('This Month'),
      revenue: d.month_revenue_quota,
      cost: d.month_cost_quota,
      profit: d.month_profit_quota,
      margin: d.month_margin_percent,
    },
    {
      key: 'year',
      label: t('This Year'),
      revenue: d.year_revenue_quota,
      cost: d.year_cost_quota,
      profit: d.year_profit_quota,
      margin: d.year_margin_percent,
    },
  ]
}

/**
 * 财务卡：营收 / 上游成本 / 毛利 / 毛利率（今日、本月、本年）。
 *
 * 成本来自每条请求结算时写入 `logs.cost_quota` 的**快照**：之后调整成本规则
 * 只追加历史行，不会改写旧日志 —— 因此调价前后的利润各自准确、可复现。
 * 未配置任何成本规则时成本恒为 0，必须显式提示，不能把利润当成真实值。
 */
export function ProfitOverview() {
  const { t } = useTranslation()
  const query = useQuery<OpsOverview>({
    queryKey: [OPS_QUERY_KEY, 'overview', OPS_AVERAGE_DAYS],
    queryFn: () => getOpsOverview(OPS_AVERAGE_DAYS),
    staleTime: 60_000,
    retry: false,
  })

  if (query.isLoading) {
    return (
      <div className='rounded-lg border p-4'>
        <Skeleton className='h-5 w-40' />
        <div className='mt-3 grid gap-3 sm:grid-cols-3'>
          <Skeleton className='h-24 w-full' />
          <Skeleton className='h-24 w-full' />
          <Skeleton className='h-24 w-full' />
        </div>
      </div>
    )
  }
  if (query.isError || !query.data) {
    return (
      <div className='text-destructive rounded-lg border p-4 text-sm'>
        {t('Failed to load')}
      </div>
    )
  }

  const d = query.data
  const rows = buildRows(d, t)
  const since = d.cost_tracking_since
    ? new Date(d.cost_tracking_since * 1000).toLocaleDateString()
    : ''

  return (
    <div className='overflow-hidden rounded-lg border'>
      <div className='flex flex-wrap items-center gap-2 border-b px-4 py-3 sm:px-5'>
        <TrendingUp className='text-primary size-4' />
        <h3 className='text-sm font-semibold'>{t('Profit & Cost')}</h3>
        <span className='text-muted-foreground text-xs'>
          {t('Revenue minus upstream cost. Costs are snapshotted per request.')}
        </span>
      </div>

      {!d.cost_configured && (
        <div className='text-warning flex items-start gap-2 border-b px-4 py-2 text-xs sm:px-5'>
          <TriangleAlert className='mt-0.5 size-4 shrink-0' />
          <span>
            {t(
              'No upstream cost rule is configured. Profit is calculated with cost = 0 and is NOT accurate until you add cost rules below.'
            )}
          </span>
        </div>
      )}
      {d.cost_configured &&
        d.today_cost_coverage_percent != null &&
        d.today_cost_coverage_percent < 99.9 && (
          <div className='text-warning flex items-start gap-2 border-b px-4 py-2 text-xs sm:px-5'>
            <TriangleAlert className='mt-0.5 size-4 shrink-0' />
            <span>
              {t(
                "Cost coverage: only {{percent}}% of today's revenue matched a cost rule. Unconfigured requests are counted at cost 0, so the margin is overstated.",
                { percent: formatNumber(d.today_cost_coverage_percent) }
              )}
            </span>
          </div>
        )}

      <div className='divide-border/60 grid gap-3 p-4 sm:grid-cols-3 sm:p-5'>
        {rows.map((row) => (
          <div key={row.key} className='bg-muted/30 rounded-lg border p-3'>
            <p className='text-muted-foreground text-xs'>{row.label}</p>
            <div className='mt-2 space-y-1 text-sm'>
              <div className='flex justify-between gap-2'>
                <span className='text-muted-foreground'>{t('Revenue')}</span>
                <span className='tabular-nums'>{formatQuota(row.revenue)}</span>
              </div>
              <div className='flex justify-between gap-2'>
                <span className='text-muted-foreground'>
                  {t('Upstream Cost')}
                </span>
                <span className='tabular-nums'>{formatQuota(row.cost)}</span>
              </div>
              <div className='flex justify-between gap-2 font-medium'>
                <span>{t('Gross Profit')}</span>
                <span
                  className={cn(
                    'tabular-nums',
                    row.profit < 0 && 'text-destructive'
                  )}
                >
                  {formatQuota(row.profit)}
                </span>
              </div>
              <div className='flex justify-between gap-2'>
                <span className='text-muted-foreground'>{t('Gross Margin')}</span>
                <span className='tabular-nums'>
                  {row.margin == null
                    ? t('N/A')
                    : `${formatNumber(row.margin)}%`}
                </span>
              </div>
            </div>
          </div>
        ))}
      </div>

      <div className='divide-border/60 text-muted-foreground grid gap-2 border-t px-4 py-3 text-xs sm:grid-cols-3 sm:px-5'>
        <div>
          <span>{t('Projected Monthly Profit')}: </span>
          <span
            className={cn(
              'text-foreground font-medium tabular-nums',
              d.projected_month_profit_quota < 0 && 'text-destructive'
            )}
          >
            {formatQuota(d.projected_month_profit_quota)}
          </span>
        </div>
        <div>
          <span>{t('Projected Yearly Profit')}: </span>
          <span
            className={cn(
              'text-foreground font-medium tabular-nums',
              d.projected_year_profit_quota < 0 && 'text-destructive'
            )}
          >
            {formatQuota(d.projected_year_profit_quota)}
          </span>
        </div>
        <div>
          <span>{t('Recommended Daily Upstream Top-up')}: </span>
          <span className='text-foreground font-medium tabular-nums'>
            {formatQuota(d.recommended_daily_topup_quota)}
          </span>
          <span className='ml-1'>
            {t('(average cost × 1.5 buffer)')}
          </span>
        </div>
      </div>

      {since && (
        <p className='text-muted-foreground border-t px-4 py-2 text-[11px] sm:px-5'>
          {t(
            'Cost tracking started at {{date}}. Requests before that date have no cost snapshot (cost = 0).',
            { date: since }
          )}
        </p>
      )}
    </div>
  )
}
