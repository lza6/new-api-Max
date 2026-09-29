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
 * API key plaintext disclosure.
 *
 * 默认（站点配置 require_verification_to_read_own_key=false）用户查看**自己的**
 * 密钥不再要求 step-up：用户已通过 session 登录，后端 GetTokenByIds(id, userId)
 * 已保证归属，二次验证属重复校验，徒增操作步骤。
 *
 * 兼容策略：先不带 proof 直接请求；仅当服务端返回 SECURITY_PROOF_REQUIRED 类
 * 错误时，才弹出共享验证弹窗并用一次性 proof 重试。这样无论站点开关如何配置
 * （含管理员改为强制验证），前端都能正确工作，无需额外配置端点。
 */

/** 服务端要求二次验证时返回的错误码前缀。 */
function isProofRequiredError(error: unknown): boolean {
  const message = readServerCode(error)
  if (!message) {return false}
  return (
    message === 'SECURITY_PROOF_REQUIRED' ||
    message === 'SECURITY_PROOF_EXPIRED' ||
    message === 'SECURITY_PROOF_INVALID' ||
    message === 'SECURITY_PROOF_CONSUMED' ||
    message === 'SECURITY_PROOF_CONTEXT_MISMATCH' ||
    message === 'SECURITY_PROOF_SCOPE_MISMATCH' ||
    message === 'SECURITY_PROOF_METHOD_MISMATCH'
  )
}

/**
 * 从各类错误形态里取出**服务端业务 code**。
 *
 * 注意：必须先读 `response.data.code`，不能先读顶层 `error.code` ——
 * axios 对所有 4xx/5xx 都会把 `error.code` 设成 `ERR_BAD_REQUEST` /
 * `ERR_BAD_RESPONSE` 这类传输层标识，业务 code 只存在于 `response.data.code`。
 * 顺序写反会让 SECURITY_PROOF_* 永远读不到，回退弹验证的分支变成死代码。
 */
function readServerCode(error: unknown): string | null {
  if (!error || typeof error !== 'object') {return null}
  const record = error as Record<string, unknown>
  // 1) 服务端业务 code（最可靠）
  const response = record.response as Record<string, unknown> | undefined
  const data = response?.data as Record<string, unknown> | undefined
  if (typeof data?.code === 'string' && data.code) {return data.code}
  // 2) 顶层 code：仅当它不是 axios 的传输层前缀时才采用
  const direct = record.code
  if (typeof direct === 'string' && direct && !direct.startsWith('ERR_')) {
    return direct
  }
  return null
}

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
        let res: Awaited<ReturnType<typeof fetchTokenKey>>
        try {
          // 先不带 proof：站点默认不再要求二次验证。
          res = await fetchTokenKey(tokenId)
        } catch (error) {
          if (!isProofRequiredError(error)) {throw error}
          // 站点仍强制验证：弹出验证弹窗并重试一次。
          const proof = await requestVerification({
            scope: 'token.key.read',
            context: { token_id: tokenId },
            title: t('Verify to view API key'),
            description: t(
              'Confirm your identity before revealing this API key.'
            ),
          })
          if (!proof || operation.current !== current) {return null}
          res = await fetchTokenKey(tokenId, proof.proof_token)
        }
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
        let res: Awaited<ReturnType<typeof fetchTokenKeysBatch>>
        try {
          res = await fetchTokenKeysBatch(tokenIds)
        } catch (error) {
          if (!isProofRequiredError(error)) {throw error}
          const proof = await requestVerification({
            scope: 'token.key.read',
            context: { token_ids: tokenIds },
            title: t('Verify to view API keys'),
            description: t(
              'Confirm your identity before revealing these API keys.'
            ),
          })
          if (!proof || operation.current !== current) {return {}}
          res = await fetchTokenKeysBatch(tokenIds, proof.proof_token)
        }
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
