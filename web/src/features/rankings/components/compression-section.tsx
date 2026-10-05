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
import { FileArchive } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Skeleton } from '@/components/ui/skeleton'

import { useCompressionStats } from '../hooks/use-compression'

function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) {
    return '0 B'
  }
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.min(
    Math.floor(Math.log(bytes) / Math.log(1024)),
    units.length - 1
  )
  return `${(bytes / 1024 ** i).toFixed(i === 0 ? 0 : 1)} ${units[i]}`
}

/**
 * 按模型出站压缩节省榜：次数 / 节省流量（GB）。
 * 数据持久化在 DB，重启容器不丢。对应后端 /api/rankings/compression。
 */
export function CompressionSection() {
  const { t } = useTranslation()
  const query = useCompressionStats(50)
  const rows = query.data ?? []

  const totalSaved = rows.reduce((s, r) => s + r.saved_bytes, 0)
  const totalCount = rows.reduce((s, r) => s + r.count, 0)

  return (
    <section className='bg-card rounded-2xl border p-5 shadow-xs'>
      <div className='mb-4 flex items-center gap-2'>
        <FileArchive className='text-muted-foreground size-4' />
        <div>
          <h2 className='text-lg font-semibold tracking-tight'>
            {t('Compression Savings Leaderboard')}
          </h2>
          <p className='text-muted-foreground text-xs'>
            {t(
              'Request-body gzip savings by model (persisted, survives restarts)'
            )}
          </p>
        </div>
      </div>

      {query.isLoading && (
        <div className='space-y-2'>
          {Array.from({ length: 6 }, (_, i) => (
            <Skeleton key={i} className='h-8 w-full' />
          ))}
        </div>
      )}
      {!query.isLoading && query.error && (
        <p className='text-muted-foreground text-sm'>
          {t('Unable to load compression data')}
        </p>
      )}
      {!query.isLoading && !query.error && rows.length === 0 && (
        <p className='text-muted-foreground text-sm'>
          {t('No compression data yet')}
        </p>
      )}
      {!query.isLoading && !query.error && rows.length > 0 && (
        <>
          <div className='text-muted-foreground mb-2 flex flex-wrap gap-x-6 gap-y-1 text-xs'>
            <span>
              {t('Total saved')}:{' '}
              <span className='text-foreground font-medium'>
                {formatBytes(totalSaved)}
              </span>
            </span>
            <span>
              {t('Compressed requests')}:{' '}
              <span className='text-foreground font-medium'>{totalCount}</span>
            </span>
          </div>
          <ul className='divide-border/60 divide-y'>
            {rows.map((row, index) => {
              const ratioPct = Math.round((1 - row.ratio) * 100)
              return (
                <li
                  key={row.model_name}
                  className='flex items-center gap-3 py-2 text-sm'
                >
                  <span className='text-muted-foreground w-6 shrink-0 tabular-nums'>
                    {index + 1}
                  </span>
                  <span className='min-w-0 flex-1 truncate font-medium'>
                    {row.model_name}
                  </span>
                  <span className='text-muted-foreground shrink-0 text-xs tabular-nums'>
                    {t('{{n}} times', { n: row.count })}
                  </span>
                  <span className='text-emerald-600 dark:text-emerald-400 w-20 shrink-0 text-right text-xs tabular-nums'>
                    -{ratioPct}%
                  </span>
                  <span className='w-20 shrink-0 text-right font-mono text-xs tabular-nums'>
                    {formatBytes(row.saved_bytes)}
                  </span>
                </li>
              )
            })}
          </ul>
        </>
      )}

      {/* 科普：什么是压缩率 / 采用什么技术 */}
      <details className='bg-muted/30 mt-4 rounded-lg border p-3 text-xs'>
        <summary className='cursor-pointer font-medium'>
          {t('What is compression ratio & how does it work?')}
        </summary>
        <div className='text-muted-foreground mt-2 space-y-1.5 leading-relaxed'>
          <p>
            {t(
              'Compression ratio = compressed size ÷ original size. 30% means a 2.1 MB prompt is sent upstream as ~0.6 MB.'
            )}
          </p>
          <p>
            {t(
              'Technology: the gateway gzip-compresses the request body (Content-Encoding: gzip) before upload; the upstream decompresses transparently. Technical basis: RFC 1952 gzip (DEFLATE, RFC 1951).'
            )}
          </p>
          <p>
            {t(
              'Why not 99%: gzip only removes redundancy. Plain text/code compresses to ~1%; Base64 images and high-entropy data approach their Shannon entropy limit and barely compress.'
            )}
          </p>
        </div>
      </details>
    </section>
  )
}
