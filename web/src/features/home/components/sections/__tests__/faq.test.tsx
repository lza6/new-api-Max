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
import { afterAll, beforeAll, describe, expect, it } from 'vitest'

import { FAQ } from '../faq'

// Reveal uses Motion's whileInView, which needs an IntersectionObserver to exist.
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

function readFaqJsonLd(): { mainEntity: { name: string; acceptedAnswer: { text: string } }[] } {
  const el = document.getElementById('landing-faq-jsonld')
  if (!el) {throw new Error('FAQ JSON-LD script not found')}
  return JSON.parse(el.textContent ?? '{}')
}

describe('FAQ', () => {
  it('renders every question as visible content', () => {
    render(<FAQ />)

    expect(screen.getByText('What is an AI API gateway?')).toBeInTheDocument()
    expect(
      screen.getByText('Which AI models and providers are supported?')
    ).toBeInTheDocument()
    expect(screen.getByText('How is usage billed?')).toBeInTheDocument()
  })

  it('emits FAQPage structured data matching the rendered questions', () => {
    render(<FAQ />)

    const data = readFaqJsonLd()
    expect(data.mainEntity).toHaveLength(6)
    // The structured data must echo the same question text the user sees, or
    // search engines flag a mismatch between markup and page content.
    expect(data.mainEntity[0].name).toBe('What is an AI API gateway?')
    expect(data.mainEntity[0].acceptedAnswer.text.length).toBeGreaterThan(40)
  })

  it('removes the injected structured data on unmount', () => {
    const { unmount } = render(<FAQ />)
    expect(document.getElementById('landing-faq-jsonld')).not.toBeNull()

    unmount()

    expect(document.getElementById('landing-faq-jsonld')).toBeNull()
  })
})
