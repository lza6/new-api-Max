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
import { Ban, Plus, RotateCcw, Save, Trash2 } from 'lucide-react'
import {
  useCallback,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Combobox } from '@/components/ui/combobox'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { formatQuota } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'

import {
  createUserSubscription,
  deleteUserSubscription,
  getAdminPlans,
  getUserSubscriptions,
  invalidateUserSubscription,
  resetUserSubscriptionsByPlan,
  setUserSubscriptionTier,
} from '../api'
import { formatPlanPrice, formatTimestamp } from '../lib'
import type { PlanRecord, UserSubscriptionRecord } from '../types'

interface Props {
  userId: number
  username?: string
  onChanged?: () => void
}

function effectiveTier(
  planConcurrency: number,
  planRpm: number,
  overrideConcurrency: number,
  overrideRpm: number
): { concurrency: number; rpm: number; overridden: boolean } {
  let concurrency = planConcurrency
  let rpm = planRpm
  let overridden = false
  if (overrideConcurrency > 0) {
    concurrency = overrideConcurrency
    overridden = true
  }
  if (overrideRpm > 0) {
    rpm = overrideRpm
    overridden = true
  }
  return { concurrency, rpm, overridden }
}

function SubscriptionStatusBadge(props: {
  sub: UserSubscriptionRecord['subscription']
  t: (key: string) => string
}) {
  const now = Date.now() / 1000
  const isExpired =
    (props.sub.end_time || 0) > 0 && props.sub.end_time < now
  const isActive = props.sub.status === 'active' && !isExpired
  if (isActive) {
    return (
      <StatusBadge label={props.t('Active')} variant='success' copyable={false} />
    )
  }
  if (props.sub.status === 'cancelled') {
    return (
      <StatusBadge
        label={props.t('Invalidated')}
        variant='neutral'
        copyable={false}
      />
    )
  }
  return (
    <StatusBadge label={props.t('Expired')} variant='neutral' copyable={false} />
  )
}

/**
 * 管理员在「更新用户」抽屉内查看/管理目标用户的订阅与限速：
 * - 展示每个订阅的套餐、来源、状态、有效期、额度与「当前生效并发/RPM」
 *   （套餐档位 + 管理员覆盖 override，实时生效）
 * - 直接修改并发/RPM override、分配订阅、重置额度、作废、删除
 */
