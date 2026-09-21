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
 * Mobile touch-target compliance (WCAG 2.5.5 / mobile HIG >= 44px).
 *
 * Only applied when the pointer is coarse (touch screens) via an arbitrary
 * media variant, so desktop density is untouched. Compact sizes used inside
 * dense tables (`xs`/`sm`/`icon-xs`/`icon-sm`) are exempt on purpose; the
 * dense layouts they live in cannot afford 44px row growth.
 */
const COARSE_POINTER_MIN_SIZE =
  '[@media(pointer:coarse)]:min-h-11 [@media(pointer:coarse)]:min-w-11'

const COMPACT_BUTTON_SIZES = new Set(['xs', 'sm', 'icon-xs', 'icon-sm'])

export function getCoarsePointerTouchTargetClass(size: string): string {
  return COMPACT_BUTTON_SIZES.has(size) ? '' : COARSE_POINTER_MIN_SIZE
}
