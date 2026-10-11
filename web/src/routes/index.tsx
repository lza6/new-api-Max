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
import { createFileRoute } from '@tanstack/react-router'

import { Home } from '@/features/home'
import { homePageContentQueryOptions } from '@/features/home/hooks'

export const Route = createFileRoute('/')({
  // 只预热、不 await：XHR 与路由解析/Hero 渲染并发启动，缩短数据到达时间，
  // 但绝不把自定义首页内容重新变成首屏阻塞点。即使管理端配置了自定义首页，
  // 内容也通常在这棵组件树首次渲染前就已 settled，从而避免默认页闪现。
  beforeLoad: ({ context }) => {
    void context.queryClient.prefetchQuery(homePageContentQueryOptions)
  },
  component: Home,
})
