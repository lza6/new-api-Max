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
import { useMemo } from 'react'

import { usePricingData } from '@/features/pricing/hooks/use-pricing-data'

/**
 * 站点示例/默认模型：取站点当前定价清单里的首个可用模型，随站点模型配置动态变化，
 * 避免示例与工具接入配置写死某个模型名（模型改名后三处自动跟随）。
 *
 * fallback 仅在站点尚未配置任何模型时使用（正常站点不应命中）。
 */
const FALLBACK_SITE_MODEL = 'deepseek-v4-flash'

export function useSiteModel(): string {
  const { models } = usePricingData()
  return useMemo(
    () => models[0]?.model_name ?? FALLBACK_SITE_MODEL,
    [models]
  )
}
