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
import { describe, expect, it } from 'vitest'

import {
  AUTH_SECTION_IDS,
  getAuthSectionNavItems,
} from '../auth/sections'
import {
  AUTH_SECTION_IDS as AUTH_SECTION_IDS_REGISTRY,
  getAuthSectionNavItems as getAuthSectionNavItemsRegistry,
} from '../auth/section-registry'
import {
  BILLING_SECTION_IDS,
  getBillingSectionNavItems,
} from '../billing/sections'
import {
  BILLING_SECTION_IDS as BILLING_SECTION_IDS_REGISTRY,
  getBillingSectionNavItems as getBillingSectionNavItemsRegistry,
} from '../billing/section-registry'
import {
  CONTENT_SECTION_IDS,
  getContentSectionNavItems,
} from '../content/sections'
import {
  CONTENT_SECTION_IDS as CONTENT_SECTION_IDS_REGISTRY,
  getContentSectionNavItems as getContentSectionNavItemsRegistry,
} from '../content/section-registry'
import {
  FEATURE_SWITCH_SECTION_IDS,
  getFeatureSwitchSectionNavItems,
} from '../feature-switches/sections'
import {
  FEATURE_SWITCH_SECTION_IDS as FEATURE_SWITCH_SECTION_IDS_REGISTRY,
  getFeatureSwitchSectionNavItems as getFeatureSwitchSectionNavItemsRegistry,
} from '../feature-switches/section-registry'
import { MODELS_SECTION_IDS, getModelsSectionNavItems } from '../models/sections'
import {
  MODELS_SECTION_IDS as MODELS_SECTION_IDS_REGISTRY,
  getModelsSectionNavItems as getModelsSectionNavItemsRegistry,
} from '../models/section-registry'
import {
  OPERATIONS_SECTION_IDS,
  getOperationsSectionNavItems,
} from '../operations/sections'
import {
  OPERATIONS_SECTION_IDS as OPERATIONS_SECTION_IDS_REGISTRY,
  getOperationsSectionNavItems as getOperationsSectionNavItemsRegistry,
} from '../operations/section-registry'
import {
  SECURITY_SECTION_IDS,
  getSecuritySectionNavItems,
} from '../security/sections'
import {
  SECURITY_SECTION_IDS as SECURITY_SECTION_IDS_REGISTRY,
  getSecuritySectionNavItems as getSecuritySectionNavItemsRegistry,
} from '../security/section-registry'
import { SITE_SECTION_IDS, getSiteSectionNavItems } from '../site/sections'
import {
  SITE_SECTION_IDS as SITE_SECTION_IDS_REGISTRY,
  getSiteSectionNavItems as getSiteSectionNavItemsRegistry,
} from '../site/section-registry'

type NavFn = (t: TFunction) => { title: string; url: string }[]

/**
 * Routes and the sidebar read section ids/titles from the lightweight
 * `sections.ts` modules; the page components still build from
 * `section-registry.tsx`. Two sources only stay safe while they agree — this
 * test is the contract. A drift here shows up as a redirect to a non-existent
 * section (or a sidebar entry that renders nothing), which is exactly the kind
 * of breakage the entry-chunk split could introduce.
 */
function expectInSync(
  metaIds: readonly string[],
  metaNav: NavFn,
  registryIds: readonly string[],
  registryNav: NavFn
) {
  const t = ((key: string) => `T:${key}`) as unknown as TFunction
  expect([...metaIds]).toEqual([...registryIds])
  expect(metaNav(t)).toEqual(registryNav(t))
}

describe('system-settings section metadata', () => {
  it('auth matches its registry', () => {
    expectInSync(
      AUTH_SECTION_IDS,
      getAuthSectionNavItems,
      AUTH_SECTION_IDS_REGISTRY,
      getAuthSectionNavItemsRegistry
    )
  })

  it('billing matches its registry', () => {
    expectInSync(
      BILLING_SECTION_IDS,
      getBillingSectionNavItems,
      BILLING_SECTION_IDS_REGISTRY,
      getBillingSectionNavItemsRegistry
    )
  })

  it('content matches its registry', () => {
    expectInSync(
      CONTENT_SECTION_IDS,
      getContentSectionNavItems,
      CONTENT_SECTION_IDS_REGISTRY,
      getContentSectionNavItemsRegistry
    )
  })

  it('feature switches match their registry', () => {
    expectInSync(
      FEATURE_SWITCH_SECTION_IDS,
      getFeatureSwitchSectionNavItems,
      FEATURE_SWITCH_SECTION_IDS_REGISTRY,
      getFeatureSwitchSectionNavItemsRegistry
    )
  })

  it('models matches its registry', () => {
    expectInSync(
      MODELS_SECTION_IDS,
      getModelsSectionNavItems,
      MODELS_SECTION_IDS_REGISTRY,
      getModelsSectionNavItemsRegistry
    )
  })

  it('operations matches its registry', () => {
    expectInSync(
      OPERATIONS_SECTION_IDS,
      getOperationsSectionNavItems,
      OPERATIONS_SECTION_IDS_REGISTRY,
      getOperationsSectionNavItemsRegistry
    )
  })

  it('security matches its registry', () => {
    expectInSync(
      SECURITY_SECTION_IDS,
      getSecuritySectionNavItems,
      SECURITY_SECTION_IDS_REGISTRY,
      getSecuritySectionNavItemsRegistry
    )
  })

  it('site matches its registry', () => {
    expectInSync(
      SITE_SECTION_IDS,
      getSiteSectionNavItems,
      SITE_SECTION_IDS_REGISTRY,
      getSiteSectionNavItemsRegistry
    )
  })
})
