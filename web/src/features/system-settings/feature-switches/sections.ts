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
 * Lightweight section metadata for /system-settings/feature-switches.
 *
 * Routes and the sidebar only need ids and titles. They must NOT import
 * section-registry.tsx: that module statically imports every settings component
 * of this group, which pulled ~139KB gzip of admin-only code into the entry
 * chunk. __tests__/section-metadata.test.ts keeps this list and the registry in
 * sync.
 */
const FEATURE_SWITCH_SECTIONS = [
  { id: 'switches', titleKey: 'Feature Switches' },
] as const

export type FeatureSwitchSectionId = (typeof FEATURE_SWITCH_SECTIONS)[number]['id']

export const FEATURE_SWITCH_SECTION_IDS = FEATURE_SWITCH_SECTIONS.map((section) => section.id)

export const FEATURE_SWITCH_DEFAULT_SECTION: FeatureSwitchSectionId = 'switches'

export function getFeatureSwitchSectionNavItems(t: TFunction) {
  return FEATURE_SWITCH_SECTIONS.map((section) => ({
    title: t(section.titleKey),
    url: '/system-settings/feature-switches/' + section.id,
  }))
}
