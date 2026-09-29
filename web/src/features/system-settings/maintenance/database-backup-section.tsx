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
import { AlertTriangle, Database, Download, Loader2, Upload } from 'lucide-react'
import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Label } from '@/components/ui/label'
import { handleServerError } from '@/lib/handle-server-error'

import {
  downloadDatabaseBackup,
  getDatabaseBackupInfo,
  importDatabaseBackup,
} from '../api'
import { SettingsSection } from '../components/settings-section'

/**
 * 数据库导出/导入（灾备）。
 *
 * 设计遵守尼尔森可用性原则：
 * - #1 状态可见：导出行数、导入进度、成功/失败结果实时可见
 * - #5 防错：导入前必须通过确认对话框（明确说明"只新增、不删除"）
 * - #3 用户控制：确认弹窗可取消
 * - #9 错误可恢复：失败给出可行动的提示，而非错误码
 */
export function DatabaseBackupSection() {
  const { t } = useTranslation()
  const fileInputRef = useRef<HTMLInputElement>(null)
  const [includeLogs, setIncludeLogs] = useState(false)
  const [exporting, setExporting] = useState(false)
  const [pendingFile, setPendingFile] = useState<File | null>(null)
  const [importing, setImporting] = useState(false)
  const [importResult, setImportResult] = useState<{
    total: number
    tables: number
    errors: string[]
  } | null>(null)

  const { data: info } = useQuery({
    queryKey: ['db-backup-info'],
    queryFn: async () => {
      const res = await getDatabaseBackupInfo()
      return res.success ? res.data : undefined
    },
    staleTime: 60_000,
  })

  const handleExport = async () => {
    setExporting(true)
    try {
      const res = await downloadDatabaseBackup(includeLogs)
      const blob = res.data instanceof Blob ? res.data : new Blob([res.data])
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = `new-api-backup-${new Date()
        .toISOString()
        .slice(0, 19)
        .replaceAll(':', '-')}.sqljson.gz`
      document.body.append(a)
      a.click()
      a.remove()
      // 延迟释放，确保下载已开始
      setTimeout(() => URL.revokeObjectURL(url), 10_000)
      toast.success(t('Backup downloaded'))
    } catch (error) {
      handleServerError(error, t('Backup failed'))
    } finally {
      setExporting(false)
    }
  }

  const handleFilePicked = (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0]
    if (file) {setPendingFile(file)}
    // 允许重复选择同一文件
    event.target.value = ''
  }

  const handleConfirmImport = async () => {
    if (!pendingFile) {return}
    setImporting(true)
    try {
      const res = await importDatabaseBackup(pendingFile)
      if (res.success && res.data) {
        setImportResult({
          total: res.data.total,
          tables: Object.keys(res.data.inserted).length,
          errors: res.data.errors ?? [],
        })
        toast.success(
          t('Import done: {{count}} rows added', { count: res.data.total })
        )
      } else {
        toast.error(res.message || t('Import failed'))
      }
    } catch (error) {
      handleServerError(error, t('Import failed'))
    } finally {
      setImporting(false)
      setPendingFile(null)
    }
  }

  return (
    <SettingsSection title={t('Database Backup')}>
      <div className='space-y-4'>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Export a full snapshot of the database, or restore one. Imports only add missing rows — they never delete or overwrite existing data.'
          )}
        </p>

        {/* 导出 */}
        <div className='space-y-3 rounded-lg border p-4'>
          <div className='flex items-center gap-2'>
            <Database className='text-muted-foreground size-4' />
            <span className='text-sm font-medium'>{t('Export')}</span>
            {info && (
              <Badge variant='secondary'>
                {t('{{count}} rows', { count: info.total })}
              </Badge>
            )}
          </div>
          <p className='text-muted-foreground text-xs'>
            {info
              ? t('{{count}} tables will be included', {
                  count: info.tables.length,
                })
              : t('Loading table list…')}
          </p>
          <div className='flex items-center gap-2'>
            <Checkbox
              id='db-backup-include-logs'
              checked={includeLogs}
              onCheckedChange={(v) => setIncludeLogs(v === true)}
            />
            <Label
              htmlFor='db-backup-include-logs'
              className='cursor-pointer text-sm font-normal'
            >
              {t('Include log database (large)')}
            </Label>
          </div>
          <Button onClick={handleExport} disabled={exporting}>
            {exporting ? (
              <Loader2 className='mr-2 size-4 animate-spin' />
            ) : (
              <Download className='mr-2 size-4' />
            )}
            {t('Download backup')}
          </Button>
        </div>

        {/* 导入 */}
        <div className='space-y-3 rounded-lg border p-4'>
          <div className='flex items-center gap-2'>
            <Upload className='text-muted-foreground size-4' />
            <span className='text-sm font-medium'>{t('Import')}</span>
          </div>
          <div className='text-muted-foreground flex items-start gap-2 text-xs'>
            <AlertTriangle className='mt-0.5 size-3.5 shrink-0 text-amber-500' />
            <span>
              {t(
                'Importing adds missing rows only. Existing rows with the same ID are skipped — your current data will not be deleted or modified.'
              )}
            </span>
          </div>
          <input
            ref={fileInputRef}
            type='file'
            accept='.gz,application/gzip'
            className='hidden'
            onChange={handleFilePicked}
          />
          <Button
            variant='outline'
            onClick={() => fileInputRef.current?.click()}
            disabled={importing}
          >
            {importing ? (
              <Loader2 className='mr-2 size-4 animate-spin' />
            ) : (
              <Upload className='mr-2 size-4' />
            )}
            {t('Choose backup file')}
          </Button>

          {importResult && (
            <div className='space-y-1 rounded-md bg-muted/40 p-3 text-xs'>
              <div className='font-medium'>
                {t('Import done: {{count}} rows added', {
                  count: importResult.total,
                })}
              </div>
              <div className='text-muted-foreground'>
                {t('{{count}} tables affected', {
                  count: importResult.tables,
                })}
              </div>
              {importResult.errors.length > 0 && (
                <div className='text-destructive'>
                  {t('{{count}} warnings', { count: importResult.errors.length })}
                  <ul className='mt-1 list-inside list-disc'>
                    {importResult.errors.slice(0, 5).map((e) => (
                      <li key={e} className='break-all'>
                        {e}
                      </li>
                    ))}
                  </ul>
                </div>
              )}
            </div>
          )}
        </div>
      </div>

      {/* 防错：导入前确认（不可逆的批量写入） */}
      <ConfirmDialog
        open={pendingFile !== null}
        onOpenChange={(open) => !open && setPendingFile(null)}
        title={t('Restore database backup?')}
        desc={t(
          'Import "{{name}}"? Missing rows will be added. Existing rows are kept and never overwritten.',
          { name: pendingFile?.name ?? '' }
        )}
        isLoading={importing}
        handleConfirm={handleConfirmImport}
      />
    </SettingsSection>
  )
}
