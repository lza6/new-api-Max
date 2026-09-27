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
import { api } from '@/lib/api'

export interface WebhookSettings {
  enabled: boolean
  url: string
  secret: string
  events: string[]
}

export const WEBHOOK_EVENT_OPTIONS = [
  {
    value: 'epay.topup.success',
    label: 'Recharge (topup) success',
  },
  {
    value: 'epay.subscription.success',
    label: 'Subscription success',
  },
  {
    value: 'task.settled',
    label: 'Task settled',
  },
] as const

export function getWebhookSettings(): Promise<WebhookSettings> {
  return api.get('/api/admin/webhook/settings').then((r) => r.data.data)
}

export function updateWebhookSettings(patch: Partial<WebhookSettings>) {
  return api.put('/api/admin/webhook/settings', patch)
}