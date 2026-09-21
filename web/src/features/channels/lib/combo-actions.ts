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
import i18next from 'i18next'
import { toast } from 'sonner'

import { handleServerError } from '@/lib/handle-server-error'
import { createServerError } from '@/lib/server-error-message'

import {
  createCombo,
  deleteCombo,
  updateCombo,
  type ComboPayload,
} from '../lib/combo-api'

type SyncMutationResult = Promise<void>

export async function handleCreateCombo(payload: ComboPayload): Promise<string> {
  const created = await createCombo(payload)
  toast.success(i18next.t('Combo created successfully'))
  return created.name
}

export async function handleUpdateCombo(payload: ComboPayload): Promise<void> {
  await updateCombo(payload)
  toast.success(i18next.t('Combo updated successfully'))
}

export async function handleDeleteCombo(id: number): Promise<void> {
  await deleteCombo(id)
  toast.success(i18next.t('Combo deleted successfully'))
}

type ComboMutateContext = {
  queryClient: {
    invalidateQueries: (filters: { queryKey: unknown[] }) => void
  }
}

export async function syncComboMutation(
  ctx: ComboMutateContext,
  action: 'create' | 'update',
  payload: ComboPayload
): SyncMutationResult {
  try {
    if (action === 'create') {
      await handleCreateCombo(payload)
    } else {
      await handleUpdateCombo(payload)
    }
    ctx.queryClient.invalidateQueries({
      queryKey: ['channel-combos', 'list'],
    })
  } catch (error) {
    handleServerError(
      error,
      i18next.t(action === 'create' ? 'Failed to create combo' : 'Failed to update combo')
    )
    throw error
  }
}

export async function deleteComboWithToast(
  ctx: ComboMutateContext,
  id: number
): Promise<void> {
  try {
    await handleDeleteCombo(id)
    ctx.queryClient.invalidateQueries({ queryKey: ['channel-combos', 'list'] })
  } catch (error) {
    handleServerError(error, i18next.t('Failed to delete combo'))
    throw error
  }
}

/**
 * Validate a combo payload locally before submitting.
 * Returns a localized error message, or `null` when valid.
 */
export function validateComboPayload(
  payload: ComboPayload,
  t: (key: string) => string
): string | null {
  const name = payload.name.trim()
  if (!name) {return t('Combo name is required')}
  if (name.length > 64) {return t('Combo name must be 1-64 characters')}

  if (!['fallback', 'round-robin', 'weighted'].includes(payload.strategy)) {
    return t('Invalid combo strategy')
  }

  let items: Array<{
    channel_id: number
    model?: string
    weight?: number
  }> = []
  try {
    const parsed: unknown = JSON.parse(payload.models)
    if (Array.isArray(parsed)) {items = parsed as typeof items}
  } catch {
    return t('Combo candidates must be a valid JSON array')
  }

  if (items.length === 0) {
    return t('At least one combo candidate is required')
  }

  for (const item of items) {
    if (!item || typeof item !== 'object') {
      return t('Each combo candidate must be an object')
    }
    if (!Number.isInteger(item.channel_id) || item.channel_id <= 0) {
      return t('Each combo candidate must select a channel')
    }
    if (!item.model || !item.model.trim()) {
      return t('Each combo candidate must specify a model')
    }
  }

  if (payload.strategy === 'weighted') {
    const invalidWeight = items.some(
      (item) =>
        typeof item.weight === 'number' &&
        (!Number.isInteger(item.weight) || item.weight < 0)
    )
    if (invalidWeight) {
      return t('Combo weights must be non-negative integers')
    }
  }

  return null
}

export function makeCreateServerError(
  response: { success: boolean; message?: string },
  fallback: string
): Error {
  return createServerError(response, fallback)
}

// Re-export the payload type for consumers that want a single import surface.
export type { ComboPayload as ComboMutatePayload }