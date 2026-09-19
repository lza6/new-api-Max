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
import {
  Area,
  AreaChart,
  Bar,
  BarChart,
  CartesianGrid,
  XAxis,
  YAxis,
} from 'recharts'

import { Main } from '@/components/layout'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
} from '@/components/ui/chart'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import {
  formatCompactNumber,
  formatQuota,
  formatTimestampToDate,
} from '@/lib/format'

import { getProfileInsights } from './api'
import type { ProfileInsights, Suggestion } from './types'

const SKELETON_CARD_IDS = ['a', 'b', 'c', 'd']

const modelConfig = {
  quota: { label: 'Quota', color: 'hsl(var(--chart-1))' },
}
const trendConfig = {
  quota: { label: 'Quota', color: 'hsl(var(--chart-2))' },
}
const hourConfig = {
  requests: { label: 'Requests', color: 'hsl(var(--chart-3))' },
}

function StatCard(props: { label: string; value: string; hint?: string }) {
  return (
    <Card>
      <CardHeader>
        <CardDescription className='text-xs'>{props.label}</CardDescription>
      </CardHeader>
      <CardContent>
        <p className='text-2xl font-semibold tracking-tight'>{props.value}</p>
        {props.hint ? (
          <p className='text-muted-foreground mt-1 text-xs'>{props.hint}</p>
        ) : null}
      </CardContent>
    </Card>
  )
}

function SuggestionText(props: { suggestion: Suggestion }) {
  const { t } = useTranslation()
  switch (props.suggestion.code) {
    case 'night_usage':
      return t('You are most active around {{hour}}:00 at night', {
        hour: props.suggestion.args?.hour,
      })
    case 'concentrated_model':
      return t('{{share}}% of your usage is concentrated on {{model}}', {
        share: props.suggestion.args?.share,
        model: props.suggestion.args?.model,
      })
    case 'high_error_channel':
      return t('Channel {{channel}} has a high error rate ({{rate}}%)', {
        channel: props.suggestion.args?.channel,
        rate: props.suggestion.args?.rate,
      })
    case 'recent_spike':
      return t(
        'The last 7 days account for {{percent}}% of your 30-day usage',
        { percent: props.suggestion.args?.percent }
      )
    default:
      return t('No recent usage in the last 30 days')
  }
}

function OverviewGrid(props: { win: ProfileInsights['overview_30d']; prefix: string }) {
  const { t } = useTranslation()
  const win = props.win
  const cards = [
    {
      label: t(props.prefix === '7d' ? 'Last 7 days requests' : 'Last 30 days requests'),
      value: formatCompactNumber(win.requests),
      hint: `${formatCompactNumber(win.errors)} ${t('Errors')}`,
    },
    {
      label: t(props.prefix === '7d' ? 'Last 7 days tokens' : 'Last 30 days tokens'),
      value: formatCompactNumber(win.tokens),
    },
    {
      label: t(props.prefix === '7d' ? 'Last 7 days quota' : 'Last 30 days quota'),
      value: formatQuota(win.quota),
    },
    {
      label: t(props.prefix === '7d' ? 'Last 7 days error rate' : 'Last 30 days error rate'),
      value: `${Math.round((win.error_rate ?? 0) * 100)}%`,
      hint: `${win.channels} ${t('Channels')}`,
    },
  ]
  return (
    <div className='grid gap-4 sm:grid-cols-2 lg:grid-cols-4'>
      {cards.map((s) => (
        <StatCard key={s.label} {...s} />
      ))}
    </div>
  )
}

function EmptyState(props: { text: string }) {
  const { t } = useTranslation()
  return <p className='text-muted-foreground text-sm'>{t(props.text)}</p>
}

