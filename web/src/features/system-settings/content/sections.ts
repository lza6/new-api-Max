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
 * Lightweight section metadata for /system-settings/content.
 *
 * Routes and the sidebar only need ids and titles. They must NOT import
 * section-registry.tsx: that module statically imports every settings component
 * of this group, which pulled ~139KB gzip of admin-only code into the entry
 * chunk. __tests__/section-metadata.test.ts keeps this list and the registry in
 * sync.
 */
const CONTENT_SECTIONS = [
  { id: 'dashboard', titleKey: 'Data Dashboard' },
  { id: 'announcements', titleKey: 'Announcements' },
  { id: 'api-info', titleKey: 'API Addresses' },
  { id: 'faq', titleKey: 'FAQ' },
  { id: 'uptime-kuma', titleKey: 'Uptime Kuma' },
  { id: 'chat', titleKey: 'Chat Presets' },
  { id: 'drawing', titleKey: 'Drawing' },
] as const

export type ContentSectionId = (typeof CONTENT_SECTIONS)[number]['id']

export const CONTENT_SECTION_IDS = CONTENT_SECTIONS.map((section) => section.id)

export const CONTENT_DEFAULT_SECTION: ContentSectionId = 'dashboard'

export function getContentSectionNavItems(t: TFunction) {
  return CONTENT_SECTIONS.map((section) => ({
    title: t(section.titleKey),
    url: '/system-settings/content/' + section.id,
  }))
}
