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
import { ChevronRight, FileText, Folder, Loader2, Trash2 } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { handleServerError } from '@/lib/handle-server-error'

import {
  deleteTrainingDataFile,
  listTrainingDataFiles,
  previewTrainingDataFile,
} from '../api'
import { formatBytes } from '../lib/format'
import type {
  FilePreviewResponse,
  TrainingDataFileEntry,
} from '../types'

/** 路径 → 面包屑片段（供下钻与返回上级）。 */
function pathSegments(path: string): string[] {
  return path.split('/').filter(Boolean)
}

/** 面包屑项：每级带上「到这里为止的完整路径」与显示名。 */
function buildCrumbs(path: string): { label: string; path: string }[] {
  return pathSegments(path).map((label, idx, all) => ({
    label,
    path: all.slice(0, idx + 1).join('/'),
  }))
}

/**
 * 云盘文件浏览器（B4/B7）。
 *
 * 目录树是**一级一级下钻**的（不是一次拉全树）：云盘可能有几十万文件，
 * 一次拉全既慢又会打爆 Graph API 配额。用 `path` 参数逐层查看，
 * 这也与后端 `ListTree` 的 maxEntries 硬顶相配。
 */
export function FileBrowserPanel(props: { onAnalyze?: (path: string) => void }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [cwd, setCwd] = useState('')
  const [previewPath, setPreviewPath] = useState<string | null>(null)
  const [pendingDelete, setPendingDelete] =
    useState<TrainingDataFileEntry | null>(null)

  const crumbs = useMemo(() => buildCrumbs(cwd), [cwd])

  const listQuery = useQuery({
    queryKey: ['training-data', 'files', cwd],
    queryFn: async () => {
      const res = await listTrainingDataFiles(cwd, 500)
      if (!res.success) {
        throw new Error(res.message || t('Failed to load'))
      }
      return res.data
    },
  })

  const previewQuery = useQuery({
    queryKey: ['training-data', 'preview', previewPath],
    enabled: previewPath !== null,
    queryFn: async () => {
      const res = await previewTrainingDataFile(previewPath ?? '')
      if (!res.success) {
        throw new Error(res.message || t('Failed to load'))
      }
      return res.data
    },
  })

  const deleteMutation = useMutation({
    mutationFn: async (path: string) => {
      const res = await deleteTrainingDataFile(path)
      if (!res.success) {
        throw new Error(res.message || t('Failed to delete'))
      }
      return res.data
    },
    onSuccess: () => {
      toast.success(t('Deleted'))
      setPendingDelete(null)
      if (previewPath === pendingDelete?.path) {
        setPreviewPath(null)
      }
      void queryClient.invalidateQueries({ queryKey: ['training-data'] })
    },
    onError: (error) => handleServerError(error, t('Failed to delete')),
  })

  // 当前目录的**直接子项**：后端给的是相对 path 的完整列表，
  // 这里按「再往下一层」过滤，避免把整棵子树平铺出来。
  const directChildren = useMemo(() => {
    const entries = listQuery.data?.entries ?? []
    const prefix = cwd ? `${cwd}/` : ''
    const seen = new Map<string, TrainingDataFileEntry>()

    for (const entry of entries) {
      const rel = entry.path.startsWith(prefix)
        ? entry.path.slice(prefix.length)
        : entry.path
      if (!rel) {
        continue
      }
      const slash = rel.indexOf('/')
      if (slash === -1) {
        // 直接子项：原样展示（以服务端条目为准，保留真实大小/时间）。
        seen.set(rel, { ...entry, path: entry.path })
        continue
      }
      // 更深层的项：只暴露它的「下一级目录」，不展开内容。
      const childDir = rel.slice(0, slash)
      const childPath = prefix + childDir
      if (!seen.has(childDir)) {
        seen.set(childDir, {
          path: childPath,
          is_dir: true,
          size_bytes: 0,
          modified_at: 0,
        })
      }
    }
    // 目录在前，名称排序。
    return [...seen.values()].sort((a, b) => {
      if (a.is_dir !== b.is_dir) {
        return a.is_dir ? -1 : 1
      }
      return a.path.localeCompare(b.path)
    })
  }, [listQuery.data, cwd])

  if (listQuery.isLoading) {
    return <LoadingState />
  }
  if (listQuery.isError) {
    return (
      <ErrorState
        title={t('Failed to load')}
        description={listQuery.error?.message}
        onRetry={() => void listQuery.refetch()}
      />
    )
  }

  return (
    <div className='grid gap-4 xl:grid-cols-[minmax(0,1fr)_minmax(420px,1fr)]'>
      <Card>
        <CardHeader className='flex-row items-center justify-between gap-2'>
          <CardTitle className='text-sm'>{t('Cloud storage files')}</CardTitle>
          {cwd && (
            <Button
              variant='ghost'
              size='sm'
              onClick={() => {
                const segs = pathSegments(cwd)
                setCwd(segs.slice(0, -1).join('/'))
              }}
            >
              {t('Up one level')}
            </Button>
          )}
        </CardHeader>
        <CardContent className='space-y-2'>
          {/* 面包屑：让管理员随时知道自己在哪一层。 */}
          <div className='text-muted-foreground flex flex-wrap items-center gap-1 text-xs'>
            <button
              type='button'
              className='hover:text-foreground underline-offset-2 hover:underline'
              onClick={() => setCwd('')}
            >
              {t('Root')}
            </button>
            {crumbs.map((crumb) => (
              <span key={crumb.path} className='flex items-center gap-1'>
                <ChevronRight className='size-3' />
                <button
                  type='button'
                  className='hover:text-foreground underline-offset-2 hover:underline'
                  onClick={() => setCwd(crumb.path)}
                >
                  {crumb.label}
                </button>
              </span>
            ))}
          </div>

          {directChildren.length === 0 ? (
            <EmptyState
              title={t('No files yet')}
              description={t('Nothing has been collected here yet.')}
            />
          ) : (
            <ul className='divide-border divide-y'>
              {directChildren.map((entry) => (
                <li
                  key={entry.path}
                  className='flex items-center gap-2 py-2 text-sm'
                >
                  {entry.is_dir ? (
                    <Folder className='text-muted-foreground size-4 shrink-0' />
                  ) : (
                    <FileText className='text-muted-foreground size-4 shrink-0' />
                  )}
                  <button
                    type='button'
                    className='min-w-0 flex-1 truncate text-left hover:underline'
                    onClick={() => {
                      if (entry.is_dir) {
                        setCwd(entry.path)
                        return
                      }
                      setPreviewPath(entry.path)
                    }}
                  >
                    {entry.path.split('/').pop()}
                  </button>
                  {!entry.is_dir && (
                    <>
                      <span className='text-muted-foreground shrink-0 font-mono text-xs tabular-nums'>
                        {formatBytes(entry.size_bytes)}
                      </span>
                      {props.onAnalyze && (
                        <Button
                          variant='ghost'
                          size='sm'
                          onClick={() => props.onAnalyze?.(entry.path)}
                        >
                          {t('Analyze')}
                        </Button>
                      )}
                      <Button
                        variant='ghost'
                        size='icon'
                        aria-label={t('Delete')}
                        onClick={() => setPendingDelete(entry)}
                      >
                        <Trash2 className='text-destructive size-4' />
                      </Button>
                    </>
                  )}
                </li>
              ))}
            </ul>
          )}

          {listQuery.data?.truncated && (
            <p className='text-muted-foreground text-xs'>
              {t('List was truncated at the server limit; not all files shown.')}
            </p>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className='text-sm'>{t('Preview')}</CardTitle>
        </CardHeader>
        <CardContent>
          {renderPreview(
            previewPath,
            previewQuery.isLoading,
            previewQuery.isError,
            previewQuery.error,
            previewQuery.data,
            t
          )}
        </CardContent>
      </Card>

      <ConfirmDialog
        open={pendingDelete !== null}
        onOpenChange={(open) => {
          if (!open) {
            setPendingDelete(null)
          }
        }}
        title={t('Delete this file?')}
        desc={t(
          'The part will be removed from cloud storage. This cannot be undone.'
        )}
        destructive
        isLoading={deleteMutation.isPending}
        confirmText={t('Delete')}
        handleConfirm={() => {
          if (pendingDelete) {
            deleteMutation.mutate(pendingDelete.path)
          }
        }}
      />
    </div>
  )
}

/**
 * 渲染预览区。
 *
 * 抽成函数而不是在 JSX 里叠三元：预览有四种状态
 * （未选文件 / 加载中 / 出错 / 有内容），嵌套三元可读性差且
 * 容易被 oxlint 的 no-nested-ternary 拦下。state 的**判断顺序**
 * 也有语义：未选文件优先于加载中（否则会显示一个假的 loading）。
 */
function renderPreview(
  previewPath: string | null,
  isLoading: boolean,
  isError: boolean,
  error: Error | null,
  data: FilePreviewResponse | undefined,
  t: (key: string) => string
): React.ReactNode {
  if (previewPath === null) {
    return (
      <p className='text-muted-foreground text-sm'>
        {t('Select a file to preview its contents.')}
      </p>
    )
  }
  if (isLoading) {
    return (
      <div className='text-muted-foreground flex items-center gap-2 text-sm'>
        <Loader2 className='size-4 animate-spin' />
        {t('Loading…')}
      </div>
    )
  }
  if (isError) {
    return (
      <ErrorState title={t('Failed to load')} description={error?.message} />
    )
  }
  return (
    <div className='space-y-2'>
      <div className='text-muted-foreground flex flex-wrap items-center gap-2 text-xs'>
        <Badge variant='secondary' className='font-normal'>
          {data?.line_count ?? 0} {t('records')}
        </Badge>
        {data?.compressed && (
          <Badge variant='outline' className='font-normal'>
            gzip · {formatBytes(data.compressed_bytes)} →{' '}
            {formatBytes(data.plain_bytes)}
          </Badge>
        )}
      </div>
      {/*
        每行是一条记录。**不做「漂亮格式化」**：管理员要看到原始数据
        长什么样 —— 这正是他判断采集质量的方式。
      */}
      <pre className='bg-muted/40 max-h-[420px] overflow-auto rounded-lg p-3 text-xs leading-relaxed break-all whitespace-pre-wrap'>
        {(data?.lines ?? []).join('\n')}
      </pre>
    </div>
  )
}
