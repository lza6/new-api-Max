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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import en from '@/i18n/locales/en.json'

import type { TaskLog } from '../../types'
import { TaskArtifactsCell } from '../task-artifacts'

vi.mock('@/components/dialog', () => ({
  Dialog: (props: {
    open?: boolean
    title?: ReactNode
    children?: ReactNode
  }) => (props.open ? <div data-testid='mock-dialog'>{props.title}{props.children}</div> : null),
}))

vi.mock('../../api', () => ({
  getTaskArtifacts: vi.fn().mockResolvedValue({
    artifacts: [],
    legacyContentUrl: 'https://media.example.com/video.mp4',
  }),
}))

const i18n = createInstance()
let client: QueryClient

beforeEach(async () => {
  await i18n.init({
    lng: 'en',
    resources: { en: { translation: en } },
    fallbackLng: 'en',
    interpolation: { escapeValue: false },
  })
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
})

function renderCell(log: TaskLog) {
  return render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        <TaskArtifactsCell log={log} />
      </QueryClientProvider>
    </I18nextProvider>
  )
}

function sunoLog(): TaskLog {
  return {
    id: 1,
    user_id: 7,
    platform: 'suno',
    task_id: 'task-suno',
    action: 'generate',
    channel_id: 3,
    group: 'default',
    quota: 100,
    submit_time: 1,
    status: 'SUCCESS',
    data: [
      {
        clip_id: 'clip-1',
        title: 'My First Song',
        duration: 90,
        audio_url: 'https://media.example.com/audio.mp3',
      },
    ],
  }
}

function legacyVideoLog(): TaskLog {
  return {
    id: 1,
    user_id: 7,
    platform: 'openrouter',
    task_id: 'task-video',
    action: 'generate',
    channel_id: 3,
    group: 'default',
    quota: 100,
    submit_time: 1,
    status: 'SUCCESS',
    legacy_video_available: true,
  }
}

afterEach(() => vi.clearAllMocks())

describe('TaskArtifactsCell preview triggers', () => {
  test('legacy audio trigger is an accessible button that opens the audio preview', () => {
    renderCell(sunoLog())

    const trigger = screen.getByRole('button', { name: 'Click to preview audio' })
    expect(document.querySelector('button')?.tagName).toBe('BUTTON')
    expect(screen.queryByTestId('mock-dialog')).toBeNull()

    fireEvent.click(trigger)
    expect(screen.getByTestId('mock-dialog')).toBeInTheDocument()
    expect(screen.getByText('My First Song')).toBeInTheDocument()
  })

  test('legacy video trigger is an accessible button that opens the preview', async () => {
    renderCell(legacyVideoLog())

    const trigger = screen.getByRole('button', { name: 'Click to preview video' })
    expect(document.querySelector('button')?.tagName).toBe('BUTTON')

    fireEvent.click(trigger)
    expect(await screen.findByTestId('mock-dialog')).toBeInTheDocument()
    expect(screen.getByText('Preview')).toBeInTheDocument()
    await waitFor(() => {
      expect(document.querySelector('video')?.getAttribute('src')).toBe(
        'https://media.example.com/video.mp4'
      )
    })
  })
})
