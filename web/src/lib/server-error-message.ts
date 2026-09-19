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
import { isCancelledError } from '@tanstack/react-query'
import { isCancel } from 'axios'
import i18next from 'i18next'

export const safeServerErrorMessage = Symbol('safeServerErrorMessage')

const serverErrorMessageKeys = {
  TELEGRAM_OAUTH_NOT_CONFIGURED:
    'Telegram OAuth is not configured or enabled. Please contact your administrator.',
  TELEGRAM_OAUTH_CONFLICT:
    'The telegram OAuth provider name is reserved. Ask your administrator to rename the conflicting custom provider.',
  TELEGRAM_OAUTH_FAILED: 'Telegram authorization failed. Please try again.',
  TELEGRAM_ACCOUNT_NOT_BOUND:
    'This Telegram account is not linked. Sign in using another method and link it first.',
  TELEGRAM_LEGACY_AUTH_REMOVED:
    'Telegram login has changed. Reload the page and start Telegram OAuth again.',
  AUTH_INTERNAL_ERROR: 'Please try again later.',
  SECURITY_VERIFICATION_FAILED: 'Verification failed. Please try again.',
  SECURITY_VERIFICATION_FLOW_REQUIRED:
    'This verification method requires its dedicated verification flow.',
  SECURITY_VERIFICATION_LOCKED:
    'Two-factor authentication is temporarily locked.',
  OAUTH_ACCOUNT_MISMATCH:
    'The OAuth account does not match the account linked to your profile.',
  TWOFA_CODE_INVALID: 'The authenticator code is incorrect.',
  TWOFA_ALREADY_ENABLED: 'Two-factor authentication is already enabled.',
  TWOFA_NOT_ENABLED: 'Two-factor authentication is not enabled.',
  PASSKEY_NOT_FOUND: 'No Passkey is registered.',
  AUTH_FLOW_INVALID: 'Verification flow expired',
  SECURITY_PROOF_REQUIRED: 'Additional verification required',
  SECURITY_PROOF_EXPIRED:
    'Security verification has expired. Please verify again.',
  SECURITY_PROOF_INVALID:
    'Security verification is no longer valid. Please verify again.',
  SECURITY_PROOF_SCOPE_MISMATCH: 'Verification does not match this action.',
  SECURITY_PROOF_CONSUMED:
    'This verification has already been used. Please verify again.',
  SECURITY_PROOF_CONTEXT_MISMATCH:
    "Verification does not match this action's details. Please verify again.",
  SECURITY_CONTEXT_INVALID: 'The action details are invalid.',
  SECURITY_ACTION_FORBIDDEN:
    'You do not have permission to perform this action.',
  SECURITY_PROOF_METHOD_MISMATCH:
    'This verification method is not allowed for this action.',
  SECURITY_METHOD_UNAVAILABLE:
    'This verification method is currently unavailable.',
  TWOFA_SETUP_INVALID:
    'The two-factor setup has expired or changed. Start setup again.',

  AUTH_SESSION_LIMIT:
    'Too many active login sessions. On a device where you are already signed in, open Login sessions and use “Sign out other sessions” to revoke them. If you cannot access a signed-in device, reset your password to sign out all sessions.',
  AUTH_SESSION_ISSUANCE_LIMIT:
    'Too many login sessions were created recently. Please wait for the rolling window to pass, then try again.',
  TELEGRAM_BIND_DISABLED: 'Telegram binding is disabled.',
  TELEGRAM_BIND_INVALID_REQUEST:
    'The Telegram authorization request is invalid or expired.',
  TELEGRAM_BIND_FLOW_INVALID:
    'This Telegram binding request has expired or has already been used.',
  TELEGRAM_BIND_SESSION_INVALID:
    'The login session that started this Telegram binding is no longer valid.',
  TELEGRAM_BIND_ALREADY_BOUND: 'This Telegram account is already bound.',
  TELEGRAM_BIND_USER_DELETED: 'This user account no longer exists.',
  TELEGRAM_BIND_USER_DISABLED: 'This user account is disabled.',
  TELEGRAM_BIND_INTERNAL_ERROR: 'Telegram binding failed. Please try again.',
} as const

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === 'object'
}

