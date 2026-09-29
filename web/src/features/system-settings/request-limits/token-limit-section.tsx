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

const tokenLimitSchema = z.object({
  token_setting: z.object({
    max_user_tokens: z.number().min(1),
    require_verification_to_read_own_key: z.boolean(),
    require_verification_to_read_channel_key: z.boolean(),
  }),
})

type TokenLimitFormValues = z.output<typeof tokenLimitSchema>
type TokenLimitFormInput = z.input<typeof tokenLimitSchema>

type NormalizedTokenLimitValues = {
  'token_setting.max_user_tokens': number
  'token_setting.require_verification_to_read_own_key': boolean
  'token_setting.require_verification_to_read_channel_key': boolean
}

type TokenLimitSectionProps = {
  defaultValues: NormalizedTokenLimitValues
}

const buildFormDefaults = (
  defaults: TokenLimitSectionProps['defaultValues']
): TokenLimitFormInput => ({
  token_setting: {
    max_user_tokens: defaults['token_setting.max_user_tokens'],
    require_verification_to_read_own_key:
      defaults['token_setting.require_verification_to_read_own_key'],
    require_verification_to_read_channel_key:
      defaults['token_setting.require_verification_to_read_channel_key'],
  },
})

const normalizeFormValues = (
  values: TokenLimitFormValues
): NormalizedTokenLimitValues => ({
  'token_setting.max_user_tokens': values.token_setting.max_user_tokens,
  'token_setting.require_verification_to_read_own_key':
    values.token_setting.require_verification_to_read_own_key,
  'token_setting.require_verification_to_read_channel_key':
    values.token_setting.require_verification_to_read_channel_key,
})

export function TokenLimitSection({ defaultValues }: TokenLimitSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const form = useForm<TokenLimitFormInput, unknown, TokenLimitFormValues>({
    resolver: zodResolver(tokenLimitSchema),
    mode: 'onChange',
    defaultValues: buildFormDefaults(defaultValues),
  })

  useEffect(() => {
    form.reset(buildFormDefaults(defaultValues))
  }, [defaultValues, form])

  const onSubmit = async (values: TokenLimitFormValues) => {
    const normalized = normalizeFormValues(values)
    for (const key of Object.keys(normalized) as Array<
      keyof NormalizedTokenLimitValues
    >) {
      if (normalized[key] !== defaultValues[key]) {
        await updateOption.mutateAsync({ key, value: normalized[key] })
      }
    }
  }

  return (
    <SettingsSection title={t('Token Limits')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending}
            saveLabel='Save token limits'
          />
          <FormField
            control={form.control}
            name='token_setting.max_user_tokens'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Maximum tokens per user')}</FormLabel>
                <FormControl>
                  <Input
                    type='number'
                    min={1}
                    step={1}
                    {...field}
                    onChange={(e) =>
                      field.onChange(Number.parseInt(e.target.value) || 1)
                    }
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'Maximum number of tokens each user can create. Default 1000. Setting too large may affect performance.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='token_setting.require_verification_to_read_own_key'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>
                    {t('Require verification to view own API key')}
                  </FormLabel>
                  <FormDescription>
                    {t(
                      'When off (default), signed-in users reveal their own API keys without a second verification step. Turn on to restore step-up verification.'
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
            name='token_setting.require_verification_to_read_channel_key'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>
                    {t('Require verification to view channel keys')}
                  </FormLabel>
                  <FormDescription>
                    {t(
                      'When off (default), administrators can view and test channel keys without a second verification step. Turn on to restore step-up verification.'
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
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
