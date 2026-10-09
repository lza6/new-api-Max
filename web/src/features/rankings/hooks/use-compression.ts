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
import { useQuery } from '@tanstack/react-query'

import { requireServerSuccess } from '@/lib/server-error-message'

import { getCompressionStats, getSavingsBaseline } from '../api'

export function useCompressionStats(limit = 50) {
  return useQuery({
    queryKey: ['rankings', 'compression', limit],
    queryFn: async () =>
      requireServerSuccess(await getCompressionStats(limit)).data,
    staleTime: 5 * 60 * 1000,
  })
}

/** T1 反事实节省基准（与压缩统计同源，口径统一）。 */
export function useSavingsBaseline(limit = 50) {
  return useQuery({
    queryKey: ['rankings', 'savings-baseline', limit],
    queryFn: async () =>
      requireServerSuccess(await getSavingsBaseline(limit)).data,
    staleTime: 5 * 60 * 1000,
  })
}
