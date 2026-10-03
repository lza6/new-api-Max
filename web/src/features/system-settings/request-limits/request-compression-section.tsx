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
import { useQuery } from '@tanstack/react-query'
import { ArrowDown } from 'lucide-react'
import { useEffect, useMemo } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import * as z from 'zod'

import { Button } from '@/components/ui/button'
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
import { getLiveRequests } from '@/features/system-info/api'
import { cn } from '@/lib/utils'

import {
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'

// 预设档位（KB）：常见请求体尺寸，一键选择。管理员也可自定义任意正整数值。
const THRESHOLD_PRESETS = [50, 128, 256, 512, 1024] as const

const createSchema = () =>
  z.object({
    enabled: z.boolean(),
    thresholdKb: z
      .number()
      .int()
      .min(1, 'Must be at least 1 KB')
      .max(1024 * 1024, 'Too large'),
  })

type FormValues = z.infer<ReturnType<typeof createSchema>>

interface RequestCompressionSectionProps {
  defaultValues: {
    enabled: boolean
    thresholdKb: number
  }
}

function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) {return '0 B'}
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const index = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1)
  const value = bytes / 1024 ** index
  return `${new Intl.NumberFormat(undefined, {
    maximumFractionDigits: index === 0 ? 0 : 1,
  }).format(value)} ${units[index]}`
}

/**
 * 出站请求体压缩：开关 + 阈值（预设档位/自定义）。大 prompt 上传会占用出口带宽
 * 并拖慢首字与并发小请求；gzip 可把 JSON 文本压到 ~1%。此处展示累计压缩字节与
 * 节省带宽总量（自进程启动累计，来自系统信息实时接口）。
 */
export function RequestCompressionSection({
  defaultValues,
}: RequestCompressionSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const schema = useMemo(() => createSchema(), [])

  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    mode: 'onChange',
    defaultValues: {
      enabled: defaultValues.enabled,
      thresholdKb: defaultValues.thresholdKb,
    },
  })

  useEffect(() => {
    form.reset({
      enabled: defaultValues.enabled,
      thresholdKb: defaultValues.thresholdKb,
    })
  }, [defaultValues.enabled, defaultValues.thresholdKb, form])

  const thresholdValue = form.watch('thresholdKb')

  // 累积统计：复用系统信息实时接口（管理员可见），每 30s 刷新一次。
  const totalsQuery = useQuery({
    queryKey: ['system-info', 'live-requests', 'compression-totals'],
    queryFn: async () => {
      const res = await getLiveRequests()
      return res.success && res.data ? res.data : null
    },
    refetchInterval: 30000,
    retry: false,
  })

  const totals = totalsQuery.data
  const hasTotals =
    (totals?.compression_total_count ?? 0) > 0 &&
    (totals?.compression_total_original_bytes ?? 0) > 0

  const onSubmit = async (values: FormValues) => {
    const updates: Array<[string, string | number | boolean]> = [
      ['relay.request_compression_enabled', values.enabled],
      ['relay.request_compression_threshold_kb', values.thresholdKb],
    ]
    for (const [key, value] of updates) {
      await updateOption.mutateAsync({ key, value })
    }
  }

  return (
    <SettingsSection title={t('Request Body Compression')}>
      <p className='text-muted-foreground text-sm'>
        {t(
          'Compress large request bodies (gzip) before sending upstream to cut upload bandwidth and first-token latency. Applies only to channels that enable it.'
        )}
      </p>

      {hasTotals ? (
        <div className='bg-muted/40 mb-4 flex flex-wrap items-center gap-4 rounded-lg border p-4'>
          <span className='flex items-center gap-2 text-sm font-medium'>
            <ArrowDown className='text-muted-foreground size-4' aria-hidden />
            {t('Total saved')}
          </span>
          <span className='text-muted-foreground text-sm'>
            {t('Compressed')}:{' '}
            <span className='font-mono tabular-nums'>
              {totals?.compression_total_count ?? 0}
            </span>
          </span>
          <span className='text-muted-foreground text-sm'>
            {t('Saved bandwidth')}:{' '}
            <span className='font-mono tabular-nums'>
              {formatBytes(totals?.compression_total_saved_bytes ?? 0)}
            </span>
          </span>
          <span className='text-muted-foreground text-sm'>
            {formatBytes(totals?.compression_total_original_bytes ?? 0)} →{' '}
            {formatBytes(totals?.compression_total_compressed_bytes ?? 0)}
          </span>
        </div>
      ) : null}

      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending}
            saveLabel='Save request compression'
          />

          <FormField
            control={form.control}
            name='enabled'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('Enable request body compression')}</FormLabel>
                  <FormDescription>
                    {t(
                      'Compress outbound request bodies at or above the threshold. Requires the channel to opt in.'
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

          <FormField
            control={form.control}
            name='thresholdKb'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Compression threshold (KB)')}</FormLabel>
                <div className='flex flex-wrap items-center gap-2'>
                  {THRESHOLD_PRESETS.map((preset) => (
                    <Button
                      key={preset}
                      type='button'
                      size='sm'
                      variant={field.value === preset ? 'default' : 'outline'}
                      className={cn('tabular-nums')}
                      onClick={() => field.onChange(preset)}
                    >
                      {preset} KB
                    </Button>
                  ))}
                </div>
                <FormControl>
                  <Input
                    type='number'
                    min={1}
                    step={1}
                    value={field.value}
                    onChange={(e) =>
                      field.onChange(Number.parseInt(e.target.value, 10) || 0)
                    }
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'Request bodies below this size are sent uncompressed. Presets cover common sizes; enter any value to customize.'
                  )}
                  {thresholdValue > 0 ? (
                    <>
                      {' '}
                      {t('Current')}:{' '}
                      <span className='font-mono tabular-nums'>
                        {thresholdValue} KB
                      </span>
                    </>
                  ) : null}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}

