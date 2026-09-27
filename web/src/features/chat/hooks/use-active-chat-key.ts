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
import { useQuery } from '@tanstack/react-query'
import { t } from 'i18next'
import { useCallback, useEffect, useState } from 'react'

import { getApiKeys } from '@/features/keys/api'
import { API_KEY_STATUS } from '@/features/keys/constants'
import { useTokenKeyDisclosure } from '@/features/keys/hooks/use-token-key-disclosure'
import { createServerError } from '@/lib/server-error-message'
import { useAuthStore } from '@/stores/auth-store'

/**
 * G1/T8: locate the user's first enabled API key. It returns only the token id;
 * the plaintext key itself is disclosed through step-up verification.
 */
export async function getActiveChatTokenId(): Promise<number> {
  const result = await getApiKeys({ p: 1, size: 50 })
  if (!result.success) {
    throw createServerError(result, t('Failed to load API keys'))
  }

  const items = result.data?.items ?? []
  const active = items.find((item) => item.status === API_KEY_STATUS.ENABLED)
  if (!active) {
    throw new Error('No enabled API keys found. Create or enable one first.')
  }

  return active.id
}

/**
 * Get the currently active chat key for chat links. The plaintext key is only
 * disclosed after a step-up security verification (G1/T8); consumers render
 * `<SecureVerificationDialog {...verification.dialogProps} />`.
 *
 * 取消/失败验证不会造成「加载中」死循环：一旦 reveal 未成功即标记 revealFailed，
 * isPending 回到 false、isError 变为 true，消费方可调用 retry() 重新触发。
 */
export function useActiveChatKey(enabled: boolean) {
  const userId = useAuthStore((state) => state.auth.user?.id)
  const disclosure = useTokenKeyDisclosure()
  const [revealedKey, setRevealedKey] = useState<string | null>(null)
  const [revealFailed, setRevealFailed] = useState(false)

  const query = useQuery({
    queryKey: ['chat-active-key', userId],
    queryFn: getActiveChatTokenId,
    enabled: enabled && Boolean(userId),
    staleTime: 5 * 60 * 1000,
    gcTime: 10 * 60 * 1000,
  })

  const tokenId = query.data

  const retry = useCallback(() => {
    setRevealFailed(false)
  }, [])

  useEffect(() => {
    if (!enabled || !tokenId || revealedKey || revealFailed) {return}
    let cancelled = false
    void disclosure.revealSingleKey(tokenId).then((key) => {
      if (cancelled) {return}
      if (key) {
        setRevealedKey(key)
        setRevealFailed(false)
      } else {
        // 取消或失败：终止 pending，避免死循环；retry() 可重新拉起。
        setRevealFailed(true)
      }
    })
    return () => {
      cancelled = true
    }
  }, [enabled, tokenId, revealedKey, revealFailed, disclosure])

  const revealError = revealFailed
    ? new Error(t('Verification cancelled. Please try again.'))
    : null

  return {
    data: revealedKey ?? undefined,
    isPending:
      query.isPending ||
      (enabled && Boolean(tokenId) && !revealedKey && !revealFailed),
    isError: query.isError || query.error !== null || revealError !== null,
    error: revealError ?? query.error,
    retry,
    verification: disclosure.verification,
  }
}