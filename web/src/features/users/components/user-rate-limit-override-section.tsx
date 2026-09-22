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
import { Gauge, RotateCcw } from 'lucide-react'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { handleServerError } from '@/lib/handle-server-error'

import {
  getRelayRateLimitOverrides,
  setUserRateLimitOverride,
  type RelayRateLimitOverrides,
} from '../api'
import {
  rateLimitText,
  resolveEffectiveRate,
  type EffectiveRateSource,
} from '../lib/user-rate-limit'

interface Props {
  userId: number
  /** 用户当前分组：用于展示「分组覆盖」来源与生效档位 */
  group?: string
  onChanged?: () => void
}

/**
 * 管理员在「更新用户」抽屉内为单个用户直接查看/修改限速（并发/RPM，实时生效）：
 * - 展示该用户当前生效的真实档位及其来源（用户覆盖 / 分组覆盖 / 系统默认 / 已禁用）
 * - 输入并发与 RPM 保存即写入用户覆盖（0 表示该项不限）；「移除覆盖」恢复继承
 */
export function UserRateLimitOverrideSection(props: Props) {
  const { t } = useTranslation()
  const [overrides, setOverrides] = useState<RelayRateLimitOverrides | null>(
    null
  )
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [concurrency, setConcurrency] = useState(0)
  const [rpm, setRpm] = useState(0)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const res = await getRelayRateLimitOverrides()
      if (res.success && res.data) {
        setOverrides(res.data)
        const userTier = res.data.user_overrides?.[String(props.userId)]
        const effective = resolveEffectiveRate(
          res.data,
          props.userId,
          props.group
        )
        // 已有用户覆盖则回填其值；否则回填当前生效值，便于管理员直接调整
        setConcurrency(
          userTier ? userTier.concurrency : effective.tier.concurrency
        )
        setRpm(userTier ? userTier.rpm : effective.tier.rpm)
      } else {
        handleServerError(res, t('Failed to load rate limits'))
      }
    } catch (error) {
      handleServerError(error, t('Failed to load rate limits'))
    } finally {
      setLoading(false)
    }
  }, [props.userId, props.group, t])

  useEffect(() => {
    void load()
  }, [load])

  const effective = useMemo(
    () => resolveEffectiveRate(overrides, props.userId, props.group),
    [overrides, props.userId, props.group]
  )

  const save = async (values?: { concurrency: number; rpm: number }) => {
    const nextConcurrency = values ? values.concurrency : concurrency
    const nextRpm = values ? values.rpm : rpm
    setSaving(true)
    try {
      const res = await setUserRateLimitOverride(
        props.userId,
        nextConcurrency,
        nextRpm
      )
      if (res.success) {
        toast.success(t('User rate limit saved'))
        await load()
        props.onChanged?.()
      } else {
        handleServerError(res, t('Failed to save user rate limit'))
      }
    } catch (error) {
      handleServerError(error, t('Failed to save user rate limit'))
    } finally {
      setSaving(false)
    }
  }

  const sourceBadge = (source: EffectiveRateSource) => {
    if (source === 'user') {
      return (
        <StatusBadge label={t('User override')} variant='info' copyable={false} />
      )
    }
    if (source === 'group') {
      return (
        <StatusBadge
          label={t('Group override ({{group}})', { group: props.group || '-' })}
          variant='warning'
          copyable={false}
        />
      )
    }
    if (source === 'base') {
      return (
        <StatusBadge
          label={t('System default')}
          variant='neutral'
          copyable={false}
        />
      )
    }
    return (
      <StatusBadge
        label={t('Base limit disabled')}
        variant='danger'
        copyable={false}
      />
    )
  }

  if (loading) {
    return <p className='text-muted-foreground text-sm'>{t('Loading...')}</p>
  }

  return (
    <div className='flex flex-col gap-3 rounded-md border p-3'>
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <div className='flex items-center gap-2'>
          <Gauge className='text-muted-foreground size-4' />
          <span className='text-sm font-medium'>
            {t('Effective rate limit')}
          </span>
          {sourceBadge(effective.source)}
        </div>
        <div className='text-muted-foreground text-xs'>
          {t('Requests per second')} · {t('RPM')}
        </div>
      </div>

      <div className='text-muted-foreground grid grid-cols-2 gap-x-3 gap-y-1 text-xs'>
        <span>
          {t('Concurrency / s')}:{' '}
          <span className='tabular-nums'>
            {rateLimitText(effective.tier.concurrency, t)}
          </span>
        </span>
        <span>
          {t('RPM')}:{' '}
          <span className='tabular-nums'>
            {rateLimitText(effective.tier.rpm, t)}
          </span>
        </span>
      </div>

      <div className='grid grid-cols-2 gap-3'>
        <Label className='flex flex-col gap-1.5 text-xs'>
          {t('Concurrency / s')}
          <Input
            type='number'
            min={0}
            step={1}
            value={concurrency}
            onChange={(e) =>
              setConcurrency(Number.parseInt(e.target.value, 10) || 0)
            }
          />
        </Label>
        <Label className='flex flex-col gap-1.5 text-xs'>
          {t('RPM')}
          <Input
            type='number'
            min={0}
            step={1}
            value={rpm}
            onChange={(e) => setRpm(Number.parseInt(e.target.value, 10) || 0)}
          />
        </Label>
      </div>

      <div className='flex flex-wrap items-center gap-2'>
        <Button
          type='button'
          size='sm'
          disabled={saving}
          onClick={() => void save()}
        >
          {t('Save rate limit')}
        </Button>
        {effective.source === 'user' && (
          <Button
            type='button'
            size='sm'
            variant='outline'
            disabled={saving}
            onClick={() => void save({ concurrency: 0, rpm: 0 })}
          >
            <RotateCcw data-icon='inline-start' />
            {t('Remove override')}
          </Button>
        )}
      </div>
      <p className='text-muted-foreground text-xs'>
        {t(
          'Save writes a per-user override that takes effect immediately. 0 means unlimited for that item; removing the override falls back to group or system default.'
        )}
      </p>
    </div>
  )
}
