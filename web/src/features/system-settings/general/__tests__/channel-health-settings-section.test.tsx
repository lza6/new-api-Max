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
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, test, vi } from 'vitest'

vi.mock('../../components/settings-page-context', () => ({
  SettingsPageProvider: (props: { children: ReactNode }) => props.children,
  useSuppressSettingsSectionHeader: () => false,
  SettingsPageTitleStatusPortal: () => null,
  SettingsPageActionsPortal: (props: { children: ReactNode }) => props.children,
  SettingsPageFormActions: (props: { onSave: () => void; isSaving?: boolean; isSaveDisabled?: boolean; saveLabel?: string }) => (
    <button type='button' onClick={props.onSave}>{props.saveLabel ?? 'Save Changes'}</button>
  ),
}))

const { mutateAsync } = vi.hoisted(() => ({ mutateAsync: vi.fn() }))
vi.mock('../../hooks/use-update-option', () => ({
  useUpdateOption: () => ({ mutateAsync, isPending: false }),
}))

import { ChannelHealthSettingsSection } from '../channel-health-settings-section'

const defaults = { window_seconds: 3600, ring_size: 256, success_weight: 70, latency_best_ms: 1500, latency_worst_ms: 10000, min_score: 0 }

function renderSection(values: typeof defaults = defaults) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(<QueryClientProvider client={qc}><ChannelHealthSettingsSection defaultValues={values} /></QueryClientProvider>)
}

afterEach(() => { mutateAsync.mockReset() })

describe('ChannelHealthSettingsSection (T2-1 param UI)', () => {
  test('renders all six labeled numeric fields with default values', () => {
    renderSection()
    expect(screen.getByLabelText('Channel health window seconds')).toHaveValue(3600)
    expect(screen.getByLabelText('Per-channel sample ring size')).toHaveValue(256)
    expect(screen.getByLabelText('Success weight (%)')).toHaveValue(70)
    expect(screen.getByLabelText('Latency best threshold (ms)')).toHaveValue(1500)
    expect(screen.getByLabelText('Latency worst threshold (ms)')).toHaveValue(10000)
    expect(screen.getByLabelText('Minimum health score for routing')).toHaveValue(0)
  })

  test('saves changed key with dotted channel_health prefix', async () => {
    const user = userEvent.setup()
    renderSection()
    const windowInput = screen.getByLabelText('Channel health window seconds')
    await user.clear(windowInput)
    await user.type(windowInput, '7200')
    await user.click(screen.getByText('Save channel health settings'))
    await waitFor(() => expect(mutateAsync).toHaveBeenCalledWith({ key: 'channel_health.window_seconds', value: 7200 }))
  })

  test('blocks invalid latency bounds and does not save', async () => {
    const user = userEvent.setup()
    renderSection()
    const best = screen.getByLabelText('Latency best threshold (ms)')
    await user.clear(best)
    await user.type(best, '20000')
    await user.click(screen.getByText('Save channel health settings'))
    expect(await screen.findByText('Latency worst threshold must be greater than best')).toBeInTheDocument()
    expect(mutateAsync).not.toHaveBeenCalled()
  })
})
