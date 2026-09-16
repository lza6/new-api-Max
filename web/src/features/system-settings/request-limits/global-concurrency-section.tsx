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
import { Activity } from 'lucide-react'
import { useEffect } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import * as z from 'zod'

import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { useStatus } from '@/hooks/use-status'

import {
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'

const createSchema = () =>
  z.object({
    enabled: z.boolean(),
    limit: z.number().int().min(0).max(100000000),
    queue: z.number().int().min(0).max(100000000),
    waitTimeout: z.number().int().min(1).max(3600),
  })

type FormValues = z.infer<ReturnType<typeof createSchema>>

interface GlobalConcurrencySectionProps {
  defaultValues: FormValues
}

/**
 * T6 全局真实并发桶：限制整个网关同时处理的模型请求数；超出的请求进入
 * 有界排队等待而不是直接 429。面板实时显示当前并发/排队水位。
 */
export function GlobalConcurrencySection({
  defaultValues,
}: GlobalConcurrencySectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const schema = createSchema()
  const { status } = useStatus()

  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    mode: 'onChange',
    defaultValues,
  })

  useEffect(() => {
    form.reset(defaultValues)
  }, [defaultValues, form])

  const onSubmit = async (values: FormValues) => {
    const updates: Array<{
      key: string
      value: string | number | boolean
    }> = (
      [
        ['relay.global_concurrency_enabled', values.enabled],
        ['relay.global_concurrency_limit', values.limit],
        ['relay.global_concurrency_queue', values.queue],
        ['relay.global_concurrency_wait_timeout', values.waitTimeout],
      ] as Array<[string, string | number | boolean]>
    ).map(([key, value]) => ({ key, value }))
    for (const { key, value } of updates) {
      if (value !== defaultValues[key as keyof FormValues]) {
        await updateOption.mutateAsync({ key, value })
      }
    }
  }

  // 并发水位（/api/status.global_concurrency）
  const concurrency = status?.global_concurrency as
    | { enabled?: boolean; active?: number; waiting?: number; limit?: number }
    | undefined

  return (
    <SettingsSection title={t('Global Concurrency Bucket')}>
      <p className='text-muted-foreground text-sm'>
        {t(
          'Limit how many model requests the whole gateway processes at once. Extra requests wait in a bounded queue instead of being rejected.'
        )}
      </p>
      <div className='bg-muted/40 mb-4 flex flex-wrap items-center gap-4 rounded-lg border p-4'>
        <span className='flex items-center gap-2 text-sm font-medium'>
          <Activity className='text-muted-foreground size-4' aria-hidden />
          {t('Current load')}
        </span>
        <span className='text-muted-foreground text-sm'>
          {t('Active')}:{' '}
          <span className='font-mono tabular-nums'>
            {concurrency?.active ?? 0}
          </span>
        </span>
        <span className='text-muted-foreground text-sm'>
          {t('Waiting')}:{' '}
          <span className='font-mono tabular-nums'>
            {concurrency?.waiting ?? 0}
          </span>
        </span>
        <span className='text-muted-foreground text-sm'>
          {t('Limit')}:{' '}
          <span className='font-mono tabular-nums'>
            {concurrency?.limit ?? defaultValues.limit}
          </span>
        </span>
      </div>

      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending}
            saveLabel='Save global concurrency'
          />
          <FormField
            control={form.control}
            name='enabled'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('Enable global concurrency bucket')}</FormLabel>
                  <FormDescription>
                    {t(
                      'When enabled, the gateway queues excess requests instead of overloading the upstream. Disabled by default.'
                    )}
                  </FormDescription>
                </SettingsSwitchContent>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                  />
                </FormControl>
              </SettingsSwitchItem>
            )}
          />

          <div className='grid gap-4 md:grid-cols-3'>
            <FormField
              control={form.control}
              name='limit'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Concurrency limit')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={0}
                      step={1}
                      {...field}
                      onChange={(e) =>
                        field.onChange(parseInt(e.target.value) || 0)
                      }
                    />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'Max in-flight requests processed at once. 0 = unlimited (bucket off).'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='queue'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Queue capacity')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={0}
                      step={1}
                      {...field}
                      onChange={(e) =>
                        field.onChange(parseInt(e.target.value) || 0)
                      }
                    />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'How many requests may wait in line. 0 = no waiting (immediate 429).'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='waitTimeout'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Queue wait timeout (seconds)')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={1}
                      step={1}
                      {...field}
                      onChange={(e) =>
                        field.onChange(parseInt(e.target.value) || 30)
                      }
                    />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'Requests still queued after this long are rejected with 429.'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
