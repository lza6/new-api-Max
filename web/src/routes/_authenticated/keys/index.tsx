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
import { createFileRoute } from '@tanstack/react-router'

import { ApiKeys } from '@/features/keys'
import { API_KEY_STATUS_OPTIONS } from '@/features/keys/constants'
import {
  defineSearchSchema,
  enumListParam,
  numberParam,
  textParam,
} from '@/lib/search-schema'

const apiKeySearchSchema = defineSearchSchema({
  page: numberParam(1),
  pageSize: numberParam(undefined),
  status: enumListParam(
    API_KEY_STATUS_OPTIONS.map((s) => s.value as `${number}`),
    []
  ),
  filter: textParam,
  token: textParam,
})

export const Route = createFileRoute('/_authenticated/keys/')({
  validateSearch: apiKeySearchSchema,
  component: ApiKeys,
})
