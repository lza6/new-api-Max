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
import { useMutation } from '@tanstack/react-query'
import type { Table } from '@tanstack/react-table'
import { Coins, Power, PowerOff, Trash2 } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { DataTableBulkActions as BulkActionsToolbar } from '@/components/data-table'
import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { getCurrencyDisplay, getCurrencyLabel } from '@/lib/currency'
import { parseQuotaFromDollars } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'
import { createServerError } from '@/lib/server-error-message'
import { cn } from '@/lib/utils'

import { batchManageUsers } from '../api'
import { USER_ROLE } from '../constants'
import type { QuotaAdjustMode, User } from '../types'
import { useUsers } from './users-provider'

type DataTableBulkActionsProps = {
  table: Table<User>
}

function isRootUser(user: User) {
  return user.role === USER_ROLE.ROOT
}

function isDeletedUser(user: User) {
  return user.DeletedAt != null
}

function batchResultLabel(
  t: (key: string, options?: Record<string, unknown>) => string,
  action: string,
  count: number
): string {
  switch (action) {
    case 'enable':
      return t('Successfully enabled {{count}} users', { count })
    case 'disable':
      return t('Successfully disabled {{count}} users', { count })
    case 'delete':
      return t('Successfully deleted {{count}} users', { count })
    default:
      return t('Successfully adjusted quota for {{count}} users', { count })
  }
}

function batchFailureLabel(
  t: (key: string, options?: Record<string, unknown>) => string,
  action: string,
  count: number
): string {
  switch (action) {
    case 'enable':
      return t('Failed to enable selected users', { count })
    case 'disable':
      return t('Failed to disable selected users', { count })
    case 'delete':
      return t('Failed to delete selected users', { count })
    default:
      return t('Failed to adjust quota for selected users', { count })
  }
}

