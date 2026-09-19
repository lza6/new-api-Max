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
export interface TimeWindow {
  requests: number
  tokens: number
  quota: number
  errors: number
  error_rate: number
  channels: number
}

export interface ModelUsage {
  model: string
  requests: number
  tokens: number
  quota: number
  errors: number
  error_rate: number
  share: number
}

export interface HourBucket {
  hour: number
  requests: number
}

export interface CostPoint {
  date: string
  quota: number
}

export interface ChannelUsage {
  channel_id: number
  channel_name: string
  requests: number
  errors: number
  error_rate: number
  quota: number
}

export interface Suggestion {
  code: string
  args?: Record<string, string | number | undefined>
}

export interface ProfileInsights {
  user_id: number
  generated_at: number
  overview_7d: TimeWindow
  overview_30d: TimeWindow
  trend: CostPoint[]
  model_usage: ModelUsage[]
  time_heatmap: HourBucket[]
  channels: ChannelUsage[]
  suggestions: Suggestion[]
  evidence: { source: string; window: string; samples: number }
}