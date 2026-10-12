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
import { formatTimestampRelative } from '@/lib/format'

import type { PublicStatusBackground } from '../types'

/**
 * Background task activity, straight from the server's loop heartbeats.
 *
 * The activity window is printed verbatim: it is the definition of the answer
 * ("any heartbeat within this window"), not each task's own schedule — saying
 * it any other way would overstate what the server knows.
 */
export function StatusBackgroundCard(props: {
  background: PublicStatusBackground
}) {
  const { t } = useTranslation()
  const background = props.background
  const windowSeconds = background.activity_window_seconds

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('Background tasks')}</CardTitle>
        <CardDescription>
          {t(
            'A heartbeat within the last {{seconds}} seconds counts as running. That window is a coarse activity check, not each task’s own schedule.',
            { seconds: windowSeconds }
          )}
        </CardDescription>
      </CardHeader>
      <CardContent>
        {background.loops.length === 0 ? (
          <p className='text-muted-foreground text-sm'>
            {t('No heartbeat has been recorded yet.')}
          </p>
        ) : (
          <ul>
            {background.loops.map((loop) => (
              <li
                key={loop.name}
                className='border-border/50 flex items-center justify-between gap-4 border-b py-2.5 last:border-b-0'
              >
                <span className='truncate font-mono text-xs'>{loop.name}</span>
                <span className='text-muted-foreground shrink-0 text-xs tabular-nums'>
                  {formatTimestampRelative(loop.last_run_at)}
                </span>
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  )
}
