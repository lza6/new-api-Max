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
import { useQuery } from '@tanstack/react-query'
import i18next from 'i18next'

import { isHttpUrl } from '@/lib/content-format'
import { handleServerError } from '@/lib/handle-server-error'

import { getHomePageContent } from '../api'
import type { HomePageContentResult } from '../types'

const STORAGE_KEY = 'home_page_content'

/**
 * 自定义首页内容的缓存读取。
 *
 * 同步读 localStorage 是刻意的：重复访客首帧就能拿到上次的自定义首页，
 * 不必等 XHR 往返，也不必先渲染默认落地页再闪切。
 */
function readCachedHomeContent(): string {
  try {
    return localStorage.getItem(STORAGE_KEY) ?? ''
  } catch {
    return ''
  }
}

async function fetchHomePageContent(): Promise<string> {
  try {
    const response = await getHomePageContent()
    const content = response.success && response.data ? response.data : ''
    if (content) {
      localStorage.setItem(STORAGE_KEY, content)
    } else {
      localStorage.removeItem(STORAGE_KEY)
    }
    return content
  } catch (error) {
    // 保留原有提示文案；失败时按「无自定义首页」处理，让默认落地页照常渲染。
    handleServerError(
      error,
      i18next.t('Failed to load home page content')
    )
    return ''
  }
}

/**
 * 自定义首页内容的查询配置。
 *
 * 独立导出，供首页路由在 `beforeLoad` 里预热：请求与路由解析并发启动但不
 * await，因此只缩短数据到达时间，不会把 XHR 变成新的渲染阻塞。
 *
 * `initialData` 来自 localStorage（重复访客首帧即有内容），`initialDataUpdatedAt: 0`
 * 让这份缓存一挂载就被视为过期并触发一次后台校验，管理端的改动仍然及时生效。
 */
export const homePageContentQueryOptions = {
  queryKey: ['home-page-content'] as const,
  queryFn: fetchHomePageContent,
  initialData: readCachedHomeContent,
  initialDataUpdatedAt: 0,
  staleTime: 5 * 60 * 1000,
  // 与改造前一致：一次请求定胜负，失败即渲染默认落地页，不做重试。
  retry: false,
  // 失败提示已在 queryFn 内以原文案上报，避免 QueryCache 再弹一次。
  meta: { errorToast: false },
}

/**
 * Hook to load and manage custom home page content
 * Supports both Markdown/HTML content and iframe URLs
 */
export function useHomePageContent(): HomePageContentResult {
  const query = useQuery(homePageContentQueryOptions)
  const content = query.data ?? ''

  return {
    content,
    // 语义：服务端是否已确认过内容（false 表示首次请求仍在路上）。
    isLoaded: query.isFetched,
    isUrl: isHttpUrl(content),
  }
}
