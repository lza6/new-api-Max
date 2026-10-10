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
import { FeatureSwitchList } from './components/feature-switch-list'
import { createSectionRegistry } from '../utils/section-registry'

/**
 * 本分区不消费通用 option 映射（数据来自专用的
 * `/api/option/feature-switches` 接口），但 settings-page 的泛型要求
 * `TSettings` 是 Record，故留一个空壳类型。
 */
export type FeatureSwitchSettings = Record<string, string>

const FEATURE_SWITCH_SECTIONS = [
  {
    id: 'switches',
    titleKey: 'Feature Switches',
    build: () => <FeatureSwitchList />,
  },
] as const

export type FeatureSwitchSectionId =
  (typeof FEATURE_SWITCH_SECTIONS)[number]['id']

const featureSwitchRegistry = createSectionRegistry<
  FeatureSwitchSectionId,
  FeatureSwitchSettings
>({
  sections: FEATURE_SWITCH_SECTIONS,
  defaultSection: 'switches',
  basePath: '/system-settings/feature-switches',
  urlStyle: 'path',
})

export const FEATURE_SWITCH_SECTION_IDS = featureSwitchRegistry.sectionIds
export const FEATURE_SWITCH_DEFAULT_SECTION = featureSwitchRegistry.defaultSection
export const getFeatureSwitchSectionNavItems =
  featureSwitchRegistry.getSectionNavItems
export const getFeatureSwitchSectionContent =
  featureSwitchRegistry.getSectionContent
export const getFeatureSwitchSectionMeta = featureSwitchRegistry.getSectionMeta
