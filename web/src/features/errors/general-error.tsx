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
import { useState } from 'react'
import { useNavigate, useRouter } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

const FEEDBACK_URL = 'https://github.com/lza6/new-api-Max/issues'

type GeneralErrorProps = React.HTMLAttributes<HTMLDivElement> & {
  minimal?: boolean
  error?: unknown
}

function getHttpStatus(error: unknown): number | undefined {
  if (typeof error !== 'object' || error === null) {return undefined}
  const response = (error as Record<string, unknown>).response
  if (typeof response !== 'object' || response === null) {return undefined}
  const status = (response as Record<string, unknown>).status
  return typeof status === 'number' ? status : undefined
}

export function GeneralError({
  className,
  minimal = false,
  error,
}: GeneralErrorProps) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { history } = useRouter()
  const status = getHttpStatus(error)
  const isRateLimited = status === 429
  const title = isRateLimited
    ? t('Too many requests')
    : t('Oops! Something went wrong')
  const description = isRateLimited
    ? t('Please wait a moment before trying again.')
    : t('Please try again later.')
  // 诊断信息：用户报障时能直接贴给客服，不用再猜「哪个请求挂了」。
  const requestId =
    (typeof error === 'object' &&
      error !== null &&
      (error as { response?: { headers?: Record<string, string> } }).response
        ?.headers?.['x-request-id']) ||
    globalThis.crypto?.randomUUID?.().slice(0, 8) ||
    'unknown'
  const [copied, setCopied] = useState(false)
  const copyDiagnostics = async () => {
    const payload = [
      `status=${status ?? 'n/a'}`,
      `requestId=${requestId}`,
      `url=${globalThis.location?.href ?? ''}`,
      `time=${new Date().toISOString()}`,
      `message=${String((error as { message?: string })?.message ?? '')}`,
    ].join('\n')
    try {
      await navigator.clipboard.writeText(payload)
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    } catch {
      // 剪贴板不可用时保持静默，用户仍可手动复制 request id
    }
  }

  return (
    <div className={cn('h-svh w-full', className)}>
      <div className='m-auto flex h-full w-full flex-col items-center justify-center gap-3 px-6 text-center'>
        {/* 人话先说：状态码退成小标签，不再是页面主体。 */}
        <span className='text-foreground text-xl font-semibold sm:text-2xl'>
          {title}
        </span>
        {!minimal && (
          <span className='border-border/60 text-muted-foreground rounded-full border px-2.5 py-0.5 font-mono text-xs'>
            HTTP {status ?? 500}
          </span>
        )}
        <p className='text-muted-foreground max-w-md text-sm'>
          {t('We apologize for the inconvenience.')} {description}
        </p>
        {!minimal && (
          <div className='bg-muted/40 text-muted-foreground flex items-center gap-2 rounded-md px-3 py-1.5 font-mono text-xs'>
            <span>{t('Request ID')}</span>
            <code className='text-foreground select-all'>{requestId}</code>
            <Button
              size='sm'
              variant='ghost'
              className='h-6 px-2 text-xs'
              onClick={() => {
                void copyDiagnostics()
              }}
            >
              {copied ? t('Copied') : t('Copy')}
            </Button>
          </div>
        )}
        {!minimal && (
          <p className='text-muted-foreground/80 max-w-md text-xs'>
            {t('If this keeps happening, please report it on GitHub Issues.')}
          </p>
        )}
        {!minimal && (
          <div className='mt-4 flex flex-wrap justify-center gap-3'>
            <Button variant='outline' onClick={() => history.go(-1)}>
              {t('Go Back')}
            </Button>
            <Button
              variant='outline'
              render={
                <a
                  href={FEEDBACK_URL}
                  target='_blank'
                  rel='noopener noreferrer'
                />
              }
            >
              {t('Report an issue')}
            </Button>
            <Button onClick={() => navigate({ to: '/' })}>
              {t('Back to Home')}
            </Button>
          </div>
        )}
      </div>
    </div>
  )
}
