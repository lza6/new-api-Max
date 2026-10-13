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
// 图片工具函数。**单独成文件**而不是与 `chat-image.tsx` 放一起：
// 组件文件混导出非组件会让 React Fast Refresh 失效
// （`react(only-export-components)`）。

/**
 * 从 URL 猜一个可用的下载文件名。
 *
 * 取路径末段并忽略 query/hash：上游图片 URL 普遍带签名参数
 * （如 `.../a.png?X-Amz-Signature=...`），带上它们既不是合法文件名，
 * 也会让浏览器把整串当扩展名处理。
 *
 * 末段不含扩展名时返回 `fallback`——下载一个没有扩展名的文件会让系统
 * 无法用图片查看器打开。
 */
export function guessImageFileName(src: string, fallback = 'image.png'): string {
  const withoutQuery = src.split('?')[0].split('#')[0]
  // 去掉尾部斜杠再取末段（上游 URL 有时以 `/` 结尾，直接取末段会得到空串）。
  const trimmed = withoutQuery.replace(/\/+$/, '')
  const segments = trimmed.split('/')
  const last = segments.at(-1) ?? ''
  if (/\.[a-z0-9]{2,5}$/i.test(last)) {
    return last
  }
  return fallback
}
