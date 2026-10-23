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
/** Mirrors service.PublicStatusState on the backend. */
export type PublicStatusState =
  | 'operational'
  | 'degraded'
  | 'down'
  | 'disabled'
  | 'unknown'

/**
 * One checked item. `state` is the only thing the backend reports: probe
 * failure reasons are deliberately not returned (they can contain internal
 * host names), so there is no detail field to render.
 */
export interface PublicStatusComponent {
  key: string
  state: PublicStatusState
  /** Only meaningful for log_database: same instance as the primary database. */
  shared?: boolean
}

/**
 * Request-log statistics over the window named by `window_seconds`.
 *
 * `requests`, `errors` and `success_rate` are `null` when the backend cannot
 * prove them (the corresponding log switch is off, or the count failed). The
 * page must render that as "not available" — never as 0 or 100%.
 */
export interface PublicStatusTraffic {
  window_seconds: number
  requests: number | null
  errors: number | null
  success_rate: number | null
  consume_logging_enabled: boolean
  error_logging_enabled: boolean
}

interface PublicStatusLoop {
  name: string
  /** Unix seconds of the loop's last completed tick. */
  last_run_at: number
}

export interface PublicStatusBackground {
  /** The window the "is anything running" answer is defined by. */
  activity_window_seconds: number
  /** 0 when no loop has recorded a heartbeat yet. */
  most_recent_run_at: number
  loops: PublicStatusLoop[]
}

export interface PublicStatusData {
  status: 'operational' | 'degraded'
  version: string
  started_at: number
  uptime_seconds: number
  generated_at: number
  components: PublicStatusComponent[]
  traffic: PublicStatusTraffic
  background: PublicStatusBackground
}

export interface PublicStatusResponse {
  success: boolean
  message?: string
  data?: PublicStatusData
}
