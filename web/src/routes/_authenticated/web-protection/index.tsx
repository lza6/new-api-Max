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
import { createFileRoute, redirect } from "@tanstack/react-router"

const WebProtectionPage = lazy(() => import("@/features/web-protection/web-protection-page").then((m) => ({ default: m.WebProtectionPage })))
import { ROLE } from "@/lib/roles"
import { defineSearchSchema, numberParam } from '@/lib/search-schema'
import { useAuthStore } from "@/stores/auth-store"

// URL As State: 分页页码/每页条数写进 search params，刷新/分享后状态不丢。
const webProtectionSearchSchema = defineSearchSchema({
  page: numberParam(1),
  pageSize: numberParam(undefined),
})

export const Route = createFileRoute("/_authenticated/web-protection/")({
  beforeLoad: () => {
    const { auth } = useAuthStore.getState()
    if (auth.user?.role !== ROLE.SUPER_ADMIN) {
      throw redirect({ to: "/403" })
    }
  },
  validateSearch: webProtectionSearchSchema,
  component: WebProtectionPage,
})
