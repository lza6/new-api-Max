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
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  StaticDataTable,
  type StaticDataTableColumn,
} from '@/components/data-table'
import { Button } from '@/components/ui/button'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Skeleton } from '@/components/ui/skeleton'
import { formatNumber, formatQuota } from '@/lib/format'

import { getOpsConsumptionRanking } from '../api'
import {
  OPS_QUERY_KEY,
  OPS_RANKING_DAY_LABEL_KEYS,
  OPS_RANKING_DAY_OPTIONS,
  OPS_RANKING_DEFAULT_DAYS,
  OPS_RANKING_LIMIT,
} from '../constants'
import type { OpsRankingItem } from '../types'
import { GrantQuotaDialog } from './grant-quota-dialog'

/**
 * 消耗排行榜：按用户聚合窗口内 consume 额度（降序），支持一键给该用户加额度。
 */
export function ConsumptionRanking() {
  const { t } = useTranslation()
  const [days, setDays] = useState<number>(OPS_RANKING_DEFAULT_DAYS)
  const [target, setTarget] = useState<OpsRankingItem | null>(null)

  const query = useQuery({
    queryKey: [OPS_QUERY_KEY, 'ranking', days, OPS_RANKING_LIMIT],
    queryFn: () => getOpsConsumptionRanking(days, OPS_RANKING_LIMIT),
    staleTime: 60_000,
    retry: false,
  })

  const items = query.data?.items ?? []

  const columns = useMemo<StaticDataTableColumn<OpsRankingItem>[]>(
    () => [
      {
        id: 'rank',
        header: '#',
        className: 'w-12',
        cell: (_row, index) => (
          <span className='text-muted-foreground tabular-nums'>
            {index + 1}
          </span>
        ),
      },
      {
        id: 'username',
        header: t('Username'),
        cell: (row) => row.username || `#${row.user_id}`,
      },
      {
        id: 'user_id',
        header: t('User ID'),
        cell: (row) => <span className='tabular-nums'>{row.user_id}</span>,
      },
      {
        id: 'quota',
        header: t('Quota'),
        className: 'text-right',
        cellClassName: 'text-right',
        cell: (row) => (
          <span className='tabular-nums'>{formatQuota(row.quota)}</span>
        ),
      },
      {
        id: 'requests',
        header: t('Requests'),
        className: 'text-right',
        cellClassName: 'text-right',
        cell: (row) => (
          <span className='tabular-nums'>{formatNumber(row.requests)}</span>
        ),
      },
      {
        id: 'actions',
        header: '',
        className: 'w-32 text-right',
        cellClassName: 'text-right',
        cell: (row) => (
          <Button variant='outline' size='sm' onClick={() => setTarget(row)}>
            {t('Grant Quota')}
          </Button>
        ),
      },
    ],
    [t]
  )

  const emptyContent = query.isError
    ? t('Failed to load')
    : t('No consumption data in this period')

  return (
    <div className='overflow-hidden rounded-lg border'>
      <div className='flex flex-wrap items-center justify-between gap-2 border-b px-4 py-3 sm:px-5'>
        <div className='min-w-0'>
          <h3 className='truncate text-sm font-semibold'>
            {t('Consumption Ranking')}
          </h3>
          <p className='text-muted-foreground mt-0.5 text-xs'>
            {t('Ranked by quota consumed in the last {{days}} days', { days })}
          </p>
        </div>
        <NativeSelect
          value={String(days)}
          onChange={(e) => setDays(Number(e.target.value))}
          aria-label={t('Time range')}
        >
          {OPS_RANKING_DAY_OPTIONS.map((option) => (
            <NativeSelectOption key={option} value={String(option)}>
              {t(OPS_RANKING_DAY_LABEL_KEYS[option])}
            </NativeSelectOption>
          ))}
        </NativeSelect>
      </div>

      <StaticDataTable<OpsRankingItem>
        columns={columns}
        data={items}
        getRowKey={(row) => row.user_id}
        empty={items.length === 0}
        emptyContent={
          query.isLoading ? (
            <div className='mx-auto flex max-w-md flex-col gap-2'>
              <Skeleton className='h-5 w-full' />
              <Skeleton className='h-5 w-full' />
            </div>
          ) : (
            emptyContent
          )
        }
      />

      <GrantQuotaDialog
        open={target != null}
        onOpenChange={(open) => {
          if (!open) {
            setTarget(null)
          }
        }}
        userId={target?.user_id ?? 0}
        username={target?.username ?? ''}
      />
    </div>
  )
}