export function UserSubscriptionRateLimitSection(props: Props) {
  const { t } = useTranslation()
  const [loading, setLoading] = useState(false)
  const [creating, setCreating] = useState(false)
  const [plans, setPlans] = useState<PlanRecord[]>([])
  const [subs, setSubs] = useState<UserSubscriptionRecord[]>([])
  const [selectedPlanId, setSelectedPlanId] = useState<string>('')
  const [tierSavingId, setTierSavingId] = useState<number | null>(null)
  const [confirmAction, setConfirmAction] = useState<{
    type: 'invalidate' | 'delete'
    subId: number
  } | null>(null)
  const [resetAction, setResetAction] = useState<{
    planId: number
    planTitle: string
  } | null>(null)
  const [resetting, setResetting] = useState(false)
  const [advanceResetTime, setAdvanceResetTime] = useState(true)

  const planMap = useMemo(() => {
    const map = new Map<number, PlanRecord['plan']>()
    plans.forEach((p) => {
      if (p.plan.id) {map.set(p.plan.id, p.plan)}
    })
    return map
  }, [plans])

  const loadData = useCallback(async () => {
    setLoading(true)
    try {
      const [plansRes, subsRes] = await Promise.all([
        getAdminPlans(),
        getUserSubscriptions(props.userId),
      ])
      if (plansRes.success) {
        setPlans(plansRes.data || [])
      } else {
        handleServerError(plansRes)
      }
      if (subsRes.success) {
        setSubs(subsRes.data || [])
      } else {
        handleServerError(subsRes)
      }
    } catch (error) {
      handleServerError(error, t('Loading failed'))
    } finally {
      setLoading(false)
    }
  }, [props.userId, t])

  useEffect(() => {
    void loadData()
  }, [loadData])

  const handleCreate = async () => {
    if (!selectedPlanId) {
      toast.error(t('Please select a subscription plan'))
      return
    }
    setCreating(true)
    try {
      const res = await createUserSubscription(props.userId, {
        plan_id: Number(selectedPlanId),
      })
      if (res.success) {
        toast.success(res.data?.message || t('Added successfully'))
        setSelectedPlanId('')
        await loadData()
        props.onChanged?.()
      } else {
        handleServerError(res)
      }
    } catch (error) {
      handleServerError(error, t('Request failed'))
    } finally {
      setCreating(false)
    }
  }

  const handleTierOverride = async (
    subId: number,
    rpm: number,
    concurrency: number
  ) => {
    setTierSavingId(subId)
    try {
      const res = await setUserSubscriptionTier(subId, {
        rpm_override: rpm,
        concurrency_override: concurrency,
      })
      if (res.success) {
        toast.success(t('Tier override saved'))
        await loadData()
        props.onChanged?.()
      } else {
        handleServerError(res)
      }
    } catch (error) {
      handleServerError(error, t('Operation failed'))
    } finally {
      setTierSavingId(null)
    }
  }

  const handleConfirmAction = async () => {
    if (!confirmAction) {return}
    try {
      if (confirmAction.type === 'invalidate') {
        const res = await invalidateUserSubscription(confirmAction.subId)
        if (res.success) {
          toast.success(res.data?.message || t('Has been invalidated'))
          await loadData()
          props.onChanged?.()
        } else {
          handleServerError(res)
        }
      } else {
        const res = await deleteUserSubscription(confirmAction.subId)
        if (res.success) {
          toast.success(t('Deleted'))
          await loadData()
          props.onChanged?.()
        } else {
          handleServerError(res)
        }
      }
    } catch (error) {
      handleServerError(error, t('Operation failed'))
    } finally {
      setConfirmAction(null)
    }
  }

  const handleResetConfirm = async () => {
    if (!resetAction) {return}
    setResetting(true)
    try {
      const res = await resetUserSubscriptionsByPlan(props.userId, {
        plan_id: resetAction.planId,
        advance_reset_time: advanceResetTime,
      })
      if (res.success) {
        toast.success(
          t('Reset {{count}} active subscriptions', {
            count: res.data?.reset_count || 0,
          })
        )
        await loadData()
        props.onChanged?.()
      } else {
        handleServerError(res)
      }
    } catch (error) {
      handleServerError(error, t('Operation failed'))
    } finally {
      setResetting(false)
      setResetAction(null)
    }
  }

  let listContent: ReactNode
  if (loading) {
    listContent = (
      <p className='text-muted-foreground text-sm'>{t('Loading...')}</p>
    )
  } else if (subs.length === 0) {
    listContent = (
      <p className='text-muted-foreground text-sm'>
        {t('No subscription records')}
      </p>
    )
  } else {
    listContent = (
      <div className='flex flex-col gap-2'>
        {subs.map((record) => {
          const sub = record.subscription
          const plan = planMap.get(sub.plan_id)
          const now = Date.now() / 1000
          const isActive =
            sub.status === 'active' &&
            ((sub.end_time || 0) <= 0 || sub.end_time >= now)
          const tier = effectiveTier(
            plan?.concurrency_limit || 0,
            plan?.rpm_limit || 0,
            sub.concurrency_override || 0,
            sub.rpm_override || 0
          )
          return (
            <div
              key={sub.id}
              className='border-border/60 flex flex-col gap-2 rounded-md border p-3'
            >
              <div className='flex items-center justify-between gap-2'>
                <div className='min-w-0'>
                  <div className='truncate text-sm font-medium'>
                    {plan?.title || `${t('Plan')} #${sub.plan_id}`}
                  </div>
                  <div className='text-muted-foreground text-xs'>
                    {t('Source')}: {sub.source || '-'} · {t('ID')}: {sub.id}
                  </div>
                </div>
                <SubscriptionStatusBadge sub={sub} t={t} />
              </div>

              <div className='text-muted-foreground grid grid-cols-2 gap-x-3 gap-y-1 text-xs'>
                <span>
                  {t('Start')}: {formatTimestamp(sub.start_time)}
                </span>
                <span>
                  {t('End')}: {formatTimestamp(sub.end_time)}
                </span>
                <span>
                  {t('Total Quota')}:{' '}
                  {Number(sub.amount_total || 0) > 0
                    ? `${formatQuota(sub.amount_used || 0)}/${formatQuota(sub.amount_total)}`
                    : t('Unlimited')}
                </span>
                <span>
                  {t('Current')}:{' '}
                  {tier.concurrency > 0 ? tier.concurrency : '-'}{' '}
                  {t('concurrency/s')} ·{' '}
                  {tier.rpm > 0 ? tier.rpm : '-'} {t('RPM')}
                  {tier.overridden ? ` (${t('Override')})` : ''}
                </span>
              </div>

              <div className='flex flex-wrap items-center gap-2'>
                <TierOverrideInputs
                  sub={sub}
                  saving={tierSavingId === sub.id}
                  onSave={(rpm, concurrency) =>
                    void handleTierOverride(sub.id, rpm, concurrency)
                  }
                />
                <div className='ml-auto flex items-center gap-1'>
                  <Button
                    type='button'
                    size='icon-xs'
                    variant='outline'
                    disabled={!isActive}
                    title={t('Reset quota')}
                    onClick={() => {
                      setAdvanceResetTime(true)
                      setResetAction({
                        planId: sub.plan_id,
                        planTitle:
                          plan?.title || `${t('Plan')} #${sub.plan_id}`,
                      })
                    }}
                  >
                    <RotateCcw size={14} />
                  </Button>
                  <Button
                    type='button'
                    size='icon-xs'
                    variant='outline'
                    disabled={!isActive}
                    title={t('Invalidate')}
                    onClick={() =>
                      setConfirmAction({ type: 'invalidate', subId: sub.id })
                    }
                  >
                    <Ban size={14} />
                  </Button>
                  <Button
                    type='button'
                    size='icon-xs'
                    variant='outline'
                    title={t('Delete')}
                    onClick={() =>
                      setConfirmAction({ type: 'delete', subId: sub.id })
                    }
                  >
                    <Trash2 size={14} />
                  </Button>
                </div>
              </div>
            </div>
          )
        })}
      </div>
    )
  }

  return (
    <div className='flex flex-col gap-3'>
      <div className='flex gap-2'>
        <Combobox
          options={plans.map((p) => ({
            value: String(p.plan.id),
            label: `${p.plan.title} (${formatPlanPrice(p.plan)})`,
          }))}
          value={selectedPlanId}
          onValueChange={(v) => v !== null && setSelectedPlanId(v)}
          className='flex-1'
          placeholder={t('Select subscription plan')}
        />
        <Button
          type='button'
          onClick={handleCreate}
          disabled={creating || !selectedPlanId}
        >
          <Plus className='mr-1 h-4 w-4' />
          {t('Add subscription')}
        </Button>
      </div>

      {listContent}

      {confirmAction && (
        <ConfirmDialog
          open
          onOpenChange={(v) => !v && setConfirmAction(null)}
          title={
            confirmAction.type === 'invalidate'
              ? t('Confirm invalidate')
              : t('Confirm delete')
          }
          desc={
            confirmAction.type === 'invalidate'
              ? t(
                  'After invalidating, this subscription will be immediately deactivated. Historical records are not affected. Continue?'
                )
              : t(
                  'Deleting will permanently remove this subscription record (including benefit details). Continue?'
                )
          }
          handleConfirm={handleConfirmAction}
          destructive={confirmAction.type === 'delete'}
        />
      )}

      {resetAction && (
        <ConfirmDialog
          open
          onOpenChange={(v) => !v && setResetAction(null)}
          title={t('Reset subscription quota')}
          desc={t('Reset active {{plan}} subscriptions for this user?', {
            plan: resetAction.planTitle,
          })}
          confirmText={t('Reset quota')}
          handleConfirm={handleResetConfirm}
          isLoading={resetting}
        >
          <label className='flex items-center justify-between gap-3 rounded-md border px-3 py-2 text-sm'>
            <span>{t('Advance next reset time')}</span>
            <Switch
              checked={advanceResetTime}
              onCheckedChange={(checked) => setAdvanceResetTime(!!checked)}
              aria-label={t('Advance next reset time')}
            />
          </label>
        </ConfirmDialog>
      )}
    </div>
  )
}

