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
import { useSyncExternalStore } from 'react'

/**
 * Whether the document is currently visible (tab foregrounded). Combine with a
 * query's `enabled`/`refetchInterval` to stop background polling when the user
 * switches tabs — otherwise an admin who leaves the dashboard open keeps
 * hammering the API on a fixed interval.
 *
 * Returns `true` when `document` is unavailable (SSR / test env without it), so
 * callers keep their default behaviour there.
 */
export function useDocumentVisible(): boolean {
  return useSyncExternalStore(
    (onStoreChange) => {
      if (typeof document === 'undefined') {
        return () => {}
      }
      document.addEventListener('visibilitychange', onStoreChange)
      return () =>
        document.removeEventListener('visibilitychange', onStoreChange)
    },
    () => {
      if (typeof document === 'undefined') {
        return true
      }
      return document.visibilityState !== 'hidden'
    },
    () => true
  )
}
