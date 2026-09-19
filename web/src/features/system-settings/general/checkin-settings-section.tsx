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
import { zodResolver } from '@hookform/resolvers/zod'
import { RotateCcw } from 'lucide-react'
import { useState } from 'react'
import { useForm, type Resolver } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import {
  getCurrencyDisplay,
  getCurrencyLabel,
} from '@/lib/currency'
import {
  formatQuota,
  parseQuotaFromDollars,
  quotaUnitsToEditableAmount,
} from '@/lib/format'
import { api } from '@/lib/api'
import { handleServerError } from '@/lib/handle-server-error'

import {
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'

const schema = z.object({
  enabled: z.boolean(),
  // 表单字段是显示货币金额（如 ¥0.5 = 0.5），必须允许小数；
  // 内部 quota 的整数换算在 parseQuotaFromDollars 内完成。
  minQuota: z.coerce.number().min(0),
  maxQuota: z.coerce.number().min(0),
})

type Values = z.infer<typeof schema>

export function CheckinSettingsSection({
  defaultValues,
}: {
  defaultValues: {
    enabled: boolean
    minQuota: number
    maxQuota: number
  }
}) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const { meta: currencyMeta } = getCurrencyDisplay()
  const currencyLabel = getCurrencyLabel()
  const tokensOnly = currencyMeta.kind === 'tokens'

  const form = useForm<Values>({
    resolver: zodResolver(schema) as unknown as Resolver<Values>,
    defaultValues: {
      enabled: defaultValues.enabled,
      // 额度奖励按显示货币金额回填（设 1 就是 1 美元/自定义单位）。
      minQuota: quotaUnitsToEditableAmount(defaultValues.minQuota),
      maxQuota: quotaUnitsToEditableAmount(defaultValues.maxQuota),
    },
  })

  const { isDirty, isSubmitting } = form.formState
  const enabled = form.watch('enabled')

  // P1 管理员重置所有签到（福利重置）：清空全部签到记录，用户可重新签到。
  const [resetting, setResetting] = useState(false)
  const resetAllCheckins = async () => {
    if (!window.confirm(t('Reset all check-in records? Users can check in again.'))) {
      return
    }
    setResetting(true)
    try {
      const res = await api.post('/api/user/checkins/reset')
      const data = res.data as { success?: boolean; data?: { deleted?: number } }
      if (data?.success) {
        toast.success(
          t('Check-in records reset ({{count}} deleted)', {
            count: data.data?.deleted ?? 0,
          })
        )
      } else {
        handleServerError(res, t('Failed to reset check-in records'))
      }
    } catch (error) {
      handleServerError(error, t('Failed to reset check-in records'))
    } finally {
      setResetting(false)
    }
  }

  function formatAwardQuota(value: number): string {
    return formatQuota(parseQuotaFromDollars(Number.isFinite(value) ? value : 0))
  }

  async function onSubmit(values: Values) {
    const updates: Array<{ key: string; value: string }> = []

    if (values.enabled !== defaultValues.enabled) {
      updates.push({
        key: 'checkin_setting.enabled',
        value: String(values.enabled),
      })
    }

    if (values.minQuota !== quotaUnitsToEditableAmount(defaultValues.minQuota)) {
      updates.push({
        key: 'checkin_setting.min_quota',
        // 显示货币金额换算回内部 quota 单位（设 1 存 1 美元对应额度）。
        value: String(parseQuotaFromDollars(values.minQuota)),
      })
    }

    if (values.maxQuota !== quotaUnitsToEditableAmount(defaultValues.maxQuota)) {
      updates.push({
        key: 'checkin_setting.max_quota',
        value: String(parseQuotaFromDollars(values.maxQuota)),
      })
    }

    if (updates.length === 0) {
      toast.info(t('No changes to save'))
      return
    }

    for (const update of updates) {
      await updateOption.mutateAsync(update)
    }

    form.reset(values)
  }

  return (
    <SettingsSection title={t('Check-in Settings')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)} autoComplete='off'>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending || isSubmitting}
            isSaveDisabled={!isDirty}
            saveLabel='Save check-in settings'
          />
          <FormField
            control={form.control}
            name='enabled'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('Enable check-in feature')}</FormLabel>
                  <FormDescription>
                    {t(
                      'Allow users to check in daily for random quota rewards'
                    )}
                  </FormDescription>
                </SettingsSwitchContent>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                    disabled={updateOption.isPending || isSubmitting}
                  />
                </FormControl>
              </SettingsSwitchItem>
            )}
          />

          {enabled && (
            <div className='grid gap-6 sm:grid-cols-2'>
              <FormField
                control={form.control}
                name='minQuota'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Minimum check-in quota')}</FormLabel>
                    <FormControl>
                      <Input
                        type='number'
                        min={0}
                        step={tokensOnly ? '1' : '0.01'}
                        placeholder={tokensOnly ? t('1000') : t('e.g. 0.1')}
                        {...field}
                      />
                    </FormControl>
                    <FormDescription>
                      {tokensOnly
                        ? t('Minimum quota amount awarded for check-in')
                        : t(
                            'Minimum amount in {{currency}} awarded for check-in (= {{formattedQuota}})',
                            {
                              currency: currencyLabel,
                              formattedQuota: formatAwardQuota(
                                Number(field.value) || 0
                              ),
                            }
                          )}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='maxQuota'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Maximum check-in quota')}</FormLabel>
                    <FormControl>
                      <Input
                        type='number'
                        min={0}
                        step={tokensOnly ? '1' : '0.01'}
                        placeholder={tokensOnly ? t('10000') : t('e.g. 1')}
                        {...field}
                      />
                    </FormControl>
                    <FormDescription>
                      {tokensOnly
                        ? t('Maximum quota amount awarded for check-in')
                        : t(
                            'Maximum amount in {{currency}} awarded for check-in (= {{formattedQuota}})',
                            {
                              currency: currencyLabel,
                              formattedQuota: formatAwardQuota(
                                Number(field.value) || 0
                              ),
                            }
                          )}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>
          )}
        </SettingsForm>
      </Form>

      {/* P1 管理员重置所有签到（福利） */}
      <div className='border-border/60 border-t pt-4'>
        <Button
          variant='outline'
          size='sm'
          onClick={resetAllCheckins}
          disabled={resetting}
          className='text-muted-foreground'
        >
          <RotateCcw className='size-4' aria-hidden />
          {resetting ? t('Resetting…') : t('Reset all check-in records')}
        </Button>
        <p className='text-muted-foreground/70 mt-2 text-xs'>
          {t(
            'Clear all users check-in history so everyone can check in again (e.g. as a one-time bonus).'
          )}
        </p>
      </div>
    </SettingsSection>
  )
}
