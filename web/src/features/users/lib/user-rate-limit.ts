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
import type { RateLimitTier, RelayRateLimitOverrides } from '../api'

export type EffectiveRateSource = 'user' | 'group' | 'base' | 'off'

export interface EffectiveRate {
  tier: RateLimitTier
  source: EffectiveRateSource
}

/**
 * 解析用户「当前生效」限速档位（与后端 GetUserRateLimitTier 一致）：
 * 用户覆盖 > 分组覆盖 > 系统默认（3/s + 120RPM，可关闭=不限），0 表示该项不限。
 */
export function resolveEffectiveRate(
  overrides: RelayRateLimitOverrides | null,
  userId: number,
  group?: string
): EffectiveRate {
  if (!overrides) {
    return { tier: { concurrency: 0, rpm: 0 }, source: 'off' }
  }
  const userTier = overrides.user_overrides?.[String(userId)]
  if (userTier) {
    return { tier: userTier, source: 'user' }
  }
  const groupTier = group ? overrides.group_overrides?.[group] : undefined
  if (groupTier) {
    return { tier: groupTier, source: 'group' }
  }
  if (overrides.base_enabled) {
    return {
      tier: {
        concurrency:
          overrides.base_concurrency > 0 ? overrides.base_concurrency : 3,
        rpm: overrides.base_rpm > 0 ? overrides.base_rpm : 120,
      },
      source: 'base',
    }
  }
  return { tier: { concurrency: 0, rpm: 0 }, source: 'off' }
}

/** 0 表示该项不限，展示为「不限」。 */
export function rateLimitText(
  value: number,
  t: (key: string) => string
): string {
  return value > 0 ? String(value) : t('Unlimited')
}
