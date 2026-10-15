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
import { getRouteApi } from '@tanstack/react-router'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { DataTablePage, useDataTable } from '@/components/data-table'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { useMediaQuery } from '@/hooks'
import { useTableUrlState } from '@/hooks/use-table-url-state'
import { handleServerError } from '@/lib/handle-server-error'

import {
  banIP,
  getBannedIPs,
  unbanIP,
  unbanIPsBatch,
  type BannedIPRow,
} from '../api'
import { useBannedIPColumns } from './banned-ips-columns'

const route = getRouteApi('/_authenticated/web-protection/')

export function BannedIPsTable() {
  const { t } = useTranslation()
  const isMobile = useMediaQuery('(max-width: 640px)')
  const [ip, setIp] = useState('')
  const [minutes, setMinutes] = useState(1440)
  const [reason, setReason] = useState('admin_ban')

  const {
    pagination,
    onPaginationChange,
    ensurePageInRange,
  } = useTableUrlState({
    search: route.useSearch(),
    navigate: route.useNavigate(),
    pagination: { defaultPage: 1, defaultPageSize: isMobile ? 10 : 20 },
  })

  // 服务端分页：/api/admin/banned-ips 支持 page/size 并返回 total。
  const { data, isLoading, isFetching, isError, refetch } = useQuery({
    queryKey: [
      'banned-ips',
      pagination.pageIndex + 1,
      pagination.pageSize,
    ],
    queryFn: () =>
      getBannedIPs(pagination.pageIndex + 1, pagination.pageSize),
    placeholderData: (previousData) => previousData,
  })

  const rows = data?.items ?? []

  const doUnban = (row: BannedIPRow) => {
    void (async () => {
      try {
        await unbanIP(row.ip)
        toast.success(t('IP unbanned'))
        await refetch()
      } catch (e) {
        handleServerError(e)
      }
    })()
  }

  const columns = useBannedIPColumns({ onUnban: doUnban })

  const { table } = useDataTable({
    data: rows,
    columns,
    pagination,
    onPaginationChange,
    enableRowSelection: true,
    getRowId: (row) => String(row.id),
    manualPagination: true,
    totalCount: data?.total ?? 0,
    ensurePageInRange,
  })

  const doBan = () => {
    if (!ip) {
      return
    }
    void (async () => {
      try {
        await banIP(ip, Number(minutes) || 1440, reason)
        toast.success(t('IP banned'))
        setIp('')
        await refetch()
      } catch (e) {
        handleServerError(e)
      }
    })()
  }

  const selectedRows = table.getFilteredSelectedRowModel().rows
  const selectedIps = selectedRows.map((row) => row.original.ip)

  const doUnbanSelected = () => {
    if (selectedIps.length === 0) {
      return
    }
    void (async () => {
      try {
        await unbanIPsBatch(selectedIps)
        toast.success(t('IPs unbanned'))
        table.resetRowSelection()
        await refetch()
      } catch (e) {
        handleServerError(e)
      }
    })()
  }

  return (
    <DataTablePage
      table={table}
      columns={columns}
      isLoading={isLoading}
      isFetching={isFetching}
      isError={isError}
      errorTitle={t('Failed to load banned IPs')}
      errorDescription={t('Please check your connection and try again.')}
      onRetry={() => refetch()}
      emptyTitle={t('No banned IPs')}
      emptyDescription={t('No IPs are currently banned.')}
      skeletonKeyPrefix='banned-ips-skeleton'
      applyHeaderSize
      toolbarProps={{
        customSearch: (
          <div className='flex flex-wrap items-end gap-2'>
            <div>
              <Label htmlFor='wp-ban-ip'>{t('IP')}</Label>
              <Input
                id='wp-ban-ip'
                value={ip}
                onChange={(e) => setIp(e.target.value)}
                className='w-44'
              />
            </div>
            <div>
              <Label htmlFor='wp-ban-minutes'>{t('Minutes')}</Label>
              <Input
                id='wp-ban-minutes'
                type='number'
                value={minutes}
                onChange={(e) => setMinutes(Number(e.target.value))}
                className='w-24'
              />
            </div>
            <div>
              <Label htmlFor='wp-ban-reason'>{t('Reason')}</Label>
              <Input
                id='wp-ban-reason'
                value={reason}
                onChange={(e) => setReason(e.target.value)}
                className='w-40'
              />
            </div>
            <Button type='button' onClick={doBan}>
              {t('Ban IP')}
            </Button>
          </div>
        ),
        preActions: (
          <Button
            type='button'
            size='sm'
            variant='destructive'
            disabled={selectedIps.length === 0}
            onClick={doUnbanSelected}
          >
            {t('Unban selected')}
            {selectedIps.length > 0 ? ` (${selectedIps.length})` : ''}
          </Button>
        ),
      }}
    />
  )
}
