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
import { describe, expect, it } from 'vitest'

import { Metric, MetricLabel, MetricValue } from '../metric'

/**
 * 读数排版契约（视觉体系 §4.2）：
 * 1. 空数据必须渲染占位符，不能把 `--`、`N/A` 或 0 当"没有数据"；
 * 2. 单位是与数字分离的次要元素，不能拼进数字串（否则列无法右对齐）；
 * 3. 变化方向必须同时有图标与读屏文本，不得只靠颜色表达。
 */
describe('MetricValue', () => {
  it('renders a placeholder when the value is empty instead of showing an empty box', () => {
    render(<MetricValue emptyValue='No data' />)

    expect(screen.getByText('No data')).toBeInTheDocument()
  })

  it('treats the legacy "--" error marker as empty so error states do not read as a value', () => {
    render(<MetricValue>--</MetricValue>)

    const valueSlot = document.querySelector('[data-slot="metric-value"]')
    expect(valueSlot).toHaveAttribute('data-empty', 'true')
    expect(screen.getByText('—')).toBeInTheDocument()
  })

  it('keeps the unit out of the numeric string so columns stay aligned', () => {
    const { container } = render(<MetricValue unit='M'>128.4</MetricValue>)

    const valueSpan = container.querySelector('.font-numeric')
    expect(valueSpan).toHaveTextContent(/^128\.4$/)
    expect(valueSpan).not.toHaveTextContent('M')
    expect(screen.getByText('M')).toBeInTheDocument()
  })

  it('exposes the untruncated value through title when a compact value is shown', () => {
    render(<MetricValue title='128,412,900'>128.4M</MetricValue>)

    expect(screen.getByText('128.4M')).toHaveAttribute('title', '128,412,900')
  })
})

describe('Metric composition', () => {
  it('lays out label above value as one readable block', () => {
    const { container } = render(
      <Metric>
        <MetricLabel>Total tokens</MetricLabel>
        <MetricValue unit='M'>512.9</MetricValue>
      </Metric>
    )

    expect(container.querySelector('[data-slot="metric"]')).toContainElement(
      screen.getByText('Total tokens')
    )
    expect(container.querySelector('[data-slot="metric"]')).toContainElement(
      screen.getByText('512.9')
    )
  })
})
