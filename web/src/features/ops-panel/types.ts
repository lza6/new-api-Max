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

  // ---- 财务（营收 / 成本 / 利润）----
  // 全部为**额度单位**；前端用 formatQuota 换算为人民币展示。
  today_revenue_quota: number
  today_cost_quota: number
  today_profit_quota: number
  today_margin_percent: number | null
  month_revenue_quota: number
  month_cost_quota: number
  month_profit_quota: number
  month_margin_percent: number | null
  year_revenue_quota: number
  year_cost_quota: number
  year_profit_quota: number
  year_margin_percent: number | null
  projected_month_profit_quota: number
  projected_year_profit_quota: number
  daily_average_cost_quota: number
  recommended_daily_topup_quota: number
  cost_configured: boolean
  /** 今日/本月成本规则覆盖率（营收加权，0~100；营收为 0 时为 null）。 */
  today_cost_coverage_percent: number | null
  month_cost_coverage_percent: number | null
  /** 最早的成本生效时间（unix 秒）；早于它的日志没有成本快照。 */
  cost_tracking_since: number
}

/** 一条上游成本配置（金额为**微元**，1 元 = 1e6）。 */
export interface ModelCostRule {
  id: number
  channel_id: number
  /** 空字符串表示渠道级默认成本。 */
  model: string
  billing_mode: 'call' | 'token'
  cost_per_call_micros: number
  cost_input_per_million_micros: number
  cost_output_per_million_micros: number
  effective_from: number
  created_by: number
  note: string
  created_at: string
}

export interface OpsDaySeriesItem {
  date: string
  revenue_quota: number
  cost_quota: number
  profit_quota: number
  requests: number
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
