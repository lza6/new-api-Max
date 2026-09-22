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
import { RateLimitSection } from '../request-limits/rate-limit-section'
import { SensitiveWordsSection } from '../request-limits/sensitive-words-section'
import { SSRFSection } from '../request-limits/ssrf-section'
import { TokenLimitSection } from '../request-limits/token-limit-section'
import { GlobalConcurrencySection } from '../request-limits/global-concurrency-section'
import { UserRateLimitSection } from '../request-limits/user-rate-limit-section'
import { RateLimitOverridesSection } from '../request-limits/rate-limit-overrides-section'
import { LoginSessionLimitSection } from './login-session-limit-section'
import type { SecuritySettings } from '../types'
import { createSectionRegistry } from '../utils/section-registry'

const SECURITY_SECTIONS = [
  {
    id: 'rate-limit',
    titleKey: 'Rate Limiting',
    build: (settings: SecuritySettings) => (
      <RateLimitSection
        defaultValues={{
          ModelRequestRateLimitEnabled: settings.ModelRequestRateLimitEnabled,
          ModelRequestRateLimitCount: settings.ModelRequestRateLimitCount,
          ModelRequestRateLimitSuccessCount:
            settings.ModelRequestRateLimitSuccessCount,
          ModelRequestRateLimitDurationMinutes:
            settings.ModelRequestRateLimitDurationMinutes,
          ModelRequestRateLimitGroup: settings.ModelRequestRateLimitGroup,
        }}
      />
    ),
  },
  {
    id: 'global-concurrency',
    titleKey: 'Global Concurrency Bucket',
    build: (settings: SecuritySettings) => (
      <GlobalConcurrencySection
        defaultValues={{
          enabled: settings['relay.global_concurrency_enabled'],
          limit: settings['relay.global_concurrency_limit'],
          queue: settings['relay.global_concurrency_queue'],
          waitTimeout: settings['relay.global_concurrency_wait_timeout'],
        }}
      />
    ),
  },
  {
    id: 'user-rate-limit',
    titleKey: 'User Base Rate Limit',
    build: (settings: SecuritySettings) => (
      <UserRateLimitSection
        defaultValues={{
          enabled:
            settings['relay.user_base_rate_limit_enabled'] ?? true,
          concurrency:
            Number(settings['relay.user_base_concurrency_limit']) || 3,
          rpm: Number(settings['relay.user_base_rpm_limit']) || 120,
          exemptModels: parseExemptModels(settings['relay.user_rate_limit_exempt_models']),
        }}
      />
    ),
  },
  {
    id: 'rate-limit-overrides',
    titleKey: 'Rate Limit Overrides',
    build: () => <RateLimitOverridesSection />,
  },
  {
    id: 'sensitive-words',
    titleKey: 'Sensitive Words',
    build: (settings: SecuritySettings) => (
      <SensitiveWordsSection
        defaultValues={{
          CheckSensitiveEnabled: settings.CheckSensitiveEnabled,
          CheckSensitiveOnPromptEnabled: settings.CheckSensitiveOnPromptEnabled,
          SensitiveWords: settings.SensitiveWords,
        }}
      />
    ),
  },
  {
    id: 'ssrf',
    titleKey: 'SSRF Protection',
    build: (settings: SecuritySettings) => (
      <SSRFSection
        defaultValues={{
          'fetch_setting.enable_ssrf_protection':
            settings['fetch_setting.enable_ssrf_protection'],
          'fetch_setting.allow_private_ip':
            settings['fetch_setting.allow_private_ip'],
          'fetch_setting.domain_filter_mode':
            settings['fetch_setting.domain_filter_mode'],
          'fetch_setting.ip_filter_mode':
            settings['fetch_setting.ip_filter_mode'],
          'fetch_setting.domain_list': settings['fetch_setting.domain_list'],
          'fetch_setting.ip_list': settings['fetch_setting.ip_list'],
          'fetch_setting.allowed_ports':
            settings['fetch_setting.allowed_ports'],
          'fetch_setting.apply_ip_filter_for_domain':
            settings['fetch_setting.apply_ip_filter_for_domain'],
        }}
      />
    ),
  },
  {
    id: 'login-session-limit',
    titleKey: 'Login Session Limit',
    build: (settings: SecuritySettings) => (
      <LoginSessionLimitSection
        defaultValues={{ enabled: settings.SessionLimitEnabled }}
      />
    ),
  },
  {
    id: 'token-limits',
    titleKey: 'Token Limits',
    build: (settings: SecuritySettings) => (
      <TokenLimitSection
        defaultValues={{
          'token_setting.max_user_tokens':
            settings['token_setting.max_user_tokens'],
        }}
      />
    ),
  },
] as const

export type SecuritySectionId = (typeof SECURITY_SECTIONS)[number]['id']

const securityRegistry = createSectionRegistry<
  SecuritySectionId,
  SecuritySettings
>({
  sections: SECURITY_SECTIONS,
  defaultSection: 'rate-limit',
  basePath: '/system-settings/security',
  urlStyle: 'path',
})

function parseExemptModels(raw: string | undefined): string {
  if (!raw) return ''
  try {
    const arr = JSON.parse(raw)
    if (Array.isArray(arr)) return arr.join(', ')
  } catch {
    /* fall through */
  }
  return raw.replace(/^\[/, '').replace(/\]$/, '')
}

export const SECURITY_SECTION_IDS = securityRegistry.sectionIds
export const SECURITY_DEFAULT_SECTION = securityRegistry.defaultSection
export const getSecuritySectionNavItems = securityRegistry.getSectionNavItems
export const getSecuritySectionContent = securityRegistry.getSectionContent
export const getSecuritySectionMeta = securityRegistry.getSectionMeta
