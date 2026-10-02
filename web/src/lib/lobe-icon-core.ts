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
 * Icon resolution core for @lobehub/icons.
 *
 * Icons are imported on demand (one chunk per icon) instead of pulling the
 * whole barrel into the bundle. Icon name suggestions come from static metadata
 * (`lobe-icon-meta.ts`), so listing icons never compiles the icon set either.
 */
import { IconSub2api } from '@/assets/custom/icon-sub2api'
import { LOBE_ICON_META } from '@/lib/lobe-icon-meta'

type IconProps = Record<string, unknown>
export type IconComponent = React.ComponentType<IconProps>

export type ResolvedIcon = {
  Comp: IconComponent
  props: IconProps
}

export const FALLBACK_SIZE = 20

export const CUSTOM_ICONS: Record<string, IconComponent> = {
  Sub2API: IconSub2api as unknown as IconComponent,
}

// One lazy chunk per icon module. `import.meta.glob` is expanded at build time,
// so each entry stays a separate dynamic import and only requested icons load.
const iconModules = import.meta.glob<{ default: unknown }>(
  '/node_modules/@lobehub/icons/es/*/index.js'
)

function loaderForIcon(id: string): (() => Promise<{ default: unknown }>) | null {
  const suffix = `/@lobehub/icons/es/${id}/index.js`
  for (const key of Object.keys(iconModules)) {
    if (key.endsWith(suffix)) {
      return iconModules[key]
    }
  }
  return null
}

async function loadLobeIcon(id: string): Promise<IconComponent | null> {
  const loader = loaderForIcon(id)
  if (!loader) {return null}
  try {
    const mod = await loader()
    const Component = mod?.default
    return typeof Component === 'function'
      ? (Component as IconComponent)
      : (Component as unknown as IconComponent) ?? null
  } catch {
    return null
  }
}

/**
 * Resolve an icon name to a component + props, loading the icon module on
 * demand. Mirrors the historical resolution semantics: `OpenAI.Text` selects the
 * `Text` sub-component, trailing `key=value` segments become props.
 */
export async function resolveIcon(
  iconName: string,
  size: number
): Promise<ResolvedIcon | null> {
  const segments = iconName.trim().split('.')
  const baseKey = segments[0]
  if (!baseKey) {return null}

  const CustomIcon = CUSTOM_ICONS[baseKey]
  if (CustomIcon) {
    return { Comp: CustomIcon, props: { size } }
  }

  const loaded = await loadLobeIcon(baseKey)
  if (!loaded) {return null}

  const baseIcon = loaded as unknown as Record<string, unknown>
  let Comp: IconComponent
  let propStartIndex: number

  if (segments.length > 1 && baseIcon[segments[1]]) {
    Comp = baseIcon[segments[1]] as IconComponent
    propStartIndex = 2
  } else {
    Comp = loaded
    propStartIndex = segments.length > 1 && /^[A-Z]/.test(segments[1]) ? 2 : 1
  }

  const props: IconProps = {}
  for (let i = propStartIndex; i < segments.length; i++) {
    const seg = segments[i]
    if (!seg) {continue}

    const eqIdx = seg.indexOf('=')
    if (eqIdx === -1) {
      props[seg.trim()] = true
      continue
    }

    const key = seg.slice(0, eqIdx).trim()
    let v = seg.slice(eqIdx + 1).trim()
    if (v.startsWith('{') && v.endsWith('}')) {v = v.slice(1, -1).trim()}
    if (
      (v.startsWith('"') && v.endsWith('"')) ||
      (v.startsWith("'") && v.endsWith("'"))
    ) {
      props[key] = v.slice(1, -1)
    } else if (v === 'true') {
      props[key] = true
    } else if (v === 'false') {
      props[key] = false
    } else if (/^-?\d+(?:\.\d+)?$/.test(v)) {
      props[key] = Number(v)
    } else {
      props[key] = v
    }
  }

  if (props.size == null) {
    props.size = size
  }

  return { Comp, props }
}

// Static metadata: available icon names, without pulling icon components.
export function getLobeIconNames(): string[] {
  const names = LOBE_ICON_META.flatMap((icon) =>
    icon.hasColor ? [icon.id, `${icon.id}.Color`] : [icon.id]
  )
  return [...new Set([...names, ...Object.keys(CUSTOM_ICONS)])].sort()
}
