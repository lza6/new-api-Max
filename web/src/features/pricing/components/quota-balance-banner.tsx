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
import { Wallet } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Link } from '@tanstack/react-router'

import { Button } from '@/components/ui/button'
import { formatQuota } from '@/lib/format'
import { useAuthStore } from '@/stores/auth-store'

/**
 * §4.3.2: Key-page balance transparency on the public pricing page.
 * Shows the signed-in user's remaining quota; renders nothing for visitors.
 */
export function QuotaBalanceBanner() {
  const { t } = useTranslation()
  const user = useAuthStore((s) => s.auth.user)

  if (!user) {
    return null
  }

  const quota = user.quota ?? 0

  return (
    <div className='mx-auto mb-6 w-full max-w-2xl'>
      <div className='bg-background/60 flex flex-wrap items-center justify-between gap-3 rounded-2xl border px-4 py-3 shadow-card'>
        <div className='flex items-center gap-2.5'>
          <Wallet className='text-muted-foreground h-4 w-4 shrink-0' aria-hidden />
          <span className='text-muted-foreground text-sm'>
            {t('Remaining Balance')}
          </span>
        </div>
        <div className='flex items-center gap-3'>
          <span className='tabular-nums text-sm font-semibold sm:text-base'>
            {formatQuota(quota)}
          </span>
          <Button variant='outline' size='sm' className='h-8' render={<Link to='/wallet' />}>
            {t('Top-up')}
          </Button>
        </div>
      </div>
    </div>
  )
}