export function DataTableBulkActions(props: DataTableBulkActionsProps) {
  const { t } = useTranslation()
  const { triggerRefresh } = useUsers()
  const [deleteTargets, setDeleteTargets] = useState<User[] | null>(null)
  const [quotaDialogOpen, setQuotaDialogOpen] = useState(false)
  const [quotaMode, setQuotaMode] = useState<QuotaAdjustMode>('add')
  const [quotaAmount, setQuotaAmount] = useState('')

  const selectedRows = props.table.getFilteredSelectedRowModel().rows
  const selectedUsers = useMemo(
    () => selectedRows.map((row) => row.original),
    [selectedRows]
  )

  const disableableUsers = useMemo(
    () => selectedUsers.filter((user) => !isRootUser(user)),
    [selectedUsers]
  )
  const deletableUsers = useMemo(
    () => selectedUsers.filter((user) => !isRootUser(user) && !isDeletedUser(user)),
    [selectedUsers]
  )

  const { meta: currencyMeta } = getCurrencyDisplay()
  const currencyLabel = getCurrencyLabel()
  const tokensOnly = currencyMeta.kind === 'tokens'

  const amountValue = Number.parseFloat(quotaAmount) || 0
  const quotaValue =
    quotaMode === 'override'
      ? parseQuotaFromDollars(amountValue)
      : parseQuotaFromDollars(Math.abs(amountValue))

  const runBatch = async (
    action: 'enable' | 'disable' | 'delete' | 'add_quota',
    users: User[],
    mode?: QuotaAdjustMode,
    value?: number
  ): Promise<{ processed: number; total: number }> => {
    const payload = {
      ids: users.map((user) => user.id),
      action,
      ...(mode !== undefined ? { mode } : {}),
      ...(value !== undefined ? { value } : {}),
    }
    const result = await batchManageUsers(payload)
    if (!result.success) throw createServerError(result)
    return result.data ?? { processed: 0, total: 0 }
  }

  const enable = useMutation({
    mutationFn: (users: User[]) => runBatch('enable', users),
    onSuccess: (data) => {
      toast.success(batchResultLabel(t, 'enable', data.processed ?? 0))
      props.table.resetRowSelection()
      triggerRefresh()
    },
    onError: (error) => {
      handleServerError(error, batchFailureLabel(t, 'enable', selectedUsers.length))
    },
  })

  const disable = useMutation({
    mutationFn: (users: User[]) => runBatch('disable', users),
    onSuccess: (data) => {
      toast.success(batchResultLabel(t, 'disable', data.processed ?? 0))
      props.table.resetRowSelection()
      triggerRefresh()
    },
    onError: (error) => {
      handleServerError(error, batchFailureLabel(t, 'disable', disableableUsers.length))
    },
  })

  const deletion = useMutation({
    mutationFn: (users: User[]) => runBatch('delete', users),
    onSuccess: (data) => {
      toast.success(batchResultLabel(t, 'delete', data.processed ?? 0))
      props.table.resetRowSelection()
      setDeleteTargets(null)
      triggerRefresh()
    },
    onError: (error) => {
      handleServerError(error, batchFailureLabel(t, 'delete', deletableUsers.length))
    },
  })

  const quota = useMutation({
    mutationFn: () =>
      runBatch('add_quota', disableableUsers, quotaMode, quotaValue),
    onSuccess: (data) => {
      toast.success(batchResultLabel(t, 'add_quota', data.processed ?? 0))
      setQuotaDialogOpen(false)
      setQuotaAmount('')
      setQuotaMode('add')
      props.table.resetRowSelection()
      triggerRefresh()
    },
    onError: (error) => {
      handleServerError(error, batchFailureLabel(t, 'add_quota', disableableUsers.length))
    },
  })

  const anyPending =
    enable.isPending || disable.isPending || deletion.isPending || quota.isPending

  const hasDisableable = disableableUsers.length > 0
  const hasDeletable = deletableUsers.length > 0

  const handleQuotaConfirm = () => {
    if (quotaMode !== 'override' && quotaValue <= 0) return
    if (quota.isPending) return
    quota.mutate()
  }

  const placeholder = tokensOnly
    ? t('Enter amount in tokens')
    : t('Enter amount in {{currency}}', { currency: currencyLabel })

  return (
    <>
      <BulkActionsToolbar table={props.table} entityName={t('user')}>
        <Tooltip>
          <TooltipTrigger
            render={
              <Button
                variant='outline'
                size='icon'
                className='size-8'
                aria-label={t('Enable selected users')}
                disabled={anyPending}
                onClick={() => enable.mutate(selectedUsers)}
              />
            }
          >
            <Power aria-hidden='true' />
          </TooltipTrigger>
          <TooltipContent>{t('Enable selected users')}</TooltipContent>
        </Tooltip>
        <Tooltip>
          <TooltipTrigger
            render={
              <Button
                variant='outline'
                size='icon'
                className='size-8'
                aria-label={t('Disable selected users')}
                disabled={anyPending || !hasDisableable}
                onClick={() => disable.mutate(disableableUsers)}
              />
            }
          >
            <PowerOff aria-hidden='true' />
          </TooltipTrigger>
          <TooltipContent>{t('Disable selected users')}</TooltipContent>
        </Tooltip>
        <Tooltip>
          <TooltipTrigger
            render={
              <Button
                variant='destructive'
                size='icon'
                className='size-8'
                aria-label={t('Delete selected users')}
                disabled={anyPending || !hasDeletable}
                onClick={() => setDeleteTargets(deletableUsers)}
              />
            }
          >
            <Trash2 aria-hidden='true' />
          </TooltipTrigger>
          <TooltipContent>{t('Delete selected users')}</TooltipContent>
        </Tooltip>
        <Tooltip>
          <TooltipTrigger
            render={
              <Button
                variant='outline'
                size='icon'
                className='size-8'
                aria-label={t('Adjust quota for selected users')}
                disabled={anyPending}
                onClick={() => setQuotaDialogOpen(true)}
              />
            }
          >
            <Coins aria-hidden='true' />
          </TooltipTrigger>
          <TooltipContent>{t('Adjust quota for selected users')}</TooltipContent>
        </Tooltip>
      </BulkActionsToolbar>

      <ConfirmDialog
        destructive
        open={deleteTargets !== null}
        onOpenChange={(open) => {
          if (!open && !deletion.isPending) setDeleteTargets(null)
        }}
        title={t('Delete {{count}} users?', { count: deleteTargets?.length ?? 0 })}
        desc={t('This action cannot be undone.')}
        confirmText={deletion.isPending ? t('Deleting...') : t('Delete')}
        isLoading={deletion.isPending}
        disabled={!deleteTargets?.length}
        handleConfirm={() => {
          if (deleteTargets?.length && !deletion.isPending) {
            deletion.mutate(deleteTargets)
          }
        }}
      />

      <Dialog
        open={quotaDialogOpen}
        onOpenChange={setQuotaDialogOpen}
        title={t('Adjust Quota')}
        description={t('Select an operation mode and enter the amount')}
        contentHeight='auto'
        bodyClassName='space-y-4'
        footer={
          <>
            <Button
              variant='outline'
              onClick={() => {
                setQuotaDialogOpen(false)
                setQuotaAmount('')
                setQuotaMode('add')
              }}
            >
              {t('Cancel')}
            </Button>
            <Button onClick={handleQuotaConfirm} disabled={quota.isPending}>
              {quota.isPending ? t('Processing...') : t('Confirm')}
            </Button>
          </>
        }
      >
        <div className='space-y-4'>
          <div className='text-muted-foreground text-sm'>
            {t('Adjust quota for {{count}} selected users', {
              count: selectedUsers.length,
            })}
          </div>
          <div className='space-y-2'>
            <Label>{t('Mode')}</Label>
            <div className='flex gap-1'>
              {(['add', 'subtract', 'override'] as const).map((m) => (
                <Button
                  key={m}
                  type='button'
                  variant='outline'
                  size='sm'
                  className={cn(
                    quotaMode === m &&
                      'bg-primary text-primary-foreground hover:bg-primary/90 hover:text-primary-foreground'
                  )}
                  onClick={() => {
                    setQuotaMode(m)
                    setQuotaAmount('')
                  }}
                >
                  {m === 'add' && t('Add')}
                  {m === 'subtract' && t('Subtract')}
                  {m === 'override' && t('Override')}
                </Button>
              ))}
            </div>
          </div>
          <div className='space-y-2'>
            <Label>
              {t('Amount')} ({currencyLabel})
            </Label>
            <Input
              type='number'
              step={tokensOnly ? 1 : 0.000001}
              min={quotaMode === 'override' ? undefined : 0}
              placeholder={placeholder}
              value={quotaAmount}
              onChange={(e) => setQuotaAmount(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') handleQuotaConfirm()
              }}
            />
          </div>
        </div>
      </Dialog>
    </>
  )
}