/** Walk only error origins, never request configs or arbitrary response data. */
export function getServerErrorSources(
  value: unknown
): Record<string | symbol, unknown>[] {
  const sources: Record<string | symbol, unknown>[] = []
  const pending = [value]
  const seen = new Set<object>()
  for (let index = 0; index < pending.length; index++) {
    const source = pending[index]
    if (!isRecord(source) || seen.has(source)) continue
    seen.add(source)
    sources.push(source)
    if (isRecord(source.response)) pending.push(source.response.data)
    pending.push(source.cause, source.error)
  }
  return sources
}

export function getServerErrorMessageKey(value: unknown): string | null {
  for (const source of getServerErrorSources(value)) {
    if (typeof source.code === 'string') {
      const key =
        serverErrorMessageKeys[
          source.code as keyof typeof serverErrorMessageKeys
        ]
      if (key) return key
    }
    if (source[safeServerErrorMessage]) break
  }
  return null
}

export function getServerErrorStatus(value: unknown): number | undefined {
  for (const source of getServerErrorSources(value)) {
    const status = isRecord(source.response)
      ? source.response.status
      : source.status
    if (typeof status === 'number') return status
  }
  return undefined
}

export function isServerErrorCancelled(value: unknown): boolean {
  return getServerErrorSources(value).some(
    (source) =>
      isCancel(source) ||
      isCancelledError(source) ||
      source.name === 'AbortError'
  )
}

function messageText(value: unknown): string | undefined {
  if (
    typeof value !== 'string' ||
    !value.trim() ||
    value.trim() === 'success'
  ) {
    return undefined
  }
  // A proxy's HTML error document is not an actionable API error message.
  if (/^\s*(?:<!doctype|<html[\s>])/i.test(value)) return undefined
  return value
}

// B6-2/B2-3 错误码→人话映射：按序匹配，命中即用友好文案替换技术性消息首行。
// 顺序即优先级：额度 → 密钥 → 限流 → 渠道冷却 → IP 封禁 → 模型不可用 → 上游 → 内容安全。
const FRIENDLY_ERROR_PATTERNS: Array<{ pattern: RegExp; messageKey: string }> = [
  { pattern: /insufficient[_ ]?(quota|balance|credit)|quota/i, messageKey: 'Insufficient quota. Please top up or redeem a quota card.' },
  { pattern: /key[_ ]?invalid|invalid[_ ]?api[_ ]?key|\b401\b/i, messageKey: 'The key is invalid or expired. Please rotate it on the channels page.' },
  { pattern: /rate[_ ]?limit|\b429\b/i, messageKey: 'Too many requests. Please try again later.' },
  // B2-3 渠道冷却：上游/渠道进入冷却窗口（cooldown / cooling down）。
  { pattern: /cooldown|cooling[_ ]?down|is cooling/i, messageKey: 'This channel is cooling down after recent failures. Please try again in a moment.' },
  // G1 无可用渠道：冷却/无渠道的真实出口（get_channel_failed / channel_no_available_key / 可用渠道不存在）。
  { pattern: /get[_ ]?channel[_ ]?failed|channel[_ ]?no[_ ]?available[_ ]?key|可用渠道不存在/i, messageKey: 'No available channel for this model right now. Please try again in a moment or choose another model.' },
  // G2 channel 域保留码（后端不改写，如 channel:invalid_key / channel:no_available_key）。
  { pattern: /channel\s*:/i, messageKey: 'The channel connection has an issue. Contact the administrator or try again later.' },
  // G3 内部技术码：计费/传输类失败，映射为对应人话。
  { pattern: /pre[_ ]?consume[_ ]?token[_ ]?quota[_ ]?failed/i, messageKey: 'Insufficient quota. Please top up or redeem a quota card.' },
  // B2-3 IP 封禁：Web 防护自动/手动封禁（type=ip_banned）。
  { pattern: /ip[_ ]?banned|ip banned|banned[_ ]?ip/i, messageKey: 'Your IP address is temporarily banned due to unusual traffic. Please try again later.' },
  // B2-3 模型暂不可用：模型未找到 / 无可用模型。
  { pattern: /model[_ ]?not[_ ]?found|no.*model|model.*unavailable|model.*not.*available/i, messageKey: 'The requested model is not available. Please pick another model.' },
  { pattern: /upstream|bad[_ ]?gateway|\b502\b|\b503\b/i, messageKey: 'The upstream service is temporarily unavailable. Please try again later.' },
  { pattern: /content[_ ]?(filter|policy)|safety|moderation|prompt[_ ]?block/i, messageKey: 'The content was blocked by a safety policy.' },
]

