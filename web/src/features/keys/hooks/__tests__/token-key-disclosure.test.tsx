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

it('reveals a single API key only after a step-up proof', async () => {
  const proof = tokenVerification()
  const post = vi.spyOn(api, 'post').mockImplementation(async (url) => {
    if (url === '/api/verify') {return Promise.resolve(proof)}
    if (url === '/api/token/7/key') {
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
  const [, , config] = post.mock.calls.find(
    ([url]) => url === '/api/token/7/key'
  ) as unknown as [string, unknown, { headers?: Record<string, string> }]
  expect(config?.headers?.['X-Security-Proof']).toBe('token-key-proof')
})

it('reveals a batch of API keys only after a step-up proof bound to the id set', async () => {
  const proof = tokenVerification()
  const post = vi.spyOn(api, 'post').mockImplementation(async (url) => {
    if (url === '/api/verify') {return Promise.resolve(proof)}
    if (url === '/api/token/batch/keys') {
      return {
        data: {
          success: true,
          data: {
            keys: { 7: 'fake-key-7', 8: 'fake-key-8' },
          },
        },
      }
    }
    throw new Error(`Unexpected POST ${url}`)
  })
  render(<Harness />)
  await userEvent.click(screen.getByRole('button', { name: 'Reveal Batch' }))
  await completeVerification()
  await waitFor(() =>
    expect(screen.getByLabelText('Batch keys')).toHaveTextContent(
      '7:sk-fake-key-7,8:sk-fake-key-8'
    )
  )
  const call = post.mock.calls.find(([url]) => url === '/api/token/batch/keys')
  expect(call).toBeDefined()
  const [, body, config] = call as unknown as [
    string,
    { ids: number[] },
    { headers?: Record<string, string> },
  ]
  expect(body?.ids).toEqual([7, 8])
  expect(config?.headers?.['X-Security-Proof']).toBe('token-key-proof')
})

it('cancelling verification never calls the key endpoint', async () => {
  tokenVerification()
  const post = vi
    .spyOn(api, 'post')
    .mockImplementation(async (url) =>
      url === '/api/verify'
        ? { data: { success: true, data: {} } }
        : Promise.reject(new Error(`Unexpected POST ${url}`))
    )
  render(<Harness />)
  await userEvent.click(screen.getByRole('button', { name: 'Reveal Single' }))
  await userEvent.click(await screen.findByRole('button', { name: 'Cancel' }))
  await waitFor(() =>
    expect(screen.getByLabelText('Single key')).toHaveTextContent('Hidden')
  )
  expect(post.mock.calls.map(([url]) => url)).toEqual([])
})
