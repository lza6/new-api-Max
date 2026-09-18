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
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { OnboardingGuide } from '../components/onboarding-guide'
import { getOnboardingStorageKey } from '../lib/storage'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, options?: Record<string, unknown>) =>
      options
        ? key.replaceAll(/\{\{(\w+)\}\}/g, (_, name: string) =>
            String(options[name])
          )
        : key,
  }),
}))

vi.mock('@tanstack/react-router', () => ({
  Link: ({
    to,
    children,
    ...props
  }: { to: string; children: ReactNode } & Record<string, unknown>) => (
    <a href={to} {...props}>
      {children}
    </a>
  ),
}))

const originalAuth = useAuthStore.getState().auth

beforeEach(() => {
  localStorage.clear()
  useAuthStore.setState({
    auth: {
      ...originalAuth,
      user: { id: 7, username: 'newbie', role: ROLE.USER },
    },
  })
})

afterEach(() => {
  cleanup()
  useAuthStore.setState({ auth: originalAuth })
})

describe('onboarding guide (B2-3)', () => {
  test('walks through the three first-run steps with Next', () => {
    render(<OnboardingGuide />)

    expect(screen.getByTestId('onboarding-guide')).toBeInTheDocument()
    expect(screen.getByText('Create an API Token')).toBeInTheDocument()
    expect(screen.getByText('Step 1 of 3')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Go to API keys/ })).toHaveAttribute(
      'href',
      '/keys'
    )

    fireEvent.click(screen.getByRole('button', { name: 'Next' }))
    expect(screen.getByText('Browse the model plaza')).toBeInTheDocument()
    expect(screen.getByText('Step 2 of 3')).toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: /Go to model plaza/ })
    ).toHaveAttribute('href', '/pricing')

    fireEvent.click(screen.getByRole('button', { name: 'Next' }))
    expect(screen.getByText('Make your first request')).toBeInTheDocument()
    expect(screen.getByText('Step 3 of 3')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Open playground/ })).toHaveAttribute(
      'href',
      '/playground'
    )
  })

  test('close button dismisses and persists per account', () => {
    const { unmount } = render(<OnboardingGuide />)
    fireEvent.click(screen.getByRole('button', { name: 'Dismiss onboarding' }))

    expect(screen.queryByTestId('onboarding-guide')).not.toBeInTheDocument()
    expect(localStorage.getItem(getOnboardingStorageKey(7))).toBe('1')

    unmount()
    render(<OnboardingGuide />)
    expect(screen.queryByTestId('onboarding-guide')).not.toBeInTheDocument()
  })

  test('skip dismisses and persists per account', () => {
    render(<OnboardingGuide />)
    fireEvent.click(screen.getByRole('button', { name: 'Skip for now' }))

    expect(screen.queryByTestId('onboarding-guide')).not.toBeInTheDocument()
    expect(localStorage.getItem(getOnboardingStorageKey(7))).toBe('1')
  })

  test('already-dismissed account never sees the guide', () => {
    localStorage.setItem(getOnboardingStorageKey(7), '1')
    render(<OnboardingGuide />)
    expect(screen.queryByTestId('onboarding-guide')).not.toBeInTheDocument()
  })

  test('dismissal is isolated between accounts', () => {
    localStorage.setItem(getOnboardingStorageKey(7), '1')
    render(<OnboardingGuide />)
    expect(screen.queryByTestId('onboarding-guide')).not.toBeInTheDocument()

    useAuthStore.setState({
      auth: { ...originalAuth, user: { id: 8, username: 'other', role: ROLE.USER } },
    })
    cleanup()
    render(<OnboardingGuide />)
    expect(screen.getByTestId('onboarding-guide')).toBeInTheDocument()
  })

  test('back returns to the previous step and never leaves step 1', () => {
    render(<OnboardingGuide />)
    const back = screen.getByRole('button', { name: 'Back' })
    expect(back).toBeDisabled()

    fireEvent.click(screen.getByRole('button', { name: 'Next' }))
    fireEvent.click(screen.getByRole('button', { name: 'Back' }))
    expect(screen.getByText('Create an API Token')).toBeInTheDocument()
  })

  test('finishes on the last step by dismissing', () => {
    render(<OnboardingGuide />)
    fireEvent.click(screen.getByRole('button', { name: 'Next' }))
    fireEvent.click(screen.getByRole('button', { name: 'Next' }))
    fireEvent.click(screen.getByRole('button', { name: 'Done' }))

    expect(screen.queryByTestId('onboarding-guide')).not.toBeInTheDocument()
    expect(localStorage.getItem(getOnboardingStorageKey(7))).toBe('1')
  })
})