import { zodResolver } from '@hookform/resolvers/zod'
import { Gauge } from 'lucide-react'
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
    concurrency: z.number().int().min(0).max(100000),
    rpm: z.number().int().min(0).max(1000000),
    exemptModels: z.string(),
  })

type FormValues = z.infer<ReturnType<typeof createSchema>>

interface UserRateLimitSectionProps {
  defaultValues: FormValues
}

/**
 * T7 每用户基础限速：默认对所有用户生效（并发/秒 + RPM/分钟，超限 429）。
 * 分组/用户覆盖通过 /api/option/relay/rate_limit/overrides 管理端点调整。
 */

/** 把逗号分隔的豁免模型输入转换为后端要求的 JSON 数组字符串（空 = []）。 */
function toExemptModelsValue(input: string): string {
  const list = input
    .split(',')
    .map((s) => s.trim())
    .filter(Boolean)
  return JSON.stringify(list)
}

export function UserRateLimitSection({
  defaultValues,
}: UserRateLimitSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const schema = createSchema()

  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    mode: 'onChange',
    defaultValues,
  })

  useEffect(() => {
    form.reset(defaultValues)
  }, [defaultValues, form])

  const onSubmit = async (values: FormValues) => {
    const updates: Array<{ key: string; value: string | number | boolean }> = (
      [
        ['relay.user_base_rate_limit_enabled', values.enabled],
        ['relay.user_base_concurrency_limit', values.concurrency],
        ['relay.user_base_rpm_limit', values.rpm],
        ['relay.user_rate_limit_exempt_models', toExemptModelsValue(values.exemptModels)],
      ] as Array<[string, string | number | boolean]>
    ).map(([key, value]) => ({ key, value }))
    for (const { key, value } of updates) {
      if (value !== defaultValues[key as keyof FormValues]) {
        await updateOption.mutateAsync({ key, value })
      }
    }
  }

  return (
    <SettingsSection title={t('User Base Rate Limit')}>
      <p className='text-muted-foreground text-sm'>
        {t(
          'Every user is rate limited by default: concurrency (requests per second) and RPM (requests per minute). Over-limit requests get 429. Group/user overrides are managed via the rate-limit overrides API. Users with an active subscription are governed by their subscription tier instead of this base limit.'
        )}
      </p>

      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending}
            saveLabel='Save user base rate limit'
          />
          <FormField
            control={form.control}
            name='enabled'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('Enable per-user base rate limit')}</FormLabel>
                  <FormDescription>
                    {t(
                      'On by default (3 concurrent requests/second, 120 RPM). Turn off to disable the base limit for all users.'
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

          <div className='grid gap-4 md:grid-cols-2'>
            <FormField
              control={form.control}
              name='concurrency'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Concurrency per second')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={0}
                      step={1}
                      {...field}
                      onChange={(e) =>
                        field.onChange(Number.parseInt(e.target.value, 10) || 0)
                      }
                    />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'Max in-flight requests per user. 3 means ~3 requests/second. 0 = unlimited.'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='rpm'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('RPM (requests per minute)')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={0}
                      step={1}
                      {...field}
                      onChange={(e) =>
                        field.onChange(Number.parseInt(e.target.value, 10) || 0)
                      }
                    />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'Max requests per user in a 60s sliding window. 0 = unlimited.'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>

          <FormField
            control={form.control}
            name='exemptModels'
            render={({ field }) => (
              <FormItem>
                <FormLabel>
                  {t('Rate limit exempt models (comma separated)')}
                </FormLabel>
                <FormControl>
                  <Input
                    placeholder='google-translate'
                    {...field}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'Models that bypass the per-user concurrency and RPM base limits for everyone (e.g. the free translation model google-translate = unlimited). Other models keep per-user limits.'
                  )}
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

/** Icon export keeps the section self-describing in the registry list. */
export const USER_RATE_LIMIT_SECTION_ICON = Gauge
