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
import { useEffect, useState } from 'react'
import type { IconBaseProps, IconType } from 'react-icons'

/**
 * The icon table lives in its own module (`payment-icons.ts`) and is imported
 * dynamically so the icon data stays in an async chunk instead of the first
 * paint bundle. See that module for why the table is fixed rather than
 * resolving arbitrary `react-icons` names.
 */
type IconTable = Record<string, IconType>

let iconTablePromise: Promise<IconTable> | null = null

function loadIconTable(): Promise<IconTable> {
  iconTablePromise ??= import('./payment-icons').then(
    (module) => module.ICON_BY_NAME
  )
  return iconTablePromise
}

type ReactIconByNameProps = IconBaseProps & {
  name?: string | null
}

type ResolvedIconState = {
  iconName: string
  Icon: IconType | null
}

export function ReactIconByName({ name, ...props }: ReactIconByNameProps) {
  const iconName = name?.trim()
  const [resolvedIcon, setResolvedIcon] = useState<ResolvedIconState | null>(
    null
  )

  useEffect(() => {
    let cancelled = false

    if (!iconName) {
      return
    }

    void loadIconTable().then((table) => {
      if (!cancelled) {
        setResolvedIcon({ iconName, Icon: table[iconName] ?? null })
      }
    })

    return () => {
      cancelled = true
    }
  }, [iconName])

  if (!iconName || resolvedIcon?.iconName !== iconName || !resolvedIcon.Icon) {
    return null
  }

  const Icon = resolvedIcon.Icon

  return <Icon {...props} />
}
