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
/**
 * 运营面板数据类型。所有 quota 字段都是**原始额度单位**（与 logs.quota / users.quota 一致），
 * 展示时须经 `@/lib/format` 的 formatQuota 换算为站点配置的展示货币——不要在数据层换算。
 */
export interface OpsOverview {
  /** 本地时区当天 00:00 起至今的 consume 额度。 */
  today_quota: number
  /** today_quota 所用时区名。采集统计用 UTC 日，两者口径不同，必须分别展示。 */
  today_quota_timezone?: string
  /** 全部时间累计 consume 额度。 */
  total_quota: number
  /** 非管理员用户剩余额度之和（排除管理员/超管）。 */
  total_remaining_quota: number
  /** 计入 total_remaining_quota 的用户数。 */
  user_count: number
  /** 近 average_over_days 天的日均消耗额度。 */
  daily_average_quota: number
  /** 日均消耗的统计窗口天数。 */
  average_over_days: number
  /** = total_remaining_quota / daily_average_quota；日均消耗为 0 时为 null。 */
  estimated_remaining_days: number | null
}

export interface OpsRankingItem {
  user_id: number
  /** 当前用户名；用户已被删除时为空字符串。 */
  username: string
  /** 窗口内消耗额度（原始单位）。 */
  quota: number
  /** 窗口内请求数。 */
  requests: number
}

export interface OpsRankingResult {
  days: number
  limit: number
  items: OpsRankingItem[]
}
