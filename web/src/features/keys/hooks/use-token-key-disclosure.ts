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
import { useCallback, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { useSecureVerification } from '@/features/auth/secure-verification'
import { handleServerError } from '@/lib/handle-server-error'
import { AuthOperationError } from '@/lib/secure-verification'
import { createServerError } from '@/lib/server-error-message'

import { fetchTokenKey, fetchTokenKeysBatch } from '../api'

/**
 * G1/T8: API key plaintext disclosure requires a step-up security proof.
 * Each reveal request first runs the shared secure-verification dialog
 * (passkey / 2FA / password), then calls the protected endpoint with the
 * single-use proof. A cancelled or failed verification yields no key.
 */
export function useTokenKeyDisclosure() {
  const { t } = useTranslation()
  const verification = useSecureVerification()
  const requestVerification = verification.requestVerification
  const operation = useRef<AbortController | null>(null)

  const revealSingleKey = useCallback(
    async (tokenId: number): Promise<string | null> => {
      if (operation.current) {return null}
      const current = new AbortController()
      operation.current = current
      try {
        const proof = await requestVerification({
          scope: 'token.key.read',
          context: { token_id: tokenId },
          title: t('Verify to view API key'),
          description: t(
            'Confirm your identity before revealing this API key.'
          ),
        })
        if (!proof || operation.current !== current) {return null}
        const res = await fetchTokenKey(tokenId, proof.proof_token)
        if (operation.current !== current) {return null}
        if (!res.success) {
          throw createServerError(res, t('Failed to fetch API key'))
        }
        const fullKey = res.data?.key ? `sk-${res.data.key}` : ''
        if (fullKey) {
          toast.success(t('API key unlocked'))
        }
        return fullKey || null
      } catch (error) {
        if (operation.current === current) {
          handleServerError(AuthOperationError.from(error))
        }
        return null
      } finally {
        if (operation.current === current) {
          operation.current = null
        }
      }
    },
    [requestVerification, t]
  )

  const revealKeysBatch = useCallback(
    async (tokenIds: number[]): Promise<Record<number, string>> => {
      if (operation.current || tokenIds.length === 0) {return {}}
      const current = new AbortController()
      operation.current = current
      try {
        const proof = await requestVerification({
          scope: 'token.key.read',
          context: { token_ids: tokenIds },
          title: t('Verify to view API keys'),
          description: t(
            'Confirm your identity before revealing these API keys.'
          ),
        })
        if (!proof || operation.current !== current) {return {}}
        const res = await fetchTokenKeysBatch(tokenIds, proof.proof_token)
        if (operation.current !== current) {return {}}
        if (!res.success) {
          throw createServerError(res, t('Failed to fetch API keys'))
        }
        const keys: Record<number, string> = {}
        for (const [idStr, key] of Object.entries(res.data?.keys ?? {})) {
          keys[Number(idStr)] = `sk-${key}`
        }
        if (Object.keys(keys).length > 0) {
          toast.success(t('API keys unlocked'))
        }
        return keys
      } catch (error) {
        if (operation.current === current) {
          handleServerError(AuthOperationError.from(error))
        }
        return {}
      } finally {
        if (operation.current === current) {
          operation.current = null
        }
      }
    },
    [requestVerification, t]
  )

  return { revealSingleKey, revealKeysBatch, verification }
}
