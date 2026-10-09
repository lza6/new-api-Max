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
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { afterEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { SavingsBaselineSummary, CompressionSection } from '../components/compression-section'

const i18n = createInstance()
await i18n.init({
  lng: 'en',
  resources: { en: { translation: {} } },
  initAsync: false,
})

function renderSummary() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        <SavingsBaselineSummary />
      </QueryClientProvider>
    </I18nextProvider>
  )
}

afterEach(() => vi.restoreAllMocks())

it('renders the counterfactual baseline in human-readable units', async () => {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: {
        total_count: 120,
        total_original_bytes: 2147483648,
        total_compressed_bytes: 536870912,
        total_saved_bytes: 1610612736,
        overall_saved_ratio: 0.75,
        counterfactual_bytes: 2147483648,
        saved_time_seconds: 42.5,
        bandwidth_bps: 100000000,
        saved_text: '1.5 GB',
        models: [],
      },
    },
  })
  renderSummary()

  // 反事实叙述必须出现（「如果不压缩会多传多少」），而不是只报节省绝对值。
  expect(
    await screen.findByText(/Without compression these 120 requests/)
  ).toBeInTheDocument()
  // 服务端已格式化的节省量与统一口径百分比。
  expect(screen.getByText('1.5 GB (75.0%)')).toBeInTheDocument()
  // 配置了观测带宽时才折算省时。
  expect(screen.getByText('42.5 s')).toBeInTheDocument()
})

it('hides the time-saved figure when no upload bandwidth is configured', async () => {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: {
        total_count: 3,
        total_original_bytes: 1024,
        total_compressed_bytes: 512,
        total_saved_bytes: 512,
        overall_saved_ratio: 0.5,
        counterfactual_bytes: 1024,
        saved_time_seconds: 0,
        bandwidth_bps: 0,
        saved_text: '512 B',
        models: [],
      },
    },
  })
  renderSummary()

  await screen.findByText(/Without compression these 3 requests/)
  // 未配置带宽时不给出无依据的「省时」数字。
  expect(screen.queryByText('Upload time saved:')).toBeNull()
})

it('renders nothing when there is no compression data', async () => {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: {
        total_count: 0,
        total_original_bytes: 0,
        total_compressed_bytes: 0,
        total_saved_bytes: 0,
        overall_saved_ratio: 0,
        counterfactual_bytes: 0,
        saved_time_seconds: 0,
        bandwidth_bps: 0,
        saved_text: '0 B',
        models: [],
      },
    },
  })
  const { container } = renderSummary()
  // 等待查询落定后仍应保持空渲染。
  await new Promise((resolve) => setTimeout(resolve, 0))
  expect(container).toBeEmptyDOMElement()
})

// 接线检查：反事实基准必须真的出现在**压缩榜这个生产组件**里。
// 只测 SavingsBaselineSummary 自身，证明不了 CompressionSection 挂载了它
// ——这正是「库全绿但生产零调用」能骗过单测的漏洞。
it('CompressionSection actually mounts the counterfactual baseline', async () => {
  vi.spyOn(api, 'get').mockImplementation(async (url: string) => {
    if (url === '/api/rankings/savings-baseline') {
      return {
        data: {
          success: true,
          data: {
            total_count: 7,
            total_original_bytes: 2048,
            total_compressed_bytes: 1024,
            total_saved_bytes: 1024,
            overall_saved_ratio: 0.5,
            counterfactual_bytes: 2048,
            saved_time_seconds: 0,
            bandwidth_bps: 0,
            saved_text: '1.0 KB',
            models: [],
          },
        },
      }
    }
    return {
      data: {
        success: true,
        data: [
          {
            model_name: 'deepseek-v4-flash',
            count: 7,
            original_bytes: 2048,
            compressed_bytes: 1024,
            saved_bytes: 1024,
            saved_gb: 0,
            saved_mb: 0,
            ratio: 0.5,
          },
        ],
      },
    }
  })

  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        <CompressionSection />
      </QueryClientProvider>
    </I18nextProvider>
  )

  expect(
    await screen.findByText(/Without compression these 7 requests/)
  ).toBeInTheDocument()
})
