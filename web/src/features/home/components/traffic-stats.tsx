/*
Copyright (C) 2023-2026 QuantumNous
Licensed under GNU AGPL v3. See LICENSE.
*/
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { Skeleton } from '@/components/ui/skeleton'
import { formatCompactNumber, formatPercent } from '@/lib/format'
import { requireServerSuccess } from '@/lib/server-error-message'

import { getSiteVisitorSummary } from '../api'
import type { SiteVisitorSummary, TrafficSource } from '../types'

/**
 * Visitor / traffic statistics for the marketing home page.
 *
 * ## 口径（必须真实，因此这里刻意把口径写进注释与 UI）
 *
 * - **一个 IP 一天只算一次**：后端用 `(day, ip_hash)` 唯一键去重（`model.SiteDailyVisitor`），
 *   所以「今日访客 / 近 7 天 / 近 30 天 / 累计」都是**去重后的人头数**，不是请求数。
 * - **排除爬虫**：UA 判定为机器人/自动化工具的行被排除在全部展示口径之外
 *   （爬虫行仍入库，但不参与展示）。
 * - **IP 已匿名化**：后端只存 `HMAC-SHA256(secret, day + "|" + ip)`，本组件拿到的
 *   是纯聚合计数 —— 响应里没有任何 IP、哈希或用户标识。
 * - **DAU/MAU 口径是「有 API 消费」**：不是登录数。这一点在 UI 文案里也如实标明，
 *   否则把「打开过后台」当成「活跃」会让数字虚高。
 *
 * ## 三态
 *
 * loading → 骨架；API 失败 → **整块隐藏**（return null）。理由与既有 `SiteStats` 一致：
 * 访客数是营销信息而非用户数据，读不到时静默隐藏比在落地页弹错误更合适。
 * 但「隐藏」只针对**整块**，不影响页面上任何功能性内容。
 */
export function TrafficStats() {
  const { t } = useTranslation()
  const { data, isLoading, isError } = useQuery({
    queryKey: ['site-visitors'],
    queryFn: async () =>
      requireServerSuccess(await getSiteVisitorSummary()).data || null,
    staleTime: 60_000,
    refetchOnWindowFocus: false,
  })

  if (isLoading) {
    return <TrafficStatsSkeleton />
  }
  // `isError` 与「响应结构不完整」走同一出口：都不显示，避免半截数字误导访客。
  if (isError || !data || !data.visitors) {
    return null
  }

  return <TrafficStatsBody data={data} label={t as (k: string) => string} />
}

interface TrafficStatsBodyProps {
  data: SiteVisitorSummary
  label: (key: string) => string
}

function TrafficStatsBody({ data, label }: TrafficStatsBodyProps) {
  const v = data.visitors
  const cards = [
    {
      key: 'today',
      value: v.today,
      title: label('Visitors today'),
      hint: label('Unique visitors, one per IP per day'),
    },
    {
      key: 'd30',
      value: v.last_30d,
      title: label('Visitors in the last 30 days'),
      hint: label('Deduplicated by IP and day'),
    },
    {
      key: 'total',
      value: v.total,
      title: label('Total visits'),
      hint: label('Accumulated deduplicated visitors'),
    },
    {
      key: 'pv',
      value: v.page_views_total,
      title: label('Page views'),
      hint: label('Total page views'),
    },
    {
      key: 'users',
      value: data.registered_users,
      title: label('Registered users'),
      hint: label('Accounts registered on this site'),
    },
  ]

  const sourceRows = normalizeSources(data.sources)

  return (
    <div className='pt-10'>
      <div className='grid grid-cols-2 gap-x-6 gap-y-8 md:grid-cols-5 md:gap-x-8'>
        {cards.map((c) => (
          <div
            key={c.key}
            className='flex flex-col items-center gap-1.5 text-center'
            title={c.hint}
          >
            <span className='font-serif text-3xl font-medium tracking-tight tabular-nums md:text-4xl'>
              {formatCompactNumber(c.value || 0)}
            </span>
            <span className='text-muted-foreground/70 text-[11px] tracking-wide'>
              {c.title}
            </span>
          </div>
        ))}
      </div>

      {sourceRows.length > 0 && <TrafficSources rows={sourceRows} label={label} />}
    </div>
  )
}

