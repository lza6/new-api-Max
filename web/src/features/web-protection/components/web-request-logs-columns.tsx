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
import type { ColumnDef } from '@tanstack/react-table'
import { useTranslation } from 'react-i18next'

import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

import { formatTimestampToDate } from '@/lib/format'

import { formatBytes, type WebRequestLogRow } from '../api'

export interface WebRequestLogActions {
  banMinutes: number
  onBanMinutesChange: (value: number) => void
  onBan: (row: WebRequestLogRow) => void
  detailOpen: (row: WebRequestLogRow) => boolean
  onToggleDetail: (row: WebRequestLogRow) => void
}

export function useWebRequestLogColumns(
  actions: WebRequestLogActions
): ColumnDef<WebRequestLogRow>[] {
  const { t } = useTranslation()
  return [
    {
      accessorKey: 'ip',
      header: t('IP'),
      meta: { mobileTitle: true },
      cell: ({ row }) => (
        <span className='font-mono text-sm'>{row.original.ip}</span>
      ),
      size: 180,
    },
    {
      accessorKey: 'request_count',
      header: t('Request count'),
      meta: { mobileOrder: 1 },
      cell: ({ row }) => (
        <span className='tabular-nums'>{row.original.request_count}</span>
      ),
      size: 120,
    },
    {
      accessorKey: 'rate_per_second',
      header: t('Rate (req/s)'),
      meta: { mobileOrder: 2 },
      cell: ({ row }) => (
        <span className='tabular-nums'>
          {row.original.rate_per_second.toFixed(2)}
        </span>
      ),
      size: 120,
    },
    {
      accessorKey: 'bytes_total',
      header: t('Bandwidth'),
      meta: { mobileOrder: 3 },
      cell: ({ row }) => (
        <span className='tabular-nums'>{formatBytes(row.original.bytes_total)}</span>
      ),
      size: 120,
    },
    {
      accessorKey: 'last_request_at',
      header: t('Last request'),
      meta: { mobileOrder: 4 },
      cell: ({ row }) => (
        <span className='text-muted-foreground font-mono text-xs'>
          {formatTimestampToDate(row.original.last_request_at)}
        </span>
      ),
      size: 180,
    },
    {
      id: 'status',
      header: t('Status'),
      meta: { mobileBadge: true },
      cell: ({ row }) =>
        row.original.banned ? (
          <StatusBadge label={t('Banned')} variant='danger' copyable={false} />
        ) : null,
      size: 100,
    },
    {
      id: 'actions',
      header: () => t('Actions'),
      meta: { pinned: 'right' as const },
      cell: ({ row }) => (
        <div className='flex items-center justify-end gap-2'>
          <Label className='text-muted-foreground hidden text-xs xl:inline'>
            {t('Ban minutes')}
          </Label>
          <Input
            type='number'
            aria-label={t('Ban minutes')}
            value={actions.banMinutes}
            onChange={(e) => actions.onBanMinutesChange(Number(e.target.value))}
            className='h-8 w-20'
          />
          <Button
            type='button'
            size='sm'
            variant='destructive'
            onClick={() => actions.onBan(row.original)}
          >
            {t('Ban IP')}
          </Button>
          <Button
            type='button'
            size='sm'
            variant='outline'
            onClick={() => actions.onToggleDetail(row.original)}
          >
            {actions.detailOpen(row.original) ? t('Hide') : t('Details')}
          </Button>
        </div>
      ),
      enableSorting: false,
      enableHiding: false,
    },
  ]
}
