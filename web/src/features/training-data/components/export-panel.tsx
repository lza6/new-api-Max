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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import dayjs from 'dayjs'
import { Download, History, Loader2, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { DatePicker } from '@/components/date-picker'
import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { handleServerError } from '@/lib/handle-server-error'

import {
  createTrainingDataExport,
  deleteTrainingDataExport,
  listTrainingDataExports,
} from '../api'
import type { ExportStatus, TrainingDataExport } from '../types'
import {
  ExportDownloadDialog,
  ExportHistoryDialog,
} from './export-download-dialog'

function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) {
    return '0 B'
  }
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let value = bytes
  let unit = 0
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024
    unit += 1
  }
  return `${value >= 100 ? Math.round(value) : Math.round(value * 100) / 100} ${units[unit]}`
}

/** 状态 → Badge variant 与 i18n 键。 */
function statusMeta(status: ExportStatus): {
  variant: 'default' | 'secondary' | 'destructive' | 'outline'
  key: string
} {
  if (status === 'ready') {
    return { variant: 'default', key: 'Ready' }
  }
  if (status === 'failed') {
    return { variant: 'destructive', key: 'Failed' }
  }
  if (status === 'building') {
    return { variant: 'secondary', key: 'Building' }
  }
  return { variant: 'outline', key: 'Pending' }
}

/**
 * 训练数据一键导出面板（增量需求）。
 *
 * 三段式：创建（选日期范围 + 可选用户/模型）→ 列表（状态/大小/下载次数）
 * → 下载方式弹窗（浏览器直连 / 复制直链，并告知是否支持断点续传）。
 *
 * 直链是**限时签名** URL（见服务端 export_sign.go）；这里不缓存它，
 * 每次打开弹窗重新签发，避免展示一个可能过期的链接。
 */
export function ExportPanel() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()

  const [from, setFrom] = useState<Date | undefined>(undefined)
  const [to, setTo] = useState<Date | undefined>(undefined)
  const [userId, setUserId] = useState('')
  const [model, setModel] = useState('')

  const [downloadTarget, setDownloadTarget] =
    useState<TrainingDataExport | null>(null)
  const [historyTarget, setHistoryTarget] = useState<TrainingDataExport | null>(
    null
  )
  const [pendingDelete, setPendingDelete] =
    useState<TrainingDataExport | null>(null)

  const listQuery = useQuery({
    queryKey: ['training-data', 'exports'],
    queryFn: async () => {
      const res = await listTrainingDataExports(0, 100)
      if (!res.success) {
        throw new Error(res.message || t('Failed to load'))
      }
      return res.data
    },
  })

  const createMutation = useMutation({
    mutationFn: async () => {
      if (!from || !to) {
        throw new Error(t('Export date range'))
      }
      const parsedUserId = Number.parseInt(userId, 10)
      const res = await createTrainingDataExport({
        date_from: dayjs(from).format('YYYY-MM-DD'),
        date_to: dayjs(to).format('YYYY-MM-DD'),
        user_id: Number.isNaN(parsedUserId) ? 0 : parsedUserId,
        model: model.trim(),
      })
      if (!res.success) {
        throw new Error(res.message || t('Failed to create export'))
      }
      return res.data
    },
    onSuccess: () => {
      toast.success(t('Export created'))
      void queryClient.invalidateQueries({ queryKey: ['training-data'] })
    },
    onError: (error) => handleServerError(error, t('Failed to create export')),
  })

  const deleteMutation = useMutation({
    mutationFn: async (id: number) => {
      const res = await deleteTrainingDataExport(id)
      if (!res.success) {
        throw new Error(res.message || t('Failed to delete'))
      }
      return res.data
    },
    onSuccess: () => {
      toast.success(t('Deleted'))
      setPendingDelete(null)
      void queryClient.invalidateQueries({ queryKey: ['training-data'] })
    },
    onError: (error) => handleServerError(error, t('Failed to delete')),
  })

  const exports = listQuery.data?.exports ?? []
  const canCreate = from !== undefined && to !== undefined

  return (
    <div className='space-y-4'>
      <Card>
        <CardHeader>
          <CardTitle className='text-sm'>{t('Create export')}</CardTitle>
        </CardHeader>
        <CardContent className='space-y-4'>
          <p className='text-muted-foreground text-xs'>
            {t(
              'Pack the selected range into a downloadable archive. Large ranges can take a while.'
            )}
          </p>
          <div className='grid gap-3 sm:grid-cols-2'>
            <div className='space-y-1.5'>
              <Label>{t('From date')}</Label>
              <DatePicker selected={from} onSelect={setFrom} />
            </div>
            <div className='space-y-1.5'>
              <Label>{t('To date')}</Label>
              <DatePicker selected={to} onSelect={setTo} />
            </div>
            <div className='space-y-1.5'>
              <Label>{t('User ID (optional)')}</Label>
              <Input
                inputMode='numeric'
                value={userId}
                onChange={(e) => setUserId(e.target.value.replaceAll(/\D/g, ''))}
                placeholder='0'
              />
            </div>
            <div className='space-y-1.5'>
              <Label>{t('Model (optional)')}</Label>
              <Input
                value={model}
                onChange={(e) => setModel(e.target.value)}
                placeholder='gpt-4o'
              />
            </div>
          </div>
          <Button
            type='button'
            disabled={!canCreate || createMutation.isPending}
            onClick={() => createMutation.mutate()}
          >
            {createMutation.isPending && (
              <Loader2 className='size-4 animate-spin' />
            )}
            {t('Start export')}
          </Button>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className='text-sm'>{t('Exports')}</CardTitle>
        </CardHeader>
        <CardContent>
          {renderExports(
            listQuery.isLoading,
            listQuery.isError,
            listQuery.error,
            exports,
            {
              onDownload: setDownloadTarget,
              onHistory: setHistoryTarget,
              onDelete: setPendingDelete,
            },
            t
          )}
        </CardContent>
      </Card>

      <ExportDownloadDialog
        exportRow={downloadTarget}
        onOpenChange={(open) => {
          if (!open) {
            setDownloadTarget(null)
          }
        }}
      />
      <ExportHistoryDialog
        exportRow={historyTarget}
        onOpenChange={(open) => {
          if (!open) {
            setHistoryTarget(null)
          }
        }}
      />

      <ConfirmDialog
        open={pendingDelete !== null}
        onOpenChange={(open) => {
          if (!open) {
            setPendingDelete(null)
          }
        }}
        title={t('Delete this export?')}
        desc={t('The archive and its download history will be removed.')}
        destructive
        isLoading={deleteMutation.isPending}
        confirmText={t('Delete')}
        handleConfirm={() => {
          if (pendingDelete) {
            deleteMutation.mutate(pendingDelete.id)
          }
        }}
      />
    </div>
  )
}

