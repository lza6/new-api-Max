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
import { useTranslation } from 'react-i18next'

import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { formatNumber } from '@/lib/format'

import type { PublicStatusTraffic } from '../types'

/**
 * One traffic figure. A `null` value means the backend could not prove it
 * (the log switch is off or the count failed) — it renders as "not available"
 * with the reason, never as 0.
 */
function TrafficValue(props: {
  label: string
  value: number | null
  unavailableReason: string | null
  format: (value: number) => string
}) {
  const { t } = useTranslation()
  const available = props.value != null

  return (
    <div className='border-border/50 rounded-lg border px-4 py-3'>
      <p className='text-muted-foreground text-xs'>{props.label}</p>
      <p className='mt-1 text-2xl font-semibold tabular-nums'>
        {available ? props.format(props.value as number) : t('Not available')}
      </p>
      {!available && props.unavailableReason != null && (
        <p className='text-muted-foreground/80 mt-1 text-xs'>
          {props.unavailableReason}
        </p>
      )}
    </div>
  )
}

/**
 * Request-log statistics over the window the backend reports. The window and
 * the caliber of the success rate are printed on the page so the numbers are
 * never read as an availability claim.
 */
export function StatusTrafficCard(props: { traffic: PublicStatusTraffic }) {
  const { t } = useTranslation()
  const traffic = props.traffic
  const hours = Math.round(traffic.window_seconds / 3600)

  const requestsReason = traffic.consume_logging_enabled
    ? t('The server could not compute this figure on the last refresh.')
    : t(
        'Request counting is off on this deployment: consumption logging is disabled.'
      )
  const errorsReason = traffic.error_logging_enabled
    ? t('The server could not compute this figure on the last refresh.')
    : t('Error counting needs error logging, which is off on this deployment.')
  const successRateReason =
    traffic.requests != null && traffic.errors != null
      ? t(
          'No requests or error entries were recorded in this window, so there is nothing to compute a rate from.'
        )
      : t(
          'The success rate needs both the request and the error count, so it is unavailable while either one is.'
        )

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('Request log statistics')}</CardTitle>
        <CardDescription>
          {t(
            "Window: the last {{hours}} hours, counted on the server from this deployment's request logs.",
            { hours }
          )}
        </CardDescription>
      </CardHeader>
      <CardContent className='space-y-4'>
        <div className='grid gap-3 sm:grid-cols-3'>
          <TrafficValue
            label={t('Requests')}
            value={traffic.requests}
            unavailableReason={requestsReason}
            format={(value) => formatNumber(value)}
          />
          <TrafficValue
            label={t('Error entries')}
            value={traffic.errors}
            unavailableReason={errorsReason}
            format={(value) => formatNumber(value)}
          />
          <TrafficValue
            label={t('Success rate')}
            value={
              traffic.success_rate == null ? null : traffic.success_rate * 100
            }
            unavailableReason={successRateReason}
            format={(value) => `${new Intl.NumberFormat().format(value)}%`}
          />
        </div>
        <p className='text-muted-foreground text-xs'>
          {t(
            'Success rate counts only requests that reached the request log: requests / (requests + error entries). Requests rejected before logging are not part of it.'
          )}
        </p>
      </CardContent>
    </Card>
  )
}
