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
import { createFileRoute, redirect } from '@tanstack/react-router'
import { lazy } from 'react'

import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

const TrainingDataPage = lazy(() =>
  import('@/features/training-data').then((m) => ({
    default: m.TrainingDataPage,
  }))
)

// 训练数据采集管理页。
//
// 角色门禁与后端一致：服务端的这些接口挂在 AdminAuth() 下，
// 前端同样拦住非管理员 —— 只是为了让用户看到 403 而不是一堆失败请求，
// **真正的鉴权在服务端**（前端门禁不构成安全边界）。
export const Route = createFileRoute('/_authenticated/training-data/')({
  beforeLoad: () => {
    const { auth } = useAuthStore.getState()
    const role = auth.user?.role
    if (role !== ROLE.ADMIN && role !== ROLE.SUPER_ADMIN) {
      throw redirect({ to: '/403' })
    }
  },
  component: TrainingDataPage,
})
