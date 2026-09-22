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
import { Wrench01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'

import { StatusBadge } from '@/components/status-badge'
import { Badge } from '@/components/ui/badge'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { formatBillingCurrencyFromUSD } from '@/lib/currency'
import { formatLogQuota } from '@/lib/format'

import { hasToolSurcharge } from '../lib/format'
import type { LogOtherData } from '../types'

interface LogCostDisplayProps {
  quota: number
  other: LogOtherData | null
}

function splitQuotaDisplay(value: string): { prefix: string; amount: string } {
  const match = value.match(/^([^0-9+\-.,\s]+)(.+)$/)
  if (!match) {return { prefix: '', amount: value }}
  return { prefix: match[1], amount: match[2] }
}

function formatRatioCompact(ratio: number): string {
  if (!Number.isFinite(ratio)) {return '-'}
  return ratio % 1 === 0
    ? String(ratio)
    : ratio.toFixed(4).replace(/\.?0+$/, '')
}

/**
 * Group-ratio hint shown next to the cost badge: `×0.1` with a tooltip that
 * explains `model price × group ratio = actual cost` so the group ratio is
 * visible in the consumption log instead of only in the details dialog.
 */
function GroupRatioMarker(props: { other: LogOtherData | null }) {
  const { t } = useTranslation()
  const userGroupRatio = props.other?.user_group_ratio
  const isUserRatio =
    userGroupRatio != null &&
    Number.isFinite(userGroupRatio) &&
    userGroupRatio !== -1
  const ratio = isUserRatio ? userGroupRatio : props.other?.group_ratio
  if (ratio == null || !Number.isFinite(ratio) || ratio === 1) {
    return null
  }

  const modelPrice = props.other?.model_price
  const ratioText = `${formatRatioCompact(ratio)}x`
  const modelPriceText =
    modelPrice != null && Number.isFinite(modelPrice)
      ? formatBillingCurrencyFromUSD(modelPrice, {
          digitsLarge: 4,
          digitsSmall: 6,
          abbreviate: false,
        })
      : null
  const ratioLabel = isUserRatio
    ? t('User Exclusive Ratio')
    : t('Group Ratio')
  const detail = modelPriceText
    ? `${t('Model Price')} ${modelPriceText} × ${ratioLabel} ${ratioText}`
    : `${ratioLabel} ${ratioText}`

  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <span className='text-muted-foreground/70 text-[10px] leading-none tabular-nums'>
            ×{ratioText}
          </span>
        }
      />
      <TooltipContent>{detail}</TooltipContent>
    </Tooltip>
  )
}

function ToolSurchargeMarker() {
  const { t } = useTranslation()
  const label = t('Includes tool-call surcharge')

  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <Badge
            variant='warning'
            className='h-5 min-w-5 cursor-help gap-0 rounded-full px-1'
            role='img'
            aria-label={label}
            tabIndex={0}
            data-tool-surcharge-indicator='true'
          >
            <HugeiconsIcon
              icon={Wrench01Icon}
              strokeWidth={2}
              aria-hidden='true'
            />
            <span
              className='text-[9px] leading-none font-bold'
              aria-hidden='true'
            >
              +
            </span>
          </Badge>
        }
      />
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  )
}

function QuotaBadge(props: { quota: number }) {
  const quotaDisplay = splitQuotaDisplay(formatLogQuota(props.quota))

  return (
    <span className='border-border/80 bg-muted/60 inline-flex h-6 w-fit items-center rounded-md border px-2 [font-family:var(--font-body)] text-sm leading-none font-semibold tabular-nums'>
      {quotaDisplay.prefix ? (
        <span className='mr-1'>{quotaDisplay.prefix}</span>
      ) : null}
      <span>{quotaDisplay.amount}</span>
    </span>
  )
}

function SubscriptionBadge(props: { quota: number }) {
  const { t } = useTranslation()

  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <StatusBadge
            label={t('Subscription')}
            variant='success'
            size='sm'
            copyable={false}
            className='cursor-help'
          />
        }
      />
      <TooltipContent>
        <span>
          {t('Deducted by subscription')}: {formatLogQuota(props.quota)}
        </span>
      </TooltipContent>
    </Tooltip>
  )
}

export function LogCostDisplay(props: LogCostDisplayProps) {
  const isSubscription = props.other?.billing_source === 'subscription'
  const showToolSurcharge = hasToolSurcharge(props.other)

  if (!isSubscription && !showToolSurcharge) {
    return (
      <TooltipProvider>
        <div className='flex flex-col gap-0.5'>
          <QuotaBadge quota={props.quota} />
          <GroupRatioMarker other={props.other} />
        </div>
      </TooltipProvider>
    )
  }

  return (
    <TooltipProvider>
      <div className='inline-flex items-center gap-1'>
        {isSubscription ? (
          <SubscriptionBadge quota={props.quota} />
        ) : (
          <QuotaBadge quota={props.quota} />
        )}
        {showToolSurcharge ? <ToolSurchargeMarker /> : null}
      </div>
    </TooltipProvider>
  )
}