function TierOverrideInputs(props: {
  sub: UserSubscriptionRecord['subscription']
  saving: boolean
  onSave: (rpm: number, concurrency: number) => void
}) {
  const { t } = useTranslation()
  const [rpm, setRpm] = useState(String(props.sub.rpm_override || 0))
  const [concurrency, setConcurrency] = useState(
    String(props.sub.concurrency_override || 0)
  )
  const parse = (v: string) => Math.max(0, Number.parseInt(v, 10) || 0)

  return (
    <div className='flex items-center gap-1.5'>
      <Input
        type='number'
        min={0}
        value={concurrency}
        onChange={(e) => setConcurrency(e.target.value)}
        aria-label={t('Concurrency override')}
        className='h-8 w-[70px]'
        title={t('Concurrency override')}
      />
      <Input
        type='number'
        min={0}
        value={rpm}
        onChange={(e) => setRpm(e.target.value)}
        aria-label={t('RPM override')}
        className='h-8 w-[70px]'
        title={t('RPM override')}
      />
      <Button
        type='button'
        size='icon-xs'
        variant='outline'
        disabled={props.saving}
        onClick={() => props.onSave(parse(rpm), parse(concurrency))}
        aria-label={t('Save tier override')}
      >
        <Save size={14} />
      </Button>
    </div>
  )
}
