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

import { ModelDetails } from '@/features/pricing/components/model-details'
import { getModuleAccessForGuard } from '@/lib/nav-modules'
import {
  booleanParam,
  defineSearchSchema,
  enumParam,
  textParam,
} from '@/lib/search-schema'
import { useAuthStore } from '@/stores/auth-store'

const modelDetailsSearchSchema = defineSearchSchema({
  search: textParam,
  sort: textParam,
  vendor: textParam,
  group: textParam,
  quotaType: textParam,
  endpointType: textParam,
  tag: textParam,
  tokenUnit: enumParam(['M', 'K'], undefined),
  view: enumParam(['card', 'table'], undefined),
  rechargePrice: booleanParam,
})

export const Route = createFileRoute('/pricing/$modelId/')({
  validateSearch: modelDetailsSearchSchema,
  beforeLoad: async ({ context, location }) => {
    const access = await getModuleAccessForGuard(context.queryClient, 'pricing')
    if (!access.enabled) {
      throw redirect({ to: '/' })
    }
    if (access.requireAuth) {
      const { auth } = useAuthStore.getState()
      if (!auth.user) {
        throw redirect({
          to: '/sign-in',
          search: { redirect: location.href },
        })
      }
    }
  },
  component: ModelDetails,
})
