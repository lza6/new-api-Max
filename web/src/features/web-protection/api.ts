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

export interface WebProtectionSettings {
  enabled: boolean
  limit_per_second: number
  burst: number
  auto_ban: boolean
  auto_ban_threshold_per_minute: number
  auto_ban_minutes: number
  log_enabled: boolean
  window_seconds: number
}

export interface WebRequestLogRow {
  ip: string
  request_count: number
  rate_per_second: number
  bytes_total: number
  last_request_at: number
  banned: boolean
  ban_expires_at: number
}

export interface WebRequestLogDetailRow {
  ip: string; path: string; method: string; status: number;
  request_count: number; bytes_sent: number; bytes_received: number;
  window_start: number; user_agent: string
}

export interface BannedIPRow {
  id: number; ip: string; reason: string; banned_at: number;
  expires_at: number; banned_by: string
}

export function getWebProtectionSettings(): Promise<WebProtectionSettings> {
  return api.get('/api/admin/web-protection/settings').then((r) => r.data.data)
}

export function updateWebProtectionSettings(patch: Partial<WebProtectionSettings>) {
  return api.put('/api/admin/web-protection/settings', patch)
}

export async function getWebRequestLogs(params: {
  page?: number; size?: number; ip?: string; sort?: string; order?: string
}): Promise<{ items: WebRequestLogRow[]; total: number }> {
  const r = await api.get('/api/admin/web-request-logs', { params });
  return r.data.data;
}

export async function getWebRequestLogDetail(ip: string, page = 1, size = 50) {
  const r = await api.get('/api/admin/web-request-logs/detail', { params: { ip, page, size } });
  return r.data.data;
}

export async function getBannedIPs(page = 1, size = 50) {
  const r = await api.get('/api/admin/banned-ips', { params: { page, size } });
  return r.data.data;
}

export function banIP(ip: string, minutes: number, reason: string) {
  return api.post('/api/admin/banned-ips', { ip, minutes, reason });
}

export function unbanIP(ip: string) {
  return api.post('/api/admin/banned-ips/unban', { ip });
}

export function formatBytes(n: number): string {
  if (!Number.isFinite(n) || n < 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let v = n; let i = 0;
  while (v >= 1024 && i < units.length - 1) { v /= 1024; i++; }
  return v.toFixed(v >= 100 || i === 0 ? 0 : 1) + ' ' + units[i];
}
