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
  FormLabel,
} from '@/components/ui/form'
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
  })

type FormValues = z.infer<ReturnType<typeof createSchema>>

interface LoginSessionLimitSectionProps {
  defaultValues: FormValues
}

/**
 * B: per-account login session limit (active sessions + issuance rate).
 * Defaults to disabled so normal users are never locked out; administrators
 * can opt in from Security settings. Writes the SessionLimitEnabled option.
 */
export function LoginSessionLimitSection({
  defaultValues,
}: LoginSessionLimitSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()

  const form = useForm<FormValues>({
    resolver: zodResolver(createSchema()),
    mode: 'onChange',
    defaultValues,
  })

  useEffect(() => {
    form.reset(defaultValues)
  }, [defaultValues, form])

  const onSubmit = async (values: FormValues) => {
    if (values.enabled !== defaultValues.enabled) {
      await updateOption.mutateAsync({
        key: 'SessionLimitEnabled',
        value: values.enabled,
      })
    }
  }

  return (
    <SettingsSection title={t('Login Session Limit')}>
      <p className='text-muted-foreground text-sm'>
        {t(
          'Login session limit description'
        )}
      </p>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending}
            saveLabel='Save login session limit'
          />
          <FormField
            control={form.control}
            name='enabled'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('Enable login session limit')}</FormLabel>
                  <FormDescription>
                    {t('Login session limit switch description')}
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