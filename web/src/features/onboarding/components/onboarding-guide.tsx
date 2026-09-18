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
import { ArrowRight, X } from 'lucide-react'
import { useState } from 'react'
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
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/stores/auth-store'

import { dismissOnboarding, isOnboardingDismissed } from '../lib/storage'

interface OnboardingStep {
  title: string
  description: string
  cta: string
  to: string
}

/**
 * B2-3 first-run guide: Create Token -> Model plaza -> First request.
 *
 * The card is shown once per account until dismissed (localStorage keyed by
 * user id). Every control is a native Button/Link so the whole flow is
 * keyboard accessible. Existing Card/Button components are reused as-is.
 */
export function OnboardingGuide() {
  const { t } = useTranslation()
  const user = useAuthStore((state) => state.auth.user)
  const [dismissed, setDismissed] = useState(() =>
    user ? isOnboardingDismissed(user.id) : true
  )
  const [stepIndex, setStepIndex] = useState(0)

  if (!user || dismissed) {
    return null
  }

  const steps: OnboardingStep[] = [
    {
      title: t('Create an API Token'),
      description: t(
        'Generate a key to call models from your own code or tools.'
      ),
      cta: t('Go to API keys'),
      to: '/keys',
    },
    {
      title: t('Browse the model plaza'),
      description: t('See which models are available and how pricing works.'),
      cta: t('Go to model plaza'),
      to: '/pricing',
    },
    {
      title: t('Make your first request'),
      description: t('Open the playground and send your first message.'),
      cta: t('Open playground'),
      to: '/playground',
    },
  ]

  const current = steps[stepIndex]
  const isLastStep = stepIndex === steps.length - 1

  const handleDismiss = () => {
    dismissOnboarding(user.id)
    setDismissed(true)
  }

  return (
    <Card size='sm' className='mx-3 mt-3 sm:mx-4 sm:mt-4' data-testid='onboarding-guide'>
      <CardHeader>
        <CardTitle>{t('Get started in 3 steps')}</CardTitle>
        <CardDescription>
          {t('Step {{current}} of {{total}}', {
            current: stepIndex + 1,
            total: steps.length,
          })}
        </CardDescription>
        <CardAction>
          <Button
            variant='ghost'
            size='icon-sm'
            aria-label={t('Dismiss onboarding')}
            onClick={handleDismiss}
          >
            <X className='h-4 w-4' />
          </Button>
        </CardAction>
      </CardHeader>
      <CardContent className='space-y-2'>
        <h3 className='text-sm font-medium'>{current.title}</h3>
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
        <Button variant='ghost' size='sm' onClick={handleDismiss}>
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
            <Button size='sm' onClick={handleDismiss}>
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