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
import { render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'

import { WebglHero } from '../webgl-hero'

// The CSS fallback is the safety net: whenever WebGL is unavailable or the user
// asked for reduced motion, this decorative label must still render so the hero
// visual is never blank.
const SHOWCASE_MARKER = 'Smart Routing'

function forceReducedMotion() {
  const original = Object.getOwnPropertyDescriptor(window, 'matchMedia')
  Object.defineProperty(window, 'matchMedia', {
    configurable: true,
    writable: true,
    value: (query: string): MediaQueryList =>
      ({
        matches: query.includes('reduced-motion'),
        media: query,
        onchange: null,
        addListener: () => undefined,
        removeListener: () => undefined,
        addEventListener: () => undefined,
        removeEventListener: () => undefined,
        dispatchEvent: () => false,
      }) as unknown as MediaQueryList,
  })
  return () => {
    if (original) {
      Object.defineProperty(window, 'matchMedia', original)
    }
  }
}

describe('WebglHero', () => {
  let restore: (() => void) | undefined

  afterEach(() => {
    restore?.()
    restore = undefined
  })

  it('falls back to the CSS showcase when the device has no WebGL context', () => {
    // jsdom returns null for canvas.getContext('webgl'), standing in for an
    // unsupported device.
    render(<WebglHero />)

    expect(screen.getByText(SHOWCASE_MARKER)).toBeInTheDocument()
    expect(document.querySelector('canvas')).toBeNull()
  })

  it('falls back to the CSS showcase under prefers-reduced-motion', () => {
    restore = forceReducedMotion()

    render(<WebglHero />)

    expect(screen.getByText(SHOWCASE_MARKER)).toBeInTheDocument()
    expect(document.querySelector('canvas')).toBeNull()
  })
})
