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
import { CalendarClock, RefreshCw, Wallet } from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from '@tanstack/react-router'

import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { formatQuota } from '@/lib/format'
import { cn } from '@/lib/utils'

import { formatDuration, formatPlanPrice } from '../lib'
import type {
  PlanRecord,
  SubscriptionPlan,
  UserSubscriptionRecord,
} from '../types'

interface PlansCompareTableProps {
  plans: PlanRecord[]
  subscriptions: UserSubscriptionRecord[]
  isSignedIn: boolean
  renewingPlanId: number | null
  onRenew: (planId: number) => void
  onSubscribe: (plan: PlanRecord) => void
}

interface PlanSubscriptionState {
  subscription: UserSubscriptionRecord['subscription'] | null
  isActive: boolean
}

function getPlanSubscriptionState(
  planId: number,
  subscriptions: UserSubscriptionRecord[]
): PlanSubscriptionState {
  if (!subscriptions || subscriptions.length === 0) {
    return { subscription: null, isActive: false }
  }
  const now = Date.now() / 1000
  let active: UserSubscriptionRecord['subscription'] | null = null
  let latest: UserSubscriptionRecord['subscription'] | null = null
  for (const record of subscriptions) {
    const sub = record.subscription
    if (!sub || sub.plan_id !== planId) {
      continue
    }
    if (!latest || sub.end_time > latest.end_time) {
      latest = sub
    }
    if (sub.status === 'active' && sub.end_time > now) {
      if (!active || sub.end_time > active.end_time) {
        active = sub
      }
    }
  }
  if (active) {
    return { subscription: active, isActive: true }
  }
  return { subscription: latest, isActive: false }
}

function getRemainingQuota(
  subscription: UserSubscriptionRecord['subscription']
): string {
  if (!subscription) {
    return '0'
  }
  const total = Number(subscription.amount_total || 0)
  if (total <= 0) {
    return 'unlimited'
  }
  const used = Number(subscription.amount_used || 0)
  return String(Math.max(0, total - used))
}

function FormatValue({ value, unit }: { value: string; unit?: string }) {
  return (
    <span className='text-muted-foreground whitespace-nowrap text-xs'>
      {value}
      {unit ? ` ${unit}` : ''}
    </span>
  )
}

