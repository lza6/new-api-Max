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
import { api } from '@/lib/api'

import type {
  FeatureSwitchListResponse,
  FeatureSwitchUpdateRequest,
  FeatureSwitchUpdateResponse,
} from './types'

/** 读取全部能力开关及其效果度量。仅 root 可用。 */
export async function getFeatureSwitches() {
  const res = await api.get<FeatureSwitchListResponse>(
    '/api/option/feature-switches'
  )
  return res.data
}

/** 变更单个能力开关（或 reset 回退 env 默认）。仅 root 可用。 */
export async function updateFeatureSwitch(request: FeatureSwitchUpdateRequest) {
  const res = await api.put<FeatureSwitchUpdateResponse>(
    '/api/option/feature-switches',
    request
  )
  return res.data
}
