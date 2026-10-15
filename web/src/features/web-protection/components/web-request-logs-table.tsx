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
/* oxlint-disable react/no-array-index-key -- string detail values may repeat; index keeps keys unique */
import { useQuery } from '@tanstack/react-query'
import { getRouteApi } from '@tanstack/react-router'
import { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  DataTablePage,
  useDataTable,
} from '@/components/data-table'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { useMediaQuery } from '@/hooks'
import { useTableUrlState } from '@/hooks/use-table-url-state'
import { handleServerError } from '@/lib/handle-server-error'
import { createServerError } from '@/lib/server-error-message'

import {
  banIP,
  formatBytes,
  getWebRequestLogDetail,
  getWebRequestLogs,
  type WebRequestLogRow,
} from '../api'
import { useWebRequestLogColumns } from './web-request-logs-columns'

const route = getRouteApi('/_authenticated/web-protection/')

const SORT_KEYS = ['rate', 'count', 'bytes'] as const
type SortKey = (typeof SORT_KEYS)[number]

export function WebRequestLogsTable() {
  const { t } = useTranslation()
  const isMobile = useMediaQuery('(max-width: 640px)')
  const [sort, setSort] = useState<SortKey>('rate')
  const [order, setOrder] = useState<'asc' | 'desc'>('desc')
  const [ipFilter, setIpFilter] = useState('')
  const [banMinutes, setBanMinutes] = useState(1440)
  const [detail, setDetail] = useState<Record<string, string[]>>({})

  const {
    pagination,
    onPaginationChange,
    ensurePageInRange,
  } = useTableUrlState({
    search: route.useSearch(),
    navigate: route.useNavigate(),
    pagination: { defaultPage: 1, defaultPageSize: isMobile ? 10 : 20 },
  })

  // 服务端分页：/api/admin/web-request-logs 支持 page/size 并返回 total。
  const { data, isLoading, isFetching, isError, refetch } = useQuery({
    queryKey: [
      'web-request-logs',
      pagination.pageIndex + 1,
      pagination.pageSize,
      ipFilter,
      sort,
      order,
    ],
    queryFn: async () => {
      const result = await getWebRequestLogs({
        page: pagination.pageIndex + 1,
        size: pagination.pageSize,
        ip: ipFilter || undefined,
        sort,
        order,
      })
      if (!result) {
        throw createServerError(
          { success: false },
          t('Failed to load web request logs')
        )
      }
      return result
    },
    placeholderData: (previousData) => previousData,
  })

  const rows = data?.items ?? []

  const loadDetail = useCallback(
    async (row: WebRequestLogRow) => {
      try {
        const d = await getWebRequestLogDetail(row.ip)
        setDetail((prev) => ({
          ...prev,
          [row.ip]: (d.items || []).map(
            (x) =>
              x.method +
              ' ' +
              x.path +
              ' [' +
              x.status +
              '] x' +
              x.request_count +
              ' ' +
              formatBytes(x.bytes_sent) +
              ' @' +
              new Date(x.window_start * 1000).toLocaleString()
          ),
        }))
      } catch (e) {
        handleServerError(e)
      }
    },
    []
  )

  const toggleDetail = useCallback(
    async (row: WebRequestLogRow) => {
      if (detail[row.ip]) {
        setDetail((prev) => {
          const next = { ...prev }
          delete next[row.ip]
          return next
        })
        return
      }
      await loadDetail(row)
    },
    [detail, loadDetail]
  )

  const columns = useWebRequestLogColumns({
    banMinutes,
    onBanMinutesChange: setBanMinutes,
    onBan: (row) => {
      void (async () => {
        try {
          await banIP(row.ip, Number(banMinutes) || 1440, 'admin_ban')
          toast.success(t('IP banned'))
          await refetch()
        } catch (e) {
          handleServerError(e)
        }
      })()
    },
    detailOpen: (row) => Boolean(detail[row.ip]),
    onToggleDetail: (row) => {
      void toggleDetail(row)
    },
  })

  const { table } = useDataTable({
    data: rows,
    columns,
    pagination,
    onPaginationChange,
    manualPagination: true,
    manualSorting: true,
    totalCount: data?.total ?? 0,
    ensurePageInRange,
  })

  const sortButton = (key: SortKey, label: string) => {
    let indicator: string | null = null
    if (sort === key) {
      indicator = order === 'desc' ? ' ↓' : ' ↑'
    }
    return (
      <Button
        key={key}
        type='button'
        size='sm'
        variant={sort === key ? 'default' : 'outline'}
        onClick={() => {
          if (sort === key) {
            setOrder(order === 'desc' ? 'asc' : 'desc')
          } else {
            setSort(key)
            setOrder('desc')
          }
          onPaginationChange({ ...pagination, pageIndex: 0 })
        }}
      >
        {t(label)}
        {indicator}
      </Button>
    )
  }

  const openDetails = rows.filter((r) => detail[r.ip])
  const detailPanel = openDetails.length > 0 && (
    <Card>
      <CardHeader>
        <CardTitle>{t('Request details')}</CardTitle>
      </CardHeader>
      <CardContent className='space-y-3'>
        {openDetails.map((r) => (
          <div key={r.ip} className='rounded-md border p-3'>
            <div className='flex items-center justify-between gap-2'>
              <span className='font-mono text-sm'>{r.ip}</span>
              <Button
                type='button'
                size='sm'
                variant='ghost'
                onClick={() => {
                  void toggleDetail(r)
                }}
              >
                {t('Hide')}
              </Button>
            </div>
            <ul className='text-muted-foreground mt-2 space-y-1 border-t pt-2 text-xs'>
              {detail[r.ip].map((line, index) => (
                <li key={index} className='break-all'>
                  {line}
                </li>
              ))}
            </ul>
          </div>
        ))}
      </CardContent>
    </Card>
  )

  return (
    <div className='space-y-3'>
      <DataTablePage
        table={table}
        columns={columns}
        isLoading={isLoading}
        isFetching={isFetching}
        isError={isError}
        errorTitle={t('Failed to load web request logs')}
        errorDescription={t('Please check your connection and try again.')}
        onRetry={() => refetch()}
        emptyTitle={t('No web request logs found')}
        emptyDescription={t(
          'No web request logs yet. Enable web request logging to collect data.'
        )}
        skeletonKeyPrefix='web-request-logs-skeleton'
        applyHeaderSize
        toolbarProps={{
          customSearch: (
            <Input
              placeholder={t('Filter by IP')}
              value={ipFilter}
              onChange={(e) => {
                setIpFilter(e.target.value)
                onPaginationChange({ ...pagination, pageIndex: 0 })
              }}
              className='w-full sm:w-[200px]'
            />
          ),
          additionalSearch: (
            <>
              {sortButton('rate', 'Rate')}
              {sortButton('count', 'Count')}
              {sortButton('bytes', 'Bandwidth')}
            </>
          ),
          hasAdditionalFilters: ipFilter !== '' || sort !== 'rate' || order !== 'desc',
          onReset: () => {
            setIpFilter('')
            setSort('rate')
            setOrder('desc')
            onPaginationChange({ ...pagination, pageIndex: 0 })
          },
          preActions: (
            <Button
              type='button'
              size='sm'
              variant='outline'
              onClick={() => refetch()}
            >
              {t('Refresh')}
            </Button>
          ),
        }}
      />
      {detailPanel}
    </div>
  )
}
