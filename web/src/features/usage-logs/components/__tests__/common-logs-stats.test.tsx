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
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { CommonLogsStats } from '../common-logs-stats'
import { UsageLogsProvider } from '../usage-logs-provider'

async function renderStats(role: number) {
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    const u = String(url)
    if (u.includes('/api/log/traffic')) {
      return {
        data: {
          success: true,
          data: {
            days: 30,
            total_requests: 0,
            total_bytes: 0,
            total_mb: 0,
            by_day: [],
          },
        },
      }
    }
    const isAdminStat = u.includes('/api/log/stat') && !u.includes('/self')
    return {
      data: {
        success: true,
        data: isAdminStat
          ? {
              quota: 1000,
              rpm: 12,
              tpm: 34,
              concurrent_requests: 5,
              completed_last_minute: 120,
            }
          : { quota: 1000, rpm: 12, tpm: 34 },
      },
    }
  })
  useAuthStore.getState().auth.setUser({
    id: 1,
    username: 'alice',
    role,
    permissions: {},
    sidebar_modules: '',
  })

  const root = createRootRoute()
  const auth = createRoute({ getParentRoute: () => root, id: '_authenticated' })
  const logs = createRoute({
    getParentRoute: () => auth,
    path: '/usage-logs/$section',
    component: () => (
      <UsageLogsProvider>
        <CommonLogsStats />
      </UsageLogsProvider>
    ),
    validateSearch: (search: Record<string, unknown>) => search,
  })
  const router = createRouter({
    routeTree: root.addChildren([auth.addChildren([logs])]),
    history: createMemoryHistory({ initialEntries: ['/usage-logs/common'] }),
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
}

afterEach(() => {
  vi.restoreAllMocks()
  useAuthStore.getState().auth.reset()
})

describe('CommonLogsStats live badges', () => {
  it('admin view renders current-concurrency and completed-last-minute badges', async () => {
    await renderStats(ROLE.SUPER_ADMIN)
    await waitFor(() => {
      expect(screen.getByText('Concurrent now')).toBeInTheDocument()
    })
    expect(screen.getByText('Completed last minute')).toBeInTheDocument()
    // 值来自 admin stat 响应
    expect(screen.getByText('5')).toBeInTheDocument()
    expect(screen.getByText('120')).toBeInTheDocument()
  })

  it('non-admin view does not render live badges', async () => {
    await renderStats(ROLE.USER)
    await waitFor(() => {
      expect(screen.getByText('RPM')).toBeInTheDocument()
    })
    expect(screen.queryByText('Concurrent now')).not.toBeInTheDocument()
    expect(screen.queryByText('Completed last minute')).not.toBeInTheDocument()
  })
})
