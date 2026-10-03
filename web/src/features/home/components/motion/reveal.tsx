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
import { motion, useReducedMotion } from 'motion/react'
import type { ReactNode } from 'react'

import { cn } from '@/lib/utils'

interface RevealProps {
  children: ReactNode
  className?: string
  /** Seconds to wait before animating in; stagger siblings with an index step. */
  delay?: number
  /** Travel distance in px for the rise-in. */
  distance?: number
}

/**
 * Entrance reveal driven by `whileInView` with a one-shot `viewport`. Unlike the
 * class-based `AnimateInView`, the initial hidden state is expressed through
 * Motion's style system, so a JS-less or pre-hydration paint still holds the
 * element at `opacity: 0` only until Motion takes over on mount (no permanent
 * blank if IntersectionObserver is unavailable). Reduced motion resolves the
 * element to its resting state immediately.
 */
export function Reveal(props: RevealProps) {
  const shouldReduce = useReducedMotion()
  const distance = props.distance ?? 22

  if (shouldReduce) {
    return <div className={props.className}>{props.children}</div>
  }

  return (
    <motion.div
      className={cn('will-change-[transform,opacity]', props.className)}
      initial={{ opacity: 0, y: distance }}
      whileInView={{ opacity: 1, y: 0 }}
      viewport={{ once: true, amount: 0.2, margin: '0px 0px -40px 0px' }}
      transition={{
        duration: 0.6,
        delay: props.delay ?? 0,
        ease: [0.16, 1, 0.3, 1],
      }}
    >
      {props.children}
    </motion.div>
  )
}
