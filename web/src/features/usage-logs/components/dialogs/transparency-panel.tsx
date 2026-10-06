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
// T3 可读输出/解释页（answer-me-with-html 迁移）：把一条请求日志的原理用
// 可视化 HTML 呈现——「这次请求花了多久、慢在哪、省了多少流量、是否命中缓存」，
// 分级展示（白话版人人可读，技术版含结构化诊断）。纯展示，无副作用。
import { useQuery } from '@tanstack/react-query'
import { Clock, Gauge, HardDrive, Zap } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'

import { getLogTransparency } from '../../api'

interface TransparencyPanelProps {
  logId: number
}

function formatMs(v?: number): string {
  if (v === undefined || v < 0) {return '—'}
  if (v < 1000) {return `${v} ms`}
  return `${(v / 1000).toFixed(2)} s`
}

function formatBytes(v?: number): string {
  if (v === undefined || v <= 0) {return '—'}
  const units = ['B', 'KB', 'MB', 'GB']
  let n = v
  let i = 0
  while (n >= 1024 && i < units.length - 1) {
    n /= 1024
    i++
  }
  return `${n.toFixed(n < 10 && i > 0 ? 1 : 0)} ${units[i]}`
}

export function TransparencyPanel({ logId }: TransparencyPanelProps) {
  const { t } = useTranslation()
  const { data, isLoading } = useQuery({
    queryKey: ['log-transparency', logId],
    queryFn: () => getLogTransparency(logId),
    enabled: Number.isFinite(logId) && logId > 0,
  })

  if (isLoading) {
    return <Skeleton className='h-24 w-full rounded-md' />
  }
  const hasPlain = data?.plain && Object.keys(data.plain).length > 0
  if (!hasPlain) {
    return (
      <p className='text-muted-foreground text-sm'>
        {t('No explanation available for this request.')}
      </p>
    )
  }

  const p = data.plain
  const cachePct =
    p.cache_hit_rate !== undefined
      ? `${(p.cache_hit_rate * 100).toFixed(1)}%`
      : '—'

  const stats: { icon: React.ReactNode; label: string; value: string }[] = [
    {
      icon: <Clock className='size-4' />,
      label: t('First token'),
      value: formatMs(p.first_token_ms),
    },
    {
      icon: <Gauge className='size-4' />,
      label: t('Upstream wait'),
      value: formatMs(p.upstream_wait_ms),
    },
    {
      icon: <Zap className='size-4' />,
      label: t('Cache hit'),
      value: cachePct,
    },
    {
      icon: <HardDrive className='size-4' />,
      label: t('Traffic'),
      value: `${formatBytes(p.request_bytes)} → ${formatBytes(p.response_bytes)}`,
    },
  ]

  return (
    <Card>
      <CardHeader className='pb-2'>
        <CardTitle className='text-sm'>{t('Request Explanation')}</CardTitle>
      </CardHeader>
      <CardContent className='space-y-3'>
        <p className='text-muted-foreground text-sm'>{p.summary}</p>
        <div className='grid grid-cols-2 gap-3 sm:grid-cols-4'>
          {stats.map((s) => (
            <div key={s.label} className='space-y-1'>
              <div className='text-muted-foreground flex items-center gap-1.5 text-xs'>
                {s.icon}
                {s.label}
              </div>
              <div className='font-mono text-sm'>{s.value}</div>
            </div>
          ))}
        </div>
        {p.tokens !== undefined && (
          <div className='text-muted-foreground text-xs'>
            {t('Tokens')}: {p.prompt_tokens ?? 0} + {p.completion_tokens ?? 0} ={' '}
            {p.tokens}
          </div>
        )}
      </CardContent>
    </Card>
  )
}
