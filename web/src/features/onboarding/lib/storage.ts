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
/**
 * B2-3 first-run onboarding dismissal storage.
 *
 * Keyed per user id so every account decides independently whether to keep
 * seeing the guide. A stored value of "1" means the account dismissed it.
 */
export const ONBOARDING_STORAGE_PREFIX = 'newapi_onboarding_dismissed_'

export function getOnboardingStorageKey(userId: number): string {
  return `${ONBOARDING_STORAGE_PREFIX}${userId}`
}

export function isOnboardingDismissed(userId: number): boolean {
  try {
    return window.localStorage.getItem(getOnboardingStorageKey(userId)) === '1'
  } catch {
    return false
  }
}

export function dismissOnboarding(userId: number): void {
  try {
    window.localStorage.setItem(getOnboardingStorageKey(userId), '1')
  } catch {
    // Storage may be unavailable (private mode); keep the guide visible for
    // this session instead of crashing the authenticated layout.
  }
}