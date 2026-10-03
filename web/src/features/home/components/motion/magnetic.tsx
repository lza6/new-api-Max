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
import { motion, useMotionValue, useReducedMotion, useSpring } from 'motion/react'
import { useCallback, useRef, type PointerEvent, type ReactNode } from 'react'

import { cn } from '@/lib/utils'

interface MagneticProps {
  children: ReactNode
  /** Fraction of the pointer's offset from centre that the element follows. */
  strength?: number
  className?: string
}

/**
 * Pointer-magnetic wrapper: the child drifts toward the cursor while hovered and
 * springs back on leave. Used to give primary CTAs a tactile pull. Honours
 * `prefers-reduced-motion` (stays put) and never captures pointer events itself,
 * so the wrapped control keeps its own hit target and keyboard focus.
 */
export function Magnetic(props: MagneticProps) {
  const shouldReduce = useReducedMotion()
  const ref = useRef<HTMLDivElement>(null)
  const strength = props.strength ?? 0.28

  const x = useSpring(useMotionValue(0), { stiffness: 200, damping: 15, mass: 0.4 })
  const y = useSpring(useMotionValue(0), { stiffness: 200, damping: 15, mass: 0.4 })

  const handleMove = useCallback(
    (event: PointerEvent<HTMLDivElement>) => {
      // Mouse only: on touch, pointermove fires while the finger scrolls and
      // would yank the element around mid-swipe.
      if (shouldReduce || event.pointerType !== 'mouse' || !ref.current) {return}
      const rect = ref.current.getBoundingClientRect()
      x.set((event.clientX - (rect.left + rect.width / 2)) * strength)
      y.set((event.clientY - (rect.top + rect.height / 2)) * strength)
    },
    [shouldReduce, strength, x, y]
  )

  const reset = useCallback(() => {
    x.set(0)
    y.set(0)
  }, [x, y])

  return (
    <motion.div
      ref={ref}
      className={cn('inline-flex will-change-transform', props.className)}
      style={{ x: shouldReduce ? 0 : x, y: shouldReduce ? 0 : y }}
      onPointerMove={handleMove}
      onPointerLeave={reset}
    >
      {props.children}
    </motion.div>
  )
}

interface SpotlightCardProps {
  children: ReactNode
  className?: string
  /** Tailwind colour for the radial highlight, e.g. `rgba(99,102,241,0.18)`. */
  glow?: string
}

/**
 * Card surface that paints a soft radial highlight following the cursor, based
 * on the "spotlight border" pattern. The glow is a decorative overlay (`aria-hidden`)
 * and is driven by CSS custom properties updated on pointer move, so it costs no
 * React re-renders. Under reduced motion the highlight is simply not shown.
 */
export function SpotlightCard(props: SpotlightCardProps) {
  const shouldReduce = useReducedMotion()
  const ref = useRef<HTMLDivElement>(null)
  const glow = props.glow ?? 'rgba(244,114,182,0.16)'

  const handleMove = useCallback((event: PointerEvent<HTMLDivElement>) => {
    const el = ref.current
    // Mouse only: a touch pointermove during scroll would drag the highlight.
    if (!el || event.pointerType !== 'mouse') {return}
    const rect = el.getBoundingClientRect()
    el.style.setProperty('--spot-x', `${event.clientX - rect.left}px`)
    el.style.setProperty('--spot-y', `${event.clientY - rect.top}px`)
  }, [])

  return (
    <div
      ref={ref}
      onPointerMove={shouldReduce ? undefined : handleMove}
      className={cn('group/spot relative overflow-hidden', props.className)}
    >
      {!shouldReduce && (
        <span
          aria-hidden
          className='pointer-events-none absolute inset-0 opacity-0 transition-opacity duration-300 group-hover/spot:opacity-100'
          style={{
            background: `radial-gradient(240px circle at var(--spot-x, 50%) var(--spot-y, 50%), ${glow}, transparent 72%)`,
          }}
        />
      )}
      {props.children}
    </div>
  )
}
