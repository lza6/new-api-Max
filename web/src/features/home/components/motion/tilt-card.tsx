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

interface TiltCardProps {
  children: ReactNode
  className?: string
  /** Maximum tilt in degrees at the corners. */
  maxTilt?: number
}

/**
 * Perspective tilt surface: the card rotates toward the cursor and lifts on the
 * z-axis, giving flat content a 3D feel. Kept off touch-conflicting paths by
 * only reacting to fine pointers and disabled entirely under
 * `prefers-reduced-motion`. The transform lives on the outer element so the
 * inner layout is untouched.
 */
export function TiltCard(props: TiltCardProps) {
  const shouldReduce = useReducedMotion()
  const ref = useRef<HTMLDivElement>(null)
  const maxTilt = props.maxTilt ?? 7

  const rotateX = useSpring(useMotionValue(0), { stiffness: 160, damping: 18 })
  const rotateY = useSpring(useMotionValue(0), { stiffness: 160, damping: 18 })

  const handleMove = useCallback(
    (event: PointerEvent<HTMLDivElement>) => {
      // Mouse only: a touch pointermove while scrolling would snap the card.
      if (shouldReduce || event.pointerType !== 'mouse' || !ref.current) {return}
      const rect = ref.current.getBoundingClientRect()
      const px = (event.clientX - rect.left) / rect.width - 0.5
      const py = (event.clientY - rect.top) / rect.height - 0.5
      rotateY.set(px * maxTilt * 2)
      rotateX.set(-py * maxTilt * 2)
    },
    [maxTilt, rotateX, rotateY, shouldReduce]
  )

  const reset = useCallback(() => {
    rotateX.set(0)
    rotateY.set(0)
  }, [rotateX, rotateY])

  return (
    <div
      ref={ref}
      className={cn('h-full [perspective:1000px]', props.className)}
      onPointerMove={handleMove}
      onPointerLeave={reset}
    >
      <motion.div
        className='h-full [transform-style:preserve-3d]'
        style={{
          rotateX: shouldReduce ? 0 : rotateX,
          rotateY: shouldReduce ? 0 : rotateY,
        }}
      >
        {props.children}
      </motion.div>
    </div>
  )
}
