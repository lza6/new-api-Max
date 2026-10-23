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

const UsageLogs = lazy(() => import('@/features/usage-logs').then((m) => ({ default: m.UsageLogs })))
import {
  isUsageLogsSectionId,
  USAGE_LOGS_DEFAULT_SECTION,
} from '@/features/usage-logs/section-registry'
import {
  defineSearchSchema,
  enumListParam,
  numberParam,
  textParam,
} from '@/lib/search-schema'

const logTypeValues = ['0', '1', '2', '3', '4', '5', '6', '7'] as const

const usageLogsSearchSchema = defineSearchSchema({
  page: numberParam(1),
  pageSize: numberParam(undefined),
  // `type` 单个值与重复键都归一成数组（原来的 z.preprocess 行为）。
  // 显式取 string 作为元素类型：运行时仍按 logTypeValues 白名单过滤，
  // 静态类型保持与调用方（传 string 的筛选条）兼容。
  type: enumListParam<string>(logTypeValues, []),
  filter: textParam,
  model: textParam,
  token: textParam,
  channel: textParam,
  group: textParam,
  username: textParam,
  requestId: textParam,
  upstreamRequestId: textParam,
  // 首字/耗时筛选：min_use_time 秒，>0 时只列耗时 ≥ 该值的请求（如 20 → >20s）。
  minUseTime: numberParam(undefined),
  startTime: numberParam(undefined),
  endTime: numberParam(undefined),
})

export const Route = createFileRoute('/_authenticated/usage-logs/$section')({
  beforeLoad: ({ params, search }) => {
    if (!isUsageLogsSectionId(params.section)) {
      throw redirect({
        to: '/usage-logs/$section',
        params: { section: USAGE_LOGS_DEFAULT_SECTION },
      })
    }
    // type 仅 common 使用，非 common 时清掉 URL 里的 type
    const hasTypeSearch = Array.isArray(search?.type)
      ? search.type.length > 0
      : search?.type != null && search.type !== ''
    if (params.section !== 'common' && hasTypeSearch) {
      throw redirect({
        to: '/usage-logs/$section',
        params: { section: params.section },
        search: { ...search, type: undefined },
        replace: true,
      })
    }
  },
  validateSearch: usageLogsSearchSchema,
  component: UsageLogs,
})
