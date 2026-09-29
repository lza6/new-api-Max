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
import { Coins } from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { Card, CardContent } from '@/components/ui/card'
import { IconBadge } from '@/components/ui/icon-badge'
import { useStatus } from '@/hooks/use-status'

/**
 * 余额查询地址卡片：把站点对外暴露的 OpenAI 兼容计费端点明示给用户。
 *
 * 背景：用户此前反馈「不知道本站余额在哪里查」。本站在
 * `/v1/dashboard/billing/subscription` 与 `/v1/dashboard/billing/usage`
 * 暴露了 OpenAI 兼容的额度端点（Bearer 用 API Key 鉴权），CC Switch、Cherry
 * Studio 等第三方客户端会自动读取这两个地址展示余量。此前这些地址没有任何
 * 界面提示，用户只能靠猜。
 */
export function WalletUsageEndpointsCard() {
  const { t } = useTranslation()
  const { status } = useStatus()

  const baseUrl = useMemo(() => {
    const apiInfo = status?.api_info as Array<{ url?: string }> | undefined
    const candidate = status?.server_address || apiInfo?.[0]?.url
    if (typeof candidate === 'string' && candidate) {
      return candidate.replace(/\/+$/, '')
    }
    return typeof window !== 'undefined' ? window.location.origin : ''
  }, [status])

  const endpoints: { label: string; path: string; hint: string }[] = [
    {
      label: t('Total quota'),
      path: '/v1/dashboard/billing/subscription',
      hint: t('Returns remaining plus consumed quota as a USD amount'),
    },
    {
      label: t('Consumed quota'),
      path: '/v1/dashboard/billing/usage',
      hint: t('Returns the consumed amount for balance display'),
    },
  ]

  return (
    <Card data-card-hover='false' className='bg-muted/20 py-0'>
      <CardContent className='space-y-3 p-3 sm:p-4'>
        <div className='flex items-start gap-2.5'>
          <IconBadge tone='chart-2'>
            <Coins />
          </IconBadge>
          <div className='min-w-0 flex-1'>
            <h3 className='text-sm font-semibold'>
              {t('Balance query endpoints')}
            </h3>
            <p className='text-muted-foreground mt-0.5 text-xs'>
              {t(
                'CC Switch and other OpenAI-compatible clients read your remaining balance from these addresses automatically. Authenticate with your API key.'
              )}
            </p>
          </div>
        </div>

        <div className='grid gap-2'>
          {endpoints.map((item) => {
            const fullUrl = `${baseUrl}${item.path}`
            return (
              <div
                key={item.path}
                className='bg-background/70 rounded-md border p-2.5 sm:p-3'
              >
                <div className='flex items-center justify-between gap-2'>
                  <span className='text-muted-foreground text-xs font-medium'>
                    {item.label}
                  </span>
                  <CopyButton
                    value={fullUrl}
                    tooltip={t('Copy address')}
                    size='sm'
                  />
                </div>
                <div className='mt-1 font-mono text-xs break-all'>
                  {fullUrl}
                </div>
                <div className='text-muted-foreground/70 mt-1 text-xs'>
                  {item.hint}
                </div>
              </div>
            )
          })}
        </div>

        <p className='text-muted-foreground text-xs'>
          {t(
            'Use your API key as the Bearer token. The base URL used by every protocol is'
          )}{' '}
          <span className='text-foreground font-mono'>{baseUrl}/v1</span>
        </p>
      </CardContent>
    </Card>
  )
}
