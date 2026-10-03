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
import { useReducedMotion } from 'motion/react'
import { Component, lazy, Suspense, useEffect, useRef, useState, type ReactNode } from 'react'

import { Hero3DShowcase } from './hero-3d-showcase'

// WebGL scene + three.js stay out of the initial landing chunk; they load only
// once the canvas scrolls into view on a capable device.
const GatewayCoreScene = lazy(() => import('../motion/gateway-core-scene'))

type Capability = 'unknown' | 'ok' | 'unsupported'

/**
 * Whether this device should get the WebGL scene at all. Fails closed: anything
 * we cannot confirm — no WebGL context, reduced motion, low core count — falls
 * back to the CSS showcase rather than risking a blank canvas or a janky frame
 * budget on the marketing page.
 */
function detectWebglCapability(reducedMotion: boolean): Capability {
  if (reducedMotion) {return 'unsupported'}
  if (typeof window === 'undefined') {return 'unknown'}
  if (
    typeof navigator !== 'undefined' &&
    typeof navigator.hardwareConcurrency === 'number' &&
    navigator.hardwareConcurrency > 0 &&
    navigator.hardwareConcurrency <= 2
  ) {
    return 'unsupported'
  }
  try {
    const canvas = document.createElement('canvas')
    const gl =
      canvas.getContext('webgl2') || canvas.getContext('webgl')
    if (!gl) {return 'unsupported'}
    // Release the probe context promptly; some browsers cap concurrent contexts.
    const lose = (gl as WebGLRenderingContext).getExtension('WEBGL_lose_context')
    lose?.loseContext()
    return 'ok'
  } catch {
    return 'unsupported'
  }
}

/**
 * Local error boundary around the lazy WebGL chunk. A failed chunk (network or
 * version skew) rejects the dynamic import; without this, the rejection would
 * bubble to the route error component and take down the whole page. Here it
 * degrades to the CSS showcase, preserving the "fails closed" intent.
 */
class WebglErrorBoundary extends Component<
  { children: ReactNode },
  { failed: boolean }
> {
  state = { failed: false }

  static getDerivedStateFromError() {
    return { failed: true }
  }

  render() {
    if (this.state.failed) {return <Hero3DShowcase />}
    return this.props.children
  }
}

interface WebglHeroProps {
  className?: string
}

/**
 * Hero-visual host that upgrades to a live WebGL "gateway core" when the device
 * can afford it and the user has not asked for reduced motion, and otherwise
 * renders the existing CSS 3D showcase. Mounting is deferred until the visual
 * scrolls into view so the WebGL chunk and its first paint never compete with
 * the hero copy for the initial frame budget, and the scene unmounts again once
 * it leaves the viewport so it does not keep rendering (and burning battery)
 * behind the FAQ and footer.
 */
export function WebglHero(props: WebglHeroProps) {
  const shouldReduce = useReducedMotion()
  const containerRef = useRef<HTMLDivElement>(null)
  const [capability, setCapability] = useState<Capability>('unknown')
  const [inView, setInView] = useState(false)

  useEffect(() => {
    setCapability(detectWebglCapability(Boolean(shouldReduce)))
  }, [shouldReduce])

  useEffect(() => {
    const el = containerRef.current
    if (!el || typeof IntersectionObserver === 'undefined') {
      setInView(true)
      return
    }
    // Track both enter and leave: the canvas mounts near the viewport and is
    // torn down once scrolled well past, so an off-screen hero stops the
    // continuous 60fps render loop.
    const observer = new IntersectionObserver(
      ([entry]) => setInView(entry.isIntersecting),
      { rootMargin: '200px' }
    )
    observer.observe(el)
    return () => observer.disconnect()
  }, [])

  const useWebgl = capability === 'ok' && inView && !shouldReduce

  return (
    <div
      ref={containerRef}
      className={props.className}
      aria-hidden={useWebgl ? true : undefined}
    >
      {useWebgl ? (
        <WebglErrorBoundary>
          <Suspense
            fallback={
              <div className='border-border/40 bg-muted/20 hidden h-72 w-full animate-pulse rounded-2xl border sm:block' />
            }
          >
            {/* Matches the CSS fallback's `hidden sm:block`: the 3D visual is a
                desktop enhancement and stays off the narrow mobile column. */}
            <div className='hidden h-72 w-full sm:block sm:h-[360px]'>
              <GatewayCoreScene />
            </div>
          </Suspense>
        </WebglErrorBoundary>
      ) : (
        <Hero3DShowcase />
      )}
    </div>
  )
}
