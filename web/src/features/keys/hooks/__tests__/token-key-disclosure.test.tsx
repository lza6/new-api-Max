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
// 生产回归（2026-09-29）：用户查看**自己的**密钥默认不再要求 step-up。
// 后端归属校验 GetTokenByIds(id, userId) 才是安全边界；用户已通过 session 登录，
// 二次验证属重复校验（生产曾收到「复制密钥被要求验证」投诉）。
// 兼容：站点若把 require_verification_to_read_own_key 设为 true，服务端返回
// SECURITY_PROOF_* 时前端仍会弹出验证弹窗并重试一次。
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { afterEach, expect, it, vi } from 'vitest'

import { SecureVerificationDialog } from '@/features/auth/secure-verification'
import { api } from '@/lib/api'

import { useTokenKeyDisclosure } from '../use-token-key-disclosure'

function tokenVerification() {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: {
        scope: 'token.key.read',
        methods: [{ method: '2fa', available: true }],
        oauth_providers: [],
        password_encryption_enabled: false,
      },
    },
  })
  return {
    data: {
      success: true,
      data: {
        proof_token: 'token-key-proof',
        method: '2fa',
        scope: 'token.key.read',
        expires_at: Math.floor(Date.now() / 1000) + 60,
      },
    },
  }
}

function Harness() {
  const disclosure = useTokenKeyDisclosure()
  const [single, setSingle] = useState<string | null>(null)
  const [batch, setBatch] = useState<Record<number, string>>({})
  return (
    <>
      <button
        type='button'
        onClick={() => void disclosure.revealSingleKey(7).then(setSingle)}
      >
        Reveal Single
      </button>
      <button
        type='button'
        onClick={() => void disclosure.revealKeysBatch([7, 8]).then(setBatch)}
      >
        Reveal Batch
      </button>
      <output aria-label='Single key'>{single ?? 'Hidden'}</output>
      <output aria-label='Batch keys'>
        {Object.entries(batch)
          .map(([id, k]) => `${id}:${k}`)
          .join(',') || 'Hidden'}
      </output>
      <SecureVerificationDialog {...disclosure.verification.dialogProps} />
    </>
  )
}

afterEach(() => vi.restoreAllMocks())

async function completeVerification() {
  const user = userEvent.setup()
  await user.type(
    await screen.findByLabelText('Authenticator code or backup code'),
    '123456'
  )
  await user.click(screen.getByRole('button', { name: 'Verify' }))
}

/** 构造一个「服务端要求二次验证」的 403 错误，模拟站点把开关设为 true。 */
function proofRequiredError() {
  return Object.assign(new Error('需要安全验证'), {
    response: { status: 403, data: { code: 'SECURITY_PROOF_REQUIRED' } },
  })
}

it('reveals a single own API key without any step-up verification (default)', async () => {
  const post = vi.spyOn(api, 'post').mockImplementation(async (url) => {
    if (url === '/api/token/7/key') {
      return {
        data: { success: true, data: { key: 'fake-key-for-test-only' } },
      }
    }
    throw new Error(`Unexpected POST ${url}`)
  })
  render(<Harness />)
  await userEvent.click(screen.getByRole('button', { name: 'Reveal Single' }))
  await waitFor(() =>
    expect(screen.getByLabelText('Single key')).toHaveTextContent(
      'sk-fake-key-for-test-only'
    )
  )
  // 不得调用 /api/verify（证明没有弹二次验证）
  expect(post.mock.calls.map(([url]) => url)).toEqual(['/api/token/7/key'])
  const [, , config] = post.mock.calls[0] as unknown as [
    string,
    unknown,
    { headers?: Record<string, string> },
  ]
  expect(config?.headers?.['X-Security-Proof']).toBeUndefined()
})

it('reveals a batch of own API keys without step-up (default)', async () => {
  const post = vi.spyOn(api, 'post').mockImplementation(async (url) => {
    if (url === '/api/token/batch/keys') {
      return {
        data: { success: true, data: { keys: { 7: 'fake-key-7', 8: 'fake-key-8' } } },
      }
    }
    throw new Error(`Unexpected POST ${url}`)
  })
  render(<Harness />)
  await userEvent.click(screen.getByRole('button', { name: 'Reveal Batch' }))
  await waitFor(() =>
    expect(screen.getByLabelText('Batch keys')).toHaveTextContent(
      '7:sk-fake-key-7,8:sk-fake-key-8'
    )
  )
  expect(post.mock.calls.map(([url]) => url)).toEqual([
    '/api/token/batch/keys',
  ])
})

it('falls back to step-up verification when the server still requires it', async () => {
  const proof = tokenVerification()
  let keyAttempts = 0
  const post = vi.spyOn(api, 'post').mockImplementation(async (url) => {
    if (url === '/api/verify') {return Promise.resolve(proof)}
    if (url === '/api/token/7/key') {
      keyAttempts += 1
      if (keyAttempts === 1) {throw proofRequiredError()}
      return {
        data: { success: true, data: { key: 'fake-key-for-test-only' } },
      }
    }
    throw new Error(`Unexpected POST ${url}`)
  })
  render(<Harness />)
  await userEvent.click(screen.getByRole('button', { name: 'Reveal Single' }))
  await completeVerification()
  await waitFor(() =>
    expect(screen.getByLabelText('Single key')).toHaveTextContent(
      'sk-fake-key-for-test-only'
    )
  )
  // 第二次带 proof 重试
  const retry = post.mock.calls.filter(([url]) => url === '/api/token/7/key')[1]
  const [, , config] = retry as unknown as [
    string,
    unknown,
    { headers?: Record<string, string> },
  ]
  expect(config?.headers?.['X-Security-Proof']).toBe('token-key-proof')
})

it('cancelling the fallback verification never reveals the key', async () => {
  tokenVerification()
  vi.spyOn(api, 'post').mockImplementation(async (url) => {
    if (url === '/api/token/7/key') {throw proofRequiredError()}
    if (url === '/api/verify') {return {data: {success: true, data: {}}}}
    throw new Error(`Unexpected POST ${url}`)
  })
  render(<Harness />)
  await userEvent.click(screen.getByRole('button', { name: 'Reveal Single' }))
  await userEvent.click(await screen.findByRole('button', { name: 'Cancel' }))
  await waitFor(() =>
    expect(screen.getByLabelText('Single key')).toHaveTextContent('Hidden')
  )
})
