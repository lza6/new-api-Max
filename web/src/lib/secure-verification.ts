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
import axios from 'axios'

import {
  getServerErrorMessageKey,
  safeServerErrorMessage,
} from './server-error-message'

export class AuthOperationError extends Error {
  readonly [safeServerErrorMessage] = true
  constructor(
    message: string,
    readonly code?: string,
    options?: ErrorOptions
  ) {
    super(message, options)
    this.name = 'AuthOperationError'
  }

  static from(
    error: unknown,
    fallback = 'Verification failed. Please try again.'
  ): AuthOperationError {
    if (error instanceof AuthOperationError) {return error}
    if (axios.isAxiosError<{ message?: string; code?: string }>(error)) {
      return new AuthOperationError(
        getServerErrorMessageKey(error) ||
          (error.response && error.response.status >= 500
            ? 'Please try again later.'
            : undefined) ||
          error.response?.data?.message ||
          error.message ||
          fallback,
        error.response?.data?.code,
        { cause: error }
      )
    }
    return new AuthOperationError(
      error instanceof Error ? error.message : fallback,
      undefined,
      { cause: error }
    )
  }
}

export const authRequestOptions = {
  skipBusinessError: true,
  skipErrorHandler: true,
}

/**
 * Server business code that means "a step-up proof is required" (or a supplied
 * one was rejected). Shared by the token and channel key-disclosure flows so
 * both can use the same compatible pattern: try without a proof first, and only
 * prompt for verification when the server actually asks for it.
 */
const PROOF_REQUIRED_CODES = new Set([
  'SECURITY_PROOF_REQUIRED',
  'SECURITY_PROOF_EXPIRED',
  'SECURITY_PROOF_INVALID',
  'SECURITY_PROOF_CONSUMED',
  'SECURITY_PROOF_CONTEXT_MISMATCH',
  'SECURITY_PROOF_SCOPE_MISMATCH',
  'SECURITY_PROOF_METHOD_MISMATCH',
])

/**
 * Read the server business `code` from an error.
 *
 * Order matters: axios sets `error.code` to a transport constant
 * (`ERR_BAD_REQUEST` / `ERR_BAD_RESPONSE`) for every 4xx/5xx, so the business
 * code must be read from `response.data.code` first; the top-level `code` is
 * only trusted when it is not an `ERR_*` transport prefix.
 */
export function readServerCode(error: unknown): string | null {
  if (!error || typeof error !== 'object') {return null}
  const record = error as Record<string, unknown>
  const response = record.response as Record<string, unknown> | undefined
  const data = response?.data as Record<string, unknown> | undefined
  if (typeof data?.code === 'string' && data.code) {return data.code}
  const direct = record.code
  if (typeof direct === 'string' && direct && !direct.startsWith('ERR_')) {
    return direct
  }
  return null
}

/** Whether an error is the server asking for a step-up proof. */
export function isProofRequiredError(error: unknown): boolean {
  const code = readServerCode(error)
  return code != null && PROOF_REQUIRED_CODES.has(code)
}

export async function authResult<T>(
  request: Promise<{
    data: { success: boolean; message?: string; code?: string; data?: T }
  }>,
  fallback = 'Verification failed. Please try again.'
): Promise<T> {
  try {
    const { data: response } = await request
    if (!response.success || response.data === undefined) {
      throw new AuthOperationError(
        getServerErrorMessageKey(response) || response.message || fallback,
        response.code
      )
    }
    return response.data
  } catch (error) {
    throw AuthOperationError.from(error, fallback)
  }
}
