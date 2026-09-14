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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import i18next from 'i18next'
import { afterEach, beforeAll, describe, expect, test } from 'vitest'

import type { TaskLog } from '../../../types'
import { TaskDetailsCell } from '../task-logs-columns'

const i18nKeys = {
  'View details': 'View details',
  'Insufficient quota. Please top up or redeem a quota card.':
    'Insufficient quota. Please top up or redeem a quota card.',
}

function makeTask(overrides: Partial<TaskLog> = {}): TaskLog {
  return {
    id: 1,
    user_id: 1,
    platform: 'sora',
    task_id: 'task-1',
    action: 'GENERATE',
    channel_id: 1,
    group: 'default',
    quota: 100,
    submit_time: 1,
    status: 'FAILURE',
    fail_reason: 'Insufficient quota. Please top up or redeem a quota card.',
    ...overrides,
  }
}

function renderCell(log: TaskLog): QueryClient {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <TaskDetailsCell log={log} isAdmin={false} isRoot={false} />
    </QueryClientProvider>
  )
  return queryClient
}

describe('task fail reason B5-2', () => {
  const queryClients: QueryClient[] = []

  beforeAll(() => {
    i18next.addResourceBundle('en', 'translation', i18nKeys)
  })

  afterEach(() => {
    for (const queryClient of queryClients) {
      queryClient.clear()
    }
    queryClients.length = 0
  })

  test('shows friendly message as the visible fail reason text', () => {
    queryClients.push(renderCell(makeTask()))
    // 人话映射命中 → 单元格直接展示人话文案。
    expect(
      screen.getByText(
        'Insufficient quota. Please top up or redeem a quota card.'
      )
    ).toBeInTheDocument()
    expect(screen.getByText('View details')).toBeInTheDocument()
  })

  test('renders raw reason as visible text when no friendly mapping matches', () => {
    queryClients.push(
      renderCell(makeTask({ fail_reason: 'upstream rejected: model_x unsupported' }))
    )
    expect(
      screen.getByText('upstream rejected: model_x unsupported')
    ).toBeInTheDocument()
  })
})