export function PlansCompareTable(props: PlansCompareTableProps) {
  const { t } = useTranslation()

  const planStates = useMemo(() => {
    const map = new Map<number, PlanSubscriptionState>()
    for (const planRecord of props.plans) {
      const planId = planRecord?.plan?.id
      if (planId) {
        map.set(planId, getPlanSubscriptionState(planId, props.subscriptions))
      }
    }
    return map
  }, [props.plans, props.subscriptions])

  const renderPlanActions = (
    planRecord: PlanRecord,
    plan: SubscriptionPlan,
    state: PlanSubscriptionState
  ) => {
    if (state.isActive) {
      if (plan.allow_balance_pay === false) {
        return (
          <Tooltip>
            <TooltipTrigger render={<span tabIndex={0} />}>
              <Button
                size='sm'
                variant='outline'
                disabled
                className='cursor-not-allowed'
              >
                {t('Renew Now')}
              </Button>
            </TooltipTrigger>
            <TooltipContent>
              {t(
                'This plan does not support balance renewal. Manage it from your wallet.'
              )}
            </TooltipContent>
          </Tooltip>
        )
      }
      return (
        <Button
          size='sm'
          onClick={() => props.onRenew(plan.id)}
          disabled={props.renewingPlanId === plan.id}
        >
          <RefreshCw
            className={cn(
              'size-3.5',
              props.renewingPlanId === plan.id && 'animate-spin'
            )}
            aria-hidden
          />
          {t('Renew Now')}
        </Button>
      )
    }

    if (plan.allow_balance_pay === false) {
      return (
        <Button size='sm' variant='outline' render={<Link to='/wallet' />}>
          <Wallet className='size-3.5' aria-hidden />
          {t('Buy in Wallet')}
        </Button>
      )
    }

    return (
      <Button
        size='sm'
        variant='outline'
        onClick={() => props.onSubscribe(planRecord)}
      >
        {state.subscription ? t('Resubscribe') : t('Subscribe')}
      </Button>
    )
  }

  return (
    <div className='overflow-x-auto rounded-xl border shadow-card'>
      <table className='w-full min-w-[760px] border-collapse text-sm'>
        <caption className='sr-only'>
          {t('Subscription plan comparison')}
        </caption>
        <thead>
          <tr className='border-b bg-muted/40 text-muted-foreground text-left text-xs tracking-wider uppercase'>
            <th scope='col' className='px-3 py-3 font-medium sm:px-4'>
              {t('Plan')}
            </th>
            <th scope='col' className='px-3 py-3 font-medium sm:px-4'>
              {t('Price')}
            </th>
            <th scope='col' className='px-3 py-3 font-medium sm:px-4'>
              {t('Validity Period')}
            </th>
            <th scope='col' className='px-3 py-3 font-medium sm:px-4'>
              {t('Plan Quota')}
            </th>
            <th scope='col' className='px-3 py-3 font-medium sm:px-4'>
              {t('Concurrency')}
            </th>
            <th scope='col' className='px-3 py-3 font-medium sm:px-4'>
              {t('RPM')}
            </th>
            <th scope='col' className='px-3 py-3 font-medium sm:px-4'>
              {t('Status')}
            </th>
            <th scope='col' className='px-3 py-3 text-right font-medium sm:px-4'>
              {t('Action')}
            </th>
          </tr>
        </thead>
        <tbody>
          {props.plans.map((planRecord, index) => {
            const plan = planRecord?.plan
            if (!plan) {
              return null
            }
            const state = planStates.get(plan.id) || {
              subscription: null,
              isActive: false,
            }
            const totalAmount = Number(plan.total_amount || 0)
            const remainingRaw = state.subscription
              ? getRemainingQuota(state.subscription)
              : '0'
            const remainingLabel =
              remainingRaw === 'unlimited'
                ? t('Unlimited')
                : formatQuota(Number(remainingRaw))

            return (
              <tr
                key={plan.id}
                className={cn(
                  'border-b last:border-0',
                  index % 2 === 1 && 'bg-muted/20'
                )}
              >
                <td className='px-3 py-3 sm:px-4'>
                  <div className='min-w-0'>
                    <div className='flex items-center gap-2 font-medium'>
                      <span className='truncate'>{plan.title}</span>
                      {state.isActive && (
                        <StatusBadge
                          variant='success'
                          copyable={false}
                          className='shrink-0'
                        >
                          {t('Active')}
                        </StatusBadge>
                      )}
                    </div>
                    {plan.subtitle && (
                      <p className='text-muted-foreground truncate text-xs'>
                        {plan.subtitle}
                      </p>
                    )}
                  </div>
                </td>
                <td className='px-3 py-3 sm:px-4'>
                  <span className='font-semibold whitespace-nowrap'>
                    {formatPlanPrice(plan)}
                  </span>
                </td>
                <td className='px-3 py-3 sm:px-4'>
                  <FormatValue value={formatDuration(plan, t)} />
                </td>
                <td className='px-3 py-3 sm:px-4'>
                  <FormatValue
                    value={
                      totalAmount > 0 ? formatQuota(totalAmount) : t('Unlimited')
                    }
                  />
                </td>
                <td className='px-3 py-3 sm:px-4'>
                  <FormatValue
                    value={
                      (plan.concurrency_limit || 0) > 0
                        ? String(plan.concurrency_limit)
                        : t('Unlimited')
                    }
                  />
                </td>
                <td className='px-3 py-3 sm:px-4'>
                  <FormatValue
                    value={
                      (plan.rpm_limit || 0) > 0
                        ? String(plan.rpm_limit)
                        : t('Unlimited')
                    }
                  />
                </td>
                <td className='px-3 py-3 sm:px-4'>
                  {state.subscription ? (
                    <div className='space-y-1'>
                      <span className='text-muted-foreground flex items-center gap-1 text-xs'>
                        <CalendarClock
                          className='size-3.5 shrink-0'
                          aria-hidden
                        />
                        {new Date(
                          (state.subscription.end_time || 0) * 1000
                        ).toLocaleDateString()}
                      </span>
                      <span className='text-muted-foreground text-xs'>
                        {state.isActive
                          ? `${t('Remaining')} ${remainingLabel}`
                          : t('Expired')}
                      </span>
                    </div>
                  ) : (
                    <span className='text-muted-foreground text-xs'>
                      {t('Not subscribed')}
                    </span>
                  )}
                </td>
                <td className='px-3 py-3 text-right sm:px-4'>
                  <div className='flex justify-end'>
                    {renderPlanActions(planRecord, plan, state)}
                  </div>
                </td>
              </tr>
            )
          })}
        </tbody>
      </table>
    </div>
  )
}