/**
 * B6-2：把技术性错误文本/状态码映射为面向用户的人话（已翻译）。
 * 未命中返回 null，调用方保持原有文案行为。
 * 带 safeServerErrorMessage 标记的错误（认证安全消息）不走此映射，
 * 避免用隐藏的技术细节覆盖专用安全文案。
 */
export function getFriendlyErrorMessage(value: unknown): string | null {
  const sources = getServerErrorSources(value)
  if (sources.some((source) => source[safeServerErrorMessage])) return null
  const haystacks: string[] = []
  for (const source of sources) {
    const code = typeof source.code === 'string' ? source.code : undefined
    if (code) haystacks.push(code)
    if (typeof source.message === 'string') haystacks.push(source.message)
    if (isRecord(source.response)) {
      const data = source.response.data
      if (typeof data === 'string') haystacks.push(data)
      else if (isRecord(data)) {
        if (typeof data.message === 'string') haystacks.push(data.message)
        if (typeof data.code === 'string') haystacks.push(data.code)
        // B6-2：后端 OpenAI 信封为 {error:{message,type,code}}，type 是
        // 稳定机器可读枚举（insufficient_quota/key_invalid/rate_limited/
        // upstream_unavailable/content_filtered），优先读它作为映射来源。
        if (isRecord(data.error)) {
          if (typeof data.error.type === 'string') {
            haystacks.push(data.error.type)
          }
          if (typeof data.error.message === 'string') {
            haystacks.push(data.error.message)
          }
          if (typeof data.error.code === 'string') {
            haystacks.push(data.error.code)
          }
        }
      }
    }
  }
  const haystack = haystacks.join('\n')
  if (!haystack) return null
  for (const { pattern, messageKey } of FRIENDLY_ERROR_PATTERNS) {
    if (pattern.test(haystack)) return i18next.t(messageKey)
  }
  return null
}

export function getServerErrorMessage(
  value: unknown,
  fallback?: string
): string {
  const sources = getServerErrorSources(value)
  // AuthOperationError deliberately hides internal server details. Never unwrap
  // its cause for presentation, even when a caller wraps it in another Error.
  const safe = sources.find((source) => source[safeServerErrorMessage])
  if (safe) {
    return i18next.t(
      getServerErrorMessageKey(safe) ||
        messageText(safe.message) ||
        fallback ||
        'Something went wrong!'
    )
  }
  const key = getServerErrorMessageKey(value)
  if (key) return i18next.t(key)
  // B6-2：命中人话映射时优先使用友好文案（未知错误保持原有行为）。
  const friendly = getFriendlyErrorMessage(value)
  if (friendly) return friendly
  for (const source of sources) {
    if (
      source instanceof Error ||
      source.isAxiosError ||
      isRecord(source.response)
    ) {
      continue
    }
    const detail = isRecord(source.error)
      ? messageText(source.error.message)
      : messageText(source.error)
    const message =
      messageText(source.message) || detail || messageText(source.title)
    if (message) return message
  }
  const status = getServerErrorStatus(value)
  if (status === 304) return i18next.t('Content not modified!')
  if (status === 204) return i18next.t('Content not found.')
  for (const source of sources) {
    const message = messageText(source.message)
    if (message) return message
  }
  return messageText(value) || (fallback ?? i18next.t('Something went wrong!'))
}

/** Preserve the payload/cause so every layer can recognize the same failure. */
export function createServerError(value: unknown, fallback?: string): Error {
  return new Error(getServerErrorMessage(value, fallback ?? ''), {
    cause: value,
  })
}

/** Queries reject failed business responses without changing the raw API contract. */
export function requireServerSuccess<T>(response: T): T {
  if (isRecord(response) && response.success === false) {
    throw createServerError(response)
  }
  return response
}
