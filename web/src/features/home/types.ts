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
// ============================================================================
// Home Page Types
// ============================================================================

/**
 * Response from home page content API
 */
export interface HomePageContentResponse {
  success: boolean
  message?: string
  data?: string
}

/**
 * Home page content result from hook
 */
export interface HomePageContentResult {
  content: string
  isLoaded: boolean
  isUrl: boolean
}

/**
 * Site-wide authoritative statistics from GET /api/site/stats
 */
export interface SiteStatsResponse {
  success: boolean
  message?: string
  data?: {
    days: number
    total_requests: number
    total_bytes: number
    total_bytes_text: string
    total_tokens: number
    total_quota: number
  }
}

/** Visitor counts (deduplicated per IP per day, bots excluded). */
export interface VisitorCounts {
  today: number
  last_7d: number
  last_30d: number
  total: number
  page_views_today: number
  page_views_total: number
  page_views_last_30d: number
}

/** One row of the 30-day traffic-source breakdown. */
export interface TrafficSource {
  source: string
  count: number
}

/**
 * Site statistics from GET /api/site/visitors.
 *
 * Every field is an aggregate count — the endpoint returns no IPs, no hashes
 * and no user identifiers (see the backend's `model.SiteDailyVisitor`).
 */
export interface SiteVisitorSummary {
  visitors: VisitorCounts
  registered_users: number
  active_users_24h: number
  active_users_30d: number
  sources: TrafficSource[]
  generated_at: number
}

export interface SiteVisitorSummaryResponse {
  success: boolean
  message?: string
  data?: SiteVisitorSummary
}
