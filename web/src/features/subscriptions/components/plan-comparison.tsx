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
import { ArrowLeft, Crown, Info, LogIn, RefreshCw, Wallet } from 'lucide-react'
import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from '@tanstack/react-router'
import { toast } from 'sonner'

import { PublicLayout } from '@/components/layout'
import { PageTransition } from '@/components/page-transition'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import {
  getFriendlyErrorMessage,
  requireServerSuccess,
} from '@/lib/server-error-message'
import { handleServerError } from '@/lib/handle-server-error'
import { useAuthStore } from '@/stores/auth-store'

import {
  getPublicPlans,
  getSelfSubscriptionFull,
  paySubscriptionBalance,
} from '../api'
import { SubscriptionPurchaseDialog } from './dialogs/subscription-purchase-dialog'
import { PlansCompareTable } from './plans-compare-table'
import type { PlanRecord, UserSubscriptionRecord } from '../types'

export function PlanComparison() {
  const { t } = useTranslation()
  const user = useAuthStore((s) => s.auth.user)

  const [plans, setPlans] = useState<PlanRecord[]>([])
  const [subscriptions, setSubscriptions] = useState<
    UserSubscriptionRecord[]
  >([])
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [renewingPlanId, setRenewingPlanId] = useState<number | null>(null)
  const [purchaseOpen, setPurchaseOpen] = useState(false)
  const [selectedPlan, setSelectedPlan] = useState<PlanRecord | null>(null)

  const isSignedIn = Boolean(user)

  const loadData = useCallback(async () => {
    if (!isSignedIn) {
      setPlans([])
      setSubscriptions([])
      setLoading(false)
      return
    }
    try {
      setLoading(true)
      setLoadError(null)
      const [plansRes, selfRes] = await Promise.all([
        getPublicPlans(),
        getSelfSubscriptionFull(),
      ])
      const plansData = requireServerSuccess(plansRes)
      setPlans(plansData?.data || [])
      if (requireServerSuccess(selfRes)?.data) {
        setSubscriptions(selfRes.data?.all_subscriptions || [])
      }
    } catch (error) {
      setLoadError(
        getFriendlyErrorMessage(error) ||
          t('Failed to load subscription plans. Please try again.')
      )
      setPlans([])
      setSubscriptions([])
    } finally {
      setLoading(false)
    }
  }, [isSignedIn, t])

  useEffect(() => {
    void loadData()
  }, [loadData])

  const handleRenew = async (planId: number) => {
    setRenewingPlanId(planId)
    try {
      const res = await paySubscriptionBalance({ plan_id: planId })
      if (res.success) {
        toast.success(t('Subscription renewed successfully'))
        await loadData()
      } else {
        handleServerError(res, t('Payment request failed'))
      }
    } catch (error) {
      handleServerError(error, t('Payment request failed'))
    } finally {
      setRenewingPlanId(null)
    }
  }

  const handleSubscribe = (plan: PlanRecord) => {
    setSelectedPlan(plan)
    setPurchaseOpen(true)
  }

  const handlePurchaseSuccess = async () => {
    await loadData()
  }

  const renderHeader = () => (
    <header className='mx-auto mb-5 max-w-3xl pt-5 text-center sm:mb-10 sm:pt-10'>
      <h1 className='text-[clamp(2rem,5.5vw,3.5rem)] leading-[1.15] font-bold tracking-tight'>
        {t('Plan Comparison')}
      </h1>
      <p className='text-muted-foreground/80 mt-3 text-sm sm:mt-4 sm:text-base'>
        {t(
          'Compare subscription plans by price, quota, concurrency, RPM and validity period.'
        )}
      </p>
      <p className='text-muted-foreground/60 mx-auto mt-2 max-w-2xl text-xs leading-relaxed sm:text-sm'>
        {t(
          'Choose a plan that fits your usage. Your subscription applies immediately after purchase.'
        )}
      </p>
    </header>
  )

  const renderSignInCta = () => (
    <div className='mx-auto w-full max-w-md'>
      <div className='bg-background/60 flex flex-col items-center gap-4 rounded-2xl border p-6 text-center shadow-card sm:p-8'>
        <div className='bg-muted flex size-10 items-center justify-center rounded-full'>
          <LogIn className='text-muted-foreground size-5' aria-hidden />
        </div>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Sign in to view subscription plans, check your balances and renew with one click.'
          )}
        </p>
        <div className='flex flex-wrap items-center justify-center gap-2'>
          <Button render={<Link to='/sign-in' />}>{t('Sign in')}</Button>
          <Button
            variant='outline'
            render={<Link to='/pricing' />}
          >
            <ArrowLeft className='size-4' aria-hidden />
            {t('Back to Pricing')}
          </Button>
        </div>
      </div>
    </div>
  )

  const renderEmptyState = () => (
    <div className='mx-auto w-full max-w-md'>
      <div className='bg-background/60 flex flex-col items-center gap-4 rounded-2xl border p-6 text-center shadow-card sm:p-8'>
        <div className='bg-muted flex size-10 items-center justify-center rounded-full'>
          <Crown className='text-muted-foreground size-5' aria-hidden />
        </div>
        <p className='text-muted-foreground text-sm'>
          {t('No subscription plans are currently available.')}
        </p>
        <Button variant='outline' render={<Link to='/pricing' />}>
          <ArrowLeft className='size-4' aria-hidden />
          {t('Back to Pricing')}
        </Button>
      </div>
    </div>
  )

  const renderErrorState = () => (
    <div className='mx-auto flex w-full max-w-md flex-col items-center gap-4 px-4 py-10 text-center'>
      <p className='text-muted-foreground max-w-md text-sm'>
        {loadError || t('Failed to load subscription plans. Please try again.')}
      </p>
      <Button variant='outline' onClick={() => void loadData()}>
        <RefreshCw className='size-4' aria-hidden />
        {t('Retry')}
      </Button>
    </div>
  )

  const renderContent = () => {
    if (loading) {
      return (
        <div aria-busy='true' className='space-y-4'>
          <Skeleton className='h-9 w-48 max-w-full' />
          {Array.from({ length: 3 }, (_, index) => (
            <div key={index} className='flex flex-col gap-3'>
              <Skeleton className='h-8 w-full max-w-[30rem]' />
              <Skeleton className='h-6 w-full' />
              <Skeleton className='h-6 w-3/4' />
            </div>
          ))}
        </div>
      )
    }

    if (!isSignedIn) {
      return renderSignInCta()
    }

    if (loadError) {
      return renderErrorState()
    }

    if (plans.length === 0) {
      return renderEmptyState()
    }

    return (
      <div className='space-y-4'>
        <div className='flex flex-wrap items-center justify-between gap-2'>
          <p className='text-muted-foreground text-xs sm:text-sm'>
            {t('{{count}} plans on sale', { count: plans.length })}
          </p>
          <Button size='sm' variant='ghost' render={<Link to='/pricing' />}>
            <ArrowLeft className='size-3.5' aria-hidden />
            {t('Back to Pricing')}
          </Button>
        </div>

        <PlansCompareTable
          plans={plans}
          subscriptions={subscriptions}
          isSignedIn={isSignedIn}
          renewingPlanId={renewingPlanId}
          onRenew={(planId) => void handleRenew(planId)}
          onSubscribe={handleSubscribe}
        />

        <div className='bg-muted/40 flex flex-col gap-2 rounded-xl border px-4 py-3 text-xs sm:flex-row sm:items-center sm:justify-between sm:gap-4'>
          <p className='text-muted-foreground/80 flex items-start gap-2 leading-relaxed sm:items-center'>
            <Info className='mt-0.5 size-3.5 shrink-0 sm:mt-0' aria-hidden />
            {t(
              'Subscriptions stop automatically when they expire. No automatic renewal or charge. You can renew manually from this page at any time.'
            )}
          </p>
          {isSignedIn && (
            <Button
              size='sm'
              variant='ghost'
              className='shrink-0 self-start sm:self-auto'
              render={<Link to='/wallet' />}
            >
              <Wallet className='size-3.5' aria-hidden />
              {t('Top up balance')}
            </Button>
          )}
        </div>
      </div>
    )
  }

  return (
    <PublicLayout showMainContainer={false}>
      <div className='relative'>
        <div
          aria-hidden
          className='pointer-events-none absolute inset-x-0 top-0 h-[600px] opacity-20 dark:opacity-[0.10]'
          style={{
            background: [
              'radial-gradient(ellipse 60% 50% at 20% 20%, oklch(0.72 0.18 250 / 80%) 0%, transparent 70%)',
              'radial-gradient(ellipse 50% 40% at 80% 15%, oklch(0.65 0.15 200 / 60%) 0%, transparent 70%)',
              'radial-gradient(ellipse 40% 35% at 50% 70%, oklch(0.70 0.12 280 / 40%) 0%, transparent 70%)',
            ].join(', '),
            maskImage:
              'linear-gradient(to bottom, black 40%, transparent 100%)',
            WebkitMaskImage:
              'linear-gradient(to bottom, black 40%, transparent 100%)',
          }}
        />
        <PageTransition className='relative mx-auto w-full max-w-[1200px] px-3 pt-16 pb-8 sm:px-6 sm:pt-20 sm:pb-10 xl:px-8'>
          {renderHeader()}
          {renderContent()}
        </PageTransition>
      </div>

      <SubscriptionPurchaseDialog
        open={purchaseOpen}
        onOpenChange={(open) => {
          setPurchaseOpen(open)
          if (!open) {
            setSelectedPlan(null)
          }
        }}
        plan={selectedPlan}
        enableStripe={false}
        enableCreem={false}
        enableWaffoPancake={false}
        enableOnlineTopUp={false}
        epayMethods={[]}
        userQuota={user?.quota ?? 0}
        onPurchaseSuccess={() => void handlePurchaseSuccess()}
        purchaseLimit={
          selectedPlan?.plan?.max_purchase_per_user
            ? Number(selectedPlan.plan.max_purchase_per_user)
            : undefined
        }
        purchaseCount={
          selectedPlan?.plan?.id
            ? subscriptions.filter(
                (record) =>
                  record.subscription?.plan_id === selectedPlan.plan.id
              ).length
            : undefined
        }
      />
    </PublicLayout>
  )
}