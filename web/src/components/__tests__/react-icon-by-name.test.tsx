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
import { render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { ReactIconByName } from '../react-icon-by-name'

/**
 * Guards the payment-method icon picker after the bundle-size rework: the icon
 * table is now a fixed, lazily loaded map (see `../payment-icons.ts`) instead of
 * resolving any `react-icons` name at runtime, so the names configured by
 * default and shipped in the UI must still resolve to a rendered icon.
 */
describe('ReactIconByName', () => {
  it('renders the matching Simple Icons icon for a known name', async () => {
    const { container } = render(
      <ReactIconByName name='SiAlipay' title='SiAlipay' />
    )

    await waitFor(() => {
      expect(container.querySelector('svg')).not.toBeNull()
    })
    expect(screen.getByTitle('SiAlipay')).toBeTruthy()
  })

  it('renders the matching Lucide icon for a known name', async () => {
    const { container } = render(
      <ReactIconByName name='LuCreditCard' title='LuCreditCard' />
    )

    await waitFor(() => {
      expect(container.querySelector('svg')).not.toBeNull()
    })
    expect(screen.getByTitle('LuCreditCard')).toBeTruthy()
  })

  it('applies the caller-provided className to the rendered icon', async () => {
    const { container } = render(
      <ReactIconByName name='SiWechat' className='size-5 shrink-0' />
    )

    await waitFor(() => {
      expect(container.querySelector('svg')).not.toBeNull()
    })
    expect(container.querySelector('svg')?.getAttribute('class')).toBe(
      'size-5 shrink-0'
    )
  })

  it('renders nothing for a name outside the icon table', async () => {
    const { container } = render(<ReactIconByName name='NotARealIcon' />)

    // 未命中固定表时应保持空白，与「Invalid names show no icon」的既有行为一致。
    await waitFor(() => {
      expect(container.querySelector('svg')).toBeNull()
    })
  })

  it('renders nothing when the name is empty or missing', () => {
    const { container: empty } = render(<ReactIconByName name='   ' />)
    const { container: missing } = render(<ReactIconByName />)

    expect(empty.querySelector('svg')).toBeNull()
    expect(missing.querySelector('svg')).toBeNull()
  })
})
