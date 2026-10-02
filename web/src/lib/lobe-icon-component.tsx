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
/**
 * LobeHub icon renderer.
 *
 * Icons are loaded on demand (one chunk per icon) and rendered once resolved.
 * Use `getLobeIcon(name, size)`; the returned node reserves the icon's box while
 * loading so surrounding layout does not shift.
 */
import { useEffect, useState } from 'react'

import { FALLBACK_SIZE, resolveIcon, type ResolvedIcon } from './lobe-icon-core'

type LobeIconState =
  | { status: 'loading' }
  | { status: 'ready'; resolved: ResolvedIcon }
  | { status: 'missing' }

export function LobeIconFallback(props: { size: number; label?: string | null }) {
  const firstLetter = props.label?.trim().charAt(0).toUpperCase() || '?'
  return (
    <div
      className='bg-muted text-muted-foreground flex items-center justify-center rounded-full text-xs font-medium'
      style={{ width: props.size, height: props.size }}
    >
      {firstLetter}
    </div>
  )
}

/**
 * Render a LobeHub icon by name. Loads the icon module on demand; while loading
 * it reserves the icon's box so surrounding layout does not shift.
 */
export function LobeIcon(props: {
  name?: string | null
  size?: number
}): React.ReactNode {
  const size = props.size ?? FALLBACK_SIZE
  const trimmedName = props.name?.trim() ?? ''
  const [state, setState] = useState<LobeIconState>({ status: 'loading' })

  useEffect(() => {
    let cancelled = false

    if (!trimmedName) {
      setState({ status: 'missing' })
      return
    }

    setState({ status: 'loading' })
    void resolveIcon(trimmedName, size).then((resolved) => {
      if (cancelled) {return}
      setState(resolved ? { status: 'ready', resolved } : { status: 'missing' })
    })

    return () => {
      cancelled = true
    }
  }, [trimmedName, size])

  if (state.status === 'ready') {
    const { Comp, props: iconProps } = state.resolved
    return <Comp {...iconProps} />
  }

  // Loading: reserve the box to avoid layout shift. Missing: show the letter.
  return state.status === 'loading' ? (
    <span
      aria-hidden='true'
      style={{ width: size, height: size, display: 'inline-block' }}
    />
  ) : (
    <LobeIconFallback size={size} label={props.name} />
  )
}
