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
import { Loader2, Pencil, Plus, Trash2 } from 'lucide-react'
import { useCallback, useEffect, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { handleServerError } from '@/lib/handle-server-error'

import {
  createWebhookEndpoint,
  deleteWebhookEndpoint,
  getWebhookEndpoints,
  updateWebhookEndpoint,
  WEBHOOK_EVENT_OPTIONS,
  type WebhookEndpoint,
} from './api'

interface EndpointFormState {
  id?: number
  name: string
  url: string
  secret: string
  enabled: boolean
  events: string[]
}

const EMPTY_FORM: EndpointFormState = {
  name: '',
  url: '',
  secret: '',
  enabled: true,
  events: [],
}

export function WebhookEndpointsPage() {
  const { t } = useTranslation()
  const [endpoints, setEndpoints] = useState<WebhookEndpoint[]>([])
  const [isLoading, setIsLoading] = useState(true)
  const [isSaving, setIsSaving] = useState(false)
  const [form, setForm] = useState<EndpointFormState | null>(null)
  const [pendingDelete, setPendingDelete] = useState<WebhookEndpoint | null>(null)

  const reload = useCallback(async () => {
    setIsLoading(true)
    try {
      const list = await getWebhookEndpoints()
      setEndpoints(Array.isArray(list) ? list : [])
    } catch (error) {
      handleServerError(error, t('Failed to load webhook endpoints'))
    } finally {
      setIsLoading(false)
    }
  }, [t])

  useEffect(() => {
    void reload()
  }, [reload])

  const openCreate = () => setForm({ ...EMPTY_FORM })
  const openEdit = (endpoint: WebhookEndpoint) =>
    setForm({
      id: endpoint.id,
      name: endpoint.name,
      url: endpoint.url,
      secret: '',
      enabled: endpoint.enabled,
      events: [...endpoint.events],
    })

  const handleSave = async () => {
    if (!form) {return}
    if (!form.name.trim() || !form.url.trim()) {
      toast.error(t('Name and URL are required'))
      return
    }
    if (!form.id && !form.secret.trim()) {
      toast.error(t('Secret is required'))
      return
    }
    setIsSaving(true)
    try {
      const payload = {
        name: form.name.trim(),
        url: form.url.trim(),
        secret: form.secret.trim() || undefined,
        enabled: form.enabled,
        events: form.events,
      }
      if (form.id) {
        await updateWebhookEndpoint(form.id, payload)
      } else {
        await createWebhookEndpoint(payload)
      }
      toast.success(t('Webhook endpoint saved'))
      setForm(null)
      await reload()
    } catch (error) {
      handleServerError(error, t('Failed to save webhook endpoint'))
    } finally {
      setIsSaving(false)
    }
  }

  const handleDelete = async () => {
    if (!pendingDelete) {return}
    try {
      await deleteWebhookEndpoint(pendingDelete.id)
      toast.success(t('Webhook endpoint deleted'))
      setPendingDelete(null)
      await reload()
    } catch (error) {
      handleServerError(error, t('Failed to delete webhook endpoint'))
    }
  }

  const toggleFormEvent = (value: string) => {
    setForm((prev) => {
      if (!prev) {return prev}
      const events = prev.events.includes(value)
        ? prev.events.filter((item) => item !== value)
        : [...prev.events, value]
      return { ...prev, events }
    })
  }

  let listBody: ReactNode
  if (isLoading) {
    listBody = (
      <div className='flex items-center gap-2 p-6 text-sm text-muted-foreground'>
        <Loader2 className='size-4 animate-spin' />
        {t('Loading...')}
      </div>
    )
  } else if (endpoints.length === 0) {
    listBody = (
      <div className='text-muted-foreground rounded-md border p-6 text-center text-sm'>
        {t('No endpoints configured yet.')}
      </div>
    )
  } else {
    listBody = (
      <div className='space-y-3'>
        {endpoints.map((endpoint) => (
          <Card key={endpoint.id}>
            <CardHeader className='flex-row items-start justify-between gap-4 space-y-0'>
              <div className='min-w-0 space-y-1'>
                <CardTitle className='flex items-center gap-2 text-sm'>
                  <span className='truncate'>{endpoint.name}</span>
                  <span
                    className={
                      endpoint.enabled
                        ? 'rounded-full bg-emerald-500/15 px-2 py-0.5 text-xs text-emerald-600'
                        : 'bg-muted text-muted-foreground rounded-full px-2 py-0.5 text-xs'
                    }
                  >
                    {endpoint.enabled ? t('Enabled') : t('Disabled')}
                  </span>
                </CardTitle>
                <p className='text-muted-foreground truncate font-mono text-xs'>
                  {endpoint.url}
                </p>
              </div>
              <div className='flex shrink-0 items-center gap-1'>
                <Button
                  variant='ghost'
                  size='icon'
                  onClick={() => openEdit(endpoint)}
                  aria-label={t('Edit')}
                >
                  <Pencil className='size-4' />
                </Button>
                <Button
                  variant='ghost'
                  size='icon'
                  onClick={() => setPendingDelete(endpoint)}
                  aria-label={t('Delete')}
                >
                  <Trash2 className='text-destructive size-4' />
                </Button>
              </div>
            </CardHeader>
            <CardContent>
              <div className='flex flex-wrap gap-2'>
                {endpoint.events.length === 0 ? (
                  <span className='text-muted-foreground text-xs'>
                    {t('No events subscribed')}
                  </span>
                ) : (
                  endpoint.events.map((event) => (
                    <span
                      key={event}
                      className='bg-muted rounded px-2 py-0.5 font-mono text-xs'
                    >
                      {event}
                    </span>
                  ))
                )}
              </div>
            </CardContent>
          </Card>
        ))}
      </div>
    )
  }

  return (
    <div className='space-y-4'>
      <div className='flex items-center justify-between gap-4'>
        <div className='space-y-1'>
          <h2 className='text-base font-semibold'>{t('Webhook Endpoints')}</h2>
          <p className='text-muted-foreground text-sm'>
            {t(
              'Deliver subscribed events to multiple endpoints, each with its own secret and event subscriptions. Deliveries are idempotent and retried with backoff.'
            )}
          </p>
        </div>
        <Button onClick={openCreate}>
          <Plus data-icon='inline-start' />
          {t('Add endpoint')}
        </Button>
      </div>

      {listBody}


      <Dialog
        open={form !== null}
        onOpenChange={(open) => {
          if (!open) {setForm(null)}
        }}
      >
        <DialogContent className='max-w-lg'>
          <DialogHeader>
            <DialogTitle>
              {form?.id ? t('Edit endpoint') : t('Add endpoint')}
            </DialogTitle>
          </DialogHeader>
          {form && (
            <div className='space-y-4'>
              <div className='space-y-1.5'>
                <Label htmlFor='endpoint-name'>{t('Name')}</Label>
                <Input
                  id='endpoint-name'
                  value={form.name}
                  onChange={(e) =>
                    setForm({ ...form, name: e.target.value })
                  }
                />
              </div>
              <div className='space-y-1.5'>
                <Label htmlFor='endpoint-url'>{t('Webhook URL')}</Label>
                <Input
                  id='endpoint-url'
                  value={form.url}
                  placeholder='https://example.com/hooks/new-api'
                  onChange={(e) => setForm({ ...form, url: e.target.value })}
                />
                <p className='text-muted-foreground text-xs'>
                  {t(
                    'HTTPS recommended; private/loopback targets are rejected by the SSRF guard.'
                  )}
                </p>
              </div>
              <div className='space-y-1.5'>
                <Label htmlFor='endpoint-secret'>{t('Webhook Secret')}</Label>
                <Input
                  id='endpoint-secret'
                  type='password'
                  value={form.secret}
                  placeholder={
                    form.id ? t('Leave blank to keep current secret') : ''
                  }
                  onChange={(e) =>
                    setForm({ ...form, secret: e.target.value })
                  }
                />
                <p className='text-muted-foreground text-xs'>
                  {t('Sent as the X-New-API-Webhook-Signature header.')}
                </p>
              </div>
              <div className='flex items-center justify-between gap-4'>
                <Label htmlFor='endpoint-enabled'>{t('Enabled')}</Label>
                <Switch
                  id='endpoint-enabled'
                  checked={form.enabled}
                  onCheckedChange={(checked) =>
                    setForm({ ...form, enabled: checked })
                  }
                />
              </div>
              <div className='space-y-2'>
                <Label>{t('Subscribed Events')}</Label>
                {WEBHOOK_EVENT_OPTIONS.map((option) => (
                  <label
                    key={option.value}
                    className='flex items-center gap-2 text-sm'
                  >
                    <Checkbox
                      checked={form.events.includes(option.value)}
                      onCheckedChange={() => toggleFormEvent(option.value)}
                    />
                    <span className='font-mono text-xs'>{option.value}</span>
                    <span className='text-muted-foreground text-xs'>
                      {t(option.label)}
                    </span>
                  </label>
                ))}
              </div>
            </div>
          )}
          <DialogFooter>
            <Button
              variant='outline'
              onClick={() => setForm(null)}
              disabled={isSaving}
            >
              {t('Cancel')}
            </Button>
            <Button onClick={handleSave} disabled={isSaving}>
              {isSaving && <Loader2 className='size-4 animate-spin' />}
              {t('Save')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={pendingDelete !== null}
        onOpenChange={(open) => {
          if (!open) {setPendingDelete(null)}
        }}
        title={t('Delete webhook endpoint')}
        desc={t(
          'This endpoint will stop receiving events. This action cannot be undone.'
        )}
        confirmText={t('Delete')}
        destructive
        handleConfirm={handleDelete}
      />
    </div>
  )
}
