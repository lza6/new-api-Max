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
import userEvent from '@testing-library/user-event'
import { afterAll, beforeAll, describe, expect, it } from 'vitest'

import { RequestFlow } from '../request-flow'

// jsdom has no IntersectionObserver; AnimateInView only needs the observer to
// exist and to fire the "in view" callback once so the section becomes visible.
const originalIntersectionObserver = globalThis.IntersectionObserver

beforeAll(() => {
  globalThis.IntersectionObserver = class {
    private readonly callback: IntersectionObserverCallback

    constructor(callback: IntersectionObserverCallback) {
      this.callback = callback
    }

    observe(target: Element) {
      this.callback(
        [{ isIntersecting: true, target } as IntersectionObserverEntry],
        this as unknown as IntersectionObserver
      )
    }

    unobserve() {}
    disconnect() {}
    takeRecords() {
      return []
    }
    readonly root = null
    readonly rootMargin = ''
    readonly thresholds = []
  } as unknown as typeof IntersectionObserver
})

afterAll(() => {
  globalThis.IntersectionObserver = originalIntersectionObserver
})

describe('RequestFlow', () => {
  it('renders the pipeline stages for the default endpoint', () => {
    render(<RequestFlow />)

    // Each pipeline stage exposes its stable label regardless of animation.
    expect(screen.getByText('Client')).toBeInTheDocument()
    expect(screen.getByText('Gateway')).toBeInTheDocument()
    expect(screen.getByText('Upstream')).toBeInTheDocument()
    expect(screen.getByText('Response')).toBeInTheDocument()
  })

  it('switches the upstream target when another endpoint tab is selected', async () => {
    const user = userEvent.setup()
    render(<RequestFlow />)

    // Chat is the default; Claude should not be the active panel yet.
    expect(screen.queryByText('Anthropic')).not.toBeInTheDocument()

    await user.click(screen.getByRole('tab', { name: 'Claude' }))

    expect(await screen.findByText('Anthropic')).toBeVisible()
  })

  it('breaks the total latency into named transport and compute segments', () => {
    render(<RequestFlow />)

    const list = screen.getByRole('list')
    expect(list).toHaveTextContent('Connect')
    expect(list).toHaveTextContent('Upload')
    expect(list).toHaveTextContent('Upstream compute')
  })

  it('reveals every pipeline stage immediately under reduced motion', () => {
    // AnimateInView and the pipeline read prefers-reduced-motion; force the
    // reduce branch so the assertion does not depend on animation timers.
    // test-setup.ts defines matchMedia as read-only, so redefine the property.
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

    try {
      render(<RequestFlow />)
      // The latency total only renders once the stage model settles; under
      // reduced motion it is present without waiting on the 4-step timers.
      expect(screen.getByText('6,572')).toBeInTheDocument()
    } finally {
      if (original) {
        Object.defineProperty(window, 'matchMedia', original)
      }
    }
  })
})
