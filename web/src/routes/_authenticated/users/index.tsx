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

const Users = lazy(() => import('@/features/users').then((m) => ({ default: m.Users })))
import { ROLE } from '@/lib/roles'
import {
  defineSearchSchema,
  enumListParam,
  numberParam,
  textParam,
} from '@/lib/search-schema'
import { useAuthStore } from '@/stores/auth-store'

const usersSearchSchema = defineSearchSchema({
  page: numberParam(1),
  pageSize: numberParam(undefined),
  filter: textParam,
  status: enumListParam(['-1', '1', '2'], []),
  role: enumListParam(['1', '10', '100'], []),
  group: textParam,
})

export const Route = createFileRoute('/_authenticated/users/')({
  beforeLoad: () => {
    const { auth } = useAuthStore.getState()

    if (!auth.user || auth.user.role < ROLE.ADMIN) {
      throw redirect({
        to: '/403',
      })
    }
  },
  validateSearch: usersSearchSchema,
  component: Users,
})
