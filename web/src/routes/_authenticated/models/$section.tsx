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

import { Models } from '@/features/models'
import {
  MODELS_SECTION_IDS,
  MODELS_DEFAULT_SECTION,
} from '@/features/models/section-registry'
import { ROLE } from '@/lib/roles'
import {
  defineSearchSchema,
  enumListParam,
  numberParam,
  textListParam,
  textParam,
} from '@/lib/search-schema'
import { useAuthStore } from '@/stores/auth-store'

const modelsSearchSchema = defineSearchSchema({
  vPage: numberParam(1),
  vPageSize: numberParam(20),
  vFilter: textParam,
  vAssociation: textListParam([]),
  page: numberParam(1),
  pageSize: numberParam(10),
  filter: textParam,
  vendor: textListParam([]),
  status: textListParam([]),
  square_state: enumListParam(
    ['visible', 'unavailable', 'hidden', 'partial'],
    []
  ),
  sync: textListParam([]),
  dPage: numberParam(1),
  dPageSize: numberParam(10),
  dFilter: textParam,
  dStatus: textListParam([]),
})

export const Route = createFileRoute('/_authenticated/models/$section')({
  beforeLoad: ({ params }) => {
    const { auth } = useAuthStore.getState()

    if (!auth.user || auth.user.role < ROLE.ADMIN) {
      throw redirect({
        to: '/403',
      })
    }

    const validSections = MODELS_SECTION_IDS as unknown as string[]
    if (!validSections.includes(params.section)) {
      throw redirect({
        to: '/models/$section',
        params: { section: MODELS_DEFAULT_SECTION },
      })
    }
  },
  validateSearch: modelsSearchSchema,
  component: Models,
})
