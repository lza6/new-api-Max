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
import { describe, expect, test } from 'vitest'

import { parseHeaderNavModules } from './nav-modules'

describe('header nav modules (model test entry)', () => {
  test('modelTest defaults to visible when the backend config omits it', () => {
    const modules = parseHeaderNavModules(
      JSON.stringify({ home: true, console: true })
    )
    expect(modules.modelTest).toBe(true)
  })

  test('modelTest stays visible when explicitly enabled', () => {
    const modules = parseHeaderNavModules(
      JSON.stringify({ home: true, modelTest: true })
    )
    expect(modules.modelTest).toBe(true)
  })

  test('modelTest can be disabled from the admin config', () => {
    const modules = parseHeaderNavModules(
      JSON.stringify({ home: true, modelTest: false })
    )
    expect(modules.modelTest).toBe(false)
  })
})