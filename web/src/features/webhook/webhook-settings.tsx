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
import { Loader2, Save } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { handleServerError } from '@/lib/handle-server-error'

import {
  getWebhookSettings,
  updateWebhookSettings,
  WEBHOOK_EVENT_OPTIONS,
  type WebhookSettings,
} from './api'
import { WebhookEndpointsPage } from './webhook-endpoints'

export function WebhookSettingsPage() {
  const { t } = useTranslation()
  const [settings, setSettings] = useState<WebhookSettings | null>(null)
  const [isLoading, setIsLoading] = useState(true)
  const [isSaving, setIsSaving] = useState(false)

  useEffect(() => {
    let cancelled = false
    getWebhookSettings()
      .then((data) => {
        if (!cancelled) {setSettings(data)}
      })
      .catch((error) => {
        if (!cancelled) {
          handleServerError(error, t('Failed to load webhook settings'))
        }
      })
      .finally(() => {
        if (!cancelled) {setIsLoading(false)}
      })
    return () => {
      cancelled = true
    }
  }, [t])

  const toggleEvent = (value: string) => {
    if (!settings) {return}
    const events = settings.events?.includes(value)
      ? settings.events.filter((item) => item !== value)
      : [...(settings.events ?? []), value]
    setSettings({ ...settings, events })
  }

  const handleSave = async () => {
    if (!settings) {return}
    if (settings.enabled && (!settings.url || !settings.secret)) {
      toast.error(t('URL and secret are required when webhook is enabled'))
      return
    }
    setIsSaving(true)
    try {
      await updateWebhookSettings(settings)
      toast.success(t('Webhook settings saved'))
    } catch (error) {
      handleServerError(error, t('Failed to save webhook settings'))
    } finally {
      setIsSaving(false)
    }
  }

  if (isLoading) {
    return (
      <div className='flex items-center gap-2 p-6 text-sm text-muted-foreground'>
        <Loader2 className='size-4 animate-spin' />
        {t('Loading...')}
      </div>
    )
  }

  if (!settings) {
    return <div className='p-6 text-sm text-muted-foreground'>{t('No settings available')}</div>
  }

  return (
    <div className='mx-auto w-full max-w-3xl space-y-4 p-4 sm:p-6'>
      <h1 className='text-xl font-semibold'>{t('Webhook Notifications')}</h1>
      <p className='text-muted-foreground text-sm'>
        {t(
          'Forward selected business events to an HTTPS endpoint with an HMAC-SHA256 signature header. Each event is sent at most once per minute (idempotent by event id).'
        )}
      </p>

      <Card>
        <CardHeader>
          <CardTitle className='text-base'>{t('Delivery')}</CardTitle>
        </CardHeader>
        <CardContent className='space-y-4'>
          <div className='flex items-center justify-between gap-4'>
            <div className='space-y-1'>
              <Label htmlFor='webhook-enabled'>{t('Enabled')}</Label>
              <p className='text-muted-foreground text-xs'>
                {t('Disabled by default; no outbound request is made while off.')}
              </p>
            </div>
            <Switch
              id='webhook-enabled'
              checked={settings.enabled}
              onCheckedChange={(checked) =>
                setSettings({ ...settings, enabled: checked })
              }
            />
          </div>

          <div className='space-y-1.5'>
            <Label htmlFor='webhook-url'>{t('Webhook URL')}</Label>
            <Input
              id='webhook-url'
              value={settings.url}
              placeholder='https://example.com/hooks/new-api'
              onChange={(event) =>
                setSettings({ ...settings, url: event.target.value })
              }
            />
            <p className='text-muted-foreground text-xs'>
              {t(
                'HTTPS recommended; private/loopback targets are rejected by the SSRF guard.'
              )}
            </p>
          </div>

          <div className='space-y-1.5'>
            <Label htmlFor='webhook-secret'>{t('Webhook Secret')}</Label>
            <Input
              id='webhook-secret'
              type='password'
              value={settings.secret}
              onChange={(event) =>
                setSettings({ ...settings, secret: event.target.value })
              }
            />
            <p className='text-muted-foreground text-xs'>
              {t('Sent as the X-New-API-Webhook-Signature header.')}
            </p>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className='text-base'>{t('Subscribed Events')}</CardTitle>
        </CardHeader>
        <CardContent className='space-y-2'>
          {WEBHOOK_EVENT_OPTIONS.map((option) => (
            <label
              key={option.value}
              className='flex items-center gap-2 text-sm'
            >
              <Checkbox
                checked={settings.events?.includes(option.value) ?? false}
                onCheckedChange={() => toggleEvent(option.value)}
              />
              <span className='font-mono text-xs'>{option.value}</span>
              <span className='text-muted-foreground text-xs'>
                {t(option.label)}
              </span>
            </label>
          ))}
        </CardContent>
      </Card>

      <div className='flex justify-end'>
        <Button onClick={handleSave} disabled={isSaving}>
          <Save data-icon='inline-start' />
          {isSaving ? t('Saving...') : t('Save')}
        </Button>
      </div>

      <WebhookEndpointsPage />
    </div>
  )
}