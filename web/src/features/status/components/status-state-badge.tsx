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

import { StatusBadge } from '@/components/status-badge'

import type { PublicStatusComponent, PublicStatusState } from '../types'

/**
 * State → badge variant. `disabled` and `unknown` are intentionally not
 * "warning": the backend uses them to say "not in use here" / "no observation
 * recorded", which must not look like a fault.
 */
const STATE_VARIANTS: Record<
  PublicStatusState,
  'success' | 'warning' | 'danger' | 'neutral'
> = {
  operational: 'success',
  degraded: 'warning',
  down: 'danger',
  disabled: 'neutral',
  unknown: 'neutral',
}

const STATE_LABELS: Record<PublicStatusState, string> = {
  operational: 'Operational',
  degraded: 'Degraded',
  down: 'Down',
  disabled: 'Disabled',
  unknown: 'Unknown',
}

/** Localized display name of a checked item (unknown keys fall back to the raw key). */
function componentLabelKey(key: string): string {
  switch (key) {
    case 'database':
      return 'Database'
    case 'log_database':
      return 'Log database'
    case 'redis':
      return 'Cache'
    case 'background_jobs':
      return 'Background tasks'
    default:
      return key
  }
}

interface StatusStateBadgeProps {
  state: PublicStatusState
  size?: 'sm' | 'md' | 'lg'
}

export function StatusStateBadge(props: StatusStateBadgeProps) {
  const { t } = useTranslation()
  return (
    <StatusBadge
      variant={STATE_VARIANTS[props.state] ?? 'neutral'}
      label={t(STATE_LABELS[props.state] ?? STATE_LABELS.unknown)}
      size={props.size ?? 'sm'}
      copyable={false}
    />
  )
}

/**
 * One row of the dependency list: display name, state badge, and an optional
 * factual note (shared instance / not configured). The note is derived from
 * fields the backend returns, not guessed locally.
 */
export function StatusComponentRow(props: {
  component: PublicStatusComponent
}) {
  const { t } = useTranslation()
  let note: string | null = null
  if (props.component.shared) {
    note = t('Same instance as the primary database')
  } else if (props.component.state === 'disabled') {
    note = t('Not configured on this deployment')
  }

  return (
    <li className='border-border/50 flex items-center justify-between gap-4 border-b py-2.5 last:border-b-0'>
      <div className='min-w-0'>
        <p className='truncate text-sm font-medium'>
          {t(componentLabelKey(props.component.key))}
        </p>
        {note != null && (
          <p className='text-muted-foreground text-xs'>{note}</p>
        )}
      </div>
      <StatusStateBadge state={props.component.state} />
    </li>
  )
}
