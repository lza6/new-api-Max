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
import type { ReactNode } from 'react'

import { cn } from '@/lib/utils'

/**
 * 读数（Metric）排版件 —— 计划书/视觉识别体系 §4.2。
 *
 * 存在的理由：控制台每个数字都是"读数"，但此前 100+ 个文件各自手写
 * `text-2xl font-semibold tabular-nums`，单位字号、空值、正负号、对齐全靠自觉。
 * 这四个槽位把数字排版收敛成唯一实现：
 *   Metric       纵向容器（label → value → delta/hint）
 *   MetricLabel  11px muted 微标签（不依赖 uppercase，中文同样成立）
 *   MetricValue  等宽数字 + 小一号单位 + 空值占位
 *   MetricDelta  方向必须同时有图标与读屏文本（不得只靠颜色）
 */
export function Metric(props: { children: ReactNode; className?: string }) {
  return (
    <div
      data-slot='metric'
      className={cn('flex min-w-0 flex-col gap-1', props.className)}
    >
      {props.children}
    </div>
  )
}

export function MetricLabel(props: { children: ReactNode; className?: string }) {
  return (
    <div
      data-slot='metric-label'
      className={cn(
        'text-muted-foreground truncate text-[11px] leading-4 font-medium tracking-wide sm:text-xs',
        props.className
      )}
    >
      {props.children}
    </div>
  )
}

type MetricValueSize = 'sm' | 'md' | 'lg'

const METRIC_VALUE_SIZE_CLASSES: Record<MetricValueSize, string> = {
  sm: 'text-base leading-tight',
  md: 'text-xl leading-tight sm:text-2xl sm:leading-none',
  lg: 'text-2xl leading-none sm:text-3xl',
}

const EMPTY_PLACEHOLDER = '—'

interface MetricValueProps {
  children?: ReactNode
  /** 单位（额度 / M / ms…），渲染为小一号的次要色，不参与数字对齐 */
  unit?: string
  size?: MetricValueSize
  /** 截断前的全量文本，用于 title 与读屏 */
  title?: string
  /** 空值占位，默认 `—`（禁渲染 `--` / `N/A` / `0` 冒充无数据） */
  emptyValue?: string
  className?: string
}

function isEmptyValue(value: ReactNode): boolean {
  return (
    value === undefined ||
    value === null ||
    value === '' ||
    value === '--' ||
    value === EMPTY_PLACEHOLDER
  )
}

export function MetricValue(props: MetricValueProps) {
  const empty = isEmptyValue(props.children)
  const display = empty
    ? (props.emptyValue ?? EMPTY_PLACEHOLDER)
    : props.children
  const title = props.title ?? (empty ? undefined : String(props.children))

  return (
    <div
      data-slot='metric-value'
      data-empty={empty}
      className={cn(
        'flex min-w-0 items-baseline gap-1',
        METRIC_VALUE_SIZE_CLASSES[props.size ?? 'md'],
        props.className
      )}
    >
      <span
        className='font-numeric text-foreground truncate font-semibold tabular-nums tracking-tight'
        title={title}
      >
        {display}
      </span>
      {props.unit !== undefined && (
        <span className='text-muted-foreground shrink-0 text-[0.62em] font-medium'>
          {props.unit}
        </span>
      )}
    </div>
  )
}
