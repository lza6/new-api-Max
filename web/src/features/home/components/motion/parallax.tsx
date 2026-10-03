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
import {
  motion,
  useReducedMotion,
  useScroll,
  useSpring,
  useTransform,
} from 'motion/react'
import { useRef, type ReactNode } from 'react'

import { cn } from '@/lib/utils'

interface ParallaxProps {
  children: ReactNode
  /**
   * Vertical travel in px across the element's whole scroll span. The wrapper
   * starts `offset` px below its resting line and ends `offset` px above it, so
   * the element appears to move slower than the page. Higher = stronger depth.
   */
  offset?: number
  className?: string
}

/**
 * Scroll-linked vertical parallax for the landing page.
 *
 * Tracks the element's own progress through the viewport and maps it to a small
 * y translation, softened by a spring so wheel/trackpad jitter does not show up
 * as jitter in the layout. Under `prefers-reduced-motion` the translation is
 * pinned to 0 and only the (static) content renders.
 */
export function Parallax(props: ParallaxProps) {
  const shouldReduce = useReducedMotion()
  const ref = useRef<HTMLDivElement>(null)
  const offset = props.offset ?? 40

  const { scrollYProgress } = useScroll({
    target: ref,
    offset: ['start end', 'end start'],
  })
  const raw = useTransform(scrollYProgress, [0, 1], [offset, -offset])
  const y = useSpring(raw, { stiffness: 120, damping: 26, mass: 0.4 })

  return (
    <motion.div
      ref={ref}
      className={cn('will-change-transform', props.className)}
      style={{ y: shouldReduce ? 0 : y }}
    >
      {props.children}
    </motion.div>
  )
}
