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
import { useTranslation } from 'react-i18next'

import { BadgeCell } from '@/components/data-table/core/badge-cell'
import { StatusBadge } from '@/components/status-badge'

// 注册来源（后端 User.Source）→ 展示标签 i18n key 与徽章配色。
// 未知/自定义 source 回退为原始值显示，避免枚举缺失时空白。
const SOURCE_LABEL_KEYS: Record<string, { labelKey: string; variant: 'success' | 'neutral' | 'info' | 'warning' | 'danger' }> = {
  password: { labelKey: 'Sign up with password', variant: 'success' },
  admin: { labelKey: 'Created by admin', variant: 'neutral' },
  github: { labelKey: 'GitHub', variant: 'info' },
  discord: { labelKey: 'Discord', variant: 'info' },
  wechat: { labelKey: 'WeChat', variant: 'info' },
  telegram: { labelKey: 'Telegram', variant: 'info' },
  linuxdo: { labelKey: 'LinuxDo', variant: 'info' },
  oidc: { labelKey: 'OAuth / OIDC', variant: 'info' },
}

interface SourceBadgeProps {
  source: string
}

export function SourceBadge({ source }: SourceBadgeProps) {
  const { t } = useTranslation()
  const config = SOURCE_LABEL_KEYS[source]

  return (
    <BadgeCell>
      <StatusBadge
        variant={config?.variant ?? 'neutral'}
        label={config ? t(config.labelKey) : source}
        copyable={false}
        className='font-normal'
      />
    </BadgeCell>
  )
}
