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
import { describe, expect, it, vi } from 'vitest'

import { EmptyState } from '../empty-state'

/**
 * 空态契约（尼尔森四态矩阵 + 视觉体系 §4.7）：
 * 空态必须给出**下一步动作**，网格底纹只是装饰且可关闭。
 */
describe('EmptyState', () => {
  it('renders the next action as a real, clickable control', async () => {
    const handleClick = vi.fn()
    const user = userEvent.setup()

    render(
      <EmptyState
        title='No API keys yet'
        description='Create your first key to start.'
        action={<button type='button' onClick={handleClick}>Create API key</button>}
      />
    )

    expect(screen.getByText('No API keys yet')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Create API key' }))
    expect(handleClick).toHaveBeenCalledTimes(1)
  })

  it('draws the engineering-grid atmosphere by default without swallowing the text', () => {
    const { container } = render(<EmptyState title='Nothing here' />)

    const empty = container.querySelector('[data-slot="empty"]')
    expect(empty).toHaveClass('surface-grid')
    expect(screen.getByText('Nothing here')).toBeInTheDocument()
  })

  it('lets a dense table opt out of the atmosphere', () => {
    const { container } = render(<EmptyState title='Nothing here' atmosphere={false} />)

    expect(container.querySelector('[data-slot="empty"]')).not.toHaveClass(
      'surface-grid'
    )
  })

  it('falls back to a translated default title when none is provided', () => {
    render(<EmptyState />)

    expect(screen.getByText('No Data')).toBeInTheDocument()
  })

  it('marks the decorative icon as non-content', () => {
    const { container } = render(<EmptyState title='Nothing here' />)

    expect(container.querySelector('[data-slot="empty-icon"]')).toHaveAttribute(
      'aria-hidden',
      'true'
    )
  })

  it('does not render a stray action container when no action is supplied', () => {
    const { container } = render(<EmptyState title='Nothing here' />)

    expect(container.querySelector('[data-slot="empty-content"]')).toBeNull()
  })
})
