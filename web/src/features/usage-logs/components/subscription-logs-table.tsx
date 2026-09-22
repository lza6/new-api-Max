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

import { DataTablePage, useDataTable } from '@/components/data-table'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useMediaQuery } from '@/hooks'
import { createServerError } from '@/lib/server-error-message'

import {
  getSubscriptionLogs,
  type GetSubscriptionLogsParams,
} from '../api'
import type { SubscriptionLogItem } from '../types'
import { useSubscriptionLogsColumns } from './subscription-logs-columns'

const EMPTY_ITEMS: SubscriptionLogItem[] = []

interface SubscriptionLogFilters {
  username?: string
  status?: string
  source?: string
}

function SubscriptionLogsFilterBar(props: {
  filters: SubscriptionLogFilters
  onApply: (filters: SubscriptionLogFilters) => void
}) {
  const { t } = useTranslation()
  const [username, setUsername] = useState(props.filters.username ?? '')
  const [status, setStatus] = useState(props.filters.status ?? 'all')
  const [source, setSource] = useState(props.filters.source ?? 'all')

  return (
    <div className='flex flex-wrap items-end gap-2'>
      <div className='flex min-w-[160px] flex-1 flex-col gap-1'>
        <Label className='text-xs'>{t('User')}</Label>
        <Input
          value={username}
          onChange={(e) => setUsername(e.target.value)}
          placeholder={t('Filter by username')}
          className='h-8'
        />
      </div>
      <div className='flex min-w-[130px] flex-col gap-1'>
        <Label className='text-xs'>{t('Status')}</Label>
        <Select
          value={status}
          onValueChange={(v) => setStatus(v ?? 'all')}
        >
          <SelectTrigger className='h-8'>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value='all'>{t('All')}</SelectItem>
            <SelectItem value='active'>{t('Active')}</SelectItem>
            <SelectItem value='expired'>{t('Expired')}</SelectItem>
            <SelectItem value='cancelled'>{t('Cancelled')}</SelectItem>
          </SelectContent>
        </Select>
      </div>
      <div className='flex min-w-[130px] flex-col gap-1'>
        <Label className='text-xs'>{t('Source')}</Label>
        <Select
          value={source}
          onValueChange={(v) => setSource(v ?? 'all')}
        >
          <SelectTrigger className='h-8'>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value='all'>{t('All')}</SelectItem>
            <SelectItem value='order'>{t('Order')}</SelectItem>
            <SelectItem value='balance'>{t('Balance')}</SelectItem>
            <SelectItem value='redemption'>{t('Redemption')}</SelectItem>
            <SelectItem value='admin'>{t('Admin')}</SelectItem>
          </SelectContent>
        </Select>
      </div>
      <Button
        type='button'
        variant='outline'
        className='h-8'
        onClick={() =>
          props.onApply({
            username: username.trim() || undefined,
            status: status === 'all' ? undefined : status,
            source: source === 'all' ? undefined : source,
          })
        }
      >
        {t('Filter')}
      </Button>
    </div>
  )
}

/**
 * 管理员「订阅日志」表格：谁、何时、通过什么来源购买了哪个套餐、
 * 状态、档位与分组变化。数据来自 /api/subscription/admin/logs。
 */
export function SubscriptionLogsTable() {
  const { t } = useTranslation()
  const isMobile = useMediaQuery('(max-width: 640px)')
  const columns = useSubscriptionLogsColumns()
  const [filters, setFilters] = useState<SubscriptionLogFilters>({})
  const [page, setPage] = useState(1)
  const pageSize = isMobile ? 20 : 30

  const params = useMemo<GetSubscriptionLogsParams>(
    () => ({
      p: page,
      page_size: pageSize,
      username: filters.username,
      status: filters.status,
      source: filters.source,
    }),
    [page, pageSize, filters]
  )

  const { data, isLoading, isFetching } = useQuery({
    queryKey: ['subscription-logs', params],
    queryFn: async () => {
      const result = await getSubscriptionLogs(params)
      if (!result?.success) {
        throw createServerError(result, t('Failed to load logs'))
      }
      return result.data || { items: EMPTY_ITEMS, total: 0, page: 1, page_size: pageSize }
    },
    placeholderData: (prev) => prev,
  })

  const items = data?.items ?? EMPTY_ITEMS
  const total = data?.total ?? 0
  const { table } = useDataTable({
    data: items,
    columns,
    manualPagination: true,
    pagination: { pageIndex: page - 1, pageSize },
    onPaginationChange: (updater) => {
      if (typeof updater === 'function') {
        const next = updater({ pageIndex: page - 1, pageSize })
        setPage(next.pageIndex + 1)
      } else {
        setPage(updater.pageIndex + 1)
      }
    },
    totalCount: total,
    enableRowSelection: false,
  })

  return (
    <DataTablePage
      table={table}
      columns={columns}
      isLoading={isLoading}
      isFetching={isFetching}
      emptyTitle={t('No subscription records found')}
      emptyDescription={t(
        'Subscription purchases, redemptions and admin grants will appear here.'
      )}
      skeletonKeyPrefix='subscription-log-skeleton'
      applyHeaderSize
      toolbar={
        <SubscriptionLogsFilterBar
          filters={filters}
          onApply={(next) => {
            setFilters(next)
            setPage(1)
          }}
        />
      }
    />
  )
}
