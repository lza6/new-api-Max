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
 * LobeHub Icon Loader
 * Dynamically load and render icons from @lobehub/icons
 *
 * Supports:
 * - Basic: "OpenAI", "OpenAI.Color"
 * - Chained properties: "OpenAI.Avatar.type={'platform'}"
 * - Size parameter: getLobeIcon("OpenAI", 20)
 *
 * Performance: icons are imported on demand (one chunk per icon) instead of
 * pulling the whole @lobehub/icons barrel into the initial/route bundle. Icon
 * name suggestions come from static metadata (`lobe-icon-meta.ts`), so listing
 * available icons never compiles the full icon set either.
 */
import { LobeIcon, LobeIconFallback } from './lobe-icon-component'
import { FALLBACK_SIZE } from './lobe-icon-core'

/**
 * Get a LobeHub icon node by name.
 * @param iconName - Icon name/description (e.g., "OpenAI", "OpenAI.Color", "Claude.Avatar")
 * @param size - Icon size (default: 20)
 *
 * @example
 * getLobeIcon("OpenAI", 24)
 * getLobeIcon("OpenAI.Color", 20)
 * getLobeIcon("Claude.Avatar.type={'platform'}", 32)
 */
export function getLobeIcon(
  iconName: string | undefined | null,
  size: number = FALLBACK_SIZE
): React.ReactNode {
  if (!iconName || typeof iconName !== 'string' || !iconName.trim()) {
    return <LobeIconFallback size={size} />
  }
  return <LobeIcon name={iconName} size={size} />
}

// The selector uses the same installed icon registry as the renderer.
export { getLobeIconNames } from './lobe-icon-core'