export function ProfileInsightsPage() {
  const { t } = useTranslation()
  const query = useQuery({
    queryKey: ['profile', 'insights'],
    queryFn: getProfileInsights,
  })
  const data: ProfileInsights | undefined = query.data

  if (query.isLoading) {
    return (
      <Main>
        <div className='min-h-0 flex-1 overflow-auto px-3 py-3 sm:px-4 sm:py-6'>
          <div className='mx-auto flex w-full max-w-7xl flex-col gap-4 sm:gap-6'>
            <div className='space-y-1'>
              <h1 className='text-2xl font-semibold tracking-tight'>{t('Usage Insights')}</h1>
            </div>
            <div className='grid gap-4 sm:grid-cols-2 lg:grid-cols-4'>
              {SKELETON_CARD_IDS.map((id) => (
                <Card key={id}>
                  <CardContent className='space-y-3 p-4'>
                    <Skeleton className='h-3.5 w-24' />
                    <Skeleton className='h-7 w-28' />
                  </CardContent>
                </Card>
              ))}
            </div>
          </div>
        </div>
      </Main>
    )
  }

  if (query.isError || !data) {
    return (
      <Main>
        <div className='min-h-0 flex-1 overflow-auto px-3 py-3 sm:px-4 sm:py-6'>
          <div className='mx-auto flex w-full max-w-7xl flex-col gap-4 sm:gap-6'>
            <h1 className='text-2xl font-semibold tracking-tight'>{t('Usage Insights')}</h1>
            <Card>
              <CardContent className='p-4'>
                <p className='text-muted-foreground text-sm'>
                  {t('Failed to load usage insights')}
                </p>
              </CardContent>
            </Card>
          </div>
        </div>
      </Main>
    )
  }

  return (
    <Main>
      <div className='min-h-0 flex-1 overflow-auto px-3 py-3 sm:px-4 sm:py-6'>
        <div className='mx-auto flex w-full max-w-7xl flex-col gap-4 sm:gap-6'>
          <div className='space-y-1'>
            <h1 className='text-2xl font-semibold tracking-tight'>
              {t('Usage Insights')}
            </h1>
            <p className='text-muted-foreground text-sm'>
              {t('Usage insights description')}
            </p>
          </div>

          <OverviewGrid win={data.overview_7d} prefix='7d' />
          <OverviewGrid win={data.overview_30d} prefix='30d' />

          {data.suggestions.length > 0 && (
            <Card>
              <CardHeader>
                <CardTitle>{t('Suggestions')}</CardTitle>
              </CardHeader>
              <CardContent className='space-y-2'>
                {data.suggestions.map((s) => (
                  <p key={s.code} className='text-muted-foreground text-sm'>
                    <SuggestionText suggestion={s} />
                  </p>
                ))}
              </CardContent>
            </Card>
          )}

          <Card>
            <CardHeader>
              <CardTitle>{t('Model usage')}</CardTitle>
              <CardDescription>{t('Model usage description')}</CardDescription>
            </CardHeader>
            <CardContent>
              {data.model_usage.length === 0 ? (
                <EmptyState text='No data yet' />
              ) : (
                <ChartContainer config={modelConfig} className='h-[260px]'>
                  <BarChart data={data.model_usage} accessibilityLayer>
                    <CartesianGrid vertical={false} />
                    <XAxis dataKey='model' tickLine={false} axisLine={false} tick={{ fontSize: 11 }} />
                    <YAxis tickLine={false} axisLine={false} width={44} />
                    <ChartTooltip content={<ChartTooltipContent />} />
                    <Bar dataKey='quota' fill='var(--color-quota)' radius={4} />
                  </BarChart>
                </ChartContainer>
              )}
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>{t('Cost trend')}</CardTitle>
              <CardDescription>{t('Cost trend description')}</CardDescription>
            </CardHeader>
            <CardContent>
              <ChartContainer config={trendConfig} className='h-[240px]'>
                <AreaChart data={data.trend} accessibilityLayer>
                  <CartesianGrid vertical={false} />
                  <XAxis dataKey='date' tickLine={false} axisLine={false} tick={{ fontSize: 11 }} />
                  <YAxis tickLine={false} axisLine={false} width={44} />
                  <ChartTooltip content={<ChartTooltipContent />} />
                  <Area
                    dataKey='quota'
                    type='monotone'
                    fill='var(--color-quota)'
                    fillOpacity={0.3}
                    stroke='var(--color-quota)'
                  />
                </AreaChart>
              </ChartContainer>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>{t('Active hours')}</CardTitle>
              <CardDescription>{t('Active hours description')}</CardDescription>
            </CardHeader>
            <CardContent>
              <ChartContainer config={hourConfig} className='h-[200px]'>
                <BarChart data={data.time_heatmap} accessibilityLayer>
                  <CartesianGrid vertical={false} />
                  <XAxis
                    dataKey='hour'
                    tickLine={false}
                    axisLine={false}
                    tickFormatter={(h: number) => `${h}:00`}
                    tick={{ fontSize: 11 }}
                  />
                  <YAxis tickLine={false} axisLine={false} width={40} />
                  <ChartTooltip cursor={false} content={<ChartTooltipContent hideLabel />} />
                  <Bar dataKey='requests' fill='var(--color-requests)' radius={2} />
                </BarChart>
              </ChartContainer>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>{t('Channel usage')}</CardTitle>
            </CardHeader>
            <CardContent>
              {data.channels.length === 0 ? (
                <EmptyState text='No data yet' />
              ) : (
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>{t('Channel')}</TableHead>
                      <TableHead>{t('Requests')}</TableHead>
                      <TableHead>{t('Errors')}</TableHead>
                      <TableHead>{t('Error rate')}</TableHead>
                      <TableHead>{t('Quota')}</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {data.channels.map((ch) => (
                      <TableRow key={ch.channel_id}>
                        <TableCell className='font-medium'>{ch.channel_name}</TableCell>
                        <TableCell>{formatCompactNumber(ch.requests)}</TableCell>
                        <TableCell>{formatCompactNumber(ch.errors)}</TableCell>
                        <TableCell>{Math.round((ch.error_rate ?? 0) * 100)}%</TableCell>
                        <TableCell>{formatQuota(ch.quota)}</TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              )}
            </CardContent>
          </Card>

          <p className='text-muted-foreground text-xs'>
            {t('Generated at')} {formatTimestampToDate(data.generated_at)} ·{' '}
            {data.evidence.source} · {data.evidence.samples} {t('Samples')} ·{' '}
            {t('Last 30 days')}
          </p>
        </div>
      </div>
    </Main>
  )
}