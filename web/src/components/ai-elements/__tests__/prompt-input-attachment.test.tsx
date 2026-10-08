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
import { cleanup, render, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import { PromptInput, PromptInputTextarea } from '../prompt-input'

const createObjectURL = vi.fn(() => 'blob:attached-image')
const revokeObjectURL = vi.fn()

function renderPromptInput(props: {
  onError: (err: { code: string; message: string }) => void
  onSubmit: () => void
}) {
  return render(
    <PromptInput accept='image/*' onError={props.onError} onSubmit={props.onSubmit}>
      <PromptInputTextarea />
    </PromptInput>
  )
}

function attachImage(container: HTMLElement): void {
  const input = container.querySelector<HTMLInputElement>('input[type="file"]')
  if (!input) {
    throw new Error('file input not found')
  }
  const file = new File(['image-bytes'], 'ref.png', { type: 'image/png' })
  Object.defineProperty(input, 'files', { value: [file], configurable: true })
  input.dispatchEvent(new Event('change', { bubbles: true }))
}

async function submitForm(container: HTMLElement): Promise<void> {
  const form = container.querySelector('form')
  if (!form) {
    throw new Error('form not found')
  }
  form.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
}

beforeEach(() => {
  createObjectURL.mockClear()
  revokeObjectURL.mockClear()
  vi.stubGlobal(
    'URL',
    Object.assign(class extends URL {}, { createObjectURL, revokeObjectURL })
  )
})

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
})

describe('PromptInput attachment conversion failure', () => {
  test('reports onError instead of silently dropping the submission when the blob cannot be read', async () => {
    // Arrange：模拟 CSP 拦截 blob: 导致 fetch 被拒（真实生产故障形态）。
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.reject(new Error('Refused to connect to blob:')))
    )
    const onError = vi.fn()
    const onSubmit = vi.fn()
    const { container } = renderPromptInput({ onError, onSubmit })

    // Act
    attachImage(container)
    await waitFor(() => expect(createObjectURL).toHaveBeenCalled())
    await submitForm(container)

    // Assert：必须上报可诊断的错误，且不得调用 onSubmit。
    await waitFor(() =>
      expect(onError).toHaveBeenCalledWith(
        expect.objectContaining({ code: 'attachment_conversion' })
      )
    )
    expect(onSubmit).not.toHaveBeenCalled()
  })

  test('submits converted attachments when the blob is readable', async () => {
    // Arrange：正常路径——blob 可读，应转成 data: URL 后提交。
    vi.stubGlobal(
      'fetch',
      vi.fn(() =>
        Promise.resolve({
          blob: () => Promise.resolve(new Blob(['image-bytes'])),
        } as unknown as Response)
      )
    )
    const onError = vi.fn()
    const onSubmit = vi.fn()
    const { container } = renderPromptInput({ onError, onSubmit })

    // Act
    attachImage(container)
    await waitFor(() => expect(createObjectURL).toHaveBeenCalled())
    await submitForm(container)

    // Assert
    await waitFor(() => expect(onSubmit).toHaveBeenCalled())
    expect(onError).not.toHaveBeenCalled()
  })
})
