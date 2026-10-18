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
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import { CalendarDays } from 'lucide-react'

import {
  StaticDataTable,
  type StaticDataTableColumn,
} from '@/components/data-table'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { formatNumber } from '@/lib/format'

import { getTrainingDataByDate } from '../api'
import { formatBytes } from '../lib/format'
import type { DailyCaptureStat } from '../types'

/**
 * 每日采集历史（用户明确要求「管理员可以看到几月几日采集的」）。
 *
 * ## 数据来源与口径（界面上必须说清，否则数字会被误读）
 *
 * 本卡的数字是**云盘目录树聚合**出来的（一级目录就是日期）：
 *  - 它是**持久且真实**的落盘记录，可回溯任意历史日期、重启不丢；
 *  - 但它只反映**已上传成功**的量 —— 上传失败或还在队列里的不会出现在这里。
 *
 * 与「今日统计」卡（进程内计数）的区别：那张卡实时、含未上传量，但重启归零、
 * 只有今天。两个数字**本就允许不完全一致**，因此卡片里写明来源，
 * 避免管理员把差异当成 bug。
 */
export function DailyHistoryCard() {
  const { t } = useTranslation()

  const query = useQuery({
    queryKey: ['training-data', 'by-date'],
    queryFn: async () => {
      const res = await getTrainingDataByDate()
      if (!res.success) {
        throw new Error(res.message || t('Failed to load'))
      }
      return res.data
    },
    // 采集是后台异步写云盘的，历史不会秒变；30s 足够且省云盘 API 配额。
    refetchInterval: 30_000,
  })

  const columns = useMemo<StaticDataTableColumn<DailyCaptureStat>[]>(
    () => [
      {
        id: 'date',
        header: t('Date'),
        cell: (row) => <span className='tabular-nums'>{row.date}</span>,
      },
      {
        id: 'users',
        header: t('Users'),
        className: 'text-right',
        cellClassName: 'text-right',
        cell: (row) => <span className='tabular-nums'>{formatNumber(row.users)}</span>,
      },
      {
        id: 'models',
        header: t('Models'),
        className: 'text-right',
        cellClassName: 'text-right',
        cell: (row) => <span className='tabular-nums'>{formatNumber(row.models)}</span>,
      },
      {
        id: 'parts',
        header: t('Parts'),
        className: 'text-right',
        cellClassName: 'text-right',
        cell: (row) => <span className='tabular-nums'>{formatNumber(row.parts)}</span>,
      },
      {
        id: 'bytes',
        header: t('Size'),
        className: 'text-right',
        cellClassName: 'text-right',
        cell: (row) => <span className='tabular-nums'>{formatBytes(row.bytes)}</span>,
      },
    ],
    [t]
  )

  // 最近的在最上面：管理员看「今天/昨天」远比看月初频繁。
  // `days` 每次渲染都是新数组（`?? []` 会造新引用），故用 data 本身做依赖。
  const data = query.data
  const ordered = useMemo(
    () => [...(data?.days ?? [])].reverse(),
    [data]
  )

  const truncated = data?.daily_truncated === true
  const err = data?.daily_error

  return (
    <Card>
      <CardHeader>
        <CardTitle className='flex items-center gap-2 text-sm'>
          <CalendarDays className='h-4 w-4' />
          {t('Daily capture history')}
        </CardTitle>
      </CardHeader>
      <CardContent className='space-y-3'>
        {renderBody()}
      </CardContent>
    </Card>
  )

  function renderBody() {
    if (query.isLoading) {
      return (
        <div className='space-y-2'>
          <Skeleton className='h-8 w-full' />
          <Skeleton className='h-8 w-full' />
        </div>
      )
    }
    if (query.isError) {
      // 不静默：云盘读不到时明确说出来，而不是显示空表让管理员以为没数据。
      return (
        <p className='text-destructive text-xs'>
          {query.error?.message || t('Failed to load')}
        </p>
      )
    }
    return (
      <>
        <StaticDataTable
          columns={columns}
          data={ordered}
          getRowKey={(row) => row.date}
          emptyContent={
            // 云盘读取失败时**不能**同时说「还没有数据」：那会把
            // 「读失败」误读成「没数据」，管理员会据此以为采集停了。
            err ? (
              <p className='text-destructive py-6 text-center text-sm'>
                {t('Cloud drive read failed')}
              </p>
            ) : (
              <p className='text-muted-foreground py-6 text-center text-sm'>
                {t('No captured data yet')}
              </p>
            )
          }
        />
        {err && (
          <p className='text-destructive text-xs'>
            {t('Cloud drive read failed')}: {err}
          </p>
        )}
        {truncated && (
          <p className='text-muted-foreground text-xs'>
            {t(
              'The cloud drive listing was truncated, so these totals are a lower bound.'
            )}
          </p>
        )}
        <p className='text-muted-foreground text-[11px] leading-relaxed'>
          {t(
            'Aggregated from the cloud drive directory tree, so it reflects successfully uploaded data only (queued or failed uploads are not counted).'
          )}
        </p>
      </>
    )
  }
}
