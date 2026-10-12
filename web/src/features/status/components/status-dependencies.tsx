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

import type { PublicStatusComponent } from '../types'
import { StatusComponentRow } from './status-state-badge'

/**
 * The list of checked dependencies, exactly as the server reported them.
 * No "systems" are invented here: whatever the endpoint returns is what is
 * shown, including items it reports as disabled.
 */
export function StatusDependenciesCard(props: {
  components: PublicStatusComponent[]
}) {
  const { t } = useTranslation()

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('Dependencies')}</CardTitle>
        <CardDescription>
          {t(
            'Probed on the server when these figures were last refreshed. Failures are reported as a state only — no internal addresses are published.'
          )}
        </CardDescription>
      </CardHeader>
      <CardContent>
        {props.components.length === 0 ? (
          <p className='text-muted-foreground text-sm'>{t('No Data')}</p>
        ) : (
          <ul>
            {props.components.map((component) => (
              <StatusComponentRow key={component.key} component={component} />
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  )
}
