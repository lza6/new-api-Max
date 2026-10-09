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
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { MemoryPreferencesCard } from '../memory-preferences-card'
import type { UserProfile } from '../../types'

function profileWith(memory?: string): UserProfile {
  return {
    id: 1,
    username: 'alice',
    display_name: 'Alice',
    role: 1,
    group: 'default',
    quota: 1000000,
    used_quota: 0,
    request_count: 0,
    status: 1,
    aff_count: 0,
    aff_quota: 0,
    aff_history_quota: 0,
    setting: JSON.stringify({ memory_injection: memory ?? '' }),
  } as UserProfile
}

afterEach(() => {
  vi.restoreAllMocks()
})

describe('memory injection settings card', () => {
  it('renders nothing when the deployment has memory injection disabled', () => {
    const { container } = render(
      <MemoryPreferencesCard
        profile={profileWith()}
        onProfileUpdate={vi.fn()}
        enabled={false}
      />
    )
    // 未开启时保存不生效，不给用户一个看起来能用的开关。
    expect(container).toBeEmptyDOMElement()
  })

  it('shows the saved memory and flushes it to the user self endpoint on save', async () => {
    const put = vi
      .spyOn(api, 'put')
      .mockResolvedValue({ data: { success: true } })
    const onProfileUpdate = vi.fn()

    render(
      <MemoryPreferencesCard
        profile={profileWith('I prefer Go examples')}
        onProfileUpdate={onProfileUpdate}
        enabled
      />
    )

    const textarea = screen.getByRole('textbox', { name: 'My Memory' })
    expect(textarea).toHaveValue('I prefer Go examples')

    fireEvent.change(textarea, { target: { value: 'Answer in Chinese' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() =>
      expect(put).toHaveBeenCalledWith('/api/user/self', {
        memory_injection: 'Answer in Chinese',
      })
    )
    await waitFor(() => expect(onProfileUpdate).toHaveBeenCalled())
  })

  it('disables saving when the text is unchanged', () => {
    render(
      <MemoryPreferencesCard
        profile={profileWith('unchanged')}
        onProfileUpdate={vi.fn()}
        enabled
      />
    )
    expect(screen.getByRole('button', { name: 'Save' })).toBeDisabled()
  })

  it('caps the input at the server-side character limit', () => {
    render(
      <MemoryPreferencesCard
        profile={profileWith()}
        onProfileUpdate={vi.fn()}
        enabled
      />
    )
    const textarea = screen.getByRole('textbox', { name: 'My Memory' })
    // maxLength 与后端 relaykit/dto.MaxMemoryInjectionRunes 一致，
    // 避免用户提交后才在服务端被拒。
    expect(textarea).toHaveAttribute('maxLength', '2000')
    expect(screen.getByText('0 / 2000')).toBeInTheDocument()
  })
})
