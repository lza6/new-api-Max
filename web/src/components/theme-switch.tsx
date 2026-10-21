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
import { Check, Moon, Sun } from 'lucide-react'
import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { useTheme } from '@/context/theme-provider'
import { cn } from '@/lib/utils'

export function ThemeSwitch() {
  const { t } = useTranslation()
  const { theme, setTheme, resolvedTheme } = useTheme()

  /* Update theme-color meta tags when the theme changes.
   * index.html ships two media-scoped tags (light/dark); the old code only
   * rewrote the first one with a hard-coded #020817 that matched neither.
   * Now every tag gets the value of the `--theme-color` token, read after the
   * `.dark` class has been applied by the theme provider. */
  useEffect(() => {
    const frame = requestAnimationFrame(() => {
      const token = getComputedStyle(document.documentElement)
        .getPropertyValue('--theme-color')
        .trim()
      const color = token || (resolvedTheme === 'dark' ? '#0a0a0a' : '#ffffff')
      document
        .querySelectorAll("meta[name='theme-color']")
        .forEach((meta) => meta.setAttribute('content', color))
    })
    return () => cancelAnimationFrame(frame)
  }, [resolvedTheme])

  return (
    <DropdownMenu modal={false}>
      <DropdownMenuTrigger
        render={<Button variant='ghost' size='icon' className='h-9 w-9' />}
      >
        <Sun className='size-[1.2rem] scale-100 rotate-0 transition-all dark:scale-0 dark:-rotate-90' />
        <Moon className='absolute size-[1.2rem] scale-0 rotate-90 transition-all dark:scale-100 dark:rotate-0' />
        <span className='sr-only'>{t('Toggle theme')}</span>
      </DropdownMenuTrigger>
      <DropdownMenuContent align='end'>
        <DropdownMenuItem onClick={() => setTheme('light')}>
          {t('Light')}{' '}
          <Check
            size={14}
            className={cn('ms-auto', theme !== 'light' && 'hidden')}
          />
        </DropdownMenuItem>
        <DropdownMenuItem onClick={() => setTheme('dark')}>
          {t('Dark')}
          <Check
            size={14}
            className={cn('ms-auto', theme !== 'dark' && 'hidden')}
          />
        </DropdownMenuItem>
        <DropdownMenuItem onClick={() => setTheme('system')}>
          {t('System')}
          <Check
            size={14}
            className={cn('ms-auto', theme !== 'system' && 'hidden')}
          />
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
