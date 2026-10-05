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
import { Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { ArrowRight, Check, X } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button, buttonVariants } from '@/components/ui/button'
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { getApiKeys } from '@/features/keys/api'
import { cn } from '@/lib/utils'
import { requireServerSuccess } from '@/lib/server-error-message'
import { useAuthStore } from '@/stores/auth-store'

import { dismissOnboarding, isOnboardingDismissed } from '../lib/storage'

interface OnboardingStep {
  title: string
  description: string
  cta: string
  to: string
  completed: boolean
}

/**
 * B2-3 first-run guide: Create Token -> Model plaza -> First request.
 *
 * The card is shown once per account until dismissed (localStorage keyed by
 * user id). Every control is a native Button/Link so the whole flow is
 * keyboard accessible. Existing Card/Button components are reused as-is.
 */
export function OnboardingGuide() {
  const user = useAuthStore((state) => state.auth.user)
  const [dismissed, setDismissed] = useState(true)

  // `user` starts null and is filled in after the auth/session query settles.
  // Reading localStorage only in a useState initializer froze the decision at
  // `user === null` → dismissed=true, so the guide only appeared after a
  // refresh. Recompute whenever the user identity becomes available.
  useEffect(() => {
    if (!user) {
      setDismissed(true)
      return
    }
    setDismissed(isOnboardingDismissed(user.id))
  }, [user])

  if (!user || dismissed) {
    return null
  }

  return (
    <GuideCard
      requestCount={Number(user.request_count ?? 0)}
      quota={Number(user.quota ?? 0)}
      usedQuota={Number(user.used_quota ?? 0)}
      onDismiss={() => {
        dismissOnboarding(user.id)
        setDismissed(true)
      }}
    />
  )
}

interface GuideCardProps {
  requestCount: number
  quota: number
  usedQuota: number
  onDismiss: () => void
}

/**
 * Guide body. Split out so the per-account dismissal check and the async
 * progress query can run without re-running the identity effect above.
 */
function GuideCard(props: GuideCardProps) {
  const { t } = useTranslation()
  const [stepIndex, setStepIndex] = useState(0)

  // Progress source: same endpoints the dashboard setup panel uses.
  const apiKeysQuery = useQuery({
    queryKey: ['onboarding', 'api-keys'],
    queryFn: async () => {
      const result = requireServerSuccess(await getApiKeys({ p: 1, size: 1 }))
      return result.success ? (result.data?.total ?? 0) : 0
    },
    staleTime: 60 * 1000,
  })

  const steps: OnboardingStep[] = useMemo(
    () => [
      {
        title: t('Create an API Token'),
        description: t(
          'Generate a key to call models from your own code or tools.'
        ),
        cta: t('Go to API keys'),
        to: '/keys',
        completed: (apiKeysQuery.data ?? 0) > 0,
      },
      {
        title: t('Browse the model plaza'),
        description: t(
          'See which models are available and how pricing works.'
        ),
        cta: t('Go to model plaza'),
        to: '/pricing',
        completed: props.quota > 0 || props.usedQuota > 0,
      },
      {
        title: t('Make your first request'),
        description: t('Open the playground and send your first message.'),
        cta: t('Open playground'),
        to: '/playground',
        completed: props.requestCount > 0,
      },
    ],
    [apiKeysQuery.data, props.quota, props.requestCount, props.usedQuota, t]
  )

  const current = steps[stepIndex]
  const isLastStep = stepIndex === steps.length - 1
  const completedCount = steps.filter((step) => step.completed).length

  return (
    <Card size='sm' className='mx-3 mt-3 sm:mx-4 sm:mt-4' data-testid='onboarding-guide'>
      <CardHeader>
        <CardTitle>{t('Get started in 3 steps')}</CardTitle>
        <CardDescription>
          {t('Setup progress: {{completed}}/{{total}}', {
            completed: completedCount,
            total: steps.length,
          })}
        </CardDescription>
        <CardAction>
          <Button
            variant='ghost'
            size='icon-sm'
            aria-label={t('Dismiss onboarding')}
            onClick={props.onDismiss}
          >
            <X className='h-4 w-4' />
          </Button>
        </CardAction>
      </CardHeader>
      <CardContent className='space-y-2'>
        <h3 className='flex items-center gap-1.5 text-sm font-medium'>
          {current.completed ? (
            <Check
              aria-hidden
              className='text-success size-4 shrink-0'
            />
          ) : null}
          {current.title}
        </h3>
        <p className='text-muted-foreground text-sm'>{current.description}</p>
        <Link
          to={current.to}
          className={cn(
            buttonVariants({ variant: 'link', size: 'sm' }),
            'px-0'
          )}
        >
          {current.cta}
          <ArrowRight className='ml-1 h-3.5 w-3.5' />
        </Link>
      </CardContent>
      <CardFooter className='justify-between'>
        <Button variant='ghost' size='sm' onClick={props.onDismiss}>
          {t('Skip for now')}
        </Button>
        <div className='flex items-center gap-2'>
          <Button
            variant='outline'
            size='sm'
            disabled={stepIndex === 0}
            onClick={() => setStepIndex((index) => index - 1)}
          >
            {t('Back')}
          </Button>
          {isLastStep ? (
            <Button size='sm' onClick={props.onDismiss}>
              {t('Done')}
            </Button>
          ) : (
            <Button
              size='sm'
              onClick={() => setStepIndex((index) => index + 1)}
            >
              {t('Next')}
            </Button>
          )}
        </div>
      </CardFooter>
    </Card>
  )
}