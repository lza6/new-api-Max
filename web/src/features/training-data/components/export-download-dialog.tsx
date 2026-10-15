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
import { useQuery } from '@tanstack/react-query'
import dayjs from 'dayjs'
import { CheckCircle2, Download, Loader2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { Dialog } from '@/components/dialog'
import { ErrorState } from '@/components/error-state'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'

import { getTrainingDataExportDetail, issueTrainingDataExportLink } from '../api'
import type {
  TrainingDataExport,
  TrainingDataExportDownload,
} from '../types'

/**
 * 下载方式选择弹窗（用户明确要求「搞个窗口来显示用什么下载」）。
 *
 * 两种方式：
 *  - **浏览器直连**：把浏览器地址导航到限时签名直链，浏览器原生下载
 *    （支持断点续传 —— 浏览器自己发 Range）。
 *  - **复制直链**：给出可喂给 curl / aria2 / wget 的 URL（含 `-C -` 续传示例）。
 *
 * 弹窗打开时**重新签发**直链（不缓存），保证展示的链接一定是新鲜的。
 */
export function ExportDownloadDialog(props: {
  exportRow: TrainingDataExport | null
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()
  const open = props.exportRow !== null

  const linkQuery = useQuery({
    queryKey: ['training-data', 'export-link', props.exportRow?.id],
    enabled: open,
    queryFn: async () => {
      const id = props.exportRow?.id
      if (id === undefined) {
        return undefined
      }
      const res = await issueTrainingDataExportLink(id)
      if (!res.success) {
        throw new Error(res.message || t('Failed to create the download link'))
      }
      return res.data
    },
  })

  const link = linkQuery.data
  const curlExample = link
    ? `curl -C - -L -o "${link.filename}" "${link.url}"`
    : ''

  return (
    <Dialog
      open={open}
      onOpenChange={props.onOpenChange}
      title={t('Download method')}
      description={t(
        'Choose how to download this export. Both methods support resume.'
      )}
      footer={
        <Button variant='outline' onClick={() => props.onOpenChange(false)}>
          {t('Close')}
        </Button>
      }
    >
      {renderBody(linkQuery.isLoading, linkQuery.isError, linkQuery.error, t)}

      {link && (
        <div className='space-y-4'>
          <div className='flex flex-wrap items-center gap-2 text-xs'>
            <Badge variant='secondary' className='font-normal'>
              <CheckCircle2 className='size-3' />
              {t('This export supports HTTP range requests (resumable).')}
            </Badge>
            <span className='text-muted-foreground'>
              {t('Link expires')}:{' '}
              {dayjs.unix(link.expires_at).format('YYYY-MM-DD HH:mm:ss')}
            </span>
          </div>

          <div className='space-y-2'>
            <p className='text-sm font-medium'>{t('Download in this browser')}</p>
            <Button
              type='button'
              onClick={() => {
                // 直链路由不在 AdminAuth 下（curl/aria2 无会话也能用），
                // 因此这里直接导航即可触发浏览器原生下载（含续传）。
                window.location.assign(link.url)
              }}
            >
              <Download className='size-4' />
              {t('Download in this browser')}
            </Button>
          </div>

          <div className='space-y-2'>
            <p className='text-sm font-medium'>
              {t('Direct link for curl / aria2 / wget')}
            </p>
            <div className='flex items-center gap-2'>
              <code className='bg-muted/50 min-w-0 flex-1 truncate rounded-md px-2 py-1.5 text-xs'>
                {link.url}
              </code>
              <CopyButton value={link.url} variant='outline' size='sm' tooltip={t('Copy to clipboard')} />
            </div>
            <div className='flex items-center gap-2'>
              <code className='bg-muted/50 min-w-0 flex-1 truncate rounded-md px-2 py-1.5 text-xs'>
                {curlExample}
              </code>
              <CopyButton
                value={curlExample}
                variant='outline'
                size='sm'
                tooltip={t('Copy to clipboard')}
              />
            </div>
          </div>
        </div>
      )}
    </Dialog>
  )
}

function renderBody(
  isLoading: boolean,
  isError: boolean,
  error: Error | null,
  t: (key: string) => string
): React.ReactNode {
  if (isLoading) {
    return (
      <div className='text-muted-foreground flex items-center gap-2 text-sm'>
        <Loader2 className='size-4 animate-spin' />
        {t('Preparing…')}
      </div>
    )
  }
  if (isError) {
    return (
      <ErrorState
        title={t('Failed to create the download link')}
        description={error?.message}
      />
    )
  }
  return null
}

/**
 * 下载记录弹窗：某个导出被谁下载过、何时、是否完成。
 *
 * 数据来自 `/api/admin/training-data/export/detail`（含下载人、来源、
 * 是否完整）。用户明确要求「记录哪个已经有下载过」——这里如实展示，
 * 中断的下载显示为「Incomplete」而不是伪装成功。
 */
export function ExportHistoryDialog(props: {
  exportRow: TrainingDataExport | null
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()
  const open = props.exportRow !== null

  const detailQuery = useQuery({
    queryKey: ['training-data', 'export-detail', props.exportRow?.id],
    enabled: open,
    queryFn: async () => {
      const id = props.exportRow?.id
      if (id === undefined) {
        return undefined
      }
      const res = await getTrainingDataExportDetail(id)
      if (!res.success) {
        throw new Error(res.message || t('Failed to load'))
      }
      return res.data
    },
  })

  const downloads = detailQuery.data?.downloads ?? []

  return (
    <Dialog
      open={open}
      onOpenChange={props.onOpenChange}
      title={t('Download history')}
      description={t('Who downloaded this export, and when.')}
      footer={
        <Button variant='outline' onClick={() => props.onOpenChange(false)}>
          {t('Close')}
        </Button>
      }
    >
      {renderHistory(
        detailQuery.isLoading,
        detailQuery.isError,
        detailQuery.error,
        downloads,
        t
      )}
    </Dialog>
  )
}

function renderHistory(
  isLoading: boolean,
  isError: boolean,
  error: Error | null,
  downloads: TrainingDataExportDownload[],
  t: (key: string) => string
): React.ReactNode {
  if (isLoading) {
    return <Loader2 className='text-muted-foreground size-4 animate-spin' />
  }
  if (isError) {
    return <ErrorState title={t('Failed to load')} description={error?.message} />
  }
  if (downloads.length === 0) {
    return (
      <p className='text-muted-foreground text-sm'>{t('Never downloaded')}</p>
    )
  }
  return (
    <ul className='divide-border divide-y text-sm'>
      {downloads.map((dl) => (
        <li key={dl.id} className='flex flex-wrap items-center gap-3 py-2'>
          <span className='font-mono'>
            #{dl.downloader_id > 0 ? dl.downloader_id : t('Unknown')}
          </span>
          <Badge variant={dl.completed ? 'secondary' : 'outline'}>
            {dl.completed ? t('Completed') : t('Incomplete')}
          </Badge>
          <span className='text-muted-foreground text-xs'>
            {t('Started')}:{' '}
            {dayjs.unix(dl.started_at).format('YYYY-MM-DD HH:mm:ss')}
          </span>
          <span className='text-muted-foreground ml-auto text-xs tabular-nums'>
            {dl.bytes_sent} B
          </span>
        </li>
      ))}
    </ul>
  )
}
