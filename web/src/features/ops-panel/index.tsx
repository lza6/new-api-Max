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
import { useQueryClient } from '@tanstack/react-query'
import { RefreshCw } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'

import { ConsumptionRanking } from './components/consumption-ranking'
import { OpsOverviewCards } from './components/ops-overview-cards'
import { OPS_QUERY_KEY } from './constants'

/**
 * 运营面板（管理员专属）：当日/累计消耗、用户剩余额度合计、日均消耗与可运营天数估算，
 * 以及消耗排行榜（含一键加额度）。数据口径见 controller/ops_panel.go 文件头注释。
 */
export function OpsPanel() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Ops Panel')}</SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <Button
          variant='outline'
          size='sm'
          onClick={() =>
            void queryClient.invalidateQueries({ queryKey: [OPS_QUERY_KEY] })
          }
        >
          <RefreshCw className='size-4' />
          {t('Refresh')}
        </Button>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <div className='space-y-3 sm:space-y-4'>
          <OpsOverviewCards />
          <ConsumptionRanking />
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
