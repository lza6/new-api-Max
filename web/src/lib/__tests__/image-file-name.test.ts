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
import { describe, expect, test } from 'vitest'

import { guessImageFileName } from '../image-file-name'

describe('guessImageFileName', () => {
  test('takes the last path segment for a plain URL', () => {
    expect(guessImageFileName('https://cdn.example.com/a/b/photo.png')).toBe(
      'photo.png'
    )
  })

  test('strips the query string so signed URLs do not leak into the filename', () => {
    // 上游图片 URL 普遍带签名参数；带上它们不是合法文件名，也会让浏览器
    // 把整串当扩展名。
    expect(
      guessImageFileName('https://cdn.example.com/photo.webp?X-Amz-Signature=abc123&Expires=9')
    ).toBe('photo.webp')
  })

  test('strips the hash fragment', () => {
    expect(guessImageFileName('https://cdn.example.com/photo.jpg#preview')).toBe(
      'photo.jpg'
    )
  })

  test('handles trailing slashes without returning an empty name', () => {
    // URL 以 `/` 结尾时直接取末段会得到空串。
    expect(guessImageFileName('https://cdn.example.com/dir/')).toBe('image.png')
  })

  test('falls back when the last segment has no extension', () => {
    // 无扩展名的下载文件会让系统无法用图片查看器打开。
    expect(guessImageFileName('https://cdn.example.com/download')).toBe('image.png')
    expect(guessImageFileName('https://cdn.example.com/download', 'x.png')).toBe('x.png')
  })

  test('accepts common image extensions case-insensitively', () => {
    for (const ext of ['png', 'JPG', 'jpeg', 'webp', 'gif', 'avif']) {
      expect(guessImageFileName(`https://x/y/pic.${ext}`)).toBe(`pic.${ext}`)
    }
  })

  test('handles a data URL by falling back', () => {
    // data: URL 没有可用的路径末段，必须走 fallback 而不是把 base64 当前缀。
    expect(guessImageFileName('data:image/png;base64,iVBORw0KGgo=')).toBe('image.png')
  })

  test('handles a relative path', () => {
    expect(guessImageFileName('/images/out.png')).toBe('out.png')
  })

  test('handles an empty string', () => {
    expect(guessImageFileName('')).toBe('image.png')
  })
})
