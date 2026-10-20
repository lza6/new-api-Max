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
import type { TFunction } from 'i18next'

/**
 * Lightweight section metadata for /system-settings/security.
 *
 * Routes and the sidebar only need ids and titles. They must NOT import
 * section-registry.tsx: that module statically imports every settings component
 * of this group, which pulled ~139KB gzip of admin-only code into the entry
 * chunk. __tests__/section-metadata.test.ts keeps this list and the registry in
 * sync.
 */
export const SECURITY_SECTIONS = [
  { id: 'rate-limit', titleKey: 'Rate Limiting' },
  { id: 'global-concurrency', titleKey: 'Global Concurrency Bucket' },
  { id: 'request-compression', titleKey: 'Request Body Compression' },
  { id: 'user-rate-limit', titleKey: 'User Base Rate Limit' },
  { id: 'rate-limit-overrides', titleKey: 'Rate Limit Overrides' },
  { id: 'sensitive-words', titleKey: 'Sensitive Words' },
  { id: 'ssrf', titleKey: 'SSRF Protection' },
  { id: 'login-session-limit', titleKey: 'Login Session Limit' },
  { id: 'token-limits', titleKey: 'Token Limits' },
] as const

export type SecuritySectionId = (typeof SECURITY_SECTIONS)[number]['id']

export const SECURITY_SECTION_IDS = SECURITY_SECTIONS.map((section) => section.id)

export const SECURITY_DEFAULT_SECTION: SecuritySectionId = 'rate-limit'

export function getSecuritySectionNavItems(t: TFunction) {
  return SECURITY_SECTIONS.map((section) => ({
    title: t(section.titleKey),
    url: '/system-settings/security/' + section.id,
  }))
}
