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
import { requireServerSuccess } from '@/lib/server-error-message'

// ============================================================================
// Channel Combo API
// ============================================================================

export type ComboStrategy = 'fallback' | 'round-robin' | 'weighted'

/** One candidate (channel + model) inside a combo's `models` JSON array. */
export type ComboModelItem = {
  channel_id: number
  model: string
  weight?: number
}

export type ChannelCombo = {
  id: number
  name: string
  strategy: ComboStrategy
  /** JSON array string, e.g. `[{"channel_id":1,"model":"gpt-4o","weight":1}]` */
  models: string
  status: number // 1=enabled, 2=disabled
  sticky: number
  created?: number
  updated?: number
}

export type ComboPayload = {
  id?: number
  name: string
  strategy: ComboStrategy
  models: string
  status: number
  sticky: number
}

type ComboListResponse = {
  success: boolean
  message?: string
  data?: {
    page: number
    page_size: number
    total: number
    items: ChannelCombo[]
  }
}

type ComboWriteResponse = {
  success: boolean
  message?: string
  data?: ChannelCombo
}

export const comboQueryKeys = {
  all: ['channel-combos'] as const,
  lists: () => [...comboQueryKeys.all, 'list'] as const,
}

/** Parse a combo's `models` JSON into candidate rows (safe on malformed JSON). */
export function parseComboModels(models: string): ComboModelItem[] {
  if (!models?.trim()) {return []}
  try {
    const parsed: unknown = JSON.parse(models)
    if (!Array.isArray(parsed)) {return []}
    const items: ComboModelItem[] = []
    for (const item of parsed) {
      if (!item || typeof item !== 'object') {continue}
      const candidate = item as Partial<ComboModelItem>
      const channelId = Number(candidate.channel_id)
      const model = typeof candidate.model === 'string' ? candidate.model : ''
      const weight = Number(candidate.weight)
      if (!Number.isFinite(channelId) || channelId <= 0 || !model.trim()) {
        continue
      }
      items.push({
        channel_id: channelId,
        model: model.trim(),
        weight: Number.isFinite(weight) && weight > 0 ? weight : undefined,
      })
    }
    return items
  } catch {
    return []
  }
}

/** Serialize candidate rows back into the `models` JSON string. */
export function stringifyComboModels(items: ComboModelItem[]): string {
  return JSON.stringify(
    items.map((item) => ({
      channel_id: item.channel_id,
      model: item.model,
      weight: item.weight,
    }))
  )
}

export async function getCombos(): Promise<ChannelCombo[]> {
  const response = await api.get<ComboListResponse>('/api/channel/combos')
  return requireServerSuccess(response.data).data?.items ?? []
}

export async function createCombo(
  payload: ComboPayload
): Promise<ChannelCombo> {
  const response = await api.post<ComboWriteResponse>(
    '/api/channel/combos',
    payload
  )
  const data = requireServerSuccess(response.data).data
  if (!data) {
    throw new Error('Combo create response is missing data')
  }
  return data
}

export async function updateCombo(
  payload: ComboPayload
): Promise<ChannelCombo> {
  const response = await api.put<ComboWriteResponse>(
    '/api/channel/combos',
    payload
  )
  const data = requireServerSuccess(response.data).data
  if (!data) {
    throw new Error('Combo update response is missing data')
  }
  return data
}

export async function deleteCombo(id: number): Promise<void> {
  const response = await api.delete<{ success: boolean; message?: string }>(
    `/api/channel/combos/${id}`
  )
  requireServerSuccess(response.data)
}
