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
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'

import { ConfirmDialog } from '../confirm-dialog'

/**
 * L3 破坏性操作契约（尼尔森 N3）：
 * 传了 `requireTypedText` 时，必须逐字输入该文本，确认按钮才可用。
 * 这条保护用于计费/价格类改动——误点一次会改变所有用户看到的余额与价格。
 */
describe('ConfirmDialog requireTypedText', () => {
  it('keeps the confirm button disabled until the exact text is typed', async () => {
    const handleConfirm = vi.fn()
    const user = userEvent.setup()

    render(
      <ConfirmDialog
        open
        onOpenChange={() => undefined}
        title='Change pricing conversion?'
        desc='This affects every user.'
        confirmText='Save Changes'
        requireTypedText='QUOTA'
        handleConfirm={handleConfirm}
      />
    )

    const confirmButton = screen.getByRole('button', { name: 'Save Changes' })
    expect(confirmButton).toBeDisabled()

    const input = screen.getByLabelText('Type QUOTA to confirm')
    await user.type(input, 'QUO')
    expect(confirmButton).toBeDisabled()

    await user.type(input, 'TA')
    expect(confirmButton).toBeEnabled()

    await user.click(confirmButton)
    expect(handleConfirm).toHaveBeenCalledTimes(1)
  })

  it('behaves like the plain confirm dialog when requireTypedText is absent', () => {
    render(
      <ConfirmDialog
        open
        onOpenChange={() => undefined}
        title='Delete?'
        desc='Cannot be undone.'
        confirmText='Delete'
        destructive
        handleConfirm={() => undefined}
      />
    )

    expect(screen.getByRole('button', { name: 'Delete' })).toBeEnabled()
    expect(screen.queryByLabelText(/to confirm/i)).not.toBeInTheDocument()
  })
})
