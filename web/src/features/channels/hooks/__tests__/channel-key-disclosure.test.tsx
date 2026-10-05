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
import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, it, vi } from 'vitest'

import { SecureVerificationDialog } from '@/features/auth/secure-verification'
import { api } from '@/lib/api'

import { useChannelKeyDisclosure } from '../use-channel-key-disclosure'

function Harness(props: { open: boolean; channelId: number }) {
  const disclosure = useChannelKeyDisclosure(props.open, props.channelId)
  return (
    <>
      <button type='button' onClick={disclosure.handleRevealKey}>
        Reveal
      </button>
      <output aria-label='Channel key'>
        {disclosure.channelKey ?? 'Hidden'}
      </output>
      <SecureVerificationDialog {...disclosure.verification.dialogProps} />
    </>
  )
}

function deferredResponse<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((finish) => {
    resolve = finish
  })
  return { promise, resolve }
}

function channelVerification() {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: {
        scope: 'channel.key.read',
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
        proof_token: 'channel-proof',
        method: '2fa',
        scope: 'channel.key.read',
        expires_at: Math.floor(Date.now() / 1000) + 60,
      },
    },
  }
}

/**
 * A real axios-shaped 403 for a site that enforces channel-key step-up. The
 * business code lives in response.data.code; axios sets error.code to
 * ERR_BAD_REQUEST, so the reader must look at response.data.code first.
 */
function proofRequiredError() {
  return Object.assign(new Error('需要安全验证'), {
    code: 'ERR_BAD_REQUEST',
    isAxiosError: true,
    response: {
      status: 403,
      data: {
        success: false,
        code: 'SECURITY_PROOF_REQUIRED',
        message: '需要安全验证',
      },
    },
  })
}

afterEach(() => vi.restoreAllMocks())

// Default site config (require_verification_to_read_channel_key=false): the
// channel key must be revealed without any step-up prompt. Previously the hook
// always prompted first, so a root admin without 2FA/Passkey could never view
// the key — this is the contract direction the audit flagged.
it('reveals a channel key without any step-up when the site default allows it', async () => {
  const post = vi.spyOn(api, 'post').mockImplementation(async (url) => {
    if (url === '/api/channel/123/key') {
      return { data: { success: true, data: { key: 'CHANNEL_SECRET' } } }
    }
    throw new Error(`Unexpected POST ${url}`)
  })
  const user = userEvent.setup()
  render(<Harness open channelId={123} />)
  await user.click(screen.getByRole('button', { name: 'Reveal' }))

  await waitFor(() =>
    expect(screen.getByLabelText('Channel key')).toHaveTextContent(
      'CHANNEL_SECRET'
    )
  )
  // No /api/verify call at all — the key was fetched without a proof.
  expect(post.mock.calls.map(([url]) => url)).toEqual(['/api/channel/123/key'])
  const [, , config] = post.mock.calls[0] as unknown as [
    string,
    unknown,
    { headers?: Record<string, string> },
  ]
  expect(config?.headers?.['X-Security-Proof']).toBeUndefined()
})

// Enforcement site: the first (no-proof) attempt is rejected, so the hook must
// prompt, then retry once with the proof.
it('falls back to step-up and retries with the proof when the server requires it', async () => {
  const proof = channelVerification()
  let attempts = 0
  const post = vi.spyOn(api, 'post').mockImplementation(async (url) => {
    if (url === '/api/verify') {return proof}
    if (url === '/api/channel/123/key') {
      attempts += 1
      if (attempts === 1) {throw proofRequiredError()}
      return { data: { success: true, data: { key: 'CHANNEL_SECRET' } } }
    }
    throw new Error(`Unexpected POST ${url}`)
  })
  const user = userEvent.setup()
  render(<Harness open channelId={123} />)
  await user.click(screen.getByRole('button', { name: 'Reveal' }))
  await user.type(
    await screen.findByLabelText('Authenticator code or backup code'),
    '123456'
  )
  await user.click(screen.getByRole('button', { name: 'Verify' }))

  await waitFor(() =>
    expect(screen.getByLabelText('Channel key')).toHaveTextContent(
      'CHANNEL_SECRET'
    )
  )
  const retry = post.mock.calls.filter(
    ([url]) => url === '/api/channel/123/key'
  )[1]
  const [, , config] = retry as unknown as [
    string,
    unknown,
    { headers?: Record<string, string> },
  ]
  expect(config?.headers?.['X-Security-Proof']).toBe('channel-proof')
})

it('discards a pending channel key response after switching channel', async () => {
  const proof = channelVerification()
  const keyReply = deferredResponse<{
    data: { success: boolean; data: { key: string } }
  }>()
  let call = 0
  vi.spyOn(api, 'post').mockImplementation((url) => {
    if (url === '/api/verify') {return Promise.resolve(proof)}
    if (url === '/api/channel/123/key') {
      call += 1
      if (call === 1) {return Promise.reject(proofRequiredError())}
      return keyReply.promise
    }
    throw new Error(`Unexpected POST ${url}`)
  })
  const user = userEvent.setup()
  const view = render(<Harness open channelId={123} />)
  await user.click(screen.getByRole('button', { name: 'Reveal' }))
  await user.type(
    await screen.findByLabelText('Authenticator code or backup code'),
    '123456'
  )
  await user.click(screen.getByRole('button', { name: 'Verify' }))
  await waitFor(() => expect(call).toBeGreaterThanOrEqual(2))
  view.rerender(<Harness open channelId={456} />)
  await act(async () => {
    keyReply.resolve({
      data: { success: true, data: { key: 'CHANNEL_A_SECRET' } },
    })
    await keyReply.promise
  })
  expect(screen.getByLabelText('Channel key')).toHaveTextContent('Hidden')
  expect(screen.queryByText('CHANNEL_A_SECRET')).not.toBeInTheDocument()
})

it('cancels a pending verification when the selected channel changes', async () => {
  channelVerification()
  const reply = deferredResponse<{
    data: { success: boolean; data: Record<string, unknown> }
  }>()
  let call = 0
  const post = vi.spyOn(api, 'post').mockImplementation((url) => {
    if (url === '/api/channel/123/key') {
      call += 1
      if (call === 1) {return Promise.reject(proofRequiredError())}
    }
    return reply.promise
  })
  const user = userEvent.setup()
  const view = render(<Harness open channelId={123} />)
  await user.click(screen.getByRole('button', { name: 'Reveal' }))
  await user.type(
    await screen.findByLabelText('Authenticator code or backup code'),
    '123456'
  )
  await user.click(screen.getByRole('button', { name: 'Verify' }))
  view.rerender(<Harness open channelId={456} />)
  await act(async () => {
    reply.resolve({
      data: {
        success: true,
        data: {
          proof_token: 'late-proof',
          scope: 'channel.key.read',
          method: '2fa',
          expires_at: Math.floor(Date.now() / 1000) + 60,
        },
      },
    })
    await reply.promise
  })
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  expect(post.mock.calls.some(([url]) => url === '/api/verify')).toBe(true)
})
