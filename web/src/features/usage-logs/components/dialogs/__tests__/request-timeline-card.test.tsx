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
import { afterEach, beforeAll, describe, expect, test, vi } from 'vitest'

import type { UsageLog } from '../../../data/schema'
import type { LogOtherData } from '../../../types'
import { DetailsDialog } from '../details-dialog'

const i18nKeys = {
  'Log Details': 'Log Details',
  Consume: 'Consume',
  'Request timeline': 'Request timeline',
  Inbound: 'Inbound',
  Auth: 'Auth',
  'Channel selection': 'Channel selection',
  'Upstream call': 'Upstream call',
  'First token': 'First token',
  Complete: 'Complete',
  'Copy JSON': 'Copy JSON',
  'Copy timeline JSON': 'Copy timeline JSON',
}

function makeLog(other: LogOtherData): UsageLog {
  return {
    id: 1,
    user_id: 1,
    created_at: 1700000000,
    type: 2,
    content: '',
    username: 'user',
    token_name: 'token',
    model_name: 'gpt-4o',
    quota: 1000,
    prompt_tokens: 10,
    completion_tokens: 5,
    use_time: 3,
    is_stream: true,
    channel: 7,
    channel_name: '',
    token_id: 1,
    group: 'default',
    ip: '',
    other: JSON.stringify(other),
    request_id: 'req-1',
    upstream_request_id: '',
  }
}

const queryClients: QueryClient[] = []
function renderDetails(other: LogOtherData, isAdmin = false) {
  const client = new QueryClient()
  queryClients.push(client)
  return render(
    <QueryClientProvider client={client}>
      <DetailsDialog
        log={makeLog(other)}
        open
        onOpenChange={() => undefined}
        isAdmin={isAdmin}
        isRoot={false}
      />
    </QueryClientProvider>
  )
}

vi.mock('@/hooks/use-copy-to-clipboard', () => ({
  useCopyToClipboard: () => ({ copiedText: null, copyToClipboard: vi.fn() }),
}))

beforeAll(async () => {
  await i18next.init({
    lng: 'en',
    fallbackLng: 'en',
    resources: { en: { translation: i18nKeys } },
  })
})

afterEach(() => {
  queryClients.splice(0).forEach((client) => client.clear())
})

describe('Request timeline (B2-2)', () => {
  test('successful stream request renders all phases', () => {
    renderDetails({
      frt: 480,
      request_path: '/v1/chat/completions',
      admin_info: { use_channel: [7, 12] },
      stream_status: { status: 'ok', end_reason: 'done', end_error: '' },
    })
    expect(screen.getByText('Request timeline')).toBeInTheDocument()
    expect(screen.getByText('Inbound')).toBeInTheDocument()
    expect(screen.getByText('Auth')).toBeInTheDocument()
    expect(screen.getByText('Channel selection')).toBeInTheDocument()
    expect(screen.getByText('Upstream call')).toBeInTheDocument()
    expect(screen.getByText('First token')).toBeInTheDocument()
    expect(screen.getByText('Complete')).toBeInTheDocument()
    expect(screen.getByLabelText('Copy timeline JSON')).toBeInTheDocument()
  })

  test('legacy log without extra fields degrades to minimal timeline', () => {
    renderDetails({ group_ratio: 1 })
    // 仍有时间线（入站/鉴权/上游/完成），不报错
    expect(screen.getByText('Request timeline')).toBeInTheDocument()
    expect(screen.getByText('Inbound')).toBeInTheDocument()
    expect(screen.getByText('Complete')).toBeInTheDocument()
  })
})
