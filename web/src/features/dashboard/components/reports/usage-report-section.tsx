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
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  NativeSelect,
  NativeSelectOption,
} from '@/components/ui/native-select'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { useStatus } from '@/hooks/use-status'

import {
  exportUsageReportCsv,
  getUsageReport,
  type UsageReportData,
  type UsageReportGroupBy,
  type UsageReportRow,
} from '../../api'
import { useQuery } from '@tanstack/react-query'

const GROUP_BY_OPTIONS: { value: UsageReportGroupBy; labelKey: string }[] = [
  { value: 'model', labelKey: 'By Model' },
  { value: 'channel', labelKey: 'By Channel' },
  { value: 'day', labelKey: 'By Day' },
]

const RANGE_OPTIONS: { value: number; labelKey: string }[] = [
  { value: 1, labelKey: 'Last 24 hours' },
  { value: 7, labelKey: 'Last 7 days' },
  { value: 30, labelKey: 'Last 30 days' },
  { value: 90, labelKey: 'Last 90 days' },
]

function formatInt(n: number): string {
  return new Intl.NumberFormat().format(n)
}

function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) {
    return '0 B'
  }
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.min(
    Math.floor(Math.log(bytes) / Math.log(1024)),
    units.length - 1
  )
  return `${(bytes / 1024 ** i).toFixed(i === 0 ? 0 : 1)} ${units[i]}`
}

/**
 * 用量/成本报表：按模型/渠道/日聚合 consume 日志的请求数、token、额度与流量，
 * 支持 CSV 导出（管理员）。数据源为服务端 GROUP BY 聚合，避免全表扫描。
 */
export function UsageReportSection() {
  const { t } = useTranslation()
  const { status } = useStatus()
  const [groupBy, setGroupBy] = useState<UsageReportGroupBy>('model')
  const [days, setDays] = useState(30)
  const [exporting, setExporting] = useState(false)

  const { start, end } = useMemo(() => {
    const now = Math.floor(Date.now() / 1000)
    return { start: now - days * 24 * 3600, end: now }
  }, [days])

  const query = useQuery<UsageReportData>({
    queryKey: ['dashboard', 'usage-report', groupBy, start, end],
    queryFn: async () => {
      const res = await getUsageReport({ group_by: groupBy, start, end })
      if (!res.success || !res.data) {
        throw new Error(res.message || 'Failed to load usage report')
      }
      return res.data
    },
    staleTime: 60_000,
    retry: false,
  })

  const priceRate = Math.max(Number(status?.price ?? 1), 0.001)

  const handleExport = async () => {
    setExporting(true)
    try {
      const blob = await exportUsageReportCsv({
        group_by: groupBy,
        start,
        end,
      })
      const url = URL.createObjectURL(blob)
      const link = document.createElement('a')
      link.href = url
      link.download = `usage-report-${groupBy}-${new Date()
        .toISOString()
        .slice(0, 10)}.csv`
      document.body.append(link)
      link.click()
      link.remove()
      URL.revokeObjectURL(url)
    } finally {
      setExporting(false)
    }
  }

  const rows = query.data?.rows ?? []
  const totals = query.data?.totals

  const KEY_LABEL: Record<UsageReportGroupBy, string> = {
    model: t('Model'),
    channel: t('Channel'),
    day: t('Date'),
  }
  const keyLabel = KEY_LABEL[groupBy]

  return (
    <div className='space-y-3 sm:space-y-4'>
      <div className='flex flex-wrap items-center gap-2'>
        <NativeSelect
          value={groupBy}
          onChange={(e) => setGroupBy(e.target.value as UsageReportGroupBy)}
          aria-label={t('Group by')}
        >
          {GROUP_BY_OPTIONS.map((o) => (
            <NativeSelectOption key={o.value} value={o.value}>
              {t(o.labelKey)}
            </NativeSelectOption>
          ))}
        </NativeSelect>
        <NativeSelect
          value={String(days)}
          onChange={(e) => setDays(Number(e.target.value))}
          aria-label={t('Time range')}
        >
          {RANGE_OPTIONS.map((o) => (
            <NativeSelectOption key={o.value} value={String(o.value)}>
              {t(o.labelKey)}
            </NativeSelectOption>
          ))}
        </NativeSelect>
        <Button
          variant='outline'
          size='sm'
          onClick={handleExport}
          disabled={exporting || query.isLoading || rows.length === 0}
        >
          {t('Export CSV')}
        </Button>
      </div>

      {totals ? (
        <div className='text-muted-foreground flex flex-wrap gap-x-6 gap-y-1 text-xs'>
          <span>
            {t('Requests')}: <span className='text-foreground font-medium'>{formatInt(totals.requests)}</span>
          </span>
          <span>
            {t('Tokens')}: <span className='text-foreground font-medium'>{formatInt(totals.total_tokens)}</span>
          </span>
          <span>
            {t('Quota')}: <span className='text-foreground font-medium'>${(totals.quota / priceRate).toFixed(2)}</span>
          </span>
          <span>
            {t('Traffic')}: <span className='text-foreground font-medium'>{formatBytes(totals.total_bytes)}</span>
          </span>
        </div>
      ) : null}

      <div className='overflow-hidden rounded-lg border'>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{keyLabel}</TableHead>
              <TableHead className='text-right'>{t('Requests')}</TableHead>
              <TableHead className='text-right'>{t('Tokens')}</TableHead>
              <TableHead className='text-right'>{t('Quota')}</TableHead>
              <TableHead className='text-right'>{t('Traffic')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {renderReportBody(query, rows, priceRate, t)}
          </TableBody>
        </Table>
      </div>
    </div>
  )
}

const REPORT_SKELETON_KEYS = ['s1', 's2', 's3', 's4', 's5']

function renderReportBody(
  query: { isLoading: boolean; isError: boolean },
  rows: UsageReportRow[],
  priceRate: number,
  t: (key: string) => string
) {
  if (query.isLoading) {
    return REPORT_SKELETON_KEYS.map((key) => (
      <TableRow key={key}>
        <TableCell colSpan={5}>
          <Skeleton className='h-6 w-full' />
        </TableCell>
      </TableRow>
    ))
  }
  if (query.isError) {
    return (
      <TableRow>
        <TableCell
          colSpan={5}
          className='text-muted-foreground py-8 text-center text-sm'
        >
          {t('Failed to load usage report')}
        </TableCell>
      </TableRow>
    )
  }
  if (rows.length === 0) {
    return (
      <TableRow>
        <TableCell
          colSpan={5}
          className='text-muted-foreground py-8 text-center text-sm'
        >
          {t('No usage data in the selected range')}
        </TableCell>
      </TableRow>
    )
  }
  return rows.map((row) => (
    <TableRow key={row.key}>
      <TableCell className='font-mono text-xs'>{row.key}</TableCell>
      <TableCell className='text-right tabular-nums'>
        {formatInt(row.requests)}
      </TableCell>
      <TableCell className='text-right tabular-nums'>
        {formatInt(row.total_tokens)}
      </TableCell>
      <TableCell className='text-right tabular-nums'>
        ${(row.quota / priceRate).toFixed(4)}
      </TableCell>
      <TableCell className='text-right tabular-nums'>
        {formatBytes(row.total_bytes)}
      </TableCell>
    </TableRow>
  ))
}
