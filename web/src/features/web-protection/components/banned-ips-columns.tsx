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
import { Checkbox } from '@/components/ui/checkbox'
import { formatTimestampToDate } from '@/lib/format'

import type { BannedIPRow } from '../api'

export interface BannedIPActions {
  onUnban: (row: BannedIPRow) => void
}

export function useBannedIPColumns(
  actions: BannedIPActions
): ColumnDef<BannedIPRow>[] {
  const { t } = useTranslation()
  return [
    {
      id: 'select',
      header: ({ table }) => (
        <Checkbox
          checked={table.getIsAllPageRowsSelected()}
          indeterminate={table.getIsSomePageRowsSelected()}
          onCheckedChange={(value) => table.toggleAllPageRowsSelected(!!value)}
          aria-label={t('Select all')}
          className='translate-y-[2px]'
        />
      ),
      cell: ({ row }) => (
        <Checkbox
          checked={row.getIsSelected()}
          onCheckedChange={(value) => row.toggleSelected(!!value)}
          aria-label={t('Select row')}
          className='translate-y-[2px]'
        />
      ),
      enableSorting: false,
      enableHiding: false,
      size: 40,
    },
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
      accessorKey: 'reason',
      header: t('Reason'),
      meta: { mobileOrder: 1 },
      cell: ({ row }) => (
        <span className='text-muted-foreground text-sm'>{row.original.reason}</span>
      ),
      size: 200,
    },
    {
      accessorKey: 'banned_by',
      header: t('Banned by'),
      meta: { mobileOrder: 2 },
      cell: ({ row }) => (
        <span className='text-muted-foreground text-sm'>
          {row.original.banned_by}
        </span>
      ),
      size: 140,
    },
    {
      accessorKey: 'banned_at',
      header: t('Banned at'),
      meta: { mobileOrder: 3 },
      cell: ({ row }) => (
        <span className='text-muted-foreground font-mono text-xs'>
          {formatTimestampToDate(row.original.banned_at)}
        </span>
      ),
      size: 180,
    },
    {
      accessorKey: 'expires_at',
      header: t('Expires'),
      meta: { mobileOrder: 4 },
      cell: ({ row }) =>
        row.original.expires_at ? (
          <span className='text-muted-foreground font-mono text-xs'>
            {formatTimestampToDate(row.original.expires_at)}
          </span>
        ) : (
          <StatusBadge label={t('Permanent')} variant='neutral' copyable={false} />
        ),
      size: 180,
    },
    {
      id: 'actions',
      header: () => t('Actions'),
      meta: { pinned: 'right' as const },
      cell: ({ row }) => (
        <div className='flex justify-end'>
          <Button
            type='button'
            size='sm'
            variant='outline'
            onClick={() => actions.onUnban(row.original)}
          >
            {t('Unban')}
          </Button>
        </div>
      ),
      enableSorting: false,
      enableHiding: false,
    },
  ]
}
