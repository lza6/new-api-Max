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
import type { ChangeEvent } from 'react'
import type { Resolver } from 'react-hook-form'
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

import {
  SettingsForm,
  SettingsFormGrid,
  SettingsFormGridItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useSettingsForm } from '../hooks/use-settings-form'
import { useUpdateOption } from '../hooks/use-update-option'

// 与服务端 channel_health_setting getter 回退语义对齐（默认值 = 历史常量）。
const channelHealthSchema = z.object({
  channel_health: z.object({
    window_seconds: z.coerce.number().int().positive(),
    ring_size: z.coerce.number().int().min(1).max(4096),
    success_weight: z.coerce.number().int().min(1).max(100),
    latency_best_ms: z.coerce.number().int().positive(),
    latency_worst_ms: z.coerce.number().int().positive(),
    min_score: z.coerce.number().int().min(0).max(100),
  }).refine((v) => v.latency_worst_ms > v.latency_best_ms, {
    message: 'Latency worst threshold must be greater than best',
    path: ['latency_worst_ms'],
  }),
})

type ChannelHealthFormValues = z.infer<typeof channelHealthSchema>
type ChannelHealthInputValue = number | ''

type ChannelHealthSettingsSectionProps = {
  defaultValues: ChannelHealthFormValues['channel_health']
}

const handleNumberChange =
  (onChange: (value: ChannelHealthInputValue) => void) =>
  (event: ChangeEvent<HTMLInputElement>) => {
    const value = event.currentTarget.valueAsNumber
    onChange(Number.isNaN(value) ? '' : value)
  }

export function ChannelHealthSettingsSection({
  defaultValues,
}: ChannelHealthSettingsSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const { form, handleSubmit, isDirty, isSubmitting } =
    useSettingsForm<ChannelHealthFormValues>({
      resolver: zodResolver(channelHealthSchema) as Resolver<
        ChannelHealthFormValues,
        unknown,
        ChannelHealthFormValues
      >,
      defaultValues: { channel_health: defaultValues } as ChannelHealthFormValues,
      onSubmit: async (_data, changedFields) => {
        for (const [key, value] of Object.entries(changedFields)) {
          await updateOption.mutateAsync({
            key,
            value: value as string | number | boolean,
          })
        }
      },
    })

  const fields: Array<{
    name: 'window_seconds' | 'ring_size' | 'success_weight' | 'latency_best_ms' | 'latency_worst_ms' | 'min_score'
    labelKey: string
    descriptionKey: string
    min: number
    max?: number
  }> = [
    { name: 'window_seconds' as const, labelKey: 'Channel health window seconds', descriptionKey: 'Health score sliding window in seconds. Invalid values fall back to 3600.', min: 1 },
    { name: 'ring_size' as const, labelKey: 'Per-channel sample ring size', descriptionKey: 'Maximum health samples kept per channel. Invalid values fall back to 256; values above 4096 are clamped.', min: 1, max: 4096 },
    { name: 'success_weight' as const, labelKey: 'Success weight (%)', descriptionKey: 'Weight of success rate in the health score (latency weight = 100 - success weight). Invalid values fall back to 70.', min: 1, max: 100 },
    { name: 'latency_best_ms' as const, labelKey: 'Latency best threshold (ms)', descriptionKey: 'Latency at or below this gets full latency score. Invalid values fall back to 1500.', min: 1 },
    { name: 'latency_worst_ms' as const, labelKey: 'Latency worst threshold (ms)', descriptionKey: 'Latency at or above this gets zero latency score; must be above the best threshold. Invalid values fall back to 10000.', min: 1 },
    { name: 'min_score' as const, labelKey: 'Minimum health score for routing', descriptionKey: 'Channels below this health score are filtered from routing; 0 only removes cooled-down channels. Invalid values fall back to 0.', min: 0, max: 100 },
  ] as const

  return (
    <SettingsSection title={t('Channel Health Settings')}>
      <Form {...form}>
        <SettingsForm onSubmit={handleSubmit}>
          <SettingsFormGrid>
            {fields.map((fieldDef) => (
              <SettingsFormGridItem key={fieldDef.name}>
                <FormField
                  control={form.control}
                  name={`channel_health.${fieldDef.name}`}
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t(fieldDef.labelKey)}</FormLabel>
                      <FormControl>
                        <Input
                          type='number'
                          min={fieldDef.min}
                          max={fieldDef.max}
                          step={1}
                          value={field.value ?? ''}
                          onChange={handleNumberChange(field.onChange)}
                          name={field.name}
                          onBlur={field.onBlur}
                          ref={field.ref}
                        />
                      </FormControl>
                      <FormDescription>{t(fieldDef.descriptionKey)}</FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </SettingsFormGridItem>
            ))}
          </SettingsFormGrid>
          <SettingsPageFormActions
            onSave={handleSubmit}
            isSaving={isSubmitting}
            isSaveDisabled={!isDirty}
            saveLabel={t('Save channel health settings')}
          />
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
