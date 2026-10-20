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
 * Lightweight section metadata for /system-settings/models.
 *
 * Routes and the sidebar only need ids and titles. They must NOT import
 * section-registry.tsx: that module statically imports every settings component
 * of this group, which pulled ~139KB gzip of admin-only code into the entry
 * chunk. __tests__/section-metadata.test.ts keeps this list and the registry in
 * sync.
 */
export const MODELS_SECTIONS = [
  { id: 'global', titleKey: 'Global Model Configuration' },
  { id: 'routing-reliability', titleKey: 'Routing Reliability' },
  { id: 'gemini', titleKey: 'Gemini' },
  { id: 'claude', titleKey: 'Claude' },
  { id: 'grok', titleKey: 'Grok' },
  { id: 'channel-affinity', titleKey: 'Channel Affinity' },
  { id: 'model-deployment', titleKey: 'Model Deployment' },
] as const

export type ModelsSectionId = (typeof MODELS_SECTIONS)[number]['id']

export const MODELS_SECTION_IDS = MODELS_SECTIONS.map((section) => section.id)

export const MODELS_DEFAULT_SECTION: ModelsSectionId = 'global'

export function getModelsSectionNavItems(t: TFunction) {
  return MODELS_SECTIONS.map((section) => ({
    title: t(section.titleKey),
    url: '/system-settings/models/' + section.id,
  }))
}
