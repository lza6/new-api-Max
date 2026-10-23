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
import * as React from 'react'

import { cn } from '@/lib/utils'

/**
 * Surface tones (计划书/视觉识别体系 §4.1)。默认 `default` 保持既有观感不变；
 * 其余三档用「表面明度 + 规则线」表达层级，取代阴影堆叠：
 *   panel  — 控制台主容器（surface-2 + rule）
 *   plain  — 无背景无边框，只做分组（分区靠 --rule）
 *   inset  — Card 内部二级区（surface-1 + rule）
 */
type CardTone = 'default' | 'panel' | 'plain' | 'inset'

const CARD_TONE_CLASSES: Record<CardTone, string> = {
  default: '',
  panel: 'bg-surface-2 border-rule border ring-0 shadow-none',
  plain: 'border-0 bg-transparent shadow-none ring-0',
  inset: 'bg-surface-1 border-rule border ring-0 shadow-none',
}

function Card({
  className,
  size = 'default',
  tone = 'default',
  ...props
}: React.ComponentProps<'div'> & {
  size?: 'default' | 'sm'
  tone?: CardTone
}) {
  return (
    <div
      data-slot='card'
      data-size={size}
      data-tone={tone}
      className={cn(
        'group/card bg-card text-card-foreground ring-foreground/10 flex flex-col gap-4 overflow-hidden rounded-xl py-4 text-sm ring-1 has-data-[slot=card-footer]:pb-0 has-[>img:first-child]:pt-0 data-[size=sm]:gap-3 data-[size=sm]:py-3 data-[size=sm]:has-data-[slot=card-footer]:pb-0 *:[img:first-child]:rounded-t-xl *:[img:last-child]:rounded-b-xl',
        CARD_TONE_CLASSES[tone],
        className
      )}
      {...props}
    />
  )
}

function CardHeader({ className, ...props }: React.ComponentProps<'div'>) {
  return (
    <div
      data-slot='card-header'
      className={cn(
        'group/card-header @container/card-header grid auto-rows-min items-start gap-1 rounded-t-xl px-4 group-data-[size=sm]/card:px-3 has-data-[slot=card-action]:grid-cols-[1fr_auto] has-data-[slot=card-description]:grid-rows-[auto_auto] [.border-b]:pb-4 group-data-[size=sm]/card:[.border-b]:pb-3',
        className
      )}
      {...props}
    />
  )
}

function CardTitle({ className, ...props }: React.ComponentProps<'div'>) {
  return (
    <div
      data-slot='card-title'
      className={cn(
        'text-base leading-snug font-medium group-data-[size=sm]/card:text-sm',
        className
      )}
      {...props}
    />
  )
}

function CardDescription({ className, ...props }: React.ComponentProps<'div'>) {
  return (
    <div
      data-slot='card-description'
      className={cn('text-muted-foreground text-sm', className)}
      {...props}
    />
  )
}

function CardAction({ className, ...props }: React.ComponentProps<'div'>) {
  return (
    <div
      data-slot='card-action'
      className={cn(
        'col-start-2 row-span-2 row-start-1 self-start justify-self-end',
        className
      )}
      {...props}
    />
  )
}

function CardContent({ className, ...props }: React.ComponentProps<'div'>) {
  return (
    <div
      data-slot='card-content'
      className={cn('px-4 group-data-[size=sm]/card:px-3', className)}
      {...props}
    />
  )
}

function CardFooter({ className, ...props }: React.ComponentProps<'div'>) {
  return (
    <div
      data-slot='card-footer'
      className={cn(
        'bg-muted/50 flex items-center rounded-b-xl border-t p-4 group-data-[size=sm]/card:p-3',
        className
      )}
      {...props}
    />
  )
}

export {
  Card,
  CardHeader,
  CardFooter,
  CardTitle,
  CardAction,
  CardDescription,
  CardContent,
}
