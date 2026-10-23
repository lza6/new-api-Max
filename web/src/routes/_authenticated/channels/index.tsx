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
import { lazy } from 'react'
import { createFileRoute, redirect } from '@tanstack/react-router'

const Channels = lazy(() => import('@/features/channels').then((m) => ({ default: m.Channels })))
import { ROLE } from '@/lib/roles'
import {
  defineSearchSchema,
  numberParam,
  textListParam,
  textParam,
} from '@/lib/search-schema'
import { useAuthStore } from '@/stores/auth-store'

const channelsSearchSchema = defineSearchSchema({
  page: numberParam(1),
  pageSize: numberParam(undefined),
  filter: textParam,
  status: textListParam([]),
  type: textListParam([]),
  group: textListParam([]),
  model: textParam,
})

export const Route = createFileRoute('/_authenticated/channels/')({
  beforeLoad: () => {
    const { auth } = useAuthStore.getState()

    if (!auth.user || auth.user.role < ROLE.ADMIN) {
      throw redirect({
        to: '/403',
      })
    }
  },
  validateSearch: channelsSearchSchema,
  component: Channels,
})
