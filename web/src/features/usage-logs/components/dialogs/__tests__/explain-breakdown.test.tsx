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

import type { UsageLog } from '../../../data/schema'
import type { LogExplain, LogOtherData } from '../../../types'
import { DetailsDialog } from '../details-dialog'

const i18nKeys = {
  'Log Details': 'Log Details',
  Consume: 'Consume',
  'Fee Explanation': 'Fee Explanation',
  'Observed facts': 'Observed facts',
  'System inference': 'System inference',
  'Total Cost': 'Total Cost',
  'Billing Details': 'Billing Details',
}

function makeLog(other: LogOtherData): UsageLog {
  return {
    id: 1,
    user_id: 1,
    created_at: 1,
    type: 2,
    content: '',
    username: 'user',
    token_name: 'token',
    model_name: 'gpt-4o',
    quota: 1000,
    prompt_tokens: 10,
    completion_tokens: 5,
    use_time: 1,
    is_stream: false,
    channel: 1,
    channel_name: '',
    token_id: 1,
    group: 'default',
    ip: '',
    other: JSON.stringify(other),
    request_id: 'req-1',
    upstream_request_id: '',
  }
}

function renderDetails(other: LogOtherData): QueryClient {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const freshAt = Date.now() + 60_000
  queryClient.setQueryData(['status'], {}, { updatedAt: freshAt })
  queryClient.setQueryData(
    ['pricing'],
    { data: [], vendors: [] },
    { updatedAt: freshAt }
  )

  render(
    <QueryClientProvider client={queryClient}>
      <DetailsDialog
        log={makeLog(other)}
        isAdmin={false}
        isRoot={false}
        open
        onOpenChange={() => undefined}
      />
    </QueryClientProvider>
  )
  return queryClient
}

function explain(other: LogOtherData, explain?: LogExplain): LogOtherData {
  return explain ? { ...other, explain } : other
}

describe('details dialog fee explanation (B5-1)', () => {
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

  test('renders facts table and inferences list when explain present', () => {
    queryClients.push(
      renderDetails(
        explain(
          { group_ratio: 1 },
          {
            facts: [
              { label: 'prompt_tokens', value: 10 },
              { label: 'cached_tokens', value: 5 },
              { label: 'group_ratio', value: 1 },
              { label: 'channels_considered', value: 2 },
            ],
            inferences: [
              { text: '命中阶梯价 0-4k 档', kind: 'pricing' },
            ],
          }
        )
      )
    )

    expect(screen.getByText('Fee Explanation')).toBeInTheDocument()
    expect(screen.getByText('Observed facts')).toBeInTheDocument()

    // facts 均为 label + value 成对渲染（prompt_tokens 值 10 在 Token Breakdown
    // 也出现，用 getAllByText 验证）
    expect(screen.getByText('prompt_tokens')).toBeInTheDocument()
    expect(screen.getAllByText('10').length).toBeGreaterThan(0)
    expect(screen.getByText('cached_tokens')).toBeInTheDocument()
    expect(screen.getAllByText('5').length).toBeGreaterThan(0)
    expect(screen.getByText('channels_considered')).toBeInTheDocument()
    expect(screen.getByText('2')).toBeInTheDocument()

    // inferences 渲染文本与 kind 徽标
    expect(screen.getByText('System inference')).toBeInTheDocument()
    expect(screen.getByText('命中阶梯价 0-4k 档')).toBeInTheDocument()
    expect(screen.getByText('(pricing)')).toBeInTheDocument()
  })

  test('renders boolean fact values as text', () => {
    queryClients.push(
      renderDetails(
        explain(
          { group_ratio: 1 },
          {
            facts: [{ label: 'is_model_mapped', value: true }],
            inferences: [],
          }
        )
      )
    )
    expect(screen.getByText('Fee Explanation')).toBeInTheDocument()
    expect(screen.getByText('is_model_mapped')).toBeInTheDocument()
    expect(screen.getByText('true')).toBeInTheDocument()
  })

  test('hides the whole section for legacy logs without explain', () => {
    queryClients.push(renderDetails({ group_ratio: 1 }))
    expect(screen.queryByText('Fee Explanation')).toBeNull()
    expect(screen.queryByText('Observed facts')).toBeNull()
    expect(screen.queryByText('System inference')).toBeNull()
  })

  test('renders nothing when explain has empty facts and empty inferences', () => {
    queryClients.push(
      renderDetails(explain({ group_ratio: 1 }, { facts: [], inferences: [] }))
    )
    expect(screen.queryByText('Fee Explanation')).toBeNull()
  })

  test('drops malformed facts and inferences gracefully', () => {
    queryClients.push(
      renderDetails(
        explain(
          { group_ratio: 1 },
          {
            facts: [
              { label: '', value: 1 },
              { label: 'prompt_tokens', value: 10 },
            ],
            inferences: [
              { text: '', kind: 'pricing' },
              { text: '命中阶梯价 0-4k 档', kind: 'pricing' },
            ],
          }
        )
      )
    )
    expect(screen.getByText('Fee Explanation')).toBeInTheDocument()
    // 空 label 的 fact 被过滤，只有有效 label 的 fact 渲染
    expect(screen.getByText('prompt_tokens')).toBeInTheDocument()
    // 空 text 的 inference 被过滤
    expect(screen.getByText('命中阶梯价 0-4k 档')).toBeInTheDocument()
  })
})