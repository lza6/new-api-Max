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
import { createFileRoute, redirect } from '@tanstack/react-router'

import { Redemptions } from '@/features/redemption-codes'
import { REDEMPTION_FILTER_VALUES } from '@/features/redemption-codes/constants'
import { ROLE } from '@/lib/roles'
import {
  defineSearchSchema,
  enumListParam,
  numberParam,
  textParam,
} from '@/lib/search-schema'
import { useAuthStore } from '@/stores/auth-store'

const redemptionsSearchSchema = defineSearchSchema({
  page: numberParam(1),
  pageSize: numberParam(10),
  filter: textParam,
  status: enumListParam(REDEMPTION_FILTER_VALUES, []),
})

export const Route = createFileRoute('/_authenticated/redemption-codes/')({
  beforeLoad: () => {
    const { auth } = useAuthStore.getState()

    if (!auth.user || auth.user.role < ROLE.ADMIN) {
      throw redirect({
        to: '/403',
      })
    }
  },
  validateSearch: redemptionsSearchSchema,
  component: Redemptions,
})
