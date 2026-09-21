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
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import { api } from '@/lib/api'
import { handleServerError } from '@/lib/handle-server-error'
import { requireServerSuccess } from '@/lib/server-error-message'

import { SettingsSection } from '../components/settings-section'

interface RateLimitTier {
  concurrency: number
  rpm: number
}

interface OverridesData {
  base_enabled: boolean
  base_concurrency: number
  base_rpm: number
  group_overrides: Record<string, RateLimitTier>
  user_overrides: Record<string, RateLimitTier>
}

const OVERRIDES_KEY = ['rate-limit-overrides']

async function getOverrides(): Promise<{ success: boolean; data?: OverridesData }> {
  const res = await api.get('/api/option/relay/rate_limit/overrides')
  return res.data
}

async function putGroupOverride(group: string, concurrency: number, rpm: number) {
  const res = await api.put('/api/option/relay/rate_limit/overrides/group', {
    group,
    concurrency,
    rpm,
  })
  return res.data
}

async function putUserOverride(userId: number, concurrency: number, rpm: number) {
  const res = await api.put('/api/option/relay/rate_limit/overrides/user', {
    user_id: userId,
    concurrency,
    rpm,
  })
  return res.data
}

function TierInputs(props: {
  concurrency: number
  rpm: number
  onConcurrencyChange: (v: number) => void
  onRpmChange: (v: number) => void
}) {
  const { t } = useTranslation()
  return (
    <div className='grid grid-cols-2 gap-3'>
      <Label className='flex flex-col gap-1.5 text-xs'>
        {t('Concurrency / s')}
        <Input
          type='number'
          min={0}
          step={1}
          value={props.concurrency}
          onChange={(e) =>
            props.onConcurrencyChange(Number.parseInt(e.target.value, 10) || 0)
          }
        />
      </Label>
      <Label className='flex flex-col gap-1.5 text-xs'>
        {t('RPM')}
        <Input
          type='number'
          min={0}
          step={1}
          value={props.rpm}
          onChange={(e) =>
            props.onRpmChange(Number.parseInt(e.target.value, 10) || 0)
          }
        />
      </Label>
    </div>
  )
}

/**
 * 分组/用户限速覆盖：优先级 用户覆盖 > 分组覆盖 > 基础默认（3/s + 120RPM）。
 * 写入即热更新（/api/option/relay/rate_limit/overrides/*），0/0 表示移除覆盖。
 */
