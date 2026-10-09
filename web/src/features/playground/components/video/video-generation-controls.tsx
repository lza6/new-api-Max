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
// P1-6 视频生成参数（时长 / 分辨率）。仅在视频模型下显示。
import { useTranslation } from 'react-i18next'

import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

import { VIDEO_SECONDS_OPTIONS, VIDEO_SIZE_OPTIONS } from '../../constants'

type VideoGenerationControlsProps = {
  seconds: string
  size: string
  disabled?: boolean
  onSecondsChange: (value: string) => void
  onSizeChange: (value: string) => void
}

/** 空串在 Select 里需要一个非空哨兵值（Radix 不允许空字符串作为 item value）。 */
const UNSET = '__unset__'

export function VideoGenerationControls(props: VideoGenerationControlsProps) {
  const { t } = useTranslation()

  return (
    <div className='flex flex-wrap items-center gap-2'>
      <Select
        value={props.seconds === '' ? UNSET : props.seconds}
        onValueChange={(value) =>
          props.onSecondsChange(value === null || value === UNSET ? '' : value)
        }
      >
        <SelectTrigger
          aria-label={t('Video duration')}
          className='h-8 w-32 text-xs'
          disabled={props.disabled}
        >
          <SelectValue placeholder={t('Duration')} />
        </SelectTrigger>
        <SelectContent>
          {VIDEO_SECONDS_OPTIONS.map((value) => (
            <SelectItem key={value || UNSET} value={value === '' ? UNSET : value}>
              {value === '' ? t('Default duration') : `${value}s`}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>

      <Select
        value={props.size === '' ? UNSET : props.size}
        onValueChange={(value) =>
          props.onSizeChange(value === null || value === UNSET ? '' : value)
        }
      >
        <SelectTrigger
          aria-label={t('Video resolution')}
          className='h-8 w-36 text-xs'
          disabled={props.disabled}
        >
          <SelectValue placeholder={t('Resolution')} />
        </SelectTrigger>
        <SelectContent>
          {VIDEO_SIZE_OPTIONS.map((value) => (
            <SelectItem key={value || UNSET} value={value === '' ? UNSET : value}>
              {value === '' ? t('Default resolution') : value}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  )
}