interface ExportActions {
  onDownload: (row: TrainingDataExport) => void
  onHistory: (row: TrainingDataExport) => void
  onDelete: (row: TrainingDataExport) => void
}

function renderExports(
  isLoading: boolean,
  isError: boolean,
  error: Error | null,
  exports: TrainingDataExport[],
  actions: ExportActions,
  t: (key: string) => string
): React.ReactNode {
  if (isLoading) {
    return <LoadingState />
  }
  if (isError) {
    return <ErrorState title={t('Failed to load')} description={error?.message} />
  }
  if (exports.length === 0) {
    return (
      <EmptyState
        title={t('No exports yet')}
        description={t('Create an export to package collected data for training.')}
      />
    )
  }
  return (
    <ul className='divide-border divide-y'>
      {exports.map((row) => {
        const meta = statusMeta(row.status)
        return (
          <li key={row.id} className='flex flex-wrap items-center gap-3 py-3 text-sm'>
            <div className='min-w-0 flex-1 space-y-1'>
              <div className='flex flex-wrap items-center gap-2'>
                <span className='font-mono'>#{row.id}</span>
                <Badge variant={meta.variant}>{t(meta.key)}</Badge>
                <span className='text-muted-foreground'>
                  {t('Date range')}: {row.date_from} ~ {row.date_to}
                </span>
                {row.model !== '' && (
                  <Badge variant='outline' className='font-normal'>
                    {row.model}
                  </Badge>
                )}
              </div>
              <div className='text-muted-foreground flex flex-wrap gap-3 text-xs'>
                <span>
                  {t('Size')}: {formatBytes(row.size_bytes)}
                </span>
                <span>
                  {t('Parts')}: {row.part_count}
                </span>
                <span>
                  {t('Downloads')}: {row.download_count}
                </span>
                {row.error_msg !== '' && (
                  <span className='text-destructive'>{row.error_msg}</span>
                )}
              </div>
            </div>
            <div className='flex shrink-0 items-center gap-1'>
              <Button
                variant='outline'
                size='sm'
                disabled={row.status !== 'ready'}
                onClick={() => actions.onDownload(row)}
              >
                <Download className='size-4' />
                {t('Download')}
              </Button>
              <Button
                variant='ghost'
                size='sm'
                onClick={() => actions.onHistory(row)}
              >
                <History className='size-4' />
                {t('Download history')}
              </Button>
              <Button
                variant='ghost'
                size='icon'
                aria-label={t('Delete')}
                onClick={() => actions.onDelete(row)}
              >
                <Trash2 className='text-destructive size-4' />
              </Button>
            </div>
          </li>
        )
      })}
    </ul>
  )
}