export function RateLimitOverridesSection() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { data, isLoading } = useQuery({
    queryKey: OVERRIDES_KEY,
    queryFn: async () => requireServerSuccess(await getOverrides()).data,
    staleTime: 30_000,
  })
  const [group, setGroup] = useState('')
  const [groupConcurrency, setGroupConcurrency] = useState(0)
  const [groupRpm, setGroupRpm] = useState(0)
  const [userId, setUserId] = useState('')
  const [userConcurrency, setUserConcurrency] = useState(0)
  const [userRpm, setUserRpm] = useState(0)
  const [saving, setSaving] = useState(false)

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: OVERRIDES_KEY })
  }

  const handleGroupSave = async () => {
    const g = group.trim()
    if (!g) {
      toast.error(t('Group is required'))
      return
    }
    setSaving(true)
    try {
      const res = await putGroupOverride(g, groupConcurrency, groupRpm)
      if (!res.success) {
        handleServerError(res, t('Failed to save group rate limit override'))
        return
      }
      toast.success(t('Group rate limit override saved'))
      setGroup('')
      setGroupConcurrency(0)
      setGroupRpm(0)
      invalidate()
    } catch (error) {
      handleServerError(error, t('Failed to save group rate limit override'))
    } finally {
      setSaving(false)
    }
  }

  const handleUserSave = async () => {
    const id = Number.parseInt(userId, 10)
    if (!id || id <= 0) {
      toast.error(t('User ID is required'))
      return
    }
    setSaving(true)
    try {
      const res = await putUserOverride(id, userConcurrency, userRpm)
      if (!res.success) {
        handleServerError(res, t('Failed to save user rate limit override'))
        return
      }
      toast.success(t('User rate limit override saved'))
      setUserId('')
      setUserConcurrency(0)
      setUserRpm(0)
      invalidate()
    } catch (error) {
      handleServerError(error, t('Failed to save user rate limit override'))
    } finally {
      setSaving(false)
    }
  }

  const groupOverrides = data?.group_overrides ?? {}
  const userOverrides = data?.user_overrides ?? {}

  return (
    <SettingsSection title={t('Rate Limit Overrides')}>
      <p className='text-muted-foreground text-sm'>
        {t(
          'Per-group and per-user concurrency/RPM overrides. Priority: user override > group override > base (3/s, 120 RPM). Saving 0/0 removes an override. Changes apply immediately.'
        )}
      </p>

      {isLoading ? (
        <div className='space-y-2'>
          <Skeleton className='h-8 w-full rounded-md' />
          <Skeleton className='h-8 w-full rounded-md' />
        </div>
      ) : (
        <div className='grid gap-6 lg:grid-cols-2'>
          <div className='space-y-4'>
            <div className='flex items-center justify-between'>
              <h4 className='text-sm font-semibold'>
                {t('Group overrides')}
              </h4>
              <span className='text-muted-foreground text-xs'>
                {Object.keys(groupOverrides).length}
              </span>
            </div>
            {Object.keys(groupOverrides).length === 0 ? (
              <p className='text-muted-foreground text-xs'>
                {t('No group overrides')}
              </p>
            ) : (
              <ul className='divide-border divide-y rounded-md border text-sm'>
                {Object.entries(groupOverrides).map(([g, tier]) => (
                  <li
                    key={g}
                    className='flex flex-wrap items-center justify-between gap-2 px-3 py-2'
                  >
                    <span className='font-mono'>{g}</span>
                    <span className='text-muted-foreground text-xs'>
                      {t('{{c}}/s · {{rpm}} rpm', {
                        c: tier.concurrency,
                        rpm: tier.rpm,
                      })}
                    </span>
                    <Button
                      variant='ghost'
                      size='sm'
                      className='h-7 text-xs'
                      disabled={saving}
                      onClick={() => {
                        void (async () => {
                          setSaving(true)
                          try {
                            await putGroupOverride(g, 0, 0)
                            toast.success(
                              t('Group rate limit override removed')
                            )
                            invalidate()
                          } catch (error) {
                            handleServerError(error)
                          } finally {
                            setSaving(false)
                          }
                        })()
                      }}
                    >
                      {t('Remove')}
                    </Button>
                  </li>
                ))}
              </ul>
            )}

            <div className='space-y-2 rounded-md border p-3'>
              <Label className='flex flex-col gap-1.5 text-xs'>
                {t('Group')}
                <Input
                  value={group}
                  placeholder='vip'
                  onChange={(e) => setGroup(e.target.value)}
                />
              </Label>
              <TierInputs
                concurrency={groupConcurrency}
                rpm={groupRpm}
                onConcurrencyChange={setGroupConcurrency}
                onRpmChange={setGroupRpm}
              />
              <Button
                variant='outline'
                size='sm'
                className='w-full'
                disabled={saving}
                onClick={() => void handleGroupSave()}
              >
                {t('Save group override')}
              </Button>
            </div>
          </div>

          <div className='space-y-4'>
            <div className='flex items-center justify-between'>
              <h4 className='text-sm font-semibold'>
                {t('User overrides')}
              </h4>
              <span className='text-muted-foreground text-xs'>
                {Object.keys(userOverrides).length}
              </span>
            </div>
            {Object.keys(userOverrides).length === 0 ? (
              <p className='text-muted-foreground text-xs'>
                {t('No user overrides')}
              </p>
            ) : (
              <ul className='divide-border divide-y rounded-md border text-sm'>
                {Object.entries(userOverrides).map(([uid, tier]) => (
                  <li
                    key={uid}
                    className='flex flex-wrap items-center justify-between gap-2 px-3 py-2'
                  >
                    <span className='font-mono'>#{uid}</span>
                    <span className='text-muted-foreground text-xs'>
                      {t('{{c}}/s · {{rpm}} rpm', {
                        c: tier.concurrency,
                        rpm: tier.rpm,
                      })}
                    </span>
                    <Button
                      variant='ghost'
                      size='sm'
                      className='h-7 text-xs'
                      disabled={saving}
                      onClick={() => {
                        void (async () => {
                          setSaving(true)
                          try {
                            await putUserOverride(
                              Number.parseInt(uid, 10),
                              0,
                              0
                            )
                            toast.success(t('User rate limit override removed'))
                            invalidate()
                          } catch (error) {
                            handleServerError(error)
                          } finally {
                            setSaving(false)
                          }
                        })()
                      }}
                    >
                      {t('Remove')}
                    </Button>
                  </li>
                ))}
              </ul>
            )}

            <div className='space-y-2 rounded-md border p-3'>
              <Label className='flex flex-col gap-1.5 text-xs'>
                {t('User ID')}
                <Input
                  type='number'
                  min={1}
                  step={1}
                  value={userId}
                  placeholder='123'
                  onChange={(e) => setUserId(e.target.value)}
                />
              </Label>
              <TierInputs
                concurrency={userConcurrency}
                rpm={userRpm}
                onConcurrencyChange={setUserConcurrency}
                onRpmChange={setUserRpm}
              />
              <Button
                variant='outline'
                size='sm'
                className='w-full'
                disabled={saving}
                onClick={() => void handleUserSave()}
              >
                {t('Save user override')}
              </Button>
            </div>
          </div>
        </div>
      )}
    </SettingsSection>
  )
}
