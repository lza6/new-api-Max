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
*/
import { useQuery } from '@tanstack/react-query'
import { BarChart3 } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { formatTraffic } from '@/lib/format'
import { requireServerSuccess } from '@/lib/server-error-message'

import {
  getBandwidthLeaderboard,
  getModelBandwidthLeaderboard,
  type BandwidthLeaderboardRow,
} from '../api'

const DAYS = 30
const LIMIT = 10

function LeaderboardTable(props: {
  labelKey: string
  rows?: BandwidthLeaderboardRow[]
  isLoading: boolean
  isError: boolean
}) {
  const { t } = useTranslation()
  const rows = props.rows ?? []
  if (props.isLoading) {
    return (
      <p className='text-muted-foreground py-6 text-center text-sm'>
        {t('Loading')}…
      </p>
    )
  }
  if (props.isError) {
    return (
      <p className='text-destructive py-6 text-center text-sm'>
        {t('Failed to load')}
      </p>
    )
  }
  if (rows.length === 0) {
    return (
      <p className='text-muted-foreground py-6 text-center text-sm'>
        {t('No data')}
      </p>
    )
  }
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead className='w-12'>{t('Rank')}</TableHead>
          <TableHead>{t(props.labelKey)}</TableHead>
          <TableHead className='text-right'>{t('Requests')}</TableHead>
          <TableHead className='text-right'>{t('Bandwidth')}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {rows.map((row, index) => (
          <TableRow key={row.date || row.model || index}>
            <TableCell className='text-muted-foreground'>{index + 1}</TableCell>
            <TableCell className='font-mono'>
              {row.date || row.model || '—'}
            </TableCell>
            <TableCell className='text-right tabular-nums'>
              {row.requests ?? 0}
            </TableCell>
            <TableCell className='text-right tabular-nums'>
              {row.bytes_text || formatTraffic(row.bytes || 0)}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}

/**
 * 管理端「流量排行」弹窗：每日带宽排行 + 模型带宽排行（近 30 天 Top 10）。
 * 仅管理员视图展示；带宽自动按 B/KB/MB/GB/TB 可读单位（formatTraffic）显示。
 */
export function BandwidthLeaderboardDialog() {
  const { t } = useTranslation()
  const daily = useQuery({
    queryKey: ['bandwidth-leaderboard', DAYS, LIMIT],
    queryFn: async () =>
      requireServerSuccess(await getBandwidthLeaderboard(DAYS, LIMIT)).data,
    staleTime: 60_000,
  })
  const model = useQuery({
    queryKey: ['model-bandwidth-leaderboard', DAYS, LIMIT],
    queryFn: async () =>
      requireServerSuccess(await getModelBandwidthLeaderboard(DAYS, LIMIT)).data,
    staleTime: 60_000,
  })

  return (
    <Dialog
      title={t('Bandwidth leaderboard')}
      description={t('Daily and per-model bandwidth served over the last 30 days')}
      contentHeight='26rem'
      trigger={
        <Button
          variant='ghost'
          size='sm'
          className='text-muted-foreground hover:text-foreground h-7 gap-1.5 px-2'
          aria-label={t('Bandwidth leaderboard')}
        >
          <BarChart3 className='size-4' />
          <span className='hidden sm:inline'>{t('Traffic rank')}</span>
        </Button>
      }
    >
      <Tabs defaultValue='model'>
        <TabsList className='w-full'>
          <TabsTrigger value='model' className='flex-1'>
            {t('Model bandwidth')}
          </TabsTrigger>
          <TabsTrigger value='daily' className='flex-1'>
            {t('Daily bandwidth')}
          </TabsTrigger>
        </TabsList>
        <div className='mt-3'>
          <TabsContent value='model'>
            <LeaderboardTable
              labelKey='Model'
              rows={model.data?.leaderboard}
              isLoading={model.isLoading}
              isError={model.isError}
            />
          </TabsContent>
          <TabsContent value='daily'>
            <LeaderboardTable
              labelKey='Date'
              rows={daily.data?.leaderboard}
              isLoading={daily.isLoading}
              isError={daily.isError}
            />
          </TabsContent>
        </div>
      </Tabs>
    </Dialog>
  )
}
