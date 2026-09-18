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
import { getFriendlyErrorMessage } from '@/lib/server-error-message'

import { MESSAGE_STATUS } from '../../constants'
import type { Message } from '../../types'
import { getMessageContent } from './message-utils'

export const MODEL_PRICING_SETTINGS_PATH =
  '/system-settings/billing/model-pricing'

const MODEL_PRICE_ERROR_CODE = 'model_price_error'
export const FALLBACK_ERROR_CONTENT = 'An unknown error occurred'

type MessageErrorState = {
  content: string
  /** Raw/technical original when `content` was replaced with a friendly explainer. */
  detail?: string
  kind: 'generic' | 'model-price'
  showSettingsLink: boolean
}

export function isAdminRole(role?: number | null): boolean {
  return role != null && role >= 10
}

export function isErrorMessage(message: Message): boolean {
  return message.status === MESSAGE_STATUS.ERROR
}

/**
 * B2-3: derive what to show for an errored message.
 * Generic errors get the friendly human-readable explainer when the raw
 * technical text matches a known pattern; the original text is kept as
 * `detail` so callers can show both. Model-price errors keep their dedicated
 * admin-oriented UI untouched.
 */
export function getMessageErrorState(
  message: Message,
  isAdmin: boolean
): MessageErrorState | null {
  if (!isErrorMessage(message)) {
    return null
  }

  const isModelPriceError = message.errorCode === MODEL_PRICE_ERROR_CODE
  const rawContent = getMessageContent(message) || FALLBACK_ERROR_CONTENT
  let content = rawContent
  let detail: string | undefined

  if (!isModelPriceError) {
    const friendly = getFriendlyErrorMessage({ message: rawContent })
    if (friendly && friendly !== rawContent) {
      content = friendly
      detail = rawContent
    }
  }

  return {
    content,
    detail,
    kind: isModelPriceError ? 'model-price' : 'generic',
    showSettingsLink: isModelPriceError && isAdmin,
  }
}