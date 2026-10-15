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
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { getCurrencyDisplay, getCurrencyLabel } from '@/lib/currency'
import { parseQuotaFromDollars } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'

import { grantUserQuota } from '../api'
import { OPS_QUERY_KEY } from '../constants'

interface GrantQuotaDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  userId: number
  username: string
  onSuccess?: () => void
}

/**
 * 加额度弹窗：输入展示货币金额，换算为原始额度单位后 POST /api/ops/grant-quota。
 * 服务端复用既有「管理员加额度」实现（权限层级 / 钱包上限 / 审计 / Topup 日志）。
 */
export function GrantQuotaDialog(props: GrantQuotaDialogProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [amount, setAmount] = useState('')

  const { meta: currencyMeta } = getCurrencyDisplay()
  const currencyLabel = getCurrencyLabel()
  const tokensOnly = currencyMeta.kind === 'tokens'

  const mutation = useMutation({
    mutationFn: (quota: number) =>
      grantUserQuota({ user_id: props.userId, quota }),
    onSuccess: () => {
      toast.success(t('Quota adjusted successfully'))
      setAmount('')
      props.onOpenChange(false)
      props.onSuccess?.()
      void queryClient.invalidateQueries({ queryKey: [OPS_QUERY_KEY] })
    },
    onError: (error: unknown) => {
      handleServerError(error, t('Failed to adjust quota'))
    },
  })

  const amountValue = Number.parseFloat(amount) || 0
  const quotaValue = parseQuotaFromDollars(Math.abs(amountValue))
  const canSubmit = quotaValue > 0 && !mutation.isPending

  const handleConfirm = () => {
    if (!canSubmit) {
      return
    }
    mutation.mutate(quotaValue)
  }

  const handleCancel = () => {
    setAmount('')
    props.onOpenChange(false)
  }

  const displayName = props.username || `#${props.userId}`
  const placeholder = tokensOnly
    ? t('Enter amount in tokens')
    : t('Enter amount in {{currency}}', { currency: currencyLabel })

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={t('Grant Quota')}
      description={`${t('Add quota to this user')}: ${displayName}`}
      contentHeight='auto'
      bodyClassName='space-y-4'
      footer={
        <>
          <Button variant='outline' onClick={handleCancel}>
            {t('Cancel')}
          </Button>
          <Button onClick={handleConfirm} disabled={!canSubmit}>
            {mutation.isPending ? t('Processing...') : t('Confirm')}
          </Button>
        </>
      }
    >
      <div className='space-y-2'>
        <Label>
          {t('Amount')} ({currencyLabel})
        </Label>
        <Input
          type='number'
          step={tokensOnly ? 1 : 0.000001}
          min={0}
          placeholder={placeholder}
          value={amount}
          onChange={(e) => setAmount(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter') {
              handleConfirm()
            }
          }}
        />
      </div>
    </Dialog>
  )
}
