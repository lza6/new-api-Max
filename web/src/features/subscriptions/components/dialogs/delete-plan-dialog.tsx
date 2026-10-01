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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { handleServerError } from '@/lib/handle-server-error'

import { deletePlan } from '../../api'
import { useSubscriptions } from '../subscriptions-provider'

export function DeletePlanDialog() {
  const { t } = useTranslation()
  const { open, setOpen, currentRow, triggerRefresh } = useSubscriptions()
  const [loading, setLoading] = useState(false)
  const isOpen = open === 'delete-plan'
  const plan = currentRow?.plan
  const planLabel = plan?.title || (plan?.id ? `#${plan.id}` : '-')

  if (!isOpen || !plan) {return null}

  const handleConfirm = async () => {
    setLoading(true)
    try {
      const res = await deletePlan(plan.id)
      if (res.success) {
        toast.success(t('Deleted'))
        triggerRefresh()
        setOpen(null)
      } else {
        // A plan still referenced by user subscriptions is refused server-side;
        // surface the concrete reason instead of a generic failure.
        handleServerError(res)
      }
    } catch (error) {
      handleServerError(error, t('Operation failed'))
    } finally {
      setLoading(false)
    }
  }

  return (
    <ConfirmDialog
      open={isOpen}
      onOpenChange={(nextOpen) => !nextOpen && setOpen(null)}
      title={t('Delete plan {{plan}}?', { plan: planLabel })}
      desc={t(
        'This permanently removes the plan from the management list. Users who already subscribed keep their existing subscription until it expires; nothing is removed from them.'
      )}
      confirmText={t('Delete')}
      handleConfirm={handleConfirm}
      disabled={!plan.id}
      isLoading={loading}
      destructive
    />
  )
}
