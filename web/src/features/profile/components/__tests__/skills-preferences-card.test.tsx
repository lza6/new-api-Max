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
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { SkillsPreferencesCard } from '../skills-preferences-card'
import type { UserProfile, UserSkill } from '../../types'

const i18n = createInstance()
await i18n.init({
  lng: 'en',
  resources: { en: { translation: {} } },
  initAsync: false,
})

function profileWith(skills?: UserSkill[]): UserProfile {
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
    setting: JSON.stringify({ skills: skills ?? [] }),
  } as UserProfile
}

function renderCard(props: {
  profile: UserProfile | null
  enabled: boolean
  onProfileUpdate?: () => void
}) {
  return render(
    <I18nextProvider i18n={i18n}>
      <SkillsPreferencesCard
        profile={props.profile}
        enabled={props.enabled}
        onProfileUpdate={props.onProfileUpdate ?? vi.fn()}
      />
    </I18nextProvider>
  )
}

afterEach(() => vi.restoreAllMocks())

describe('skills preferences card', () => {
  it('renders nothing when the deployment has injection disabled', () => {
    const { container } = renderCard({
      profile: profileWith(),
      enabled: false,
    })
    // 未开启时保存不生效，不给用户一个看起来能用的开关。
    expect(container).toBeEmptyDOMElement()
  })

  it('lists saved skills with their enable state', () => {
    renderCard({
      profile: profileWith([
        { id: 's1', name: 'Summary first', prompt: 'lead with a summary', enabled: true },
        { id: 's2', name: 'Show code', prompt: 'include runnable code', enabled: false },
      ]),
      enabled: true,
    })

    expect(screen.getByText('Summary first')).toBeInTheDocument()
    expect(screen.getByText('Show code')).toBeInTheDocument()

    const toggles = screen.getAllByRole('switch')
    expect(toggles).toHaveLength(2)
    expect(toggles[0]).toBeChecked()
    expect(toggles[1]).not.toBeChecked()
  })

  it('saves the whole skills array through the user self endpoint', async () => {
    const put = vi
      .spyOn(api, 'put')
      .mockResolvedValue({ data: { success: true } })
    const onProfileUpdate = vi.fn()

    renderCard({
      profile: profileWith([
        { id: 's1', name: 'Summary first', prompt: 'lead with a summary', enabled: true },
      ]),
      enabled: true,
      onProfileUpdate,
    })

    fireEvent.click(screen.getByRole('switch'))

    await waitFor(() =>
      expect(put).toHaveBeenCalledWith('/api/user/self', {
        skills: [
          {
            id: 's1',
            name: 'Summary first',
            prompt: 'lead with a summary',
            enabled: false,
          },
        ],
      })
    )
    await waitFor(() => expect(onProfileUpdate).toHaveBeenCalled())
  })

  it('adds a skill through the dialog and persists it', async () => {
    const put = vi
      .spyOn(api, 'put')
      .mockResolvedValue({ data: { success: true } })

    renderCard({ profile: profileWith(), enabled: true })

    fireEvent.click(screen.getByRole('button', { name: 'Add skill' }))
    fireEvent.change(screen.getByLabelText('Name'), {
      target: { value: 'Be brief' },
    })
    fireEvent.change(screen.getByLabelText('Instruction'), {
      target: { value: 'answer in one paragraph' },
    })
    // 弹窗里的 Save（列表页也有一个 Save 语义的按钮，这里按 dialog 内的取）。
    const saveButtons = screen.getAllByRole('button', { name: 'Save' })
    fireEvent.click(saveButtons[saveButtons.length - 1])

    await waitFor(() => expect(put).toHaveBeenCalled())
    const body = put.mock.calls[0]?.[1] as { skills: UserSkill[] }
    expect(body.skills).toHaveLength(1)
    expect(body.skills[0]).toMatchObject({
      name: 'Be brief',
      prompt: 'answer in one paragraph',
      enabled: true,
    })
  })

  it('requires confirmation before deleting a skill', async () => {
    const put = vi
      .spyOn(api, 'put')
      .mockResolvedValue({ data: { success: true } })

    renderCard({
      profile: profileWith([
        { id: 's1', name: 'Summary first', prompt: 'lead with a summary', enabled: true },
      ]),
      enabled: true,
    })

    fireEvent.click(screen.getByRole('button', { name: 'Delete Summary first' }))

    // 打开确认框后，尚未调用接口。
    expect(await screen.findByText('Delete skill')).toBeInTheDocument()
    expect(put).not.toHaveBeenCalled()

    fireEvent.click(screen.getByRole('button', { name: 'Continue' }))

    await waitFor(() =>
      expect(put).toHaveBeenCalledWith('/api/user/self', { skills: [] })
    )
  })
})