interface TrafficSourcesProps {
  rows: Array<TrafficSource & { percent: number }>
  label: (key: string) => string
}

/**
 * Traffic-source breakdown with percentages.
 *
 * Percentages are computed here from the returned counts rather than trusted
 * from the API: the counts are the raw facts, and deriving the percentage in one
 * place keeps the bar widths and the printed number from ever disagreeing.
 *
 * Bars are pure CSS (a filled div inside a track) instead of a chart library —
 * five rows do not justify pulling a charting bundle onto the landing page.
 */
function TrafficSources({ rows, label }: TrafficSourcesProps) {
  return (
    <div className='mt-10 border-t border-border/40 pt-8'>
      <p className='text-muted-foreground/70 mb-5 text-center text-[11px] font-medium tracking-[0.28em] uppercase'>
        {label('Traffic sources (30 days)')}
      </p>
      <ul className='mx-auto flex max-w-2xl flex-col gap-3'>
        {rows.map((row) => (
          <li key={row.source} className='flex items-center gap-3'>
            <span className='w-24 shrink-0 text-right text-xs text-muted-foreground'>
              {label(sourceLabelKey(row.source))}
            </span>
            <span
              className='relative h-2 flex-1 overflow-hidden rounded-full bg-muted'
              role='img'
              aria-label={`${label(sourceLabelKey(row.source))} ${formatPercent(row.percent)}`}
            >
              <span
                className='absolute inset-y-0 left-0 rounded-full bg-primary/70'
                style={{ width: `${Math.max(row.percent, 1)}%` }}
              />
            </span>
            <span className='w-14 shrink-0 text-right text-xs tabular-nums text-muted-foreground'>
              {formatPercent(row.percent)}
            </span>
          </li>
        ))}
      </ul>
      <p className='text-muted-foreground/50 mt-4 text-center text-[11px]'>
        {label('Classified by the referrer on the first visit of each day')}
      </p>
    </div>
  )
}

/**
 * Turn raw counts into rows with percentages.
 *
 * Returns `[]` when there is nothing to show so the whole block is skipped
 * instead of rendering five 0% bars. Percentages are rounded to one decimal and
 * may therefore not add up to exactly 100 — that is honest rounding, not an error.
 */
function normalizeSources(
  sources: TrafficSource[] | undefined | null
): Array<TrafficSource & { percent: number }> {
  if (!sources || sources.length === 0) {
    return []
  }
  const total = sources.reduce((sum, s) => sum + (s.count || 0), 0)
  if (total <= 0) {
    return []
  }
  const rows = sources.map((s) => ({
    source: s.source,
    count: s.count || 0,
    percent: ((s.count || 0) / total) * 100,
  }))
  // 后端已按 count DESC 排序，这里再排一次以防将来后端顺序变化导致 UI 抖动。
  return rows.sort((a, b) => b.count - a.count)
}

/** Map a backend source key to its translation key. */
function sourceLabelKey(source: string): string {
  switch (source) {
    case 'search':
      return 'Search engines'
    case 'social':
      return 'Social & communities'
    case 'referral':
      return 'Other referrals'
    default:
      return 'Direct'
  }
}

function TrafficStatsSkeleton() {
  return (
    <div className='pt-10'>
      <div className='grid grid-cols-2 gap-x-6 gap-y-8 md:grid-cols-5 md:gap-x-8'>
        {[0, 1, 2, 3, 4].map((i) => (
          <div key={i} className='flex flex-col items-center gap-1.5'>
            <Skeleton className='h-8 w-20 rounded-md' />
            <Skeleton className='h-4 w-24 rounded-md' />
          </div>
        ))}
      </div>
    </div>
  )
}
