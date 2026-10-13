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
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, test, vi } from 'vitest'

import { ChatImage } from '../chat-image'

// 这些用例断言**用户可见行为**（能下载、能放大、有参考图入口），
// 而不是内部 state 或 DOM 层级。
describe('ChatImage', () => {
  const SRC = 'https://cdn.example.com/out.png'

  test('renders the image with the given alt text', () => {
    render(<ChatImage src={SRC} alt='generated cat' />)
    expect(screen.getByAltText('generated cat')).toBeTruthy()
  })

  test('offers a download control whose href points at the image and carries a filename', () => {
    render(<ChatImage src={SRC} />)
    const download = screen.getByLabelText('Download image')
    // 下载入口必须真的可下载：href 指向图片本身 + 带 download 文件名，
    // 否则浏览器会直接导航而不是保存。
    expect(download.getAttribute('href')).toBe(SRC)
    expect(download.getAttribute('download')).toBe('out.png')
  })

  test('download filename strips query params from signed URLs', () => {
    render(<ChatImage src='https://cdn.example.com/a.webp?sign=xyz' />)
    expect(screen.getByLabelText('Download image').getAttribute('download')).toBe('a.webp')
  })

  test('opens an enlarged preview when the image is clicked', async () => {
    render(<ChatImage src={SRC} alt='cat' />)
    // 放大前不应有放大预览（用预览里的下载按钮区分）。
    expect(screen.queryByText('Enlarged image preview')).toBeNull()

    fireEvent.click(screen.getByLabelText('View image'))

    await waitFor(() => {
      expect(screen.getByText('Enlarged image preview')).toBeTruthy()
    })
  })

  test('hides the use-as-reference control when no handler is provided', () => {
    // 文本模型没有图生图能力；给它显示一个点了没用的按钮是更糟的体验。
    render(<ChatImage src={SRC} />)
    expect(screen.queryByLabelText('Use as reference image')).toBeNull()
  })

  test('calls back with the image url when use-as-reference is clicked', () => {
    const onUse = vi.fn()
    render(<ChatImage src={SRC} onUseAsReference={onUse} />)

    fireEvent.click(screen.getByLabelText('Use as reference image'))

    expect(onUse).toHaveBeenCalledWith(SRC)
  })

  test('also offers use-as-reference inside the enlarged preview', async () => {
    const onUse = vi.fn()
    render(<ChatImage src={SRC} onUseAsReference={onUse} />)

    fireEvent.click(screen.getByLabelText('View image'))
    await waitFor(() => expect(screen.getByText('Enlarged image preview')).toBeTruthy())

    // 弹层里的按钮文案是「Use as reference image」（有文字），
    // 与小图标按钮的可访问名区分开。
    const buttons = screen.getAllByText('Use as reference image')
    expect(buttons.length).toBeGreaterThan(0)
  })

  test('falls back to a text placeholder when the image fails to load', () => {
    render(<ChatImage src={SRC} alt='broken thing' />)
    const img = screen.getByAltText('broken thing')

    fireEvent.error(img)

    // 碎图比文字占位更难理解，降级为 alt 文案。
    expect(screen.getByText('broken thing')).toBeTruthy()
    expect(screen.queryByAltText('broken thing')).toBeNull()
  })

  test('applies the caller-provided image sizing class', () => {
    // 不同位置尺寸差异很大（附件条要小、正文要大），不能写死。
    render(<ChatImage src={SRC} imgClassName='max-h-96' />)
    expect(screen.getByAltText('Image').className).toContain('max-h-96')
  })
})
