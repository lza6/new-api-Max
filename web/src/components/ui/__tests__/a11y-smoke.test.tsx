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
*/
// T7-C3 a11y 冒烟：关键表单模式（Label+Input+Button）必须通过 axe 扫描。
// 覆盖：颜色对比、aria、表单 label、焦点可见性基础项。
import { render } from '@testing-library/react'
import { axe } from 'vitest-axe'
import { describe, expect, test } from 'vitest'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

function LoginFormFixture() {
  return (
    <form aria-label='Sign in'>
      <div className='space-y-2'>
        <Label htmlFor='username'>Username</Label>
        <Input id='username' placeholder='your name' />
      </div>
      <div className='space-y-2'>
        <Label htmlFor='password'>Password</Label>
        <Input id='password' type='password' placeholder='••••••' />
      </div>
      <Button type='submit'>Sign in</Button>
    </form>
  )
}

describe('a11y smoke (T7 C3)', () => {
  test('login form passes axe scan (labels, contrast, aria)', async () => {
    const { container } = render(<LoginFormFixture />)
    const results = await axe(container)
    expect(results.violations).toEqual([])
  })
})