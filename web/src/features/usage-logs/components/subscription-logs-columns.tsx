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
import type { TFunction } from 'i18next'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { BadgeCell } from '@/components/data-table'
import { GroupBadge } from '@/components/group-badge'
import { StatusBadge } from '@/components/status-badge'
import { TableId } from '@/components/table-id'
import { formatTimestamp } from '@/lib/format'

import type { SubscriptionLogItem } from '../types'

function formatSubscriptionMoney(amount: number, currency?: string): string {
  const value = Number(amount || 0).toFixed(2)
  return currency === 'CNY' ? `¥${value}` : `$${value}`
}

function sourceBadgeContent(source: string, t: TFunction) {
  const map: Record<string, { label: string; color?: string }> = {
    order: { label: t('Order') },
    balance: { label: t('Balance') },
    redemption: { label: t('Redemption') },
    admin: { label: t('Admin') },
  }
  const entry = map[source] ?? { label: source }
  return (
    <BadgeCell>
      <StatusBadge label={entry.label} variant='neutral' size='sm' />
    </BadgeCell>
  )
}

function statusLabelText(status: string, t: TFunction): string {
  const map: Record<string, string> = {
    active: t('Active'),
    expired: t('Expired'),
    cancelled: t('Cancelled'),
  }
  return map[status] ?? status
}

export function useSubscriptionLogsColumns(): ColumnDef<SubscriptionLogItem>[] {
  const { t } = useTranslation()

  return useMemo(
    (): ColumnDef<SubscriptionLogItem>[] => [
      {
        accessorKey: 'id',
        header: t('ID'),
        meta: { mobileHidden: true },
        cell: ({ row }) => <TableId value={row.original.id} />,
        size: 70,
      },
      {
        accessorKey: 'username',
        header: t('User'),
        meta: { mobileTitle: true },
        cell: ({ row }) => (
          <span className='font-medium'>{row.original.username}</span>
        ),
        size: 130,
      },
      {
        accessorKey: 'plan_title',
        header: t('Plan'),
        cell: ({ row }) => (
          <div className='max-w-full min-w-0'>
            <div className='truncate font-medium'>{row.original.plan_title}</div>
            {row.original.plan_id > 0 && (
              <div className='text-muted-foreground truncate text-xs'>
                #{row.original.plan_id}
              </div>
            )}
          </div>
        ),
        size: 160,
      },
      {
        accessorKey: 'source',
        header: t('Source'),
        cell: ({ row }) => sourceBadgeContent(row.original.source, t),
        size: 110,
      },
      {
        accessorKey: 'status',
        header: t('Status'),
        cell: ({ row }) => (
          <StatusBadge
            label={statusLabelText(row.original.status, t)}
            variant={
              row.original.status === 'active' ? 'success' : 'neutral'
            }
            size='sm'
          />
        ),
        size: 100,
      },
      {
        accessorKey: 'price_amount',
        header: t('Price'),
        cell: ({ row }) =>
          formatSubscriptionMoney(
            row.original.price_amount,
            row.original.currency
          ),
        meta: { align: 'right' },
        size: 90,
      },
      {
        id: 'tier',
        header: t('Tier'),
        cell: ({ row }) => {
          const { concurrency_override, rpm_override } = row.original
          const concurrency = concurrency_override > 0 ? concurrency_override : '-'
          const rpm = rpm_override > 0 ? rpm_override : '-'
          return (
            <span className='text-muted-foreground tabular-nums'>
              {concurrency}
              {' / '}
              {rpm}
            </span>
          )
        },
        size: 90,
      },
      {
        accessorKey: 'start_time',
        header: t('Start Time'),
        cell: ({ row }) => formatTimestamp(row.original.start_time),
        size: 150,
      },
      {
        accessorKey: 'end_time',
        header: t('End Time'),
        cell: ({ row }) => formatTimestamp(row.original.end_time),
        size: 150,
      },
      {
        accessorKey: 'created_at',
        header: t('Created At'),
        cell: ({ row }) => formatTimestamp(row.original.created_at),
        size: 150,
      },
      {
        id: 'groups',
        header: t('Group'),
        cell: ({ row }) => {
          const { upgrade_group, prev_user_group } = row.original
          if (!upgrade_group && !prev_user_group) {return '-'}
          return (
            <div className='flex flex-col gap-0.5'>
              {upgrade_group && (
                <span className='flex items-center gap-1 text-xs'>
                  <GroupBadge group={upgrade_group} type='text' size='sm' />
                </span>
              )}
              {prev_user_group && (
                <span className='text-muted-foreground text-[10px] leading-none'>
                  {t('From')} <GroupBadge group={prev_user_group} type='text' size='sm' />
                </span>
              )}
            </div>
          )
        },
        size: 140,
      },
    ],
    [t]
  )
}
