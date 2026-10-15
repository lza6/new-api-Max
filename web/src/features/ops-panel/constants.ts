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
/** 日均消耗统计窗口（天）。与服务端 opsDefaultAverageDays 保持同一口径。 */
export const OPS_AVERAGE_DAYS = 14

/** 排行榜默认/可选窗口（天）与其 i18n 标签键。 */
export const OPS_RANKING_DAY_OPTIONS = [1, 7, 30, 90] as const
export const OPS_RANKING_DEFAULT_DAYS = 7

/** 窗口天数 → i18n 标签键。 */
export const OPS_RANKING_DAY_LABEL_KEYS: Record<number, string> = {
  1: 'Last 24 hours',
  7: 'Last 7 days',
  30: 'Last 30 days',
  90: 'Last 90 days',
}

/** 排行榜返回条数上限。 */
export const OPS_RANKING_LIMIT = 20

/** React Query key 前缀，供整页刷新统一失效。 */
export const OPS_QUERY_KEY = 'ops